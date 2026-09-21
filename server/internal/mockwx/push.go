package mockwx

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// mockOpenIDPrefix 审核结果推送里 FromUserName 的前缀（模拟微信系统账号的 OpenID）。
const mockOpenIDPrefix = "mock_openid_"

// pushClient 推送专用 HTTP 客户端：带超时，避免回调方不响应时把测试挂死。
var pushClient = &http.Client{Timeout: 10 * time.Second}

// 包体信封常数：第三方平台只允许 XML + 安全模式（纯密文）。
const (
	pushEnvelopeFormat = "<xml><ToUserName><![CDATA[%s]]></ToUserName><Encrypt><![CDATA[%s]]></Encrypt></xml>"
	headerContentType  = "text/xml"
)

// PushTicket 向平台「授权事件接收 URL」POST 一个加密的 component_verify_ticket 事件。
//
// 推送成功后，模拟器即可用该 ticket 换取 component_access_token（未推送前 api_component_token 返回 61005）。
// 返回值为回调方的响应体与 HTTP 状态码。
func (s *Server) PushTicket(ctx context.Context, callbackURL string) (string, int, error) {
	s.mu.Lock()
	s.ticketSeq++
	ticket := fmt.Sprintf("mock_ticket_%d_%d", s.ticketSeq, time.Now().Unix())
	s.ticketSet[ticket] = time.Now()
	s.lastTicket = ticket
	s.mu.Unlock()

	plain := fmt.Sprintf("<xml><AppId><![CDATA[%s]]></AppId><CreateTime>%d</CreateTime>"+
		"<InfoType><![CDATA[component_verify_ticket]]></InfoType>"+
		"<ComponentVerifyTicket><![CDATA[%s]]></ComponentVerifyTicket></xml>",
		cdata(s.opts.ComponentAppID), time.Now().Unix(), cdata(ticket))

	// ToUserName 用第三方平台的 gh_ 原始 ID；receiveid 用第三方平台 appid（授权事件接收 URL 的官方口径）。
	query, body, err := s.encryptPush(plain, s.componentRawID, s.opts.ComponentAppID)
	if err != nil {
		return "", 0, err
	}
	return s.postPush(ctx, callbackURL, query, body)
}

// PushAuthorizeEvent 推送 authorized / updateauthorized / unauthorized 事件。
//
// authorized / updateauthorized 会新生成一个授权码（AuthorizationCode）并刷新 refresh_token，
// 上层可立即用该授权码调用 api_query_auth 落库；unauthorized 会作废该小程序的全部令牌与授权码。
func (s *Server) PushAuthorizeEvent(ctx context.Context, callbackURL, infoType, appid string) (string, int, error) {
	switch infoType {
	case "authorized", "updateauthorized", "unauthorized":
	default:
		return "", 0, fmt.Errorf("mockwx: 不支持的事件类型 %q（只能是 authorized / updateauthorized / unauthorized）", infoType)
	}
	if appid == "" {
		return "", 0, errors.New("mockwx: PushAuthorizeEvent 的 appid 不能为空")
	}

	now := time.Now().Unix()
	s.mu.Lock()
	a := s.ensureAuthorizerLocked(appid)
	var extra string
	switch infoType {
	case "authorized", "updateauthorized":
		s.authCodeSeq++
		code := fmt.Sprintf("mock_auth_code_%d_%s", s.authCodeSeq, appid)
		s.authCodes[code] = authCodeRec{appid: appid, expireAt: time.Now().Add(authCodeTTL)}
		s.authCodeSeq++
		pre := fmt.Sprintf("mock_pre_auth_code_%d", s.authCodeSeq)
		s.preAuthCodes[pre] = time.Now().Add(preAuthCodeTTL)

		a.authorized = true
		a.authTime = now
		// 官方：用户重新授权后，之前的 authorizer_refresh_token 失效。
		a.refreshToken = fmt.Sprintf("mock_refresh_%d_%s", now, appid)
		extra = fmt.Sprintf("<AuthorizationCode><![CDATA[%s]]></AuthorizationCode>"+
			"<AuthorizationCodeExpiredTime>%d</AuthorizationCodeExpiredTime>"+
			"<PreAuthCode><![CDATA[%s]]></PreAuthCode>",
			cdata(code), int(authCodeTTL/time.Second), cdata(pre))
	case "unauthorized":
		a.authorized = false
		a.refreshToken = ""
		s.revokeTokensLocked(appid)
		for code, rec := range s.authCodes {
			if rec.appid == appid {
				delete(s.authCodes, code)
			}
		}
	}
	s.mu.Unlock()

	plain := fmt.Sprintf("<xml><AppId><![CDATA[%s]]></AppId><CreateTime>%d</CreateTime>"+
		"<InfoType><![CDATA[%s]]></InfoType><AuthorizerAppid><![CDATA[%s]]></AuthorizerAppid>%s</xml>",
		cdata(s.opts.ComponentAppID), now, cdata(infoType), cdata(appid), extra)

	// 授权事件同样推给「授权事件接收 URL」，ToUserName 用第三方平台原始 ID，receiveid 用第三方平台 appid。
	query, body, err := s.encryptPush(plain, s.componentRawID, s.opts.ComponentAppID)
	if err != nil {
		return "", 0, err
	}
	return s.postPush(ctx, callbackURL, query, body)
}

// PushAuditResult 向平台「消息与事件接收 URL」推送 weapp_audit_success / weapp_audit_fail / weapp_audit_delay。
//
// 包体按官方事件 XML 结构：ToUserName=小程序原始 ID，Event 为事件类型，并按事件补齐
// SuccTime / FailTime / DelayTime / Reason / ScreenShot。推送的同时把模拟器内部审核单状态改成一致，
// 这样紧随其后的 release / get_latest_auditstatus 能与推送结果对齐。
// callbackURL 中的 $APPID$ 占位符会被替换为该小程序 appid（官方支持该占位符）。
func (s *Server) PushAuditResult(ctx context.Context, callbackURL, appid, event, reason string) (string, int, error) {
	switch event {
	case "weapp_audit_success", "weapp_audit_fail", "weapp_audit_delay":
	default:
		return "", 0, fmt.Errorf("mockwx: 不支持的审核事件 %q（只能是 weapp_audit_success / weapp_audit_fail / weapp_audit_delay）", event)
	}
	if appid == "" {
		return "", 0, errors.New("mockwx: PushAuditResult 的 appid 不能为空")
	}

	now := time.Now().Unix()
	reasonText := reason
	s.mu.Lock()
	a := s.ensureAuthorizerLocked(appid)
	toUser := a.userName
	if rec, ok := a.audits[a.latestAuditID]; ok {
		switch event {
		case "weapp_audit_success":
			rec.status = 0
			rec.reason = ""
			rec.screenshot = ""
		case "weapp_audit_fail":
			rec.status = 1
			rec.reason = reasonText
			if rec.reason == "" {
				rec.reason = defaultFailReason
			}
			rec.screenshot = reasonScreenshot
		case "weapp_audit_delay":
			rec.status = 4
			rec.reason = reasonText
			if rec.reason == "" {
				rec.reason = "为了更好的服务小程序，您的服务商正在进行提审系统的优化，可能会导致审核时效的增长，请耐心等待"
			}
		}
		reasonText = rec.reason
	}
	s.mu.Unlock()

	var extra string
	switch event {
	case "weapp_audit_success":
		extra = fmt.Sprintf("<SuccTime>%d</SuccTime>", now)
	case "weapp_audit_fail":
		extra = fmt.Sprintf("<Reason><![CDATA[%s]]></Reason><FailTime>%d</FailTime><ScreenShot><![CDATA[%s]]></ScreenShot>",
			cdata(reasonText), now, cdata(reasonScreenshot))
	case "weapp_audit_delay":
		extra = fmt.Sprintf("<Reason><![CDATA[%s]]></Reason><DelayTime>%d</DelayTime>", cdata(reasonText), now)
	}

	plain := fmt.Sprintf("<xml><ToUserName><![CDATA[%s]]></ToUserName><FromUserName><![CDATA[%s]]></FromUserName>"+
		"<CreateTime>%d</CreateTime><MsgType><![CDATA[event]]></MsgType><Event><![CDATA[%s]]></Event>%s</xml>",
		cdata(toUser), cdata(mockOpenIDPrefix+appid), now, cdata(event), extra)

	target := strings.ReplaceAll(callbackURL, "$APPID$", appid)
	// 代收授权方消息的 receiveid 口径为授权方 appid（官方分流要求）。
	query, body, err := s.encryptPush(plain, toUser, appid)
	if err != nil {
		return "", 0, err
	}
	return s.postPush(ctx, target, query, body)
}

// encryptPush 用 wxcrypt 生成官方安全模式的推送请求（URL 参数 + 加密包体）。
//
// URL 形如 ?signature=&timestamp=&nonce=&encrypt_type=aes&msg_signature=：
//   - signature 是 3 字段（token/timestamp/nonce）明文签名，真实推送里也会带上；
//   - msg_signature 是 4 字段（含密文 Encrypt）签名，官方明确校验时必须用它。
//
// 两个都给，便于上层验证自己确实用了 msg_signature 而不是 signature。
func (s *Server) encryptPush(plain, toUser, receiveID string) (string, string, error) {
	if s.cryptErr != nil {
		return "", "", fmt.Errorf("mockwx: 消息加解密器不可用（请检查 Options.EncodingAESKey）: %w", s.cryptErr)
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := strconv.FormatInt(randInt63(), 10)

	random16 := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, random16); err != nil {
		return "", "", fmt.Errorf("mockwx: 生成随机串失败: %w", err)
	}
	encrypt, err := s.crypt.EncryptPayload(plain, receiveID, random16)
	if err != nil {
		return "", "", fmt.Errorf("mockwx: 加密推送包体失败: %w", err)
	}

	query := url.Values{}
	query.Set("signature", sha1HexSorted(s.opts.VerifyToken, timestamp, nonce))
	query.Set("timestamp", timestamp)
	query.Set("nonce", nonce)
	query.Set("encrypt_type", "aes")
	query.Set("msg_signature", s.crypt.ComputeSignature(timestamp, nonce, encrypt))

	return query.Encode(), fmt.Sprintf(pushEnvelopeFormat, cdata(toUser), cdata(encrypt)), nil
}

// postPush 把推送请求发给回调方，返回响应体与状态码。
func (s *Server) postPush(ctx context.Context, callbackURL, query, body string) (string, int, error) {
	if callbackURL == "" {
		return "", 0, errors.New("mockwx: callbackURL 不能为空")
	}
	full := callbackURL
	if strings.Contains(full, "?") {
		full += "&" + query
	} else {
		full += "?" + query
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, full, strings.NewReader(body))
	if err != nil {
		return "", 0, fmt.Errorf("mockwx: 构造推送请求失败: %w", err)
	}
	req.Header.Set("Content-Type", headerContentType)
	resp, err := pushClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("mockwx: 推送失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return "", resp.StatusCode, fmt.Errorf("mockwx: 读取回调响应失败: %w", err)
	}
	return string(raw), resp.StatusCode, nil
}

// rawIDFor 由 appid 派生一个稳定的 gh_ 原始 ID（推送包体的 ToUserName 使用）。
func rawIDFor(appid string) string {
	sum := sha1.Sum([]byte(appid))
	return "gh_" + hex.EncodeToString(sum[:6])
}

// sha1HexSorted 按官方签名口径计算 sha1：字段字典序排序后直接拼接。
func sha1HexSorted(fields ...string) string {
	sorted := append([]string{}, fields...)
	sort.Strings(sorted)
	sum := sha1.Sum([]byte(strings.Join(sorted, "")))
	return hex.EncodeToString(sum[:])
}

// randInt63 生成一个非负随机数（用于 nonce）。
func randInt63() int64 {
	var b [8]byte
	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return time.Now().UnixNano()
	}
	n := int64(0)
	for _, v := range b {
		n = n<<8 | int64(v)
	}
	if n < 0 {
		n = -n
	}
	return n % 10000000000
}

// cdata 转义 CDATA 中不能出现的内容（"]]>" 需要拆开），避免包体非法。
func cdata(s string) string {
	return strings.ReplaceAll(s, "]]>", "]]]]><![CDATA[>")
}

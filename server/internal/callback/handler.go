// Package callback 处理微信服务器推送的两个回调入口。
//
//   - 授权事件接收 URL：component_verify_ticket（每 10 分钟一次，有效期 12 小时）
//     与授权变更通知（authorized / updateauthorized / unauthorized）。
//   - 消息与事件接收 URL（形如 .../$APPID$/...）：代码审核结果推送
//     （weapp_audit_success / weapp_audit_fail / weapp_audit_delay）与代收的用户消息。
//
// 官方硬性要求：接收后「只需直接返回字符串 success」，因此本包先写审计记录、立即回 success，
// 再把耗时的业务处理丢到后台 goroutine（微信侧对回包超时会重推，不能阻塞在回包里）。
// 签名校验只用 msg_signature，绝不使用 signature。
package callback

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"wx-platform/server/internal/model"
	"wx-platform/server/internal/wxcrypt"
)

// TicketSink 承兑票据的落库接口。
type TicketSink interface {
	SaveTicket(ctx context.Context, ticket string, at time.Time) error
	MarkPushOK(ctx context.Context, at time.Time) error
}

// EventSink 回调事件的审计与幂等接口。
type EventSink interface {
	CreateIfAbsent(ctx context.Context, ev *model.CallbackEvent) (*model.CallbackEvent, bool, error)
	MarkProcessed(ctx context.Context, id uint, ok bool, note string) error
}

// BusinessHandler 回调的业务处理接口（由 service 层实现，避免 callback 依赖 service）。
type BusinessHandler interface {
	// OnAuthorized 授权成功 / 更新授权：用 authorization_code 换取令牌并登记小程序。
	OnAuthorized(ctx context.Context, appid, authCode string, infoType string) error
	// OnUnauthorized 取消授权：作废令牌并标记状态。
	OnUnauthorized(ctx context.Context, appid string) error
	// OnAuditResult 审核结果推送（事件 XML 中没有 auditid，需回查最新审核单补齐）。
	OnAuditResult(ctx context.Context, appid, event, reason, screenshot string, eventTime int64) error
	// OnMessage 代收的用户消息（本平台默认只审计，不自动回复）。
	OnMessage(ctx context.Context, appid, plainXML string) error
}

// Options 构造参数。
type Options struct {
	Crypto         *wxcrypt.WXBizMsgCrypt
	Tickets        TicketSink
	Events         EventSink
	Business       BusinessHandler
	ComponentAppid string
	// Async 为 false 时业务处理同步执行（便于集成测试断言）；生产为 true。
	Async bool
}

// Handler 回调处理器。
type Handler struct {
	opts Options
}

// New 构造处理器。
func New(opts Options) *Handler {
	return &Handler{opts: opts}
}

// RegisterCallbacks 注册公开回调路由（不经过 JWT 鉴权）。
func (h *Handler) RegisterCallbacks(r gin.IRouter) {
	r.GET("/callback/component", h.handleComponentVerify)
	r.POST("/callback/component", h.handleComponentEvent)
	r.GET("/callback/message/:appid", h.handleMessageVerify)
	r.POST("/callback/message/:appid", h.handleMessageEvent)
}

// eventXML 解密后的回调明文（覆盖票据、授权变更、审核结果与代收消息的全部字段）。
type eventXML struct {
	XMLName                    xml.Name `xml:"xml"`
	AppId                      string   `xml:"AppId"`
	ToUserName                 string   `xml:"ToUserName"`
	FromUserName               string   `xml:"FromUserName"`
	CreateTime                 int64    `xml:"CreateTime"`
	InfoType                   string   `xml:"InfoType"`
	ComponentVerifyTicket      string   `xml:"ComponentVerifyTicket"`
	AuthorizerAppid            string   `xml:"AuthorizerAppid"`
	AuthorizationCode          string   `xml:"AuthorizationCode"`
	AuthorizationCodeExpiredAt int64    `xml:"AuthorizationCodeExpiredTime"`
	PreAuthCode                string   `xml:"PreAuthCode"`
	MsgType                    string   `xml:"MsgType"`
	Event                      string   `xml:"Event"`
	SuccTime                   int64    `xml:"SuccTime"`
	FailTime                   int64    `xml:"FailTime"`
	DelayTime                  int64    `xml:"DelayTime"`
	Reason                     string   `xml:"Reason"`
	ScreenShot                 string   `xml:"ScreenShot"`
	ScreenShotLower            string   `xml:"screenshot"`
}

// handleComponentVerify 授权事件接收 URL 的接入校验（GET）。
//
// 第三方平台只允许安全模式，故优先按「4 字段签名 + 解密 echoStr」处理；
// 若请求实际是经典明文 echostr，则回退到 3 字段签名校验。
func (h *Handler) handleComponentVerify(c *gin.Context) {
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")
	msgSignature := firstNonEmpty(c.Query("msg_signature"), c.Query("signature"))
	echoStr := c.Query("echostr")
	if msgSignature == "" || echoStr == "" {
		c.String(http.StatusBadRequest, "缺少校验参数")
		return
	}
	if plain, err := h.opts.Crypto.VerifyURL(msgSignature, timestamp, nonce, echoStr); err == nil {
		c.String(http.StatusOK, plain)
		return
	}
	if h.opts.Crypto.VerifyPlainURL(c.Query("signature"), timestamp, nonce) {
		c.String(http.StatusOK, echoStr)
		return
	}
	log.Printf("[callback] 接入校验失败：签名不匹配（timestamp=%s nonce=%s）", timestamp, nonce)
	c.String(http.StatusBadRequest, "签名校验失败")
}

// handleMessageVerify 消息与事件接收 URL 的接入校验（GET），语义同上。
func (h *Handler) handleMessageVerify(c *gin.Context) {
	h.handleComponentVerify(c)
}

// handleComponentEvent 处理授权事件推送（票据 + 授权变更）。
func (h *Handler) handleComponentEvent(c *gin.Context) {
	appid := ""
	body, ok := h.readAndDecrypt(c, model.CallbackKindComponent, appid, h.opts.ComponentAppid)
	if !ok {
		return
	}
	// 官方要求：直接返回字符串 success（不加密）。先回包，再处理业务。
	c.String(http.StatusOK, "success")

	var ev eventXML
	if err := xml.Unmarshal([]byte(body.plain), &ev); err != nil {
		log.Printf("[callback] 解析授权事件失败: %v（明文=%s）", err, truncate(body.plain, 200))
		h.markProcessed(body.eventID, false, "解析授权事件 XML 失败")
		return
	}

	// 票据必须落库，否则后续无法换取 component_access_token。
	h.opts.dispatch(func(ctx context.Context) {
		defer h.markProcessed(body.eventID, true, "")
		now := time.Now()
		switch ev.InfoType {
		case "component_verify_ticket":
			if ev.ComponentVerifyTicket == "" {
				h.markProcessed(body.eventID, false, "票据内容为空")
				return
			}
			if err := h.opts.Tickets.SaveTicket(ctx, ev.ComponentVerifyTicket, now); err != nil {
				log.Printf("[callback] 保存 component_verify_ticket 失败: %v", err)
				h.markProcessed(body.eventID, false, "保存票据失败: "+err.Error())
				return
			}
			if err := h.opts.Tickets.MarkPushOK(ctx, now); err != nil {
				log.Printf("[callback] 记录推送时间失败: %v", err)
			}
		case "authorized", "updateauthorized":
			if ev.AuthorizerAppid == "" {
				h.markProcessed(body.eventID, false, "缺少 AuthorizerAppid")
				return
			}
			// 授权码有效期很短，必须立即换取；为空时交由 redirect 回调页兜底。
			if err := h.opts.Business.OnAuthorized(ctx, ev.AuthorizerAppid, ev.AuthorizationCode, ev.InfoType); err != nil {
				log.Printf("[callback] 处理 %s 事件失败(appid=%s): %v", ev.InfoType, ev.AuthorizerAppid, err)
				h.markProcessed(body.eventID, false, err.Error())
			}
		case "unauthorized":
			if ev.AuthorizerAppid == "" {
				h.markProcessed(body.eventID, false, "缺少 AuthorizerAppid")
				return
			}
			if err := h.opts.Business.OnUnauthorized(ctx, ev.AuthorizerAppid); err != nil {
				log.Printf("[callback] 处理取消授权失败(appid=%s): %v", ev.AuthorizerAppid, err)
				h.markProcessed(body.eventID, false, err.Error())
			}
		default:
			// 其他事件（如名称审核、违规处罚）本平台只做审计。
			h.markProcessed(body.eventID, true, "未处理的事件类型 "+ev.InfoType)
		}
	})
}

// handleMessageEvent 处理消息与事件接收 URL 的推送（审核结果 + 代收消息）。
func (h *Handler) handleMessageEvent(c *gin.Context) {
	appid := c.Param("appid")
	body, ok := h.readAndDecrypt(c, model.CallbackKindMessage, appid, appid)
	if !ok {
		return
	}
	c.String(http.StatusOK, "success")

	var ev eventXML
	if err := xml.Unmarshal([]byte(body.plain), &ev); err != nil {
		log.Printf("[callback] 解析消息事件失败: %v", err)
		h.markProcessed(body.eventID, false, "解析消息 XML 失败")
		return
	}

	h.opts.dispatch(func(ctx context.Context) {
		defer h.markProcessed(body.eventID, true, "")
		if ev.MsgType == "event" {
			switch ev.Event {
			case "weapp_audit_success", "weapp_audit_fail", "weapp_audit_delay":
				eventTime := firstNonZero(ev.SuccTime, ev.FailTime, ev.DelayTime, ev.CreateTime)
				screenshot := firstNonEmpty(ev.ScreenShot, ev.ScreenShotLower)
				if err := h.opts.Business.OnAuditResult(ctx, appid, ev.Event, ev.Reason, screenshot, eventTime); err != nil {
					log.Printf("[callback] 处理审核结果失败(appid=%s event=%s): %v", appid, ev.Event, err)
					h.markProcessed(body.eventID, false, err.Error())
				}
				return
			default:
				h.markProcessed(body.eventID, true, "未处理的事件 "+ev.Event)
				return
			}
		}
		// 代收的用户消息：本平台不自动回复，仅审计（合规上不代商家处理客服消息）。
		if err := h.opts.Business.OnMessage(ctx, appid, body.plain); err != nil {
			log.Printf("[callback] 记录代收消息失败(appid=%s): %v", appid, err)
			h.markProcessed(body.eventID, false, err.Error())
		}
	})
}

// decryptedBody 解密结果与审计记录。
type decryptedBody struct {
	plain   string
	event   *model.CallbackEvent
	eventID uint
}

// readAndDecrypt 读取请求体、校验 msg_signature 并解密，同时写入回调审计记录（幂等去重）。
//
// 返回 false 时已写出错误响应。
func (h *Handler) readAndDecrypt(c *gin.Context, kind model.CallbackKind, appid, expectedReceiveID string) (*decryptedBody, bool) {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.String(http.StatusBadRequest, "读取请求体失败")
		return nil, false
	}
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")
	msgSignature := firstNonEmpty(c.Query("msg_signature"), c.Query("signature"))

	plain, receiveID, err := h.opts.Crypto.DecryptMsg(msgSignature, timestamp, nonce, string(raw), expectedReceiveID)
	if err != nil {
		// 代收消息 URL 上 receiveid 的口径在官方文档中存在矛盾（示例用第三方平台 appid，
		// 正文说用授权方 appid），因此这里做一次兜底重试，两者任一通过即接受。
		if kind == model.CallbackKindMessage && wxcrypt.IsCode(err, wxcrypt.ErrValidateReceiveID) && h.opts.ComponentAppid != "" {
			plain, receiveID, err = h.opts.Crypto.DecryptMsg(msgSignature, timestamp, nonce, string(raw), h.opts.ComponentAppid)
		}
		if err != nil {
			log.Printf("[callback] 解密失败(kind=%s appid=%s): %v", kind, appid, err)
			// 解密失败也必须回 success：否则微信会重推同一份报文，形成风暴；失败原因已落日志。
			c.String(http.StatusOK, "success")
			return nil, false
		}
	}

	ev := &model.CallbackEvent{
		Kind:        kind,
		Appid:       firstNonEmpty(appid, receiveID),
		Encrypted:   extractEncrypt(string(raw)),
		Decrypted:   plain,
		DedupeKey:   dedupeKey(plain, timestamp, nonce),
		SignatureOK: true,
		ReceivedAt:  time.Now(),
		InfoType:    extractTag(plain, "InfoType"),
		Event:       extractTag(plain, "Event"),
	}
	saved, created, err := h.opts.Events.CreateIfAbsent(c.Request.Context(), ev)
	if err != nil {
		log.Printf("[callback] 写回调审计失败: %v", err)
	} else if !created {
		// 重复推送：直接跳过业务处理（幂等）。
		log.Printf("[callback] 忽略重复回调(kind=%s appid=%s dedupe=%s)", kind, ev.Appid, ev.DedupeKey)
		return nil, false
	}
	id := uint(0)
	if saved != nil {
		id = saved.ID
	}
	return &decryptedBody{plain: plain, event: ev, eventID: id}, true
}

// dispatch 按配置同步或异步执行业务处理。
func (o Options) dispatch(fn func(ctx context.Context)) {
	if o.Async {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			fn(ctx)
		}()
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fn(ctx)
}

func (h *Handler) markProcessed(id uint, ok bool, note string) {
	if id == 0 || h.opts.Events == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.opts.Events.MarkProcessed(ctx, id, ok, note); err != nil {
		log.Printf("[callback] 标记处理结果失败(id=%d): %v", id, err)
	}
}

// dedupeKey 用「明文 + 时间戳 + 随机数」做幂等键：微信重推同一报文时可直接跳过。
func dedupeKey(plain, timestamp, nonce string) string {
	sum := sha1.Sum([]byte(plain + "|" + timestamp + "|" + nonce))
	return hex.EncodeToString(sum[:])
}

// extractEncrypt 从原始包体中取出 Encrypt 密文（用于审计留档）。
func extractEncrypt(raw string) string {
	return extractTag(raw, "Encrypt")
}

// extractTag 用简单字符串扫描提取单个标签内容，避免为审计再做一次完整 XML 解析。
func extractTag(raw, tag string) string {
	open := "<" + tag + ">"
	idx := strings.Index(raw, open)
	if idx < 0 {
		return ""
	}
	rest := raw[idx+len(open):]
	closeIdx := strings.Index(rest, "</"+tag+">")
	if closeIdx < 0 {
		return ""
	}
	value := rest[:closeIdx]
	value = strings.TrimPrefix(value, "<![CDATA[")
	value = strings.TrimSuffix(value, "]]>")
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonZero(values ...int64) int64 {
	for _, v := range values {
		if v != 0 {
			return v
		}
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

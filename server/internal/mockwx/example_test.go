package mockwx

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"wx-platform/server/internal/wxcrypt"
)

// Example_driveFullChain 演示「上层（第三方平台服务端）如何用 mockwx 驱动全链路」的调用顺序。
//
// 该示例没有 Output 注释，因此 go test 只编译不执行；它的价值是把正确顺序固化成可复制的代码：
//
//	推送 ticket → 换 component_access_token → 授权码 → 换 authorizer_refresh_token/access_token
//	→ 草稿转模板 → commit（含 ext_json）→ 轮询隐私检测 → submit_audit → 推送审核结果 → release
//
// 三点容易踩坑的地方在示例里都有体现：
//  1. 校验回调必须用 msg_signature（本模拟器同时给出 signature，正是为了让上层验证自己没用错）；
//  2. 授权事件接收 URL 的 receiveid 是第三方平台 appid，消息与事件接收 URL 的 receiveid 是授权方 appid；
//  3. commit 之后必须等隐私检测任务结束（61039）才能 submit_audit。
func Example_driveFullChain() {
	const (
		componentAppID = "wx_mock_component"
		verifyToken    = "mock_verify_token"
		aesKey         = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"
	)

	// 0) 起一个模拟微信服务端，并把上层的「微信 API 基址」指向它。
	mock := New(Options{ComponentAppID: componentAppID, VerifyToken: verifyToken, EncodingAESKey: aesKey})
	apiServer := httptest.NewServer(mock.Handler())
	defer apiServer.Close()

	// 上层的「授权事件接收 URL」：真实实现会在这里落库 ticket / 处理授权变更通知。
	crypt, err := wxcrypt.New(verifyToken, aesKey, componentAppID, "")
	if err != nil {
		return
	}
	var ticket string
	authCallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		q := r.URL.Query()
		plain, _, err := crypt.DecryptMsg(q.Get("msg_signature"), q.Get("timestamp"), q.Get("nonce"), string(raw), componentAppID)
		if err == nil {
			ticket = between(plain, "<ComponentVerifyTicket><![CDATA[", "]]></ComponentVerifyTicket>")
		}
		_, _ = w.Write([]byte("success")) // 官方要求：直接返回纯文本 success
	}))
	defer authCallback.Close()

	// 上层的「消息与事件接收 URL」：审核结果推送走这里（receiveid 是授权方 appid）。
	msgCallback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		q := r.URL.Query()
		appid := strings.TrimSuffix(r.URL.Path, "/audit")
		_, _, _ = crypt.DecryptMsg(q.Get("msg_signature"), q.Get("timestamp"), q.Get("nonce"), string(raw), appid)
		_, _ = w.Write([]byte("success"))
	}))
	defer msgCallback.Close()

	post := func(path, token string, body any) map[string]any {
		return callAPI(apiServer.URL, http.MethodPost, path, token, body)
	}
	get := func(path, token string) map[string]any {
		return callAPI(apiServer.URL, http.MethodGet, path, token, nil)
	}
	str := func(m map[string]any, key string) string { s, _ := m[key].(string); return s }

	// 1) 微信推送 component_verify_ticket（真实环境每 10 分钟一次，有效期 12 小时）。
	_, _, _ = mock.PushTicket(context.Background(), authCallback.URL)

	// 2) 用 ticket 换 component_access_token（有效期 7200 秒，上层必须缓存）。
	cat := str(post("/cgi-bin/component/api_component_token", "", map[string]any{
		"component_appid":         componentAppID,
		"component_appsecret":     "your-appsecret",
		"component_verify_ticket": ticket,
	}), "component_access_token")

	// 3) 真实环境：api_create_preauthcode → 拼授权链接 → 商家扫码 → 回调拿 auth_code；
	//    测试环境可以先用 AddAuthorizer + CreateAuthCode 直接造出「已授权小程序 + 授权码」。
	appid := mock.AddAuthorizer("示例小程序", []int{18})
	authCode := mock.CreateAuthCode(appid)

	// 4) 授权码 → authorizer_refresh_token + authorizer_access_token。
	info, _ := post("/cgi-bin/component/api_query_auth", cat, map[string]any{
		"component_appid": componentAppID, "authorization_code": authCode,
	})["authorization_info"].(map[string]any)
	refreshToken := str(info, "authorizer_refresh_token")
	authorizerToken := str(info, "authorizer_access_token")

	// 5) 需要时用 refresh_token 刷新授权方令牌（上层应缓存，快过期再刷）。
	authorizerToken = str(post("/cgi-bin/component/api_authorizer_token", cat, map[string]any{
		"component_appid": componentAppID, "authorizer_appid": appid, "authorizer_refresh_token": refreshToken,
	}), "authorizer_access_token")

	// 6) 模板库：草稿 → 模板（模板库上限 200，用 component_access_token）。
	drafts := get("/wxa/gettemplatedraftlist", cat)["draft_list"].([]any)
	draftID := drafts[0].(map[string]any)["draft_id"]
	templateID := int(post("/wxa/addtotemplate", cat, map[string]any{"draft_id": draftID})["template_id"].(float64))

	// 7) 批量上传代码：同一个模板 + 各自 ext_json，即可给多个小程序上传体验版。
	mock.SetPrivacyCheckDelay(200 * time.Millisecond) // 真实微信的检测任务需要几十秒
	extJSON := `{"extAppid":"` + appid + `","ext":{"tenant":"demo"}}`
	_ = post("/wxa/commit", authorizerToken, map[string]any{
		"template_id": templateID, "ext_json": extJSON, "user_version": "1.0.0", "user_desc": "首个版本",
	})
	for {
		code, _ := get("/wxa/security/get_code_privacy_info", authorizerToken)["errcode"].(float64)
		if code == 0 {
			break
		}
		if code != 61039 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	// 8) 提审，并查询审核状态（截图字段是大写 ScreenShot，解析时要兼容）。
	auditID := int64(post("/wxa/submit_audit", authorizerToken, map[string]any{})["auditid"].(float64))
	_ = get("/wxa/get_latest_auditstatus", authorizerToken)
	_ = post("/wxa/get_auditstatus", authorizerToken, map[string]any{"auditid": auditID})

	// 9) 微信推送审核结果 → 上层更新状态 → 发布（body 必须发 {}）。
	_, _, _ = mock.PushAuditResult(context.Background(), msgCallback.URL+"/"+appid+"/audit", appid, "weapp_audit_success", "")
	_ = post("/wxa/release", authorizerToken, map[string]any{})

	// 10) 测试断言用状态查询接口。
	version, _ := mock.ReleasedVersionOf(appid)
	_, _, _ = mock.AuditStatusOf(appid)
	_ = mock.ExtJSONOf(appid)

	// 11) 测试收尾：核对调用记录、做故障注入（endpoint 形如 "/wxa/commit"）。
	_ = mock.Calls()
	mock.ResetCalls()
	mock.SetQuota(50, 50, 5, 5)
	mock.ForceErrcode("/wxa/commit", 85044)
	mock.ForceErrcode("/wxa/commit", 0)
	_ = version
}

// callAPI 极简的 JSON 调用助手（示例用；上层实际应使用 internal/wxapi 客户端）。
func callAPI(base, method, path, token string, body any) map[string]any {
	var reader io.Reader = strings.NewReader("")
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	target := base + path
	if token != "" {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + "access_token=" + token
	}
	req, err := http.NewRequest(method, target, reader)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

// between 取 <tag> 起始标记与结束标记之间的内容（示例用简化版 XML 取值）。
func between(s, start, end string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		return ""
	}
	return rest[:j]
}

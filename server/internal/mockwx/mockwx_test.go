package mockwx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"wx-platform/server/internal/wxcrypt"
)

const (
	testVerifyToken = "mock_verify_token_for_test"
	// testAESKey 43 字符（32 字节 "0123456789abcdef0123456789abcdef" 的 base64 去掉补位）。
	testAESKey        = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"
	testComponentApp  = "wx_mock_component_test"
	testComponentPath = "/cgi-bin/component/api_component_token"
)

// testEnv 测试脚手架：模拟器 + httptest.Server + 便捷调用方法。
type testEnv struct {
	t   *testing.T
	srv *Server
	ts  *httptest.Server
	cb  *callback
}

// newEnv 构造一套独立的模拟器与回调方。
func newEnv(t *testing.T) *testEnv {
	t.Helper()
	srv := New(Options{ComponentAppID: testComponentApp, VerifyToken: testVerifyToken, EncodingAESKey: testAESKey})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return &testEnv{t: t, srv: srv, ts: ts, cb: newCallback(t)}
}

// callback 模拟「第三方平台侧的回调接收服务」：记录收到的请求并固定返回 success。
type callback struct {
	ts *httptest.Server

	mu   sync.Mutex
	urls []url.Values
	body []string
}

// newCallback 启动回调接收服务。
func newCallback(t *testing.T) *callback {
	t.Helper()
	c := &callback{}
	c.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.urls = append(c.urls, r.URL.Query())
		c.body = append(c.body, string(raw))
		c.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("success"))
	}))
	t.Cleanup(c.ts.Close)
	return c
}

// URL 返回回调地址（支持 $APPID$ 占位符写法）。
func (c *callback) URL() string { return c.ts.URL + "/callback" }

// last 返回最近一次收到的请求（query 与包体）。
func (c *callback) last(t *testing.T) (url.Values, string) {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.urls) == 0 {
		t.Fatalf("回调方没有收到任何推送")
	}
	return c.urls[len(c.urls)-1], c.body[len(c.body)-1]
}

// count 返回收到的推送次数。
func (c *callback) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.urls)
}

// withToken 把 access_token 拼到 URL 上。
func withToken(path, token string) string {
	if token == "" {
		return path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + "access_token=" + url.QueryEscape(token)
}

// tryCall 调用模拟器接口并解析 JSON 响应；返回值形式便于在 goroutine 里断言（不会 t.Fatal）。
func (e *testEnv) tryCall(method, path string, body any) (map[string]any, error) {
	var reader io.Reader = strings.NewReader("")
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("构造请求失败: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s 请求失败: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s %s 响应不是 JSON（Content-Type=%s）: %s", method, path, resp.Header.Get("Content-Type"), raw)
	}
	return out, nil
}

// call 调用模拟器接口并解析 JSON 响应（失败即终止用例）。body 为 nil 表示不传请求体。
func (e *testEnv) call(method, path string, body any) map[string]any {
	e.t.Helper()
	out, err := e.tryCall(method, path, body)
	if err != nil {
		e.t.Fatalf("%v", err)
	}
	return out
}

// rawCall 调用模拟器接口并返回原始响应。
func (e *testEnv) rawCall(method, path string, body any) (*http.Response, []byte) {
	e.t.Helper()
	var reader io.Reader = strings.NewReader("")
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("序列化请求体失败: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, e.ts.URL+path, reader)
	if err != nil {
		e.t.Fatalf("构造请求失败: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s 请求失败: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		e.t.Fatalf("读取响应失败: %v", err)
	}
	return resp, raw
}

// errcode 读取响应里的 errcode。
func errcode(t *testing.T, resp map[string]any) int {
	t.Helper()
	v, ok := resp["errcode"].(float64)
	if !ok {
		t.Fatalf("响应缺少 errcode: %v", resp)
	}
	return int(v)
}

// mustOK 断言响应成功（errcode=0）。
func mustOK(t *testing.T, what string, resp map[string]any) {
	t.Helper()
	if code := errcode(t, resp); code != 0 {
		t.Fatalf("%s 期望成功，实际 errcode=%d errmsg=%v", what, code, resp["errmsg"])
	}
}

// mustCode 断言响应返回指定 errcode。
func mustCode(t *testing.T, what string, resp map[string]any, want int) {
	t.Helper()
	if code := errcode(t, resp); code != want {
		t.Fatalf("%s 期望 errcode=%d，实际 errcode=%d errmsg=%v", what, want, code, resp["errmsg"])
	}
}

// strField 读取字符串字段。
func strField(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	s, _ := m[key].(string)
	return s
}

// objField 读取对象字段。
func objField(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	o, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("字段 %s 不是对象: %v", key, m[key])
	}
	return o
}

// arrField 读取数组字段。
func arrField(t *testing.T, m map[string]any, key string) []any {
	t.Helper()
	a, ok := m[key].([]any)
	if !ok {
		t.Fatalf("字段 %s 不是数组: %v", key, m[key])
	}
	return a
}

// componentToken 走「推送 ticket → 换 component_access_token」链路，返回可用令牌。
func (e *testEnv) componentToken() string {
	e.t.Helper()
	if _, status, err := e.srv.PushTicket(context.Background(), e.cb.URL()); err != nil || status != http.StatusOK {
		e.t.Fatalf("推送 ticket 失败: status=%d err=%v", status, err)
	}
	resp := e.call(http.MethodPost, testComponentPath, map[string]any{
		"component_appid":         testComponentApp,
		"component_appsecret":     "mock_secret",
		"component_verify_ticket": e.srv.lastTicket,
	})
	mustOK(e.t, "api_component_token", resp)
	if got := strField(e.t, resp, "component_access_token"); got == "" {
		e.t.Fatalf("component_access_token 为空")
	}
	return strField(e.t, resp, "component_access_token")
}

// authorizer 走完整授权链路（登记 → 授权码 → query_auth），返回 appid 与 authorizer_access_token。
func (e *testEnv) authorizer() (appid, token string) {
	e.t.Helper()
	appid = e.srv.AddAuthorizer("测试小程序", []int{codePermID})
	code := e.srv.CreateAuthCode(appid)
	cat := e.componentToken()
	resp := e.call(http.MethodPost, withToken("/cgi-bin/component/api_query_auth", cat), map[string]any{
		"component_appid":    testComponentApp,
		"authorization_code": code,
	})
	mustOK(e.t, "api_query_auth", resp)
	info := objField(e.t, resp, "authorization_info")
	token = strField(e.t, info, "authorizer_access_token")
	if token == "" {
		e.t.Fatalf("authorizer_access_token 为空: %v", info)
	}
	return appid, token
}

// ------------- 1. component_access_token 与 ticket -------------

// TestComponentTokenRequiresTicket 未收到 ticket 时必须 61005，推送 ticket 之后才能换取令牌。
func TestComponentTokenRequiresTicket(t *testing.T) {
	env := newEnv(t)

	resp := env.call(http.MethodPost, testComponentPath, map[string]any{
		"component_appid": testComponentApp, "component_appsecret": "s", "component_verify_ticket": "whatever",
	})
	mustCode(t, "未收到 ticket", resp, 61005)

	// 推送 ticket
	if _, status, err := env.srv.PushTicket(context.Background(), env.cb.URL()); err != nil || status != http.StatusOK {
		t.Fatalf("PushTicket 失败: status=%d err=%v", status, err)
	}
	if got := env.cb.count(); got != 1 {
		t.Fatalf("回调方应收到 1 次推送，实际 %d", got)
	}
	ticket := env.srv.lastTicket
	if ticket == "" {
		t.Fatalf("ticket 为空")
	}

	resp = env.call(http.MethodPost, testComponentPath, map[string]any{
		"component_appid": testComponentApp, "component_appsecret": "s", "component_verify_ticket": ticket,
	})
	mustOK(t, "推送 ticket 后换取令牌", resp)
	if got := strField(t, resp, "component_access_token"); got == "" {
		t.Fatalf("component_access_token 为空")
	}
	if got := resp["expires_in"].(float64); got != 7200 {
		t.Fatalf("expires_in 期望 7200，实际 %v", got)
	}

	// 见过的旧 ticket 也应被接受（官方允许用最近可用票据兜底）。
	cat := strField(t, resp, "component_access_token")
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/api_create_preauthcode", cat), map[string]any{"component_appid": testComponentApp})
	mustOK(t, "api_create_preauthcode", resp)
	if got := resp["expires_in"].(float64); got != 1800 {
		t.Fatalf("pre_auth_code expires_in 期望 1800，实际 %v", got)
	}
	if strField(t, resp, "pre_auth_code") == "" {
		t.Fatalf("pre_auth_code 为空")
	}
}

// TestTokenValidation 校验 access_token 缺失 / 无效 / 过期与令牌归属。
func TestTokenValidation(t *testing.T) {
	env := newEnv(t)
	_, token := env.authorizer()

	// 缺失
	resp := env.call(http.MethodPost, "/wxa/commit", map[string]any{})
	mustCode(t, "缺少 access_token", resp, 41001)

	// 无效
	resp = env.call(http.MethodPost, withToken("/wxa/commit", "not-a-token"), map[string]any{})
	mustCode(t, "无效 access_token", resp, 40001)

	// 过期：直接改内部状态（同包测试）
	env.srv.mu.Lock()
	env.srv.tokens[token].expireAt = time.Now().Add(-time.Minute)
	env.srv.mu.Unlock()
	resp = env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{})
	mustCode(t, "过期 access_token", resp, 42001)

	// 令牌归属：authorizer 令牌不能调 component 接口
	_, freshToken := env.authorizer()
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/api_create_preauthcode", freshToken), map[string]any{"component_appid": testComponentApp})
	mustCode(t, "authorizer 令牌调 component 接口", resp, 61014)

	// component 令牌不能调 /wxa/* 代商家接口
	cat := env.componentToken()
	resp = env.call(http.MethodGet, withToken("/wxa/get_latest_auditstatus", cat), nil)
	mustCode(t, "component 令牌调 /wxa/*", resp, 40001)

	// 权限集不足：只授权了 1 号权限集的小程序不能做代码管理
	plainAppid := env.srv.AddAuthorizer("无代码权限小程序", []int{1})
	code := env.srv.CreateAuthCode(plainAppid)
	qresp := env.call(http.MethodPost, withToken("/cgi-bin/component/api_query_auth", cat), map[string]any{
		"component_appid": testComponentApp, "authorization_code": code,
	})
	mustOK(t, "api_query_auth", qresp)
	plainToken := strField(t, objField(t, qresp, "authorization_info"), "authorizer_access_token")
	resp = env.call(http.MethodPost, withToken("/wxa/commit", plainToken), map[string]any{
		"template_id": 1, "ext_json": `{}`, "user_version": "V1", "user_desc": "d",
	})
	mustCode(t, "权限集不足", resp, 48001)

	// 未知接口
	resp = env.call(http.MethodGet, "/not/exist", nil)
	mustCode(t, "未知接口", resp, 404)

	// 方法不符（get_category 要求 GET，POST 会得到 43001）
	resp = env.call(http.MethodPost, withToken("/wxa/get_category", token), map[string]any{})
	mustCode(t, "get_category 需要 GET", resp, 43001)
	resp = env.call(http.MethodGet, withToken("/wxa/commit", token), nil)
	mustCode(t, "commit 需要 POST", resp, 43002)
}

// ------------- 2. 授权链路 -------------

// TestAuthorizeChain 授权码 → refresh_token → 刷新令牌，以及授权方列表 / 详情 / 选项。
func TestAuthorizeChain(t *testing.T) {
	env := newEnv(t)
	appid := env.srv.AddAuthorizer("授权链路小程序", []int{18, 86})
	code := env.srv.CreateAuthCode(appid)
	cat := env.componentToken()

	resp := env.call(http.MethodPost, withToken("/cgi-bin/component/api_query_auth", cat), map[string]any{
		"component_appid":    testComponentApp,
		"authorization_code": code,
	})
	mustOK(t, "api_query_auth", resp)
	info := objField(t, resp, "authorization_info")
	if got := strField(t, info, "authorizer_appid"); got != appid {
		t.Fatalf("authorizer_appid 期望 %s，实际 %s", appid, got)
	}
	if got := info["expires_in"].(float64); got != 7200 {
		t.Fatalf("expires_in 期望 7200，实际 %v", got)
	}
	refresh := strField(t, info, "authorizer_refresh_token")
	if refresh == "" {
		t.Fatalf("authorizer_refresh_token 为空")
	}
	funcInfo := arrField(t, info, "func_info")
	if len(funcInfo) != 2 {
		t.Fatalf("func_info 期望 2 项，实际 %v", funcInfo)
	}
	if got := objField(t, funcInfo[0].(map[string]any), "funcscope_category")["id"].(float64); got != 18 {
		t.Fatalf("func_info[0].id 期望 18，实际 %v", got)
	}

	// 刷新令牌
	first := strField(t, info, "authorizer_access_token")
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/api_authorizer_token", cat), map[string]any{
		"component_appid": testComponentApp, "authorizer_appid": appid, "authorizer_refresh_token": refresh,
	})
	mustOK(t, "api_authorizer_token", resp)
	if got := strField(t, resp, "authorizer_access_token"); got == "" || got == first {
		t.Fatalf("刷新后应得到新的 authorizer_access_token，实际 %q（旧 %q）", got, first)
	}
	if got := strField(t, resp, "authorizer_refresh_token"); got != refresh {
		t.Fatalf("refresh_token 期望保持不变 %s，实际 %s", refresh, got)
	}

	// 无效授权码
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/api_query_auth", cat), map[string]any{
		"component_appid": testComponentApp, "authorization_code": "bad-code",
	})
	mustCode(t, "无效授权码", resp, 40013)

	// 错误的 refresh_token
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/api_authorizer_token", cat), map[string]any{
		"component_appid": testComponentApp, "authorizer_appid": appid, "authorizer_refresh_token": "bad",
	})
	mustCode(t, "错误 refresh_token", resp, 40013)

	// 已授权列表
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/api_get_authorizer_list", cat), map[string]any{
		"component_appid": testComponentApp, "offset": 0, "count": 100,
	})
	mustOK(t, "api_get_authorizer_list", resp)
	if got := resp["total_count"].(float64); got != 1 {
		t.Fatalf("total_count 期望 1，实际 %v", got)
	}
	row := arrField(t, resp, "list")[0].(map[string]any)
	if got := strField(t, row, "authorizer_appid"); got != appid {
		t.Fatalf("list[0].authorizer_appid 期望 %s，实际 %s", appid, got)
	}
	if strField(t, row, "refresh_token") == "" {
		t.Fatalf("list[0].refresh_token 为空")
	}

	// 授权方详情
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/api_get_authorizer_info", cat), map[string]any{
		"component_appid": testComponentApp, "authorizer_appid": appid,
	})
	mustOK(t, "api_get_authorizer_info", resp)
	authorizerInfo := objField(t, resp, "authorizer_info")
	if got := strField(t, authorizerInfo, "user_name"); !strings.HasPrefix(got, "gh_") {
		t.Fatalf("user_name 期望 gh_ 前缀，实际 %q", got)
	}
	if got := strField(t, authorizerInfo, "nick_name"); got != "授权链路小程序" {
		t.Fatalf("nick_name 期望 授权链路小程序，实际 %q", got)
	}
	if mpi, ok := authorizerInfo["MiniProgramInfo"].(map[string]any); !ok || mpi["network"] == nil {
		t.Fatalf("MiniProgramInfo.network 缺失: %v", authorizerInfo["MiniProgramInfo"])
	}
	authInfo := objField(t, resp, "authorization_info")
	if got := strField(t, authInfo, "authorizer_refresh_token"); got != refresh {
		t.Fatalf("详情里的 refresh_token 期望 %s，实际 %s", refresh, got)
	}

	// 授权方选项
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/get_authorizer_option", cat), map[string]any{
		"component_appid": testComponentApp, "authorizer_appid": appid, "option_name": "customer_service",
	})
	mustOK(t, "get_authorizer_option", resp)
	if got := strField(t, resp, "option_value"); got != "0" {
		t.Fatalf("customer_service 默认期望 0，实际 %q", got)
	}
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/set_authorizer_option", cat), map[string]any{
		"component_appid": testComponentApp, "authorizer_appid": appid, "option_name": "customer_service", "option_value": "1",
	})
	mustOK(t, "set_authorizer_option", resp)
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/get_authorizer_option", cat), map[string]any{
		"component_appid": testComponentApp, "authorizer_appid": appid, "option_name": "customer_service",
	})
	if got := strField(t, resp, "option_value"); got != "1" {
		t.Fatalf("设置后 customer_service 期望 1，实际 %q", got)
	}
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/set_authorizer_option", cat), map[string]any{
		"component_appid": testComponentApp, "authorizer_appid": appid, "option_name": "unknown_option", "option_value": "1",
	})
	mustCode(t, "未知 option_name", resp, 61012)
}

// TestMockAuthorizeAndPendingApplet /mock/authorize 把待授权小程序置为已授权。
func TestMockAuthorizeAndPendingApplet(t *testing.T) {
	env := newEnv(t)
	cat := env.componentToken()
	pending := "wx_pending_applet"

	code := env.srv.CreateAuthCode(pending)
	resp := env.call(http.MethodPost, withToken("/cgi-bin/component/api_get_authorizer_list", cat), map[string]any{
		"component_appid": testComponentApp, "offset": 0, "count": 10,
	})
	if got := resp["total_count"].(float64); got != 0 {
		t.Fatalf("待授权小程序不应出现在已授权列表，total_count=%v", got)
	}

	resp = env.call(http.MethodPost, "/mock/authorize", map[string]any{"appid": pending, "auth_code": code})
	mustOK(t, "/mock/authorize", resp)
	if got := strField(t, resp, "appid"); got != pending {
		t.Fatalf("返回 appid 期望 %s，实际 %s", pending, got)
	}
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/api_get_authorizer_list", cat), map[string]any{
		"component_appid": testComponentApp, "offset": 0, "count": 10,
	})
	if got := resp["total_count"].(float64); got != 1 {
		t.Fatalf("授权后 total_count 期望 1，实际 %v", got)
	}

	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/api_query_auth", cat), map[string]any{
		"component_appid": testComponentApp, "authorization_code": code,
	})
	mustOK(t, "授权码换取授权信息", resp)
}

// ------------- 3. 模板库链路 -------------

// TestTemplateChain 草稿 → 模板 → 列表（含 template_type 过滤）→ 删除，以及 200 个上限。
func TestTemplateChain(t *testing.T) {
	env := newEnv(t)
	cat := env.componentToken()

	resp := env.call(http.MethodGet, withToken("/wxa/gettemplatedraftlist", cat), nil)
	mustOK(t, "gettemplatedraftlist", resp)
	drafts := arrField(t, resp, "draft_list")
	if len(drafts) < 2 {
		t.Fatalf("预置草稿至少 2 条，实际 %d", len(drafts))
	}
	first := drafts[0].(map[string]any)
	if _, ok := first["draft_id"]; !ok {
		t.Fatalf("草稿缺少 draft_id: %v", first)
	}

	// 普通模板
	resp = env.call(http.MethodPost, withToken("/wxa/addtotemplate", cat), map[string]any{"draft_id": 1, "template_type": 0})
	mustOK(t, "addtotemplate", resp)
	tplID := int(resp["template_id"].(float64))
	if tplID <= 0 {
		t.Fatalf("template_id 应为正数，实际 %d", tplID)
	}
	// 标准模板
	resp = env.call(http.MethodPost, withToken("/wxa/addtotemplate", cat), map[string]any{"draft_id": 2, "template_type": 1})
	mustOK(t, "addtotemplate(标准模板)", resp)
	stdID := int(resp["template_id"].(float64))

	resp = env.call(http.MethodGet, withToken("/wxa/gettemplatelist", cat), nil)
	mustOK(t, "gettemplatelist", resp)
	if got := len(arrField(t, resp, "template_list")); got != 2 {
		t.Fatalf("模板数期望 2，实际 %d", got)
	}
	resp = env.call(http.MethodGet, withToken("/wxa/gettemplatelist?template_type=1", cat), nil)
	mustOK(t, "gettemplatelist(过滤标准模板)", resp)
	list := arrField(t, resp, "template_list")
	if len(list) != 1 {
		t.Fatalf("template_type=1 期望 1 条，实际 %v", list)
	}
	if item := list[0].(map[string]any); item["template_type"].(float64) != 1 || int(item["template_id"].(float64)) != stdID {
		t.Fatalf("template_type=1 过滤结果异常: %v", item)
	}
	resp = env.call(http.MethodGet, withToken("/wxa/gettemplatelist?template_type=0", cat), nil)
	mustOK(t, "gettemplatelist(过滤普通模板)", resp)
	normal := arrField(t, resp, "template_list")
	if len(normal) != 1 || int(normal[0].(map[string]any)["template_id"].(float64)) != tplID {
		t.Fatalf("template_type=0 过滤结果异常: %v", normal)
	}

	// 删除
	resp = env.call(http.MethodPost, withToken("/wxa/deletetemplate", cat), map[string]any{"template_id": tplID})
	mustOK(t, "deletetemplate", resp)
	resp = env.call(http.MethodGet, withToken("/wxa/gettemplatelist", cat), nil)
	if got := len(arrField(t, resp, "template_list")); got != 1 {
		t.Fatalf("删除后模板数期望 1，实际 %d", got)
	}
	// 草稿不存在
	resp = env.call(http.MethodPost, withToken("/wxa/addtotemplate", cat), map[string]any{"draft_id": 999})
	mustCode(t, "草稿不存在", resp, 85064)
}

// TestTemplateLimit 模板库上限 200，超限 85065。
func TestTemplateLimit(t *testing.T) {
	env := newEnv(t)
	cat := env.componentToken()
	for i := 0; i < templateLimit; i++ {
		resp := env.call(http.MethodPost, withToken("/wxa/addtotemplate", cat), map[string]any{"draft_id": 1})
		mustOK(t, "addtotemplate", resp)
	}
	resp := env.call(http.MethodGet, withToken("/wxa/gettemplatelist", cat), nil)
	if got := len(arrField(t, resp, "template_list")); got != templateLimit {
		t.Fatalf("模板数期望 %d，实际 %d", templateLimit, got)
	}
	resp = env.call(http.MethodPost, withToken("/wxa/addtotemplate", cat), map[string]any{"draft_id": 1})
	mustCode(t, "模板库已满", resp, 85065)
}

// ------------- 4. 代码 → 提审 → 推送 → 发布 主链路 -------------

// TestCodeChainWithPrivacyDelay 覆盖 commit / 隐私检测 / 提审 / 审核状态 / 发布。
func TestCodeChainWithPrivacyDelay(t *testing.T) {
	env := newEnv(t)
	appid, token := env.authorizer()
	env.srv.SetPrivacyCheckDelay(150 * time.Millisecond)

	// user_version 超过 64 字符 → 47001
	long := strings.Repeat("V", 65)
	resp := env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 1, "ext_json": `{"extAppid":"x"}`, "user_version": long, "user_desc": "d",
	})
	mustCode(t, "user_version 超长", resp, 47001)

	// ext_json 不是可解析 JSON → 85048
	resp = env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 1, "ext_json": `{"broken":`, "user_version": "V1", "user_desc": "d",
	})
	mustCode(t, "ext_json 非法", resp, 85048)

	extJSON := `{"extAppid":"wx_target","ext":{"k":"v"},"requiredPrivateInfos":["onLocationChange"]}`
	resp = env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 1, "ext_json": extJSON, "user_version": "V1.0.0", "user_desc": "首次提审",
	})
	mustOK(t, "commit", resp)
	if got := env.srv.ExtJSONOf(appid); got != extJSON {
		t.Fatalf("ExtJSONOf 期望 %s，实际 %s", extJSON, got)
	}

	// 隐私检测中：查检测结果与提审都应返回 61039
	resp = env.call(http.MethodGet, withToken("/wxa/security/get_code_privacy_info", token), nil)
	mustCode(t, "隐私检测未完成", resp, 61039)
	resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{
		"item_list": []any{map[string]any{"first_class": "工具", "second_class": "信息查询", "first_id": 1, "second_id": 101}},
	})
	mustCode(t, "检测未完成时提审", resp, 61039)

	time.Sleep(200 * time.Millisecond)
	resp = env.call(http.MethodGet, withToken("/wxa/security/get_code_privacy_info", token), nil)
	mustOK(t, "隐私检测完成", resp)
	if got := len(arrField(t, resp, "without_auth_list")); got != 0 {
		t.Fatalf("without_auth_list 期望空，实际 %v", resp["without_auth_list"])
	}

	// 提审
	resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
	mustOK(t, "submit_audit", resp)
	auditID := int64(resp["auditid"].(float64))
	if auditID <= 0 {
		t.Fatalf("auditid 应为正数，实际 %d", auditID)
	}
	gotID, gotStatus, ok := env.srv.AuditStatusOf(appid)
	if !ok || gotID != auditID || gotStatus != 2 {
		t.Fatalf("AuditStatusOf 期望 (%d,2,true)，实际 (%d,%d,%v)", auditID, gotID, gotStatus, ok)
	}
	// 重复提审 → 85009
	resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
	mustCode(t, "重复提审", resp, 85009)

	// 查询指定审核单
	resp = env.call(http.MethodPost, withToken("/wxa/get_auditstatus", token), map[string]any{"auditid": auditID})
	mustOK(t, "get_auditstatus", resp)
	if got := resp["status"].(float64); got != 2 {
		t.Fatalf("status 期望 2，实际 %v", got)
	}
	resp = env.call(http.MethodPost, withToken("/wxa/get_auditstatus", token), map[string]any{"auditid": 999999})
	mustCode(t, "无效 auditid", resp, 85012)

	// 最新审核单状态：截图字段故意是大写 ScreenShot
	resp = env.call(http.MethodGet, withToken("/wxa/get_latest_auditstatus", token), nil)
	mustOK(t, "get_latest_auditstatus", resp)
	if _, ok := resp["ScreenShot"]; !ok {
		t.Fatalf("get_latest_auditstatus 必须返回大写 ScreenShot: %v", resp)
	}
	if got := strField(t, resp, "user_version"); got != "V1.0.0" {
		t.Fatalf("user_version 期望 V1.0.0，实际 %q", got)
	}

	// 没有审核通过版本时不能发布 → 85019
	resp = env.call(http.MethodPost, withToken("/wxa/release", token), nil)
	mustCode(t, "release 缺 body", resp, 44002)
	resp = env.call(http.MethodPost, withToken("/wxa/release", token), map[string]any{})
	mustCode(t, "没有审核通过版本", resp, 85019)

	// 审核结果推送（成功）→ 状态置 0 → 可以发布
	if body, status, err := env.srv.PushAuditResult(context.Background(), env.cb.URL(), appid, "weapp_audit_success", ""); err != nil || status != http.StatusOK || body != "success" {
		t.Fatalf("PushAuditResult 失败: body=%q status=%d err=%v", body, status, err)
	}
	if _, gotStatus, _ := env.srv.AuditStatusOf(appid); gotStatus != 0 {
		t.Fatalf("推送成功后状态期望 0，实际 %d", gotStatus)
	}

	resp = env.call(http.MethodPost, withToken("/wxa/release", token), map[string]any{})
	mustOK(t, "release", resp)
	if got, ok := env.srv.ReleasedVersionOf(appid); !ok || got != "V1.0.0" {
		t.Fatalf("ReleasedVersionOf 期望 V1.0.0，实际 (%q,%v)", got, ok)
	}

	// 版本信息
	resp = env.call(http.MethodPost, withToken("/wxa/getversioninfo", token), map[string]any{})
	mustOK(t, "getversioninfo", resp)
	exp := objField(t, resp, "exp_info")
	if got := strField(t, exp, "exp_version"); got != "V1.0.0" {
		t.Fatalf("exp_info.exp_version 期望 V1.0.0，实际 %q", got)
	}
	rel := objField(t, resp, "release_info")
	if got := strField(t, rel, "release_version"); got != "V1.0.0" {
		t.Fatalf("release_info.release_version 期望 V1.0.0，实际 %q", got)
	}
	// 缺 body → 44002
	resp = env.call(http.MethodPost, withToken("/wxa/getversioninfo", token), nil)
	mustCode(t, "getversioninfo 缺 body", resp, 44002)

	// 额度
	resp = env.call(http.MethodGet, withToken("/wxa/queryquota", token), nil)
	mustOK(t, "queryquota", resp)
	if got := resp["rest"].(float64); got != defaultQuotaRest-1 {
		t.Fatalf("rest 期望 %d，实际 %v", defaultQuotaRest-1, got)
	}

	// 未 commit 过的小程序提审 → 85086
	_, otherToken := env.authorizer()
	resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", otherToken), map[string]any{})
	mustCode(t, "未上传代码就提审", resp, 85086)
}

// TestAuditQuotaAndSpeedup 额度用尽与加急额度。
func TestAuditQuotaAndSpeedup(t *testing.T) {
	env := newEnv(t)
	appid, token := env.authorizer()

	// 未 commit 就提审 → 85086
	resp := env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
	mustCode(t, "未上传代码", resp, 85086)

	env.srv.SetQuota(1, 10, 1, 3)
	resp = env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 1, "ext_json": `{}`, "user_version": "V1", "user_desc": "d",
	})
	mustOK(t, "commit", resp)
	resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
	mustOK(t, "submit_audit", resp)
	auditID := int64(resp["auditid"].(float64))

	resp = env.call(http.MethodGet, withToken("/wxa/queryquota", token), nil)
	if got := resp["rest"].(float64); got != 0 {
		t.Fatalf("提审后 rest 期望 0，实际 %v", got)
	}

	// 加急：第一次成功，第二次额度用尽 → 89405
	resp = env.call(http.MethodPost, withToken("/wxa/speedupaudit", token), map[string]any{"auditid": auditID})
	mustOK(t, "speedupaudit", resp)
	resp = env.call(http.MethodPost, withToken("/wxa/speedupaudit", token), map[string]any{"auditid": auditID})
	mustCode(t, "加急额度用尽", resp, 89405)

	// 驳回后额度为 0，再提审 → 85085
	if _, _, err := env.srv.PushAuditResult(context.Background(), env.cb.URL(), appid, "weapp_audit_fail", "类目不符"); err != nil {
		t.Fatalf("PushAuditResult 失败: %v", err)
	}
	_, status, _ := env.srv.AuditStatusOf(appid)
	if status != 1 {
		t.Fatalf("驳回后状态期望 1，实际 %d", status)
	}
	resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
	mustCode(t, "提审额度用尽", resp, 85085)

	// 尚未提审过的审核单加急 → 89402
	env.srv.SetQuota(10, 10, 5, 5)
	resp = env.call(http.MethodPost, withToken("/wxa/speedupaudit", token), map[string]any{"auditid": 999})
	mustCode(t, "审核单不存在", resp, 85012)
	resp = env.call(http.MethodPost, withToken("/wxa/speedupaudit", token), map[string]any{"auditid": auditID})
	mustCode(t, "不在待审核队列", resp, 89402)
}

// TestUndoCodeAudit 撤回审核的每天 5 次 / 每月 10 次限制。
func TestUndoCodeAudit(t *testing.T) {
	env := newEnv(t)
	_, token := env.authorizer()

	// 没有审核中的版本 → 85009
	resp := env.call(http.MethodGet, withToken("/wxa/undocodeaudit", token), nil)
	mustCode(t, "没有审核中版本", resp, 85009)

	resp = env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 1, "ext_json": `{}`, "user_version": "V1", "user_desc": "d",
	})
	mustOK(t, "commit", resp)

	for i := 0; i < 5; i++ {
		resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
		mustOK(t, "submit_audit", resp)
		resp = env.call(http.MethodGet, withToken("/wxa/undocodeaudit", token), nil)
		mustOK(t, "undocodeaudit", resp)
	}
	// 第 6 次撤回：当天额度已用尽
	resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
	mustOK(t, "submit_audit", resp)
	resp = env.call(http.MethodGet, withToken("/wxa/undocodeaudit", token), nil)
	mustCode(t, "撤回额度用尽", resp, 87013)
}

// TestRevertCodeRelease 版本回退链路。
func TestRevertCodeRelease(t *testing.T) {
	env := newEnv(t)
	appid, token := env.authorizer()

	release := func(version string) {
		t.Helper()
		resp := env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
			"template_id": 1, "ext_json": `{}`, "user_version": version, "user_desc": version + " 描述",
		})
		mustOK(t, "commit "+version, resp)
		resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
		mustOK(t, "submit_audit "+version, resp)
		if _, _, err := env.srv.PushAuditResult(context.Background(), env.cb.URL(), appid, "weapp_audit_success", ""); err != nil {
			t.Fatalf("PushAuditResult 失败: %v", err)
		}
		resp = env.call(http.MethodPost, withToken("/wxa/release", token), map[string]any{})
		mustOK(t, "release "+version, resp)
	}

	// 没有历史版本时回退 → 87012
	resp := env.call(http.MethodGet, withToken("/wxa/revertcoderelease", token), nil)
	mustCode(t, "没有上一个线上版本", resp, 87012)

	release("V1.0.0")
	release("V2.0.0")

	resp = env.call(http.MethodGet, withToken("/wxa/revertcoderelease?action=get_history_version", token), nil)
	mustOK(t, "get_history_version", resp)
	list := arrField(t, resp, "version_list")
	if len(list) != 1 {
		t.Fatalf("可回退版本期望 1 个，实际 %v", list)
	}
	entry := list[0].(map[string]any)
	if got := strField(t, entry, "user_version"); got != "V1.0.0" {
		t.Fatalf("可回退版本期望 V1.0.0，实际 %q", got)
	}
	appVersion := int64(entry["app_version"].(float64))

	resp = env.call(http.MethodGet, withToken("/wxa/revertcoderelease?app_version="+strconv.FormatInt(appVersion, 10), token), nil)
	mustOK(t, "revertcoderelease", resp)
	if got, ok := env.srv.ReleasedVersionOf(appid); !ok || got != "V1.0.0" {
		t.Fatalf("回退后线上版本期望 V1.0.0，实际 (%q,%v)", got, ok)
	}
	resp = env.call(http.MethodGet, withToken("/wxa/revertcoderelease", token), nil)
	mustCode(t, "回退后无可回退版本", resp, 87012)
}

// ------------- 5. 故障注入 -------------

// TestForceErrcode 故障注入：命中即立刻返回指定码，并仍然记入 Calls()。
func TestForceErrcode(t *testing.T) {
	env := newEnv(t)
	_, token := env.authorizer()

	env.srv.ForceErrcode("/wxa/commit", 85044)
	resp := env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 1, "ext_json": `{}`, "user_version": "V1", "user_desc": "d",
	})
	mustCode(t, "强制 85044", resp, 85044)
	if got := strField(t, resp, "errmsg"); got != "mock forced error" {
		t.Fatalf("errmsg 期望 mock forced error，实际 %q", got)
	}
	calls := env.srv.Calls()
	if last := calls[len(calls)-1]; last.Endpoint != "/wxa/commit" || last.Method != http.MethodPost {
		t.Fatalf("故障注入的调用也应记入 Calls()，实际 %+v", last)
	}

	env.srv.ForceErrcode("/wxa/commit", 9402202)
	resp = env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 1, "ext_json": `{}`, "user_version": "V1", "user_desc": "d",
	})
	mustCode(t, "强制 9402202", resp, 9402202)

	env.srv.ForceErrcode("/wxa/commit", 0)
	resp = env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 1, "ext_json": `{}`, "user_version": "V1", "user_desc": "d",
	})
	mustOK(t, "取消注入后恢复", resp)
}

// TestConcurrentLimit 开启并发限制后，同一 appid 的重复 commit/submit_audit 返回 9402202。
func TestConcurrentLimit(t *testing.T) {
	env := newEnv(t)
	_, token := env.authorizer()
	env.srv.concurrentWindow = 500 * time.Millisecond
	env.srv.SetConcurrentLimit(true)

	body := map[string]any{"template_id": 1, "ext_json": `{}`, "user_version": "V1", "user_desc": "d"}

	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			resp, err := env.tryCall(http.MethodPost, withToken("/wxa/commit", token), body)
			if err != nil {
				t.Errorf("并发 commit 失败: %v", err)
				return
			}
			code, _ := resp["errcode"].(float64)
			results[idx] = int(code)
		}(i)
	}
	close(start)
	wg.Wait()
	if results[0] != 9402202 && results[1] != 9402202 {
		t.Fatalf("并发 commit 期望其中一次 9402202，实际 %v", results)
	}

	// submit_audit 同理（先正常提交一次，窗口内的第二次视为并发）
	resp := env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
	mustOK(t, "submit_audit", resp)
	resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
	mustCode(t, "并发 submit_audit", resp, 9402202)

	// 关闭后恢复正常
	env.srv.SetConcurrentLimit(false)
	resp = env.call(http.MethodPost, withToken("/wxa/commit", token), body)
	mustOK(t, "关闭并发限制后恢复", resp)
}

// ------------- 6. 推送验签解密 -------------

// TestPushTicketDecrypt 校验 ticket 推送的签名、URL 参数与加密包体。
func TestPushTicketDecrypt(t *testing.T) {
	env := newEnv(t)
	crypt, err := wxcrypt.New(testVerifyToken, testAESKey, testComponentApp, "")
	if err != nil {
		t.Fatalf("构造 wxcrypt 失败: %v", err)
	}

	body, status, err := env.srv.PushTicket(context.Background(), env.cb.URL())
	if err != nil || status != http.StatusOK || body != "success" {
		t.Fatalf("PushTicket 失败: body=%q status=%d err=%v", body, status, err)
	}
	query, raw := env.cb.last(t)

	// URL 参数齐备
	for _, key := range []string{"signature", "timestamp", "nonce", "encrypt_type", "msg_signature"} {
		if query.Get(key) == "" {
			t.Fatalf("URL 缺少参数 %s: %v", key, query)
		}
	}
	if got := query.Get("encrypt_type"); got != "aes" {
		t.Fatalf("encrypt_type 期望 aes，实际 %q", got)
	}

	// 包体结构
	if !strings.HasPrefix(raw, "<xml><ToUserName><![CDATA[") || !strings.Contains(raw, "<Encrypt><![CDATA[") {
		t.Fatalf("包体结构不符: %s", raw)
	}
	if got := extractTag(t, raw, "ToUserName"); !strings.HasPrefix(got, "gh_") {
		t.Fatalf("ToUserName 期望 gh_ 原始 ID，实际 %q", got)
	}
	encrypt := extractTag(t, raw, "Encrypt")

	// signature 是 3 字段明文签名，msg_signature 是 4 字段含密文签名，两者必须不同且各自正确
	if got := query.Get("signature"); got != sha1HexSorted(testVerifyToken, query.Get("timestamp"), query.Get("nonce")) {
		t.Fatalf("signature 不是 3 字段签名: %s", got)
	}
	if got, want := query.Get("msg_signature"), crypt.ComputeSignature(query.Get("timestamp"), query.Get("nonce"), encrypt); got != want {
		t.Fatalf("msg_signature 期望 %s，实际 %s", want, got)
	}
	if query.Get("signature") == query.Get("msg_signature") {
		t.Fatalf("signature 与 msg_signature 不应相同（覆盖真实行为）")
	}

	// 解密（receiveid 为第三方平台 appid）
	plain, receiveID, err := crypt.DecryptMsg(query.Get("msg_signature"), query.Get("timestamp"), query.Get("nonce"), raw, testComponentApp)
	if err != nil {
		t.Fatalf("解密 ticket 推送失败: %v", err)
	}
	if receiveID != testComponentApp {
		t.Fatalf("receiveid 期望 %s，实际 %s", testComponentApp, receiveID)
	}
	if got := extractTag(t, plain, "InfoType"); got != "component_verify_ticket" {
		t.Fatalf("InfoType 期望 component_verify_ticket，实际 %q", got)
	}
	ticket := extractTag(t, plain, "ComponentVerifyTicket")
	if ticket == "" || ticket != env.srv.lastTicket {
		t.Fatalf("ComponentVerifyTicket 期望 %s，实际 %q", env.srv.lastTicket, ticket)
	}
	if got := extractTag(t, plain, "AppId"); got != testComponentApp {
		t.Fatalf("AppId 期望 %s，实际 %q", testComponentApp, got)
	}

	// 用推送里的 ticket 换令牌，等于把整条链路串起来
	resp := env.call(http.MethodPost, testComponentPath, map[string]any{
		"component_appid": testComponentApp, "component_appsecret": "s", "component_verify_ticket": ticket,
	})
	mustOK(t, "用推送的 ticket 换令牌", resp)
}

// TestPushAuthorizeEventDecrypt 校验授权事件推送，并验证 unauthorized 会作废令牌。
func TestPushAuthorizeEventDecrypt(t *testing.T) {
	env := newEnv(t)
	crypt, err := wxcrypt.New(testVerifyToken, testAESKey, testComponentApp, "")
	if err != nil {
		t.Fatalf("构造 wxcrypt 失败: %v", err)
	}
	appid := env.srv.AddAuthorizer("推送授权小程序", []int{18})

	if _, _, err := env.srv.PushAuthorizeEvent(context.Background(), env.cb.URL(), "authorized", appid); err != nil {
		t.Fatalf("PushAuthorizeEvent 失败: %v", err)
	}
	query, raw := env.cb.last(t)
	plain, receiveID, err := crypt.DecryptMsg(query.Get("msg_signature"), query.Get("timestamp"), query.Get("nonce"), raw, testComponentApp)
	if err != nil {
		t.Fatalf("解密授权事件失败: %v", err)
	}
	if receiveID != testComponentApp {
		t.Fatalf("授权事件 receiveid 期望 %s，实际 %s", testComponentApp, receiveID)
	}
	if got := extractTag(t, plain, "InfoType"); got != "authorized" {
		t.Fatalf("InfoType 期望 authorized，实际 %q", got)
	}
	if got := extractTag(t, plain, "AuthorizerAppid"); got != appid {
		t.Fatalf("AuthorizerAppid 期望 %s，实际 %q", appid, got)
	}
	code := extractTag(t, plain, "AuthorizationCode")
	if code == "" || extractTag(t, plain, "AuthorizationCodeExpiredTime") == "" || extractTag(t, plain, "PreAuthCode") == "" {
		t.Fatalf("授权事件缺少 AuthorizationCode / 过期时间 / PreAuthCode: %s", plain)
	}

	// 推送里的授权码可以直接换授权信息（真实链路就是这样）
	cat := env.componentToken()
	resp := env.call(http.MethodPost, withToken("/cgi-bin/component/api_query_auth", cat), map[string]any{
		"component_appid": testComponentApp, "authorization_code": code,
	})
	mustOK(t, "用推送的 authorization_code 换取授权信息", resp)
	token := strField(t, objField(t, resp, "authorization_info"), "authorizer_access_token")

	// updateauthorized：会换发新的 refresh_token
	if _, _, err := env.srv.PushAuthorizeEvent(context.Background(), env.cb.URL(), "updateauthorized", appid); err != nil {
		t.Fatalf("PushAuthorizeEvent(updateauthorized) 失败: %v", err)
	}
	query, raw = env.cb.last(t)
	plain, _, err = crypt.DecryptMsg(query.Get("msg_signature"), query.Get("timestamp"), query.Get("nonce"), raw, testComponentApp)
	if err != nil {
		t.Fatalf("解密 updateauthorized 失败: %v", err)
	}
	if got := extractTag(t, plain, "InfoType"); got != "updateauthorized" {
		t.Fatalf("InfoType 期望 updateauthorized，实际 %q", got)
	}

	// unauthorized：令牌立即失效
	if _, _, err := env.srv.PushAuthorizeEvent(context.Background(), env.cb.URL(), "unauthorized", appid); err != nil {
		t.Fatalf("PushAuthorizeEvent(unauthorized) 失败: %v", err)
	}
	resp = env.call(http.MethodGet, withToken("/wxa/get_page", token), nil)
	mustCode(t, "取消授权后令牌失效", resp, 40001)

	// 非法事件类型
	if _, _, err := env.srv.PushAuthorizeEvent(context.Background(), env.cb.URL(), "unknown_type", appid); err == nil {
		t.Fatalf("非法事件类型应当返回 error")
	}
}

// TestPushAuditResultDecrypt 校验审核结果推送的报文结构与解密口径。
func TestPushAuditResultDecrypt(t *testing.T) {
	env := newEnv(t)
	appid, token := env.authorizer()
	crypt, err := wxcrypt.New(testVerifyToken, testAESKey, testComponentApp, "")
	if err != nil {
		t.Fatalf("构造 wxcrypt 失败: %v", err)
	}

	resp := env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 1, "ext_json": `{}`, "user_version": "V1", "user_desc": "d",
	})
	mustOK(t, "commit", resp)
	resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
	mustOK(t, "submit_audit", resp)

	// 驳回推送（带原因与截图）
	reason := "1:账号信息不符合规范:<br>(1):包含色情因素"
	body, status, err := env.srv.PushAuditResult(context.Background(), env.cb.URL()+"?extra=$APPID$", appid, "weapp_audit_fail", reason)
	if err != nil || status != http.StatusOK || body != "success" {
		t.Fatalf("PushAuditResult 失败: body=%q status=%d err=%v", body, status, err)
	}
	query, raw := env.cb.last(t)
	if got := query.Get("extra"); got != appid {
		t.Fatalf("$APPID$ 占位符未替换，实际 %q", got)
	}
	// 代收授权方消息的 receiveid 是授权方 appid
	plain, receiveID, err := crypt.DecryptMsg(query.Get("msg_signature"), query.Get("timestamp"), query.Get("nonce"), raw, appid)
	if err != nil {
		t.Fatalf("解密审核结果推送失败: %v", err)
	}
	if receiveID != appid {
		t.Fatalf("receiveid 期望 %s，实际 %s", appid, receiveID)
	}
	if got := extractTag(t, plain, "MsgType"); got != "event" {
		t.Fatalf("MsgType 期望 event，实际 %q", got)
	}
	if got := extractTag(t, plain, "Event"); got != "weapp_audit_fail" {
		t.Fatalf("Event 期望 weapp_audit_fail，实际 %q", got)
	}
	if got := extractTag(t, plain, "ToUserName"); !strings.HasPrefix(got, "gh_") {
		t.Fatalf("ToUserName 期望小程序原始 ID（gh_ 前缀），实际 %q", got)
	}
	if got := extractTag(t, plain, "Reason"); got != reason {
		t.Fatalf("Reason 期望 %q，实际 %q", reason, got)
	}
	if extractTag(t, plain, "FailTime") == "" || extractTag(t, plain, "ScreenShot") == "" {
		t.Fatalf("驳回推送缺少 FailTime / ScreenShot: %s", plain)
	}

	// 延后推送
	if _, _, err := env.srv.PushAuditResult(context.Background(), env.cb.URL(), appid, "weapp_audit_delay", ""); err != nil {
		t.Fatalf("PushAuditResult(delay) 失败: %v", err)
	}
	query, raw = env.cb.last(t)
	plain, _, err = crypt.DecryptMsg(query.Get("msg_signature"), query.Get("timestamp"), query.Get("nonce"), raw, appid)
	if err != nil {
		t.Fatalf("解密延后推送失败: %v", err)
	}
	if got := extractTag(t, plain, "Event"); got != "weapp_audit_delay" || extractTag(t, plain, "DelayTime") == "" {
		t.Fatalf("延后推送结构不符: %s", plain)
	}

	// 非法事件类型
	if _, _, err := env.srv.PushAuditResult(context.Background(), env.cb.URL(), appid, "weapp_audit_unknown", ""); err == nil {
		t.Fatalf("非法审核事件应当返回 error")
	}
}

// ------------- 7. 其余接口 -------------

// TestMiscEndpoints 类目 / 隐私设置 / 域名 / 服务状态 / 基础库 / 页面 / 二维码。
func TestMiscEndpoints(t *testing.T) {
	env := newEnv(t)
	_, token := env.authorizer()

	// 页面列表
	resp := env.call(http.MethodGet, withToken("/wxa/get_page", token), nil)
	mustOK(t, "get_page", resp)
	pages := arrField(t, resp, "page_list")
	if len(pages) != 2 || pages[0] != "index" || pages[1] != "pages/list/index" {
		t.Fatalf("page_list 不符: %v", pages)
	}

	// 二维码：成功时是二进制 JPEG
	rawResp, raw := env.rawCall(http.MethodGet, withToken("/wxa/get_qrcode", token), nil)
	if got := rawResp.Header.Get("Content-Type"); !strings.HasPrefix(got, "image/jpeg") {
		t.Fatalf("Content-Type 期望 image/jpeg，实际 %q", got)
	}
	if _, err := jpeg.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatalf("二维码不是合法 JPEG: %v", err)
	}
	// 故障注入时返回 JSON
	env.srv.ForceErrcode("/wxa/get_qrcode", 40001)
	resp = env.call(http.MethodGet, withToken("/wxa/get_qrcode", token), nil)
	mustCode(t, "二维码故障注入", resp, 40001)
	env.srv.ForceErrcode("/wxa/get_qrcode", 0)

	// 类目
	resp = env.call(http.MethodGet, withToken("/wxa/get_category", token), nil)
	mustOK(t, "get_category", resp)
	cats := arrField(t, resp, "category_list")
	if len(cats) < 2 {
		t.Fatalf("category_list 至少 2 组，实际 %v", cats)
	}
	threeLevel := false
	for _, item := range cats {
		m := item.(map[string]any)
		for _, key := range []string{"first_class", "second_class", "third_class", "first_id", "second_id", "third_id"} {
			if _, ok := m[key]; !ok {
				t.Fatalf("类目缺少字段 %s: %v", key, m)
			}
		}
		if strField(t, m, "third_class") != "" {
			threeLevel = true
		}
	}
	if !threeLevel {
		t.Fatalf("category_list 必须含三级类目: %v", cats)
	}
	resp = env.call(http.MethodGet, withToken("/cgi-bin/wxopen/getallcategories", token), nil)
	mustOK(t, "getallcategories", resp)
	if got := arrField(t, resp, "categories_list"); len(got) < 2 {
		t.Fatalf("categories_list 至少 2 组，实际 %v", got)
	}
	resp = env.call(http.MethodGet, withToken("/cgi-bin/wxopen/getcategory", token), nil)
	mustOK(t, "getcategory", resp)
	if got := resp["limit"].(float64); got != 5 {
		t.Fatalf("limit 期望 5，实际 %v", got)
	}
	if got := resp["quota"].(float64); got != 4 {
		t.Fatalf("quota 期望 4，实际 %v", got)
	}
	if got := resp["category_limit"].(float64); got != 20 {
		t.Fatalf("category_limit 期望 20，实际 %v", got)
	}
	first := arrField(t, resp, "categories")[0].(map[string]any)
	if got := first["audit_status"].(float64); got != 3 {
		t.Fatalf("audit_status 期望 3，实际 %v", got)
	}
	if _, ok := first["first_name"]; !ok {
		t.Fatalf("categories[0] 缺少 first_name: %v", first)
	}

	// 隐私设置
	resp = env.call(http.MethodPost, withToken("/cgi-bin/component/getprivacysetting", token), map[string]any{})
	mustOK(t, "getprivacysetting", resp)
	if got := resp["code_exist"].(float64); got != 1 {
		t.Fatalf("code_exist 期望 1，实际 %v", got)
	}
	if got := arrField(t, resp, "setting_list"); len(got) == 0 {
		t.Fatalf("setting_list 不应为空")
	}
	if _, ok := resp["owner_setting"].(map[string]any); !ok {
		t.Fatalf("owner_setting 缺失: %v", resp["owner_setting"])
	}

	// 域名
	resp = env.call(http.MethodPost, withToken("/wxa/modify_domain", token), map[string]any{
		"action": "set", "requestdomain": []string{"https://api.example.com"}, "uploaddomain": []string{"https://up.example.com"},
	})
	mustOK(t, "modify_domain", resp)
	resp = env.call(http.MethodPost, withToken("/wxa/modify_domain", token), map[string]any{
		"action": "add", "requestdomain": []string{"https://api2.example.com"},
	})
	mustOK(t, "modify_domain(add)", resp)
	resp = env.call(http.MethodPost, withToken("/wxa/modify_domain", token), map[string]any{"action": "get"})
	mustOK(t, "modify_domain(get)", resp)
	domains := arrField(t, resp, "requestdomain")
	if len(domains) != 2 {
		t.Fatalf("requestdomain 期望 2 个，实际 %v", domains)
	}
	resp = env.call(http.MethodPost, withToken("/wxa/get_effective_domain", token), map[string]any{})
	mustOK(t, "get_effective_domain", resp)
	if got := len(arrField(t, resp, "uploaddomain")); got != 1 {
		t.Fatalf("get_effective_domain.uploaddomain 期望 1，实际 %d", got)
	}
	// 必须收到 {}
	resp = env.call(http.MethodPost, withToken("/wxa/get_effective_domain", token), nil)
	mustCode(t, "get_effective_domain 缺 body", resp, 44002)
	resp = env.call(http.MethodPost, withToken("/wxa/get_effective_domain", token), map[string]any{"a": 1})
	mustCode(t, "get_effective_domain 非空 body", resp, 44002)

	// 业务域名
	resp = env.call(http.MethodPost, withToken("/wxa/setwebviewdomain", token), map[string]any{
		"action": "set", "webviewdomain": []string{"https://web.example.com"},
	})
	mustOK(t, "setwebviewdomain", resp)
	resp = env.call(http.MethodPost, withToken("/wxa/get_effective_webviewdomain", token), map[string]any{})
	mustOK(t, "get_effective_webviewdomain", resp)
	if got := len(arrField(t, resp, "webviewdomain")); got != 1 {
		t.Fatalf("webviewdomain 期望 1，实际 %v", resp["webviewdomain"])
	}
	resp = env.call(http.MethodPost, withToken("/wxa/get_effective_webviewdomain", token), nil)
	mustCode(t, "get_effective_webviewdomain 缺 body", resp, 44002)

	// 直接配置域名（modify_domain_directly / setwebviewdomain_directly）
	resp = env.call(http.MethodPost, withToken("/wxa/modify_domain_directly", token), map[string]any{
		"action": "set", "requestdomain": []string{"https://direct.example.com"},
	})
	mustOK(t, "modify_domain_directly", resp)
	resp = env.call(http.MethodPost, withToken("/wxa/setwebviewdomain_directly", token), map[string]any{
		"action": "set", "webviewdomain": []string{"https://direct-web.example.com"},
	})
	mustOK(t, "setwebviewdomain_directly", resp)

	// 服务状态
	resp = env.call(http.MethodPost, withToken("/wxa/getvisitstatus", token), map[string]any{})
	mustOK(t, "getvisitstatus", resp)
	if got := resp["status"].(float64); got != 1 {
		t.Fatalf("默认服务状态期望 1，实际 %v", got)
	}
	resp = env.call(http.MethodPost, withToken("/wxa/change_visitstatus", token), map[string]any{"action": "close"})
	mustOK(t, "change_visitstatus", resp)
	resp = env.call(http.MethodPost, withToken("/wxa/getvisitstatus", token), map[string]any{})
	if got := resp["status"].(float64); got != 0 {
		t.Fatalf("暂停服务后期望 0，实际 %v", got)
	}
	resp = env.call(http.MethodPost, withToken("/wxa/change_visitstatus", token), map[string]any{"action": "bad"})
	mustCode(t, "非法 action", resp, 40097)
	resp = env.call(http.MethodPost, withToken("/wxa/getvisitstatus", token), nil)
	mustCode(t, "getvisitstatus 缺 body", resp, 44002)

	// 基础库
	resp = env.call(http.MethodPost, withToken("/cgi-bin/wxopen/getweappsupportversion", token), map[string]any{})
	mustOK(t, "getweappsupportversion", resp)
	if got := strField(t, resp, "now_version"); got == "" {
		t.Fatalf("now_version 为空")
	}
	if items := arrField(t, objField(t, resp, "uv_info"), "items"); len(items) == 0 {
		t.Fatalf("uv_info.items 为空")
	}
	resp = env.call(http.MethodPost, withToken("/cgi-bin/wxopen/setweappsupportversion", token), map[string]any{"version": "2.21.0"})
	mustOK(t, "setweappsupportversion", resp)
	resp = env.call(http.MethodPost, withToken("/cgi-bin/wxopen/setweappsupportversion", token), map[string]any{"version": "abc"})
	mustCode(t, "非法版本号", resp, 89014)
}

// TestGrayRelease 分阶段发布（补充能力，任务书未要求，但上层可能用到）。
func TestGrayRelease(t *testing.T) {
	env := newEnv(t)
	appid, token := env.authorizer()

	// 没有线上版本 → 85079
	resp := env.call(http.MethodPost, withToken("/wxa/grayrelease", token), map[string]any{"gray_percentage": 10})
	mustCode(t, "没有线上版本", resp, 85079)

	resp = env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 1, "ext_json": `{}`, "user_version": "V1", "user_desc": "d",
	})
	mustOK(t, "commit", resp)
	resp = env.call(http.MethodPost, withToken("/wxa/submit_audit", token), map[string]any{})
	mustOK(t, "submit_audit", resp)
	if _, _, err := env.srv.PushAuditResult(context.Background(), env.cb.URL(), appid, "weapp_audit_success", ""); err != nil {
		t.Fatalf("PushAuditResult 失败: %v", err)
	}
	resp = env.call(http.MethodPost, withToken("/wxa/release", token), map[string]any{})
	mustOK(t, "release", resp)

	resp = env.call(http.MethodGet, withToken("/wxa/getgrayreleaseplan", token), nil)
	mustOK(t, "getgrayreleaseplan", resp)
	if got := objField(t, resp, "gray_release_plan")["status"].(float64); got != 0 {
		t.Fatalf("初始状态期望 0，实际 %v", got)
	}

	resp = env.call(http.MethodPost, withToken("/wxa/grayrelease", token), map[string]any{"gray_percentage": 10, "support_debuger_first": true})
	mustOK(t, "grayrelease", resp)
	resp = env.call(http.MethodGet, withToken("/wxa/getgrayreleaseplan", token), nil)
	plan := objField(t, resp, "gray_release_plan")
	if plan["status"].(float64) != 1 || plan["gray_percentage"].(float64) != 10 {
		t.Fatalf("灰度计划不符: %v", plan)
	}
	// 比例必须递增
	resp = env.call(http.MethodPost, withToken("/wxa/grayrelease", token), map[string]any{"gray_percentage": 10})
	mustCode(t, "比例未递增", resp, 85082)
	// 非法比例
	resp = env.call(http.MethodPost, withToken("/wxa/grayrelease", token), map[string]any{"gray_percentage": 101})
	mustCode(t, "非法比例", resp, 85081)
	// 取消灰度
	resp = env.call(http.MethodGet, withToken("/wxa/revertgrayrelease", token), nil)
	mustOK(t, "revertgrayrelease", resp)
	resp = env.call(http.MethodGet, withToken("/wxa/getgrayreleaseplan", token), nil)
	if got := objField(t, resp, "gray_release_plan")["status"].(float64); got != 4 {
		t.Fatalf("取消后状态期望 4，实际 %v", got)
	}
}

// ------------- 8. 调用记录与并发 -------------

// TestCallsAndResetCalls 调用记录包含 endpoint/method/token/body，并可重置。
func TestCallsAndResetCalls(t *testing.T) {
	env := newEnv(t)
	_, token := env.authorizer()
	env.srv.ResetCalls()

	env.call(http.MethodPost, withToken("/wxa/commit", token), map[string]any{
		"template_id": 7, "ext_json": `{"extAppid":"x"}`, "user_version": "V9", "user_desc": "记录",
	})
	calls := env.srv.Calls()
	if len(calls) != 1 {
		t.Fatalf("调用记录期望 1 条，实际 %d", len(calls))
	}
	call := calls[0]
	if call.Endpoint != "/wxa/commit" || call.Method != http.MethodPost {
		t.Fatalf("调用记录 endpoint/method 不符: %+v", call)
	}
	if call.Token != token {
		t.Fatalf("调用记录 token 不符: %q", call.Token)
	}
	if call.Body == nil || call.Body["user_version"] != "V9" {
		t.Fatalf("调用记录 body 不符: %v", call.Body)
	}
	if call.At.IsZero() {
		t.Fatalf("调用记录时间缺失")
	}
	env.srv.ResetCalls()
	if got := len(env.srv.Calls()); got != 0 {
		t.Fatalf("ResetCalls 后应为 0 条，实际 %d", got)
	}
}

// TestConcurrentAccess 并发压测：所有状态都由互斥锁保护，go test -race 必须通过。
func TestConcurrentAccess(t *testing.T) {
	env := newEnv(t)
	_, token := env.authorizer()
	cat := env.componentToken()

	var wg sync.WaitGroup
	paths := []struct {
		method string
		path   string
		body   any
		token  string
	}{
		{http.MethodPost, "/wxa/commit", map[string]any{"template_id": 1, "ext_json": `{}`, "user_version": "V1", "user_desc": "d"}, token},
		{http.MethodGet, "/wxa/security/get_code_privacy_info", nil, token},
		{http.MethodPost, "/wxa/submit_audit", map[string]any{}, token},
		{http.MethodGet, "/wxa/get_latest_auditstatus", nil, token},
		{http.MethodGet, "/wxa/queryquota", nil, token},
		{http.MethodGet, "/wxa/get_page", nil, token},
		{http.MethodGet, "/wxa/get_category", nil, token},
		{http.MethodPost, "/wxa/modify_domain", map[string]any{"action": "set", "requestdomain": []string{"https://a.example.com"}}, token},
		{http.MethodPost, "/wxa/get_effective_domain", map[string]any{}, token},
		{http.MethodGet, "/wxa/gettemplatedraftlist", nil, cat},
		{http.MethodGet, "/wxa/gettemplatelist", nil, cat},
		{http.MethodPost, "/cgi-bin/component/api_get_authorizer_list", map[string]any{"component_appid": testComponentApp, "offset": 0, "count": 10}, cat},
		{http.MethodPost, "/cgi-bin/component/api_create_preauthcode", map[string]any{"component_appid": testComponentApp}, cat},
	}
	for i := 0; i < 40; i++ {
		spec := paths[i%len(paths)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := env.tryCall(spec.method, withToken(spec.path, spec.token), spec.body)
			if err != nil {
				t.Errorf("%s %s: %v", spec.method, spec.path, err)
				return
			}
			if _, ok := resp["errcode"]; !ok {
				t.Errorf("%s %s 响应缺少 errcode", spec.method, spec.path)
			}
		}()
	}
	// 推送与调用记录读写也并发跑一遍
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_, _, _ = env.srv.PushTicket(context.Background(), env.cb.URL())
			_ = env.srv.Calls()
		}
	}()
	wg.Wait()
}

// extractTag 从 XML 中取第一个指定标签的文本（值可能被 CDATA 包裹）。
func extractTag(t *testing.T, xmlText, tag string) string {
	t.Helper()
	start := strings.Index(xmlText, "<"+tag+">")
	if start < 0 {
		return ""
	}
	rest := xmlText[start+len(tag)+2:]
	end := strings.Index(rest, "</"+tag+">")
	if end < 0 {
		return ""
	}
	value := rest[:end]
	value = strings.TrimPrefix(value, "<![CDATA[")
	value = strings.TrimSuffix(value, "]]>")
	return value
}

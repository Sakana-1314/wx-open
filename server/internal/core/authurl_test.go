package core

import (
	"net/url"
	"strings"
	"testing"
)

// TestAuthURLs 校验 PC / 移动端授权链接的拼接（含 biz_appid 与权限集列表）。
func TestAuthURLs(t *testing.T) {
	req := AuthURLRequest{
		ComponentAppid: "wx_component",
		PreAuthCode:    "pre_code_123",
		RedirectURI:    "https://platform.example.com/authorize/callback",
		AuthType:       AuthTypeMiniProgram,
		BizAppid:       "wx_biz_1",
		CategoryIDs:    []int{18, 30},
	}
	pc, mobile, err := req.AuthURLs()
	if err != nil {
		t.Fatalf("拼接失败: %v", err)
	}
	if !strings.HasPrefix(pc, PcAuthorizeURL+"?") {
		t.Fatalf("PC 链接前缀异常: %s", pc)
	}
	parsed, err := url.Parse(pc)
	if err != nil {
		t.Fatalf("PC 链接无法解析: %v", err)
	}
	q := parsed.Query()
	for key, want := range map[string]string{
		"component_appid":  "wx_component",
		"pre_auth_code":    "pre_code_123",
		"redirect_uri":     "https://platform.example.com/authorize/callback",
		"auth_type":        "2",
		"biz_appid":        "wx_biz_1",
		"category_id_list": "18|30",
	} {
		if got := q.Get(key); got != want {
			t.Fatalf("PC 链接参数 %s 期望 %q，实际 %q", key, want, got)
		}
	}
	if !strings.Contains(mobile, "action=bindcomponent") || !strings.Contains(mobile, "no_scan=1") {
		t.Fatalf("移动端链接缺少必要参数: %s", mobile)
	}
	if !strings.HasSuffix(mobile, "#wechat_redirect") {
		t.Fatalf("移动端链接缺少 #wechat_redirect 锚点: %s", mobile)
	}
}

// TestAuthURLsValidation 参数校验：缺预授权码 / 缺回调地址 / 非法 auth_type。
func TestAuthURLsValidation(t *testing.T) {
	base := AuthURLRequest{ComponentAppid: "wx_c", PreAuthCode: "p", RedirectURI: "https://a.example.com/cb"}

	if _, _, err := (AuthURLRequest{ComponentAppid: "wx_c", RedirectURI: "https://a/cb"}).AuthURLs(); err == nil {
		t.Fatalf("缺预授权码应当报错")
	}
	noRedirect := base
	noRedirect.RedirectURI = ""
	if _, _, err := noRedirect.AuthURLs(); err == nil {
		t.Fatalf("缺回调地址应当报错")
	}
	badType := base
	badType.AuthType = AuthType(99)
	if _, _, err := badType.AuthURLs(); err == nil {
		t.Fatalf("非法 auth_type 应当报错")
	}
	noAppid := base
	noAppid.ComponentAppid = ""
	if _, _, err := noAppid.AuthURLs(); err == nil {
		t.Fatalf("缺 component_appid 应当报错")
	}
	// 未指定 auth_type 时应默认小程序（2）。
	pc, _, err := base.AuthURLs()
	if err != nil {
		t.Fatalf("默认 auth_type 应当可用: %v", err)
	}
	if !strings.Contains(pc, "auth_type=2") {
		t.Fatalf("默认 auth_type 应为 2: %s", pc)
	}
}

// TestCallbackURLs 回调地址拼接（消息与事件接收 URL 必须包含 $APPID$ 占位）。
func TestCallbackURLs(t *testing.T) {
	authURL, msgURL := CallbackURLs("https://platform.example.com/")
	if authURL != "https://platform.example.com/callback/component" {
		t.Fatalf("授权事件接收 URL 异常: %s", authURL)
	}
	if msgURL != "https://platform.example.com/callback/message/$APPID$" {
		t.Fatalf("消息与事件接收 URL 异常: %s", msgURL)
	}
	if got := BuildRedirectURI("https://platform.example.com"); got != "https://platform.example.com/authorize/callback" {
		t.Fatalf("授权回调地址异常: %s", got)
	}
}

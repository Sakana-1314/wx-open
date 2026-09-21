package wxauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
)

func TestVisitStatusAndPages(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	te.wx.jsonRoute("/wxa/getvisitstatus", map[string]any{"status": 0})
	te.wx.jsonRoute("/wxa/change_visitstatus", map[string]any{"errcode": 0, "errmsg": "ok"})
	te.wx.jsonRoute("/wxa/get_page", map[string]any{"page_list": []string{"pages/index/index", "pages/detail/index"}})
	svc := NewAppService(te.env)

	status, err := svc.VisitStatus(ctx, testAuthorizerAppid)
	if err != nil {
		t.Fatalf("VisitStatus 失败: %v", err)
	}
	if status.Appid != testAuthorizerAppid || !status.Paused {
		t.Fatalf("status=0 应表示已暂停服务，实际 %+v", status)
	}
	// 代调用接口必须用 authorizer_access_token（用错令牌会 61014）。
	if got := te.wx.lastCallTo("/wxa/getvisitstatus").Query.Get("access_token"); got != "AUTHORIZER-ACCESS-TOKEN" {
		t.Errorf("getvisitstatus 的 access_token = %q", got)
	}

	resumed, err := svc.SetVisitStatus(ctx, testAuthorizerAppid, gen.VisitStatusUpdateRequest{Paused: false})
	if err != nil {
		t.Fatalf("SetVisitStatus 失败: %v", err)
	}
	if resumed.Paused {
		t.Errorf("恢复服务后 Paused 应为 false，实际 %+v", resumed)
	}
	if action := te.wx.lastCallTo("/wxa/change_visitstatus").Body["action"]; action != "open" {
		t.Errorf("恢复服务应发送 action=open，实际 %v", action)
	}
	if _, err := svc.SetVisitStatus(ctx, testAuthorizerAppid, gen.VisitStatusUpdateRequest{Paused: true}); err != nil {
		t.Fatalf("暂停服务失败: %v", err)
	}
	if action := te.wx.lastCallTo("/wxa/change_visitstatus").Body["action"]; action != "close" {
		t.Errorf("暂停服务应发送 action=close，实际 %v", action)
	}
	te.requireAction(actionSetVisitStatus)

	pages, err := svc.Pages(ctx, testAuthorizerAppid)
	if err != nil {
		t.Fatalf("Pages 失败: %v", err)
	}
	if len(pages) != 2 || pages[1] != "pages/detail/index" {
		t.Fatalf("页面列表不对: %v", pages)
	}

	// 未授权的小程序不允许代调用。
	te.seedAuthorizer(secondAuthorizerAppid, "REFRESH-TOKEN-2", nil)
	if err := te.env.Repos.Authorizers.MarkUnauthorized(ctx, secondAuthorizerAppid, time.Now()); err != nil {
		t.Fatalf("标记取消授权失败: %v", err)
	}
	if _, err := svc.VisitStatus(ctx, secondAuthorizerAppid); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("已取消授权的小程序应返回冲突，实际 %v", err)
	}
	if _, err := svc.Pages(ctx, "wx-not-exists"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("不存在的小程序应返回 ErrNotFound，实际 %v", err)
	}
}

func TestSupportVersionRoundTrip(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	te.wx.jsonRoute("/cgi-bin/wxopen/getweappsupportversion", map[string]any{
		"now_version": "2.10.0",
		"uv_info": map[string]any{
			"items": []map[string]any{
				{"version": "2.10.0", "percentage": 88.5},
				{"version": "2.9.0", "percentage": 11.5},
			},
		},
	})
	te.wx.jsonRoute("/cgi-bin/wxopen/setweappsupportversion", map[string]any{"errcode": 0, "errmsg": "ok"})
	svc := NewAppService(te.env)

	before, err := svc.SupportVersion(ctx, testAuthorizerAppid)
	if err != nil {
		t.Fatalf("SupportVersion 失败: %v", err)
	}
	if before.NowVersion == nil || *before.NowVersion != "2.10.0" {
		t.Fatalf("nowVersion 不对: %v", before.NowVersion)
	}
	if before.UvItems == nil || len(*before.UvItems) != 2 || (*before.UvItems)[0].Percentage != 88.5 {
		t.Fatalf("uvItems 不对: %+v", before.UvItems)
	}

	if _, err := svc.SetSupportVersion(ctx, testAuthorizerAppid, gen.SupportVersionUpdateRequest{Version: "  "}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("空版本号应返回校验错误，实际 %v", err)
	}
	if n := te.wx.countTo("/cgi-bin/wxopen/setweappsupportversion"); n != 0 {
		t.Fatalf("校验失败不应调用微信，实际 %d 次", n)
	}

	after, err := svc.SetSupportVersion(ctx, testAuthorizerAppid, gen.SupportVersionUpdateRequest{Version: "2.20.0"})
	if err != nil {
		t.Fatalf("SetSupportVersion 失败: %v", err)
	}
	if body := te.wx.lastCallTo("/cgi-bin/wxopen/setweappsupportversion").Body; body["version"] != "2.20.0" {
		t.Errorf("setweappsupportversion 请求体不对: %v", body)
	}
	// 设置成功后回读用户占比。
	if after.NowVersion == nil || *after.NowVersion != "2.10.0" || after.UvItems == nil {
		t.Errorf("设置后应回读基础库信息: %+v", after)
	}
	te.requireAction(actionSetSupportVersion)
}

func TestApplyDomainsDirectModeUsesDirectlyEndpoints(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	te.wx.jsonRoute("/wxa/modify_domain_directly", map[string]any{"errcode": 0, "errmsg": "ok"})
	te.wx.jsonRoute("/wxa/setwebviewdomain_directly", map[string]any{
		"webviewdomain": []string{"https://web.example.com"},
	})
	svc := NewAppService(te.env)

	serverDomains := []string{"https://api.example.com", " https://api.example.com "}
	businessDomains := []string{"https://web.example.com"}
	appids := []string{testAuthorizerAppid}
	out, err := svc.ApplyDomains(ctx, gen.DomainApplyRequest{
		Action:         gen.Add,
		Mode:           gen.Direct,
		Selection:      gen.AppidSelection{Appids: &appids},
		ServerDomain:   &gen.DomainRequirements{RequestDomain: &serverDomains},
		BusinessDomain: &businessDomains,
	})
	if err != nil {
		t.Fatalf("ApplyDomains 失败: %v", err)
	}
	if len(out.Items) != 1 {
		t.Fatalf("逐项结果数量不对: %+v", out.Items)
	}
	item := out.Items[0]
	if !item.Ok || item.Errcode != nil {
		t.Fatalf("direct 模式应成功: %+v", item)
	}
	if item.Errmsg == nil || !strings.Contains(*item.Errmsg, "发布上线") {
		t.Fatalf("direct 模式必须提示「需发布上线后生效」: %v", item.Errmsg)
	}

	if n := te.wx.countTo("/wxa/modify_domain_directly"); n != 1 {
		t.Errorf("应调用 1 次 modify_domain_directly，实际 %d 次", n)
	}
	if n := te.wx.countTo("/wxa/setwebviewdomain_directly"); n != 1 {
		t.Errorf("应调用 1 次 setwebviewdomain_directly，实际 %d 次", n)
	}
	for _, path := range []string{"/wxa/modify_domain", "/wxa/setwebviewdomain", "/cgi-bin/component/modify_wxa_server_domain", "/cgi-bin/component/modify_wxa_jump_domain"} {
		if n := te.wx.countTo(path); n != 0 {
			t.Errorf("direct 模式不应调用 %s，实际 %d 次", path, n)
		}
	}
	// 域名去空白去重后按 requestdomain 下发。
	body := te.wx.lastCallTo("/wxa/modify_domain_directly").Body
	if body["action"] != "add" {
		t.Errorf("action 应为 add，实际 %v", body["action"])
	}
	list, ok := body["requestdomain"].([]any)
	if !ok || len(list) != 1 || list[0] != "https://api.example.com" {
		t.Errorf("requestdomain 应去重后下发，实际 %v", body["requestdomain"])
	}
	if got := te.wx.lastCallTo("/wxa/modify_domain_directly").Query.Get("access_token"); got != "AUTHORIZER-ACCESS-TOKEN" {
		t.Errorf("modify_domain_directly 应使用 authorizer_access_token，实际 %q", got)
	}
	te.requireAction(actionApplyDomains)
}

func TestApplyDomainsRegisteredModeRegistersOnPlatformFirst(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	te.wx.jsonRoute("/cgi-bin/component/modify_wxa_server_domain", map[string]any{
		"published_domain": "https://api.example.com",
		"tested_domain":    "https://api.example.com",
	})
	te.wx.jsonRoute("/cgi-bin/component/modify_wxa_jump_domain", map[string]any{
		"published_domain": "https://web.example.com",
	})
	te.wx.jsonRoute("/wxa/modify_domain", map[string]any{
		"invalid_requestdomain": []string{"bad.example.com"},
		"no_icp_domain":         []string{"nopicp.example.com"},
	})
	te.wx.jsonRoute("/wxa/setwebviewdomain", map[string]any{"errcode": 0, "errmsg": "ok"})
	svc := NewAppService(te.env)

	serverDomains := []string{"https://api.example.com", "bad.example.com"}
	businessDomains := []string{"https://web.example.com"}
	appids := []string{testAuthorizerAppid}
	out, err := svc.ApplyDomains(ctx, gen.DomainApplyRequest{
		Action:         gen.Add,
		Mode:           gen.Registered,
		Selection:      gen.AppidSelection{Appids: &appids},
		ServerDomain:   &gen.DomainRequirements{RequestDomain: &serverDomains},
		BusinessDomain: &businessDomains,
	})
	if err != nil {
		t.Fatalf("ApplyDomains 失败: %v", err)
	}
	item := out.Items[0]
	if item.Ok {
		t.Fatalf("存在无效/未备案域名时不应判定为成功: %+v", item)
	}
	if item.InvalidDomains == nil || (*item.InvalidDomains)[0] != "bad.example.com" {
		t.Errorf("invalidDomains 未回传: %+v", item.InvalidDomains)
	}
	if item.MissingIcpDomains == nil || (*item.MissingIcpDomains)[0] != "nopicp.example.com" {
		t.Errorf("missingIcpDomains 未回传: %+v", item.MissingIcpDomains)
	}
	if item.Errmsg == nil || !strings.Contains(*item.Errmsg, "ICP") {
		t.Errorf("错误文案应说明 ICP 备案要求: %v", item.Errmsg)
	}

	seq := te.wx.pathSequence()
	registerServer := indexOfPath(seq, "/cgi-bin/component/modify_wxa_server_domain")
	configureServer := indexOfPath(seq, "/wxa/modify_domain")
	if registerServer < 0 || configureServer < 0 || registerServer > configureServer {
		t.Fatalf("registered 模式必须先登记平台域名再配到小程序，实际顺序 %v", seq)
	}
	registerJump := indexOfPath(seq, "/cgi-bin/component/modify_wxa_jump_domain")
	configureJump := indexOfPath(seq, "/wxa/setwebviewdomain")
	if registerJump < 0 || configureJump < 0 || registerJump > configureJump {
		t.Fatalf("业务域名也要先登记再配置，实际顺序 %v", seq)
	}
	body := te.wx.lastCallTo("/cgi-bin/component/modify_wxa_server_domain").Body
	if body["wxa_server_domain"] != "https://api.example.com;bad.example.com" {
		t.Errorf("平台域名登记应以 ; 分隔，实际 %v", body["wxa_server_domain"])
	}
	if body["is_modify_published_together"] != true {
		t.Errorf("应同时修改全网发布版域名，实际 %v", body["is_modify_published_together"])
	}
}

func TestApplyDomainsExplainsPlatformQuotaLimit(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	// 45104：第三方平台域名每月修改次数已用尽（50 次）。
	te.wx.failRoute("/cgi-bin/component/modify_wxa_server_domain", 45104, "modify domain limit exceed")
	svc := NewAppService(te.env)

	serverDomains := []string{"https://api.example.com"}
	appids := []string{testAuthorizerAppid}
	out, err := svc.ApplyDomains(ctx, gen.DomainApplyRequest{
		Action:       gen.Add,
		Mode:         gen.Registered,
		Selection:    gen.AppidSelection{Appids: &appids},
		ServerDomain: &gen.DomainRequirements{RequestDomain: &serverDomains},
	})
	if err != nil {
		t.Fatalf("登记失败不应整体报错（逐项返回）: %v", err)
	}
	item := out.Items[0]
	if item.Ok || item.Errcode == nil || *item.Errcode != 45104 {
		t.Fatalf("登记失败应逐项标记: %+v", item)
	}
	if item.Errmsg == nil || !strings.Contains(*item.Errmsg, "50 次") {
		t.Errorf("错误文案应解释「每月 50 次」限制: %v", item.Errmsg)
	}
	if n := te.wx.countTo("/wxa/modify_domain"); n != 0 {
		t.Errorf("平台登记失败时不应再代小程序配置，实际调用 %d 次", n)
	}
}

func TestApplyDomainsValidatesInput(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	svc := NewAppService(te.env)
	appids := []string{testAuthorizerAppid}

	if _, err := svc.ApplyDomains(ctx, gen.DomainApplyRequest{
		Action:    gen.Add,
		Mode:      gen.DomainApplyRequestMode("bogus"),
		Selection: gen.AppidSelection{Appids: &appids},
	}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("非法 mode 应返回校验错误，实际 %v", err)
	}
	if _, err := svc.ApplyDomains(ctx, gen.DomainApplyRequest{
		Action:    gen.DomainApplyRequestAction("bogus"),
		Mode:      gen.Direct,
		Selection: gen.AppidSelection{Appids: &appids},
	}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("非法 action 应返回校验错误，实际 %v", err)
	}
	if _, err := svc.ApplyDomains(ctx, gen.DomainApplyRequest{
		Action:    gen.Add,
		Mode:      gen.Direct,
		Selection: gen.AppidSelection{Appids: &appids},
	}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("没有任何域名时应返回校验错误，实际 %v", err)
	}
	// 筛选条件匹配不到任何小程序 → 选择解析失败（空 appids 数组按契约等于「全部」）。
	noMatch := "不存在的分组"
	if _, err := svc.ApplyDomains(ctx, gen.DomainApplyRequest{
		Action:       gen.Add,
		Mode:         gen.Registered,
		Selection:    gen.AppidSelection{Filter: &gen.AuthorizerFilter{GroupName: &noMatch}},
		ServerDomain: &gen.DomainRequirements{RequestDomain: &[]string{"https://api.example.com"}},
	}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("没有匹配到小程序时应返回校验错误，实际 %v", err)
	}
	if n := len(te.wx.callsTo("/wxa/modify_domain_directly")); n != 0 {
		t.Fatalf("校验失败不应调用微信，实际 %d 次", n)
	}
}

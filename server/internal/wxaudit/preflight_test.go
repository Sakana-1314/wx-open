package wxaudit_test

import (
	"errors"
	"strings"
	"testing"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// 体检相关接口的「正常」应答。
const (
	categoryApprovedBody = `{"errcode":0,"errmsg":"ok","categories":[{"first":1,"first_name":"工具","second":101,"second_name":"信息查询","audit_status":3,"audit_reason":""}],"limit":5,"quota":4,"category_limit":20}`
	categoryEmptyBody    = `{"errcode":0,"errmsg":"ok","categories":[],"limit":5,"quota":5,"category_limit":20}`
	privacyOKBody        = `{"errcode":0,"errmsg":"ok","code_exist":1,"privacy_list":["UserInfo"]}`
	privacyMissingBody   = `{"errcode":0,"errmsg":"ok","code_exist":0,"privacy_list":[]}`
)

// registerDomains 注册两个「生效域名」接口。
//
// 注意：服务器域名接口的应答刻意不带 errcode —— wxapi.GetEffectiveServerDomain 用
// map[string]map[string][]string 解析，顶层的数字 errcode 会让解析直接失败，
// 因此这里按「分组对象」的形状返回（mp_domain / effective_domain），
// 与真实网关返回的字段结构一致。
func registerDomains(f *fixture, mpDomain, effectiveDomain, jumpDomain []string) {
	f.mock.jsonReply("/wxa/get_effective_domain", mustJSON(f.t, map[string]any{
		"mp_domain":        map[string]any{"requestdomain": mpDomain},
		"effective_domain": map[string]any{"requestdomain": effectiveDomain},
	}))
	f.mock.jsonReply("/wxa/get_effective_webviewdomain", mustJSON(f.t, map[string]any{
		"errcode":                 0,
		"errmsg":                  "ok",
		"effective_webviewdomain": jumpDomain,
		"direct_webviewdomain":    []string{},
		"third_webviewdomain":     []string{},
		"mp_webviewdomain":        []string{},
	}))
}

// registerHappyChecks 注册「全部正常」的体检接口。
func registerHappyChecks(f *fixture) {
	f.mock.jsonReply("/cgi-bin/wxopen/getcategory", categoryApprovedBody)
	f.mock.jsonReply("/cgi-bin/component/getprivacysetting", privacyOKBody)
	registerDomains(f, []string{"api.example.com"}, []string{"api.example.com", "mall.example.com"}, []string{"h5.example.com"})
}

// hasCheck 判断体检项里是否存在某个 key。
func hasCheck(item gen.PreflightItem, key string) bool {
	for _, c := range item.Checks {
		if c.Key == key {
			return true
		}
	}
	return false
}

// TestPreflightAllPass 覆盖「已授权 + 有权限集 + 有审核通过类目 + 已配置隐私」的正常路径。
func TestPreflightAllPass(t *testing.T) {
	const appid = "wx_preflight_ok"
	f := newFixture(t)
	f.app(appid, "体检正常小程序", model.PermissionSetDev)
	registerHappyChecks(f)

	f.seed()
	resp, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
		Purpose:   gen.PreflightRequestPurposeRelease,
		Selection: appids(appid),
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if len(resp.Items) != 1 || resp.ReadyCount != 1 || resp.BlockedCount != 0 {
		t.Fatalf("汇总不对：items=%d ready=%d blocked=%d", len(resp.Items), resp.ReadyCount, resp.BlockedCount)
	}
	item := resp.Items[0]
	if !item.Ready || item.NickName == nil || *item.NickName != "体检正常小程序" {
		t.Fatalf("体检项不对：%+v", item)
	}

	if c := checkOf(t, item, "authorization"); c.Status != gen.Pass {
		t.Fatalf("授权应 pass：%+v", c)
	}
	if c := checkOf(t, item, "dev_permission"); c.Status != gen.Pass {
		t.Fatalf("权限集应 pass：%+v", c)
	}
	if c := checkOf(t, item, "profile"); c.Status != gen.Pass {
		t.Fatalf("资料应 pass：%+v", c)
	}
	if c := checkOf(t, item, "category"); c.Status != gen.Pass || !strings.Contains(c.Message, "审核通过") {
		t.Fatalf("类目应 pass 且说明已审核通过：%+v", c)
	}
	if c := checkOf(t, item, "privacy"); c.Status != gen.Pass {
		t.Fatalf("隐私指引应 pass：%+v", c)
	}
	domains := checkOf(t, item, "domains")
	if domains.Status != gen.Warn || !strings.Contains(hintOf(domains), "第三方平台登记") {
		t.Fatalf("未提供要求域名时应 warn 并说明托管域名限制：%+v", domains)
	}
	// release 目的不检查额度与模板库。
	if hasCheck(item, "quota") || hasCheck(item, "template") {
		t.Fatalf("release 目的不应出现 quota/template 检查：%s", checkKeys(item))
	}
	if len(f.operationLogs("runPreflight")) != 1 {
		t.Fatalf("体检应写 operation_logs(action=runPreflight)")
	}

	// 体检会回写提审前置快照（隐私已配置 + 域名快照）。
	a, err := f.env.Repos.Authorizers.Get(f.ctx, appid)
	if err != nil {
		t.Fatalf("读取小程序失败: %v", err)
	}
	if a.PrivacyConfigured == nil || !*a.PrivacyConfigured {
		t.Fatalf("应回写隐私已配置快照：%+v", a.PrivacyConfigured)
	}
	if len(a.DomainSnapshot) == 0 {
		t.Fatalf("应回写域名快照：%+v", a.DomainSnapshot)
	}
	if a.LastPreflightAt == nil {
		t.Fatalf("应回写最近体检时间")
	}
}

// TestPreflightMissingPermissionAndCategory 覆盖「缺权限集 + 无审核通过类目」的阻断路径。
func TestPreflightMissingPermissionAndCategory(t *testing.T) {
	const appid = "wx_preflight_blocked"
	f := newFixture(t)
	f.app(appid, "缺权限与类目小程序", 17) // 17 = 获取小程序码，不含 18
	f.mock.jsonReply("/cgi-bin/wxopen/getcategory", categoryEmptyBody)
	f.mock.jsonReply("/cgi-bin/component/getprivacysetting", privacyMissingBody)
	registerDomains(f, []string{"api.example.com"}, []string{"api.example.com"}, []string{"h5.example.com"})

	f.seed()
	resp, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
		Purpose:   gen.PreflightRequestPurposeSubmitAudit,
		Selection: appids(appid),
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	item := resp.Items[0]
	if item.Ready || resp.ReadyCount != 0 || resp.BlockedCount != 1 {
		t.Fatalf("应为阻塞：ready=%v readyCount=%d blocked=%d", item.Ready, resp.ReadyCount, resp.BlockedCount)
	}

	dev := checkOf(t, item, "dev_permission")
	if dev.Status != gen.Fail || !strings.Contains(dev.Message, "18") || !strings.Contains(hintOf(dev), "互斥") {
		t.Fatalf("缺权限集应 fail 并说明权限集 18 与互斥：%+v", dev)
	}
	cat := checkOf(t, item, "category")
	if cat.Status != gen.Fail {
		t.Fatalf("无审核通过类目应 fail：%+v", cat)
	}
	if !strings.Contains(hintOf(cat), "类目管理") || !strings.Contains(hintOf(cat), "85008") {
		t.Fatalf("类目 fail 的 hint 应指向类目管理并解释 85008：%q", hintOf(cat))
	}
	privacy := checkOf(t, item, "privacy")
	if privacy.Status != gen.Warn || !strings.Contains(hintOf(privacy), "隐私保护指引") {
		t.Fatalf("未配置隐私指引应 warn（官方无专门错误码）：%+v", privacy)
	}
	// submit_audit 目的会带额度检查。
	if !hasCheck(item, "quota") {
		t.Fatalf("submit_audit 目的应包含额度检查：%s", checkKeys(item))
	}
}

// TestPreflightCategoryCallFailureIsUnknown 覆盖类目接口调用失败：只报 unknown，不臆断为 fail。
func TestPreflightCategoryCallFailureIsUnknown(t *testing.T) {
	const appid = "wx_preflight_cat_unknown"
	f := newFixture(t)
	f.app(appid, "类目接口失败小程序", model.PermissionSetDev)
	f.mock.errcode("/cgi-bin/wxopen/getcategory", 61007, "api is unauthorized to component")
	f.mock.jsonReply("/cgi-bin/component/getprivacysetting", privacyOKBody)
	registerDomains(f, []string{"api.example.com"}, []string{"api.example.com"}, []string{"h5.example.com"})

	f.seed()
	resp, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
		Purpose:   gen.PreflightRequestPurposeRelease,
		Selection: appids(appid),
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	item := resp.Items[0]
	cat := checkOf(t, item, "category")
	if cat.Status != gen.Unknown {
		t.Fatalf("调用失败应报 unknown 而不是 fail：%+v", cat)
	}
	if !strings.Contains(cat.Message, "61007") && !strings.Contains(cat.Message, "getcategory") {
		t.Fatalf("unknown 应说明原因：%q", cat.Message)
	}
	if !strings.Contains(hintOf(cat), "30") {
		t.Fatalf("应提示该类目接口需要权限集 30：%q", hintOf(cat))
	}
	if !item.Ready {
		t.Fatalf("unknown 不应阻塞（Ready 只看 fail）：%s", checkKeys(item))
	}
}

// TestPreflightUnauthorizedApplet 覆盖已取消授权：授权项 fail，其余项 unknown 且不调用微信。
func TestPreflightUnauthorizedApplet(t *testing.T) {
	const appid = "wx_preflight_unauthorized"
	f := newFixture(t)
	f.appUnauthorized(appid, "已取消授权小程序", model.PermissionSetDev)
	registerHappyChecks(f)

	f.seed()
	resp, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
		Purpose:   gen.PreflightRequestPurposeRelease,
		Selection: appids(appid),
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	item := resp.Items[0]
	auth := checkOf(t, item, "authorization")
	if auth.Status != gen.Fail || !strings.Contains(hintOf(auth), "重新扫码授权") {
		t.Fatalf("已取消授权应 fail 并提示重新授权：%+v", auth)
	}
	for _, key := range []string{"category", "privacy", "domains"} {
		if c := checkOf(t, item, key); c.Status != gen.Unknown {
			t.Fatalf("%s 应为 unknown：%+v", key, c)
		}
	}
	if item.Ready {
		t.Fatalf("已取消授权不应 Ready")
	}
	if got := len(f.mock.callsTo("/cgi-bin/wxopen/getcategory")) + len(f.mock.callsTo("/cgi-bin/component/getprivacysetting")) + len(f.mock.callsTo("/wxa/get_effective_domain")); got != 0 {
		t.Fatalf("未授权小程序不应调用微信接口，实际调用 %d 次", got)
	}
}

// TestPreflightQuotaSharedAcrossApplets 覆盖额度：服务商级共用、只查一次，用尽时 fail 并提示 85085。
func TestPreflightQuotaSharedAcrossApplets(t *testing.T) {
	const appidA, appidB = "wx_preflight_quota_a", "wx_preflight_quota_b"
	f := newFixture(t)
	f.app(appidA, "额度小程序 A", model.PermissionSetDev)
	f.app(appidB, "额度小程序 B", model.PermissionSetDev)
	registerHappyChecks(f)
	f.mock.jsonReply("/wxa/queryquota", `{"errcode":0,"errmsg":"ok","rest":0,"limit":50,"speedup_rest":1,"speedup_limit":5}`)

	f.seed()
	resp, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
		Purpose:   gen.PreflightRequestPurposeSubmitAudit,
		Selection: appids(appidA, appidB),
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if resp.ReadyCount != 0 || resp.BlockedCount != 2 {
		t.Fatalf("额度用尽应全部阻塞：ready=%d blocked=%d", resp.ReadyCount, resp.BlockedCount)
	}
	for _, item := range resp.Items {
		quota := checkOf(t, item, "quota")
		if quota.Status != gen.Fail {
			t.Fatalf("额度用尽应 fail：%+v", quota)
		}
		if !strings.Contains(hintOf(quota), "85085") || !strings.Contains(hintOf(quota), "临时额度") {
			t.Fatalf("额度 fail 的 hint 应提到 85085 与申请临时额度：%q", hintOf(quota))
		}
	}
	if got := f.mock.callCount("/wxa/queryquota"); got != 1 {
		t.Fatalf("额度是服务商级的，一次体检只应查询一次，实际 %d 次", got)
	}
	if resp.AuditQuota == nil || resp.AuditQuota.Rest == nil || *resp.AuditQuota.Rest != 0 {
		t.Fatalf("响应应带额度：%+v", resp.AuditQuota)
	}
	if cached := f.cachedQuotaStable(appidA); cached == nil || cached.Rest == nil || *cached.Rest != 0 {
		t.Fatalf("体检查询到的额度应落库缓存：%+v", cached)
	}

	// 额度恢复后重跑：不再阻断。
	f.mock.jsonReply("/wxa/queryquota", `{"errcode":0,"errmsg":"ok","rest":9,"limit":50,"speedup_rest":1,"speedup_limit":5}`)
	f.seed()
	resp, err = f.preflight.Run(f.ctx, gen.PreflightRequest{
		Purpose:   gen.PreflightRequestPurposeSubmitAudit,
		Selection: appids(appidA, appidB),
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if resp.ReadyCount != 2 {
		t.Fatalf("额度恢复后应全部 Ready：ready=%d blocked=%d", resp.ReadyCount, resp.BlockedCount)
	}
	if c := checkOf(t, resp.Items[0], "quota"); c.Status != gen.Pass || !strings.Contains(c.Message, "9/50") {
		t.Fatalf("额度恢复后应 pass 并展示剩余额度：%+v", c)
	}
}

// TestPreflightTemplatePurpose 覆盖 commit 目的的模板库检查。
func TestPreflightTemplatePurpose(t *testing.T) {
	const appid = "wx_preflight_template"
	const directAppid = "wx_preflight_template_direct"
	f := newFixture(t)
	f.app(appid, "模板库用例小程序", model.PermissionSetDev)
	// 直传（direct_commit）方式的小程序：平台无法代上传代码。
	f.appCustom(&model.Authorizer{
		Appid:      directAppid,
		NickName:   "直传用例小程序",
		HeadImg:    "https://example.com/direct.png",
		Signature:  "示例简介",
		CodeSource: model.CodeSourceDirectCommit,
		FuncInfo:   model.IntSlice{model.PermissionSetDev},
	})
	registerHappyChecks(f)

	// 模板库是共享表，其它测试包可能写入模板；这里显式清空以保证「空库」这个前提。
	clearTemplates := func() { f.clearTable(&model.CodeTemplate{}) }

	run := func(target string) gen.PreflightItem {
		f.seed()
		resp, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
			Purpose:   gen.PreflightRequestPurposeCommit,
			Selection: appids(target),
		})
		if err != nil {
			t.Fatalf("Run 失败: %v", err)
		}
		return resp.Items[0]
	}

	// 模板库为空 → fail（无法批量下发代码）。
	clearTemplates()
	item := run(appid)
	tpl := checkOf(t, item, "template")
	if tpl.Status != gen.Fail || !strings.Contains(hintOf(tpl), "模板库") {
		t.Fatalf("模板库为空应 fail 并提示先添加模板：%+v", tpl)
	}
	if item.Ready {
		t.Fatalf("模板库为空时 commit 目的不应 Ready：%s", checkKeys(item))
	}

	// 加入一个普通模板后 → pass。
	if err := f.env.Repos.Templates.ReplaceAll(f.ctx, []model.CodeTemplate{
		{TemplateID: 100, TemplateType: 0, UserVersion: "1.0.0", UserDesc: "模板"},
	}); err != nil {
		t.Fatalf("写入模板库失败: %v", err)
	}
	item = run(appid)
	if c := checkOf(t, item, "template"); c.Status != gen.Pass {
		t.Fatalf("模板库有可用模板应 pass：%+v", c)
	}
	if !item.Ready {
		t.Fatalf("模板库就绪后 commit 目的应 Ready：%s", checkKeys(item))
	}

	// 代码来源不是模板库 → fail。
	item = run(directAppid)
	if c := checkOf(t, item, "template"); c.Status != gen.Fail || !strings.Contains(c.Message, "direct_commit") {
		t.Fatalf("直传方式应 fail 并说明无法代上传：%+v", c)
	}
	if item.Ready {
		t.Fatalf("直传方式下 commit 目的不应 Ready：%s", checkKeys(item))
	}
}

// TestPreflightRequiredDomains 覆盖要求域名的核对（含域名归一化与「只看生效分组」）。
func TestPreflightRequiredDomains(t *testing.T) {
	const appid = "wx_preflight_domains"
	f := newFixture(t)
	f.app(appid, "域名用例小程序", model.PermissionSetDev)
	registerHappyChecks(f)

	run := func(req *gen.DomainRequirements) gen.PreflightCheck {
		f.seed()
		resp, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
			Purpose:         gen.PreflightRequestPurposeRelease,
			Selection:       appids(appid),
			RequiredDomains: req,
		})
		if err != nil {
			t.Fatalf("Run 失败: %v", err)
		}
		return checkOf(t, resp.Items[0], "domains")
	}

	// 带协议头 + 结尾斜杠 + 大小写的要求域名应能匹配（服务端做归一化）。
	request := []string{"https://API.example.com/"}
	if c := run(&gen.DomainRequirements{RequestDomain: &request}); c.Status != gen.Pass {
		t.Fatalf("要求域名已在生效列表中应 pass：%+v（hint=%q）", c, hintOf(c))
	}

	// 只在 effective_domain 分组里的域名也算生效（模拟器把 mp_domain 与 effective_domain 故意做成不同）。
	effectiveOnly := []string{"mall.example.com"}
	if c := run(&gen.DomainRequirements{RequestDomain: &effectiveOnly}); c.Status != gen.Pass {
		t.Fatalf("发布后生效分组里的域名应视为已生效：%+v", c)
	}

	// 业务域名同理。
	business := []string{"h5.example.com"}
	if c := run(&gen.DomainRequirements{BusinessDomain: &business}); c.Status != gen.Pass {
		t.Fatalf("业务域名已生效应 pass：%+v", c)
	}

	// 缺失的域名 → fail，并说明托管域名的登记顺序。
	missing := []string{"missing.example.com"}
	c := run(&gen.DomainRequirements{RequestDomain: &missing})
	if c.Status != gen.Fail {
		t.Fatalf("要求域名缺失应 fail：%+v", c)
	}
	if !strings.Contains(c.Message, "missing.example.com") {
		t.Fatalf("fail 消息应列出缺失域名：%q", c.Message)
	}
	if !strings.Contains(hintOf(c), "第三方平台登记") || !strings.Contains(hintOf(c), "发布上线后") {
		t.Fatalf("fail hint 应解释「先登记再配置、发布后生效」：%q", hintOf(c))
	}
}

// TestPreflightDomainCallFailure 覆盖域名接口调用失败：unknown，而不是 fail。
func TestPreflightDomainCallFailure(t *testing.T) {
	const appid = "wx_preflight_domain_fail"
	f := newFixture(t)
	f.app(appid, "域名接口失败小程序", model.PermissionSetDev)
	f.mock.jsonReply("/cgi-bin/wxopen/getcategory", categoryApprovedBody)
	f.mock.jsonReply("/cgi-bin/component/getprivacysetting", privacyOKBody)
	// 两个域名接口都不注册：模拟两端调用同时失败。
	required := []string{"api.example.com"}

	f.seed()
	resp, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
		Purpose:         gen.PreflightRequestPurposeRelease,
		Selection:       appids(appid),
		RequiredDomains: &gen.DomainRequirements{RequestDomain: &required},
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	item := resp.Items[0]
	c := checkOf(t, item, "domains")
	if c.Status != gen.Unknown {
		t.Fatalf("接口失败应报 unknown：%+v", c)
	}
	if !strings.Contains(c.Message, "调用失败") {
		t.Fatalf("unknown 应说明失败原因：%q", c.Message)
	}
	if !item.Ready {
		t.Fatalf("unknown 不应阻塞：%s", checkKeys(item))
	}
}

// TestPreflightProfileMissingWarns 覆盖资料缺失只报 warn（提审会 86002）。
func TestPreflightProfileMissingWarns(t *testing.T) {
	const appid = "wx_preflight_profile"
	f := newFixture(t)
	// 直接用「昵称/头像/简介都为空」的账号登记（而不是登记后再改库，
	// 否则并发跑测试包时的 seed() 会把改动抹掉）。
	f.appCustom(&model.Authorizer{
		Appid:    appid,
		FuncInfo: model.IntSlice{model.PermissionSetDev},
	})
	registerHappyChecks(f)

	f.seed()
	resp, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
		Purpose:   gen.PreflightRequestPurposeRelease,
		Selection: appids(appid),
	})
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	item := resp.Items[0]
	profile := checkOf(t, item, "profile")
	if profile.Status != gen.Warn {
		t.Fatalf("资料缺失应 warn：%+v", profile)
	}
	if !strings.Contains(profile.Message, "昵称") || !strings.Contains(hintOf(profile), "86002") {
		t.Fatalf("应列出缺失字段并解释 86002：%+v", profile)
	}
	if item.NickName != nil {
		t.Fatalf("昵称为空时不应回传昵称：%v", *item.NickName)
	}
	if !item.Ready {
		t.Fatalf("warn 不应阻塞：%s", checkKeys(item))
	}
}

// TestPreflightPurposeValidation 覆盖 purpose 越界与选择集为空。
func TestPreflightPurposeValidation(t *testing.T) {
	const appid = "wx_preflight_purpose"
	f := newFixture(t)
	f.app(appid, "目的校验小程序", model.PermissionSetDev)
	registerHappyChecks(f)

	for _, purpose := range []gen.PreflightRequestPurpose{"", "unknown"} {
		f.seed()
		_, err := f.preflight.Run(f.ctx, gen.PreflightRequest{Purpose: purpose, Selection: appids(appid)})
		if !errors.Is(err, core.ErrValidation) {
			t.Fatalf("purpose=%q 应返回 ErrValidation，实际 %v", purpose, err)
		}
	}
	f.seed()
	if _, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
		Purpose:   gen.PreflightRequestPurposeRelease,
		Selection: appids("wx_not_registered"),
	}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("没有匹配到小程序应返回 ErrValidation，实际 %v", err)
	}
}

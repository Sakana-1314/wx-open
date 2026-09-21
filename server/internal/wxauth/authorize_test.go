package wxauth

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"wx-platform/server/internal/config"
	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

// 微信三类接口的最小可用响应（授权信息 / 账号详情）。
func queryAuthPayload(appid, refreshToken string, funcIDs ...int) map[string]any {
	funcInfo := make([]map[string]any, 0, len(funcIDs))
	for _, id := range funcIDs {
		funcInfo = append(funcInfo, map[string]any{"funcscope_category": map[string]any{"id": id}})
	}
	return map[string]any{
		"authorization_info": map[string]any{
			"authorizer_appid":         appid,
			"authorizer_access_token":  "AUTHORIZER-ACCESS-TOKEN",
			"expires_in":               7200,
			"authorizer_refresh_token": refreshToken,
			"func_info":                funcInfo,
		},
	}
}

func authorizerInfoPayload(appid string, funcIDs ...int) map[string]any {
	if len(funcIDs) == 0 {
		funcIDs = []int{model.PermissionSetDev}
	}
	funcInfo := make([]map[string]any, 0, len(funcIDs))
	for _, id := range funcIDs {
		funcInfo = append(funcInfo, map[string]any{"funcscope_category": map[string]any{"id": id}})
	}
	return map[string]any{
		"authorizer_info": map[string]any{
			"nick_name":          "示例小程序",
			"head_img":           "https://example.com/head.png",
			"qrcode_url":         "https://example.com/qrcode.png",
			"user_name":          "gh_abcdef123456",
			"alias":              "demo-alias",
			"principal_name":     "示例科技有限公司",
			"signature":          "签名",
			"account_status":     1,
			"register_type":      1,
			"service_type_info":  map[string]any{"id": 0, "name": "小程序"},
			"verify_type_info":   map[string]any{"id": 0, "name": "未认证"},
			"business_info":      map[string]any{"open_pay": 1},
			"MiniProgramInfo":    map[string]any{"categories": []map[string]any{{"first": "教育", "second": "在线教育"}}},
			"authorizer_appid_x": appid,
		},
		"authorization_info": map[string]any{
			"authorizer_appid": appid,
			"func_info":        funcInfo,
		},
	}
}

func TestAuthorizationURLBuildsPCAndMobileLinks(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.wx.jsonRoute("/cgi-bin/component/api_create_preauthcode", map[string]any{
		"pre_auth_code": "PRE-AUTH-CODE-1",
		"expires_in":    600,
	})
	svc := NewAuthorizerService(te.env)

	authType := gen.AuthTypeN2
	bizAppid := "wxbizapp0001"
	categories := "18, 30"
	out, err := svc.AuthorizationURL(ctx, gen.GetAuthorizationUrlParams{
		AuthType:       &authType,
		BizAppid:       &bizAppid,
		CategoryIdList: &categories,
	})
	if err != nil {
		t.Fatalf("生成授权链接失败: %v", err)
	}

	pc, err := url.Parse(out.PcUrl)
	if err != nil {
		t.Fatalf("解析 pcUrl 失败: %v", err)
	}
	if got := pc.Scheme + "://" + pc.Host + pc.Path; got != core.PcAuthorizeURL {
		t.Fatalf("PC 授权页地址不对: %s", got)
	}
	q := pc.Query()
	checks := map[string]string{
		"component_appid":  testComponentAppid,
		"pre_auth_code":    "PRE-AUTH-CODE-1",
		"redirect_uri":     "https://platform.example.com/authorize/callback",
		"auth_type":        "2",
		"biz_appid":        bizAppid,
		"category_id_list": "18|30",
	}
	for key, want := range checks {
		if got := q.Get(key); got != want {
			t.Errorf("PC 链接参数 %s = %q，期望 %q", key, got, want)
		}
	}

	if !strings.HasPrefix(out.MobileUrl, core.MobileAuthorizeURL) {
		t.Fatalf("移动端授权页地址不对: %s", out.MobileUrl)
	}
	if !strings.HasSuffix(out.MobileUrl, "#wechat_redirect") {
		t.Errorf("移动端链接必须以 #wechat_redirect 结尾: %s", out.MobileUrl)
	}
	mobile, err := url.Parse(strings.TrimSuffix(out.MobileUrl, "#wechat_redirect"))
	if err != nil {
		t.Fatalf("解析 mobileUrl 失败: %v", err)
	}
	mq := mobile.Query()
	for key, want := range checks {
		if got := mq.Get(key); got != want {
			t.Errorf("移动端链接参数 %s = %q，期望 %q", key, got, want)
		}
	}
	if mq.Get("action") != "bindcomponent" || mq.Get("no_scan") != "1" {
		t.Errorf("移动端链接缺少 action=bindcomponent / no_scan=1: %s", out.MobileUrl)
	}

	if out.ExpiresInSeconds != 600 {
		t.Errorf("expiresInSeconds = %d，期望 600", out.ExpiresInSeconds)
	}
	if out.QrCodeContent != out.PcUrl {
		t.Errorf("二维码内容应等于 PC 链接，实际 %q", out.QrCodeContent)
	}
	if out.AuthType == nil || *out.AuthType != gen.AuthTypeN2 {
		t.Errorf("authType 回显不对: %v", out.AuthType)
	}
	if out.BizAppid == nil || *out.BizAppid != bizAppid {
		t.Errorf("bizAppid 回显不对: %v", out.BizAppid)
	}

	preAuth := te.wx.lastCallTo("/cgi-bin/component/api_create_preauthcode")
	if preAuth.Body["component_appid"] != testComponentAppid {
		t.Errorf("api_create_preauthcode 请求体缺少 component_appid: %v", preAuth.Body)
	}
	te.requireAction(actionAuthorizationURL)
}

func TestAuthorizationURLDefaults(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	// expires_in 为 0：按官方正文兜底 1800 秒。
	te.wx.jsonRoute("/cgi-bin/component/api_create_preauthcode", map[string]any{"pre_auth_code": "PRE-2"})
	svc := NewAuthorizerService(te.env)

	out, err := svc.AuthorizationURL(ctx, gen.GetAuthorizationUrlParams{})
	if err != nil {
		t.Fatalf("生成授权链接失败: %v", err)
	}
	if out.ExpiresInSeconds != defaultPreAuthCodeTTL {
		t.Errorf("expiresInSeconds = %d，期望兜底 %d", out.ExpiresInSeconds, defaultPreAuthCodeTTL)
	}
	if out.AuthType == nil || *out.AuthType != gen.AuthTypeN2 {
		t.Errorf("默认 authType 应为 2（仅小程序），实际 %v", out.AuthType)
	}
	if out.RedirectUri == nil || *out.RedirectUri != "https://platform.example.com/authorize/callback" {
		t.Errorf("默认 redirectUri 不对: %v", out.RedirectUri)
	}
	if out.BizAppid != nil {
		t.Errorf("未传 bizAppid 时不应回显: %v", *out.BizAppid)
	}
	pc, _ := url.Parse(out.PcUrl)
	if pc.Query().Get("biz_appid") != "" || pc.Query().Get("category_id_list") != "" {
		t.Errorf("未传 biz_appid / category_id_list 时不应带这两个参数: %s", out.PcUrl)
	}
}

func TestAuthorizationURLRejectsBadParamsWithoutCallingWeChat(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	svc := NewAuthorizerService(te.env)

	badAuthType := gen.AuthType(99)
	if _, err := svc.AuthorizationURL(ctx, gen.GetAuthorizationUrlParams{AuthType: &badAuthType}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("非法 authType 应返回校验错误，实际 %v", err)
	}
	badCategories := "18,abc"
	if _, err := svc.AuthorizationURL(ctx, gen.GetAuthorizationUrlParams{CategoryIdList: &badCategories}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("非法 categoryIdList 应返回校验错误，实际 %v", err)
	}
	if n := te.wx.countTo("/cgi-bin/component/api_create_preauthcode"); n != 0 {
		t.Fatalf("参数校验失败时不应调用微信，实际调用 %d 次", n)
	}
}

func TestAuthorizationURLWithoutComponentAppID(t *testing.T) {
	te := newTestEnvWithConfig(t, func(cfg *config.Config) { cfg.ComponentAppID = "" })
	svc := NewAuthorizerService(te.env)
	if _, err := svc.AuthorizationURL(context.Background(), gen.GetAuthorizationUrlParams{}); !errors.Is(err, core.ErrNotConfigured) {
		t.Fatalf("平台 appid 为空时应返回 ErrNotConfigured，实际 %v", err)
	}
	if n := te.wx.countTo("/cgi-bin/component/api_create_preauthcode"); n != 0 {
		t.Fatalf("凭据缺失时不应调用微信，实际调用 %d 次", n)
	}
}

func TestWeChatGateReturnsNotConfigured(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	// 模拟第三方平台凭据未配置：AttachWeChat 未被调用，Wx / Tokens 均为 nil。
	te.env.Wx = nil
	te.env.Tokens = nil

	authorizers := NewAuthorizerService(te.env)
	if _, err := authorizers.Sync(ctx, testAuthorizerAppid); !errors.Is(err, core.ErrNotConfigured) {
		t.Errorf("Sync 应返回 ErrNotConfigured，实际 %v", err)
	}
	if _, err := authorizers.SyncMany(ctx, gen.AppidSelection{}); !errors.Is(err, core.ErrNotConfigured) {
		t.Errorf("SyncMany 应返回 ErrNotConfigured，实际 %v", err)
	}
	if _, err := authorizers.ResyncTokens(ctx); !errors.Is(err, core.ErrNotConfigured) {
		t.Errorf("ResyncTokens 应返回 ErrNotConfigured，实际 %v", err)
	}
	if _, err := authorizers.CompleteAuthorize(ctx, gen.AuthorizeRequest{AuthCode: "CODE"}); !errors.Is(err, core.ErrNotConfigured) {
		t.Errorf("CompleteAuthorize 应返回 ErrNotConfigured，实际 %v", err)
	}
	if err := authorizers.OnAuthorized(ctx, testAuthorizerAppid, "CODE", "authorized"); !errors.Is(err, core.ErrNotConfigured) {
		t.Errorf("OnAuthorized 应返回 ErrNotConfigured，实际 %v", err)
	}
	if _, err := NewTemplateService(te.env).SyncDrafts(ctx); !errors.Is(err, core.ErrNotConfigured) {
		t.Errorf("SyncDrafts 应返回 ErrNotConfigured，实际 %v", err)
	}
	if _, err := NewAppService(te.env).VisitStatus(ctx, testAuthorizerAppid); !errors.Is(err, core.ErrNotConfigured) {
		t.Errorf("VisitStatus 应返回 ErrNotConfigured，实际 %v", err)
	}
}

func TestCompleteAuthorizeStoresSealedRefreshToken(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.wx.jsonRoute("/cgi-bin/component/api_query_auth", queryAuthPayload(testAuthorizerAppid, testRefreshToken, model.PermissionSetDev, model.PermissionSetBasicInfo))
	te.wx.jsonRoute("/cgi-bin/component/api_get_authorizer_info", authorizerInfoPayload(testAuthorizerAppid, model.PermissionSetDev, model.PermissionSetBasicInfo))
	svc := NewAuthorizerService(te.env)

	resp, err := svc.CompleteAuthorize(ctx, gen.AuthorizeRequest{AuthCode: "AUTH-CODE-1"})
	if err != nil {
		t.Fatalf("CompleteAuthorize 失败: %v", err)
	}
	if resp.Appid != testAuthorizerAppid {
		t.Fatalf("appid = %q，期望 %q", resp.Appid, testAuthorizerAppid)
	}
	if resp.AuthorizationStatus != gen.AuthorizationStatusAuthorized {
		t.Errorf("授权状态应为 authorized，实际 %q", resp.AuthorizationStatus)
	}
	if len(resp.FuncInfoIds) != 2 || resp.FuncInfoIds[0] != model.PermissionSetDev || resp.FuncInfoIds[1] != model.PermissionSetBasicInfo {
		t.Errorf("权限集应为 [18 30]，实际 %v", resp.FuncInfoIds)
	}
	if resp.HasDevPermission == nil || !*resp.HasDevPermission {
		t.Errorf("已勾选权限集 18 时应 HasDevPermission=true，实际 %v", resp.HasDevPermission)
	}
	if resp.Warnings != nil && len(*resp.Warnings) != 0 {
		t.Errorf("本次授权不应有告警，实际 %v", *resp.Warnings)
	}

	row := te.authorizerRow(testAuthorizerAppid)
	if row.AuthorizationStatus != model.AuthStatusAuthorized {
		t.Errorf("库中授权状态 = %q，期望 authorized", row.AuthorizationStatus)
	}
	if row.AuthorizedAt == nil {
		t.Error("库中 AuthorizedAt 不应为空")
	}
	if len(row.FuncInfo) != 2 || row.FuncInfo[0] != model.PermissionSetDev {
		t.Errorf("库中 func_info = %v，期望 [18 30]", row.FuncInfo)
	}
	// 资料补齐（api_get_authorizer_info）。
	if row.NickName != "示例小程序" || row.UserName != "gh_abcdef123456" || row.PrincipalName != "示例科技有限公司" {
		t.Errorf("账号资料未补齐: nick=%q user_name=%q principal=%q", row.NickName, row.UserName, row.PrincipalName)
	}
	if row.HeadImg == "" || row.QrcodeURL == "" || row.Alias != "demo-alias" || row.Signature != "签名" {
		t.Errorf("头像/二维码/别名/签名未补齐: %+v", row)
	}
	if len(row.BusinessInfo) == 0 {
		t.Error("business_info 未落库")
	}
	if got := categoryPairs(row.MiniProgramCats); got == nil || len(*got) != 1 || (*got)[0].First != "教育" {
		t.Errorf("小程序类目未落库: %v", row.MiniProgramCats)
	}

	// refresh_token 必须是密文，且能被 Box 解回原文。
	if row.RefreshTokenCipher == "" {
		t.Fatal("refresh_token 密文不应为空")
	}
	if row.RefreshTokenCipher == testRefreshToken || strings.Contains(row.RefreshTokenCipher, testRefreshToken) {
		t.Fatalf("refresh_token 以明文入库了: %q", row.RefreshTokenCipher)
	}
	var rawCipher []byte
	if err := te.db.Raw("SELECT refresh_token_cipher FROM authorizers WHERE appid = ?", testAuthorizerAppid).Row().Scan(&rawCipher); err != nil {
		t.Fatalf("读取密文列失败: %v", err)
	}
	if strings.Contains(string(rawCipher), testRefreshToken) {
		t.Fatalf("数据库列里含明文 refresh_token: %q", string(rawCipher))
	}
	plain, err := te.box.Open(rawCipher)
	if err != nil {
		t.Fatalf("解密 refresh_token 失败: %v", err)
	}
	if plain != testRefreshToken {
		t.Fatalf("解回的 refresh_token = %q，期望 %q", plain, testRefreshToken)
	}
	if row.RefreshTokenUpdatedAt == nil {
		t.Error("RefreshTokenUpdatedAt 不应为空")
	}

	// 授权码换回的令牌直接写缓存（省一次 api_authorizer_token 调用）。
	cached, err := te.env.Repos.Tokens.Get(ctx, model.TokenScopeAuthorizer, testAuthorizerAppid)
	if err != nil {
		t.Fatalf("授权方令牌未写缓存: %v", err)
	}
	if cached.Token != "AUTHORIZER-ACCESS-TOKEN" {
		t.Errorf("缓存令牌 = %q", cached.Token)
	}
	if n := te.wx.countTo("/cgi-bin/component/api_query_auth"); n != 1 {
		t.Errorf("api_query_auth 调用次数 = %d，期望 1", n)
	}
	if body := te.wx.lastCallTo("/cgi-bin/component/api_query_auth").Body; body["authorization_code"] != "AUTH-CODE-1" {
		t.Errorf("api_query_auth 请求体不对: %v", body)
	}
	if n := te.authorizerCount(testAuthorizerAppid); n != 1 {
		t.Errorf("授权方记录数 = %d，期望 1", n)
	}
	te.requireAction(actionAuthorize)
}

func TestCompleteAuthorizeWarnsWhenDevPermissionMissing(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.wx.jsonRoute("/cgi-bin/component/api_query_auth", queryAuthPayload(testAuthorizerAppid, testRefreshToken, 17))
	te.wx.jsonRoute("/cgi-bin/component/api_get_authorizer_info", authorizerInfoPayload(testAuthorizerAppid, 17))
	svc := NewAuthorizerService(te.env)

	resp, err := svc.CompleteAuthorize(ctx, gen.AuthorizeRequest{AuthCode: "AUTH-CODE-2"})
	if err != nil {
		t.Fatalf("CompleteAuthorize 失败: %v", err)
	}
	if resp.HasDevPermission == nil || *resp.HasDevPermission {
		t.Errorf("缺少权限集 18 时 HasDevPermission 应为 false，实际 %v", resp.HasDevPermission)
	}
	if resp.Warnings == nil {
		t.Fatal("缺少权限集 18 时应给出告警")
	}
	joined := strings.Join(*resp.Warnings, " ")
	if !strings.Contains(joined, "小程序开发与数据分析") || !strings.Contains(joined, "重新扫码授权") {
		t.Fatalf("告警文案不对: %s", joined)
	}
}

func TestOnAuthorizedIsIdempotent(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.wx.jsonRoute("/cgi-bin/component/api_query_auth", queryAuthPayload(testAuthorizerAppid, testRefreshToken, model.PermissionSetDev))
	te.wx.jsonRoute("/cgi-bin/component/api_get_authorizer_info", authorizerInfoPayload(testAuthorizerAppid))
	svc := NewAuthorizerService(te.env)

	if err := svc.OnAuthorized(ctx, testAuthorizerAppid, "AUTH-CODE-3", "authorized"); err != nil {
		t.Fatalf("首次 OnAuthorized 失败: %v", err)
	}
	first := te.authorizerRow(testAuthorizerAppid)
	if first.AuthorizedAt == nil {
		t.Fatal("首次授权后 AuthorizedAt 不应为空")
	}

	// 运维在平台上维护的字段。
	profile := &model.AuditProfile{Name: "默认提审配置", IsDefault: true}
	if err := te.db.Create(profile).Error; err != nil {
		t.Fatalf("创建提审配置失败: %v", err)
	}
	remark, group, tags := "客服已确认", "A 组", []string{"重点", " 重点 ", "自营"}
	vars := map[string]string{"env": "prod"}
	profileID := int64(profile.ID)
	if _, err := svc.Update(ctx, testAuthorizerAppid, gen.AuthorizerUpdateRequest{
		Remark:         &remark,
		GroupName:      &group,
		Tags:           &tags,
		ExtVars:        &vars,
		AuditProfileId: &profileID,
	}); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}

	// 微信重复推送 authorized / updateauthorized。
	if err := svc.OnAuthorized(ctx, testAuthorizerAppid, "AUTH-CODE-3", "authorized"); err != nil {
		t.Fatalf("重复 OnAuthorized 不应报错: %v", err)
	}
	if err := svc.OnAuthorized(ctx, testAuthorizerAppid, "AUTH-CODE-3", "updateauthorized"); err != nil {
		t.Fatalf("updateauthorized 不应报错: %v", err)
	}

	if n := te.authorizerCount(testAuthorizerAppid); n != 1 {
		t.Fatalf("重复推送产生了 %d 条记录，期望 1 条", n)
	}
	second := te.authorizerRow(testAuthorizerAppid)
	if second.Remark != remark || second.GroupName != group {
		t.Errorf("重复推送覆盖了备注/分组: remark=%q group=%q", second.Remark, second.GroupName)
	}
	if len(second.Tags) != 2 || second.Tags[0] != "重点" || second.Tags[1] != "自营" {
		t.Errorf("重复推送覆盖了标签: %v", second.Tags)
	}
	if len(second.ExtVars) != 1 || second.ExtVars["env"] != "prod" {
		t.Errorf("重复推送覆盖了 ext 变量: %v", second.ExtVars)
	}
	if second.AuditProfileID == nil || *second.AuditProfileID != profile.ID {
		t.Errorf("重复推送覆盖了 auditProfileId: %v", second.AuditProfileID)
	}
	if second.AuthorizedAt == nil || !second.AuthorizedAt.Truncate(time.Second).Equal(first.AuthorizedAt.Truncate(time.Second)) {
		t.Errorf("重复推送不应改变首次授权时间: %v → %v", first.AuthorizedAt, second.AuthorizedAt)
	}
	// 密文可以被解回最新 refresh_token（腾讯可能轮换 refresh_token，这里保持最新值）。
	plain, err := te.box.Open([]byte(second.RefreshTokenCipher))
	if err != nil {
		t.Fatalf("解密 refresh_token 失败: %v", err)
	}
	if plain != testRefreshToken {
		t.Errorf("库里的 refresh_token = %q，期望 %q", plain, testRefreshToken)
	}
}

func TestOnAuthorizedWithoutAuthCodeRefreshesExistingToken(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	svc := NewAuthorizerService(te.env)
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)

	// 清掉缓存的授权方令牌，逼真地模拟「只有 refresh_token」的场景。
	if err := te.env.Tokens.InvalidateAuthorizer(ctx, testAuthorizerAppid); err != nil {
		t.Fatalf("清空令牌缓存失败: %v", err)
	}
	if err := svc.OnAuthorized(ctx, testAuthorizerAppid, "", "authorized"); err != nil {
		t.Fatalf("授权码为空时不应报错: %v", err)
	}
	if n := te.wx.countTo("/cgi-bin/component/api_authorizer_token"); n != 1 {
		t.Fatalf("授权码为空时应尝试刷新一次令牌，实际刷新 %d 次", n)
	}
	cached, err := te.env.Repos.Tokens.Get(ctx, model.TokenScopeAuthorizer, testAuthorizerAppid)
	if err != nil {
		t.Fatalf("刷新后令牌未写缓存: %v", err)
	}
	if cached.Token != "AUTHORIZER-ACCESS-TOKEN" {
		t.Errorf("缓存令牌 = %q", cached.Token)
	}
	te.requireAction(actionAuthorizedEvent)

	// 微信未返回授权码、本地又没有 refresh_token：仍然不报错（回调必须幂等且不阻塞）。
	if err := svc.OnAuthorized(ctx, "wxunknown0001", "", "authorized"); err != nil {
		t.Fatalf("未知小程序的空授权码事件不应报错: %v", err)
	}
}

func TestOnUnauthorizedKeepsRecordAndClearsTokens(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.wx.jsonRoute("/cgi-bin/component/api_query_auth", queryAuthPayload(testAuthorizerAppid, testRefreshToken, model.PermissionSetDev))
	te.wx.jsonRoute("/cgi-bin/component/api_get_authorizer_info", authorizerInfoPayload(testAuthorizerAppid))
	svc := NewAuthorizerService(te.env)

	if _, err := svc.CompleteAuthorize(ctx, gen.AuthorizeRequest{AuthCode: "AUTH-CODE-4"}); err != nil {
		t.Fatalf("授权失败: %v", err)
	}
	remark := "历史备注要保留"
	if _, err := svc.Update(ctx, testAuthorizerAppid, gen.AuthorizerUpdateRequest{Remark: &remark}); err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	if _, err := te.env.Repos.Tokens.Get(ctx, model.TokenScopeAuthorizer, testAuthorizerAppid); err != nil {
		t.Fatalf("授权后应有令牌缓存: %v", err)
	}

	if err := svc.OnUnauthorized(ctx, testAuthorizerAppid); err != nil {
		t.Fatalf("OnUnauthorized 失败: %v", err)
	}

	row := te.authorizerRow(testAuthorizerAppid) // Get 会过滤 deleted_at，能读到说明没被软删
	if row.AuthorizationStatus != model.AuthStatusUnauthorized {
		t.Errorf("授权状态 = %q，期望 unauthorized", row.AuthorizationStatus)
	}
	if row.Remark != remark {
		t.Errorf("备注被清掉了: %q", row.Remark)
	}
	if row.RefreshTokenCipher != "" {
		t.Errorf("取消授权后应清空 refresh_token 密文，实际 %q", row.RefreshTokenCipher)
	}
	if row.RefreshTokenUpdatedAt != nil {
		t.Errorf("取消授权后应清空 refresh_token 时间戳，实际 %v", row.RefreshTokenUpdatedAt)
	}
	if _, err := te.env.Repos.Tokens.Get(ctx, model.TokenScopeAuthorizer, testAuthorizerAppid); !errors.Is(err, repo.ErrNotFound) {
		t.Errorf("令牌缓存应被清空，实际 err=%v", err)
	}
	if _, err := te.env.Tokens.AuthorizerToken(ctx, testAuthorizerAppid); err == nil {
		t.Error("取消授权后不应能再取得授权方令牌")
	}
	te.requireAction(actionUnauthorized)

	// 幂等：重复推送取消授权通知不报错。
	if err := svc.OnUnauthorized(ctx, testAuthorizerAppid); err != nil {
		t.Fatalf("重复取消授权不应报错: %v", err)
	}
	if err := svc.OnUnauthorized(ctx, "wxnever-seen01"); err != nil {
		t.Fatalf("未知 appid 的取消授权不应报错: %v", err)
	}
}

func TestAuthorizerOptionsRoundTrip(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	te.wx.jsonRoute("/cgi-bin/component/get_authorizer_option", map[string]any{
		"option_name":  "location_report",
		"option_value": "1",
	})
	svc := NewAuthorizerService(te.env)

	got, err := svc.GetOption(ctx, testAuthorizerAppid, gen.GetAuthorizerOptionParams{OptionName: gen.LocationReport})
	if err != nil {
		t.Fatalf("GetOption 失败: %v", err)
	}
	if got.OptionName != gen.LocationReport || got.OptionValue != "1" {
		t.Fatalf("GetOption 返回 %+v", got)
	}
	if body := te.wx.lastCallTo("/cgi-bin/component/get_authorizer_option").Body; body["authorizer_appid"] != testAuthorizerAppid {
		t.Errorf("get_authorizer_option 请求体缺少 authorizer_appid: %v", body)
	}

	set, err := svc.SetOption(ctx, testAuthorizerAppid, gen.AuthorizerOptionUpdateRequest{
		OptionName:  gen.LocationReport,
		OptionValue: "1",
	})
	if err != nil {
		t.Fatalf("SetOption 失败: %v", err)
	}
	if set.OptionValue != "1" {
		t.Errorf("SetOption 返回 %+v", set)
	}
	if n := te.wx.countTo("/cgi-bin/component/set_authorizer_option"); n != 1 {
		t.Errorf("set_authorizer_option 调用 %d 次，期望 1 次", n)
	}

	// 非法取值在本地就拦住，不打微信。
	if _, err := svc.SetOption(ctx, testAuthorizerAppid, gen.AuthorizerOptionUpdateRequest{
		OptionName:  gen.LocationReport,
		OptionValue: "9",
	}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("非法选项值应返回校验错误，实际 %v", err)
	}
	if n := te.wx.countTo("/cgi-bin/component/set_authorizer_option"); n != 1 {
		t.Errorf("校验失败不应调用微信，实际调用 %d 次", n)
	}
	te.requireAction(actionSetOption)
}

func TestCompleteAuthorizeRejectsEmptyCode(t *testing.T) {
	te := newTestEnv(t)
	svc := NewAuthorizerService(te.env)
	if _, err := svc.CompleteAuthorize(context.Background(), gen.AuthorizeRequest{}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("空授权码应返回校验错误，实际 %v", err)
	}
}

package wxauth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

const (
	secondAuthorizerAppid = "wxauthorizer0002"
	failingAuthorizer     = "wxauthorizer0009"
)

func TestListAuthorizersWithFilters(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	other := te.seedAuthorizer(secondAuthorizerAppid, "REFRESH-TOKEN-2", []int{})
	if err := te.env.Repos.Authorizers.UpdateFields(ctx, secondAuthorizerAppid, map[string]any{
		"authorization_status": model.AuthStatusUnauthorized,
		"nick_name":            "另一个小程序",
		"group_name":           "B 组",
		"enabled":              false,
	}); err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}
	if err := te.env.Repos.Authorizers.UpdateFields(ctx, testAuthorizerAppid, map[string]any{
		"nick_name":  "示例 A",
		"group_name": "A 组",
		"tags":       model.StringSlice{"重点"},
	}); err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}
	_ = other

	svc := NewAuthorizerService(te.env)

	all, err := svc.List(ctx, gen.ListAuthorizersParams{})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if all.Total != 2 || len(all.Items) != 2 {
		t.Fatalf("List 返回 total=%d items=%d，期望 2/2", all.Total, len(all.Items))
	}
	// 管理列表必须包含「已停用」的小程序，否则关掉开关后就再也看不到、改不回来。
	disabledVisible := false
	for _, item := range all.Items {
		if item.Appid == secondAuthorizerAppid {
			disabledVisible = true
			if item.Enabled {
				t.Errorf("该小程序应为已停用状态: %+v", item)
			}
		}
	}
	if !disabledVisible {
		t.Fatal("停用的小程序也应在列表里可见（否则无法重新启用）")
	}
	// 但批量作业的目标选择不包含停用 / 已取消授权的账号。
	targets, err := core.ResolveSelection(ctx, te.env.Repos, gen.AppidSelection{})
	if err != nil {
		t.Fatalf("ResolveSelection 失败: %v", err)
	}
	if len(targets) != 1 || targets[0].Appid != testAuthorizerAppid {
		t.Fatalf("批量目标应只含启用且已授权的小程序，实际 %+v", targets)
	}

	unauthorized := gen.AuthorizationStatusUnauthorized
	filtered, err := svc.List(ctx, gen.ListAuthorizersParams{Status: &unauthorized})
	if err != nil {
		t.Fatalf("按状态筛选失败: %v", err)
	}
	if filtered.Total != 1 || filtered.Items[0].Appid != secondAuthorizerAppid {
		t.Fatalf("按状态筛选结果不对: %+v", filtered.Items)
	}

	noDev := false
	missingDev, err := svc.List(ctx, gen.ListAuthorizersParams{HasDevPermission: &noDev})
	if err != nil {
		t.Fatalf("按权限集筛选失败: %v", err)
	}
	if missingDev.Total != 1 || missingDev.Items[0].Appid != secondAuthorizerAppid {
		t.Fatalf("按权限集筛选结果不对: %+v", missingDev.Items)
	}

	keyword, tag := "示例 A", "重点"
	byKeyword, err := svc.List(ctx, gen.ListAuthorizersParams{Keyword: &keyword, Tag: &tag})
	if err != nil {
		t.Fatalf("按关键字筛选失败: %v", err)
	}
	if byKeyword.Total != 1 || byKeyword.Items[0].Appid != testAuthorizerAppid {
		t.Fatalf("按关键字筛选结果不对: %+v", byKeyword.Items)
	}

	badStatus := gen.AuthorizationStatus("bogus")
	if _, err := svc.List(ctx, gen.ListAuthorizersParams{Status: &badStatus}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("非法 status 应返回校验错误，实际 %v", err)
	}
}

func TestGetAuthorizerDetail(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, []int{model.PermissionSetDev, model.PermissionSetBasicInfo})
	if err := te.env.Repos.Authorizers.UpdateFields(ctx, testAuthorizerAppid, map[string]any{
		"remark":             "客服已确认",
		"tags":               model.StringSlice{"重点", "自营"},
		"ext_vars":           model.JSONStringMap{"env": "prod"},
		"mini_program_cats":  model.JSONMap{"categories": []map[string]string{{"first": "教育", "second": "在线教育"}}},
		"domain_snapshot":    model.JSONMap{"effective": map[string]any{"requestDomain": []string{"https://api.example.com"}}},
		"privacy_configured": true,
	}); err != nil {
		t.Fatalf("准备数据失败: %v", err)
	}

	svc := NewAuthorizerService(te.env)
	detail, err := svc.Get(ctx, testAuthorizerAppid)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if detail.Appid != testAuthorizerAppid || detail.AuthorizationStatus != gen.AuthorizationStatusAuthorized {
		t.Fatalf("详情基础字段不对: %+v", detail)
	}
	if !detail.HasDevPermission {
		t.Error("权限集 18 存在时应 HasDevPermission=true")
	}
	if detail.FuncInfoIds == nil || len(*detail.FuncInfoIds) != 2 {
		t.Errorf("FuncInfoIds 不对: %v", detail.FuncInfoIds)
	}
	if detail.Remark == nil || *detail.Remark != "客服已确认" {
		t.Errorf("Remark 不对: %v", detail.Remark)
	}
	if detail.ExtVars == nil || (*detail.ExtVars)["env"] != "prod" {
		t.Errorf("ExtVars 不对: %v", detail.ExtVars)
	}
	if detail.Tags == nil || len(*detail.Tags) != 2 {
		t.Errorf("Tags 不对: %v", detail.Tags)
	}
	if detail.MiniProgramCategories == nil || (*detail.MiniProgramCategories)[0].First != "教育" {
		t.Errorf("MiniProgramCategories 不对: %v", detail.MiniProgramCategories)
	}
	if detail.EffectiveDomains == nil || detail.EffectiveDomains.RequestDomain == nil ||
		(*detail.EffectiveDomains.RequestDomain)[0] != "https://api.example.com" {
		t.Errorf("EffectiveDomains 不对: %+v", detail.EffectiveDomains)
	}
	if detail.PrivacySettingConfigured == nil || !*detail.PrivacySettingConfigured {
		t.Errorf("PrivacySettingConfigured 不对: %v", detail.PrivacySettingConfigured)
	}

	if _, err := svc.Get(ctx, "wx-not-exists"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("不存在的小程序应返回 ErrNotFound，实际 %v", err)
	}
}

func TestUpdateAuthorizerValidatesAndPersists(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	profile := &model.AuditProfile{Name: "默认提审配置", IsDefault: true}
	if err := te.db.Create(profile).Error; err != nil {
		t.Fatalf("创建提审配置失败: %v", err)
	}
	svc := NewAuthorizerService(te.env)

	badSource := gen.AuthorizerUpdateRequestCodeSource("bogus")
	if _, err := svc.Update(ctx, testAuthorizerAppid, gen.AuthorizerUpdateRequest{CodeSource: &badSource}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("非法 codeSource 应返回校验错误，实际 %v", err)
	}
	badVars := map[string]string{"bad key": "1"}
	if _, err := svc.Update(ctx, testAuthorizerAppid, gen.AuthorizerUpdateRequest{ExtVars: &badVars}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("非法 ext 变量名应返回校验错误，实际 %v", err)
	}
	missingProfile := int64(99999)
	if _, err := svc.Update(ctx, testAuthorizerAppid, gen.AuthorizerUpdateRequest{AuditProfileId: &missingProfile}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("不存在的提审配置应返回校验错误，实际 %v", err)
	}

	remark, group := "  运维备注  ", "A 组"
	tags := []string{" 重点 ", "重点", "自营", ""}
	vars := map[string]string{"env": "prod", "store_id": "1001"}
	enabled := false
	source := gen.AuthorizerUpdateRequestCodeSourceDirectCommit
	profileID := int64(profile.ID)
	override := map[string]any{"orderPath": "pages/order/index"}

	detail, err := svc.Update(ctx, testAuthorizerAppid, gen.AuthorizerUpdateRequest{
		Remark:         &remark,
		GroupName:      &group,
		Tags:           &tags,
		ExtVars:        &vars,
		Enabled:        &enabled,
		CodeSource:     &source,
		AuditProfileId: &profileID,
		AuditOverride:  &override,
	})
	if err != nil {
		t.Fatalf("Update 失败: %v", err)
	}
	if detail.Remark == nil || *detail.Remark != "运维备注" {
		t.Errorf("备注应去空白后落库: %v", detail.Remark)
	}
	if detail.Tags == nil || len(*detail.Tags) != 2 || (*detail.Tags)[0] != "重点" {
		t.Errorf("标签应去空白去重: %v", detail.Tags)
	}
	if detail.Enabled {
		t.Error("enabled=false 应生效")
	}
	if detail.CodeSource == nil || *detail.CodeSource != gen.AuthorizerDetailCodeSourceDirectCommit {
		t.Errorf("codeSource 应更新为 direct_commit: %v", detail.CodeSource)
	}
	if detail.ExtVars == nil || (*detail.ExtVars)["store_id"] != "1001" {
		t.Errorf("ext 变量未更新: %v", detail.ExtVars)
	}
	if detail.AuditProfileId == nil || *detail.AuditProfileId != profileID {
		t.Errorf("auditProfileId 未更新: %v", detail.AuditProfileId)
	}
	if detail.AuditOverride == nil || (*detail.AuditOverride)["orderPath"] != "pages/order/index" {
		t.Errorf("auditOverride 未更新: %v", detail.AuditOverride)
	}

	row := te.authorizerRow(testAuthorizerAppid)
	if row.ExtVars["env"] != "prod" || row.GroupName != "A 组" || row.Enabled {
		t.Errorf("库中数据不对: %+v", row)
	}
	te.requireAction(actionUpdateAuthorizer)

	if _, err := svc.Update(ctx, "wx-not-exists", gen.AuthorizerUpdateRequest{Remark: &remark}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("更新不存在的小程序应返回 ErrNotFound，实际 %v", err)
	}
}

func TestDeleteAuthorizerOnlyForUnauthorized(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	svc := NewAuthorizerService(te.env)

	if err := svc.Delete(ctx, testAuthorizerAppid); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("已授权的小程序应拒绝删除（409），实际 %v", err)
	}
	if err := svc.OnUnauthorized(ctx, testAuthorizerAppid); err != nil {
		t.Fatalf("OnUnauthorized 失败: %v", err)
	}
	if err := svc.Delete(ctx, testAuthorizerAppid); err != nil {
		t.Fatalf("取消授权后应可删除: %v", err)
	}
	if _, err := te.env.Repos.Authorizers.Get(ctx, testAuthorizerAppid); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("软删后 Get 应返回 ErrNotFound，实际 %v", err)
	}
	// 软删除：物理行还在（审计可查）。
	if n := te.authorizerCount(testAuthorizerAppid); n != 1 {
		t.Fatalf("软删后物理记录数 = %d，期望 1", n)
	}
	te.requireAction(actionDeleteAuthorizer)

	if err := svc.Delete(ctx, "wx-not-exists"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("删除不存在的小程序应返回 ErrNotFound，实际 %v", err)
	}
}

func TestSyncWritesDomainAndPrivacySnapshot(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	te.wx.jsonRoute("/cgi-bin/component/api_get_authorizer_info", authorizerInfoPayload(testAuthorizerAppid))
	te.wx.jsonRoute("/wxa/get_effective_domain", map[string]any{
		"mp_domain":        map[string]any{"requestdomain": []string{"https://mp.example.com"}},
		"third_domain":     map[string]any{"requestdomain": []string{"https://third.example.com"}},
		"effective_domain": map[string]any{"requestdomain": []string{"https://api.example.com"}, "wsrequestdomain": []string{"wss://api.example.com"}},
	})
	te.wx.jsonRoute("/wxa/get_effective_webviewdomain", map[string]any{
		"mp_webviewdomain":        []string{"https://mp-web.example.com"},
		"third_webviewdomain":     []string{"https://third-web.example.com"},
		"effective_webviewdomain": []string{"https://web.example.com"},
	})
	te.wx.jsonRoute("/cgi-bin/component/getprivacysetting", map[string]any{
		"code_exist":   1,
		"privacy_list": []string{"userInfo"},
	})
	svc := NewAuthorizerService(te.env)

	detail, err := svc.Sync(ctx, testAuthorizerAppid)
	if err != nil {
		t.Fatalf("Sync 失败: %v", err)
	}
	if detail.LastSyncAt == nil {
		t.Error("Sync 后 LastSyncAt 不应为空")
	}
	if detail.LastPreflightAt == nil {
		t.Error("Sync 后 LastPreflightAt 不应为空")
	}
	if detail.PrivacySettingConfigured == nil || !*detail.PrivacySettingConfigured {
		t.Errorf("code_exist=1 应视为隐私指引已配置: %v", detail.PrivacySettingConfigured)
	}
	if detail.EffectiveDomains == nil {
		t.Fatal("EffectiveDomains 不应为空")
	}
	if detail.EffectiveDomains.RequestDomain == nil || (*detail.EffectiveDomains.RequestDomain)[0] != "https://api.example.com" {
		t.Errorf("生效服务器域名不对: %+v", detail.EffectiveDomains.RequestDomain)
	}
	if detail.EffectiveDomains.WsRequestDomain == nil || (*detail.EffectiveDomains.WsRequestDomain)[0] != "wss://api.example.com" {
		t.Errorf("生效 ws 域名不对: %+v", detail.EffectiveDomains.WsRequestDomain)
	}
	if detail.EffectiveDomains.BusinessDomain == nil || (*detail.EffectiveDomains.BusinessDomain)[0] != "https://web.example.com" {
		t.Errorf("生效业务域名不对: %+v", detail.EffectiveDomains.BusinessDomain)
	}
	if detail.RegisteredDomains == nil || detail.RegisteredDomains.RequestDomain == nil ||
		(*detail.RegisteredDomains.RequestDomain)[0] != "https://third.example.com" {
		t.Errorf("平台已登记域名不对: %+v", detail.RegisteredDomains)
	}
	if detail.RegisteredDomains.BusinessDomain == nil || (*detail.RegisteredDomains.BusinessDomain)[0] != "https://third-web.example.com" {
		t.Errorf("平台已登记业务域名不对: %+v", detail.RegisteredDomains.BusinessDomain)
	}
	if detail.NickName == nil || *detail.NickName != "示例小程序" {
		t.Errorf("Sync 应更新账号资料: %v", detail.NickName)
	}
	te.requireAction(actionSync)
}

func TestSyncSurvivesSnapshotFailures(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	te.wx.jsonRoute("/cgi-bin/component/api_get_authorizer_info", authorizerInfoPayload(testAuthorizerAppid))
	// 域名与隐私接口都失败：同步仍应成功，只把缺失项记进快照。
	te.wx.failRoute("/wxa/get_effective_domain", 61007, "api is unauthorized to component")
	te.wx.failRoute("/wxa/get_effective_webviewdomain", 61007, "api is unauthorized to component")
	te.wx.failRoute("/cgi-bin/component/getprivacysetting", 61007, "api is unauthorized to component")
	svc := NewAuthorizerService(te.env)

	detail, err := svc.Sync(ctx, testAuthorizerAppid)
	if err != nil {
		t.Fatalf("域名/隐私取不到时 Sync 不应失败: %v", err)
	}
	if detail.NickName == nil || *detail.NickName != "示例小程序" {
		t.Errorf("资料仍应更新: %v", detail.NickName)
	}
	if detail.EffectiveDomains != nil || detail.RegisteredDomains != nil {
		t.Errorf("取不到域名时不应给出快照: %+v", detail.EffectiveDomains)
	}
	if detail.PrivacySettingConfigured != nil {
		t.Errorf("隐私指引取不到时应保持「未探测」（nil），实际 %v", *detail.PrivacySettingConfigured)
	}
	row := te.authorizerRow(testAuthorizerAppid)
	if row.DomainSnapshot["missing"] == nil {
		t.Errorf("快照应记录缺失项: %v", row.DomainSnapshot)
	}
}

func TestSyncManyIsolatesFailures(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	te.seedAuthorizer(failingAuthorizer, "REFRESH-TOKEN-9", nil)
	te.wx.route("/cgi-bin/component/api_get_authorizer_info", func(w http.ResponseWriter, call recordedCall) {
		appid, _ := call.Body["authorizer_appid"].(string)
		if appid == failingAuthorizer {
			writeMockJSON(w, map[string]any{"errcode": 61007, "errmsg": "api is unauthorized to component"})
			return
		}
		writeMockJSON(w, authorizerInfoPayload(appid))
	})
	te.wx.jsonRoute("/cgi-bin/component/getprivacysetting", map[string]any{"code_exist": 1})
	svc := NewAuthorizerService(te.env)

	appids := []string{testAuthorizerAppid, failingAuthorizer}
	summary, err := svc.SyncMany(ctx, gen.AppidSelection{Appids: &appids})
	if err != nil {
		t.Fatalf("SyncMany 不应整体失败: %v", err)
	}
	if summary.Total != 2 || summary.Succeeded != 1 || summary.Failed != 1 {
		t.Fatalf("SyncSummary = total:%d succeeded:%d failed:%d，期望 2/1/1", summary.Total, summary.Succeeded, summary.Failed)
	}
	if summary.Details == nil || len(*summary.Details) != 2 {
		t.Fatalf("Details 应有 2 项: %+v", summary.Details)
	}
	byKey := map[string]gen.SyncDetail{}
	for _, d := range *summary.Details {
		byKey[d.Key] = d
	}
	failed, ok := byKey[failingAuthorizer]
	if !ok || failed.Ok {
		t.Fatalf("失败项应标记 ok=false: %+v", byKey)
	}
	if failed.Errcode == nil || *failed.Errcode != 61007 {
		t.Errorf("失败项应带微信返回码 61007: %+v", failed.Errcode)
	}
	if failed.Errmsg == nil || !strings.Contains(*failed.Errmsg, "61007") {
		t.Errorf("失败项应带中文处置说明: %v", failed.Errmsg)
	}
	if ok := byKey[testAuthorizerAppid]; !ok.Ok {
		t.Errorf("成功项应标记 ok=true: %+v", ok)
	}
	if te.authorizerRow(testAuthorizerAppid).LastSyncAt == nil {
		t.Error("成功项应写入 LastSyncAt")
	}
	if te.authorizerRow(failingAuthorizer).LastSyncAt != nil {
		t.Error("失败项不应写入 LastSyncAt")
	}
	te.requireAction(actionSyncMany)
}

func TestResyncTokensReportsMissing(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	te.seedAuthorizer(testAuthorizerAppid, testRefreshToken, nil)
	te.wx.jsonRoute("/cgi-bin/component/api_get_authorizer_list", map[string]any{
		"total_count": 2,
		"list": []map[string]any{
			{"authorizer_appid": testAuthorizerAppid, "refresh_token": "REFRESH-TOKEN-NEW", "auth_time": 1700000000},
			{"authorizer_appid": failingAuthorizer},
		},
	})
	svc := NewAuthorizerService(te.env)

	summary, err := svc.ResyncTokens(ctx)
	if err != nil {
		t.Fatalf("ResyncTokens 失败: %v", err)
	}
	if summary.Total != 2 || summary.Succeeded != 1 || summary.Failed != 1 {
		t.Fatalf("SyncSummary = total:%d succeeded:%d failed:%d，期望 2/1/1", summary.Total, summary.Succeeded, summary.Failed)
	}
	if summary.Details == nil || len(*summary.Details) != 1 {
		t.Fatalf("缺失项应逐条列出: %+v", summary.Details)
	}
	detail := (*summary.Details)[0]
	if detail.Key != failingAuthorizer || detail.Ok {
		t.Errorf("缺失项标识不对: %+v", detail)
	}
	if detail.Errmsg == nil || !strings.Contains(*detail.Errmsg, "重新扫码授权") {
		t.Errorf("缺失项应给出官方处置建议: %v", detail.Errmsg)
	}
	plain, err := te.box.Open([]byte(te.authorizerRow(testAuthorizerAppid).RefreshTokenCipher))
	if err != nil {
		t.Fatalf("解密重拉后的 refresh_token 失败: %v", err)
	}
	if plain != "REFRESH-TOKEN-NEW" {
		t.Errorf("重拉后的 refresh_token = %q，期望 REFRESH-TOKEN-NEW", plain)
	}
	te.requireAction(actionResyncTokens)
}

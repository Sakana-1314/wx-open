package wxaudit_test

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// TestReleaseWritesLedger 验证发布成功写发布台账（版本取最近一次审核成功的版本），
// 且请求体是空 JSON {}（官方：不传 data 会报 44002）。
func TestReleaseWritesLedger(t *testing.T) {
	const appid = "wx_release_app"
	f := newFixture(t)
	f.app(appid, "发布用例小程序", model.PermissionSetDev)
	// 最近一次审核成功的版本是 2.1.0（比它更新的那条是撤回记录，不应被取用）。
	f.seedAudit(appid, 1001, 0, "2.1.0")
	f.seedAudit(appid, 1002, 3, "2.1.1")
	f.mock.errcode("/wxa/release", 0, "ok")

	f.seed()
	result, err := f.release.Release(f.ctx, appid)
	if err != nil {
		t.Fatalf("Release 失败: %v", err)
	}
	if !result.Ok || result.Message == nil {
		t.Fatalf("发布结果不对：%+v", result)
	}
	if !strings.Contains(*result.Message, "全量发布") || !strings.Contains(*result.Message, "2.1.0") {
		t.Fatalf("发布说明应解释「全量发布、立即生效」并带版本号：%q", *result.Message)
	}

	calls := f.mock.callsTo("/wxa/release")
	if len(calls) != 1 || calls[0].Method != http.MethodPost {
		t.Fatalf("官方要求 POST /wxa/release：%+v", calls)
	}
	if strings.TrimSpace(calls[0].Body) != "{}" {
		t.Fatalf("官方要求 body 为 {}，实际 %q", calls[0].Body)
	}

	records := f.releaseRecords(appid)
	if len(records) != 1 {
		t.Fatalf("应写 1 条发布台账，实际 %d", len(records))
	}
	if records[0].Action != model.ReleaseActionRelease {
		t.Fatalf("台账动作为 release，实际 %q", records[0].Action)
	}
	if records[0].UserVersion != "2.1.0" {
		t.Fatalf("台账版本应取最近一次审核成功的版本 2.1.0，实际 %q", records[0].UserVersion)
	}
	if records[0].ReleaseTime == nil {
		t.Fatalf("台账应记录发布时间")
	}
	if len(f.operationLogs("releaseApp")) != 1 {
		t.Fatalf("发布应写 operation_logs(action=releaseApp)")
	}
}

// TestReleaseNoAuditVersionHint 验证 85019（没有审核版本）转成中文动作建议。
func TestReleaseNoAuditVersionHint(t *testing.T) {
	const appid = "wx_release_no_version"
	f := newFixture(t)
	f.app(appid, "无审核版本小程序", model.PermissionSetDev)
	f.mock.errcode("/wxa/release", 85019, "no version is under auditing")

	f.seed()
	_, err := f.release.Release(f.ctx, appid)
	var wxe *core.WeChatError
	if !errors.As(err, &wxe) {
		t.Fatalf("应返回 core.WeChatError，实际 %T: %v", err, err)
	}
	if wxe.Errcode != 85019 {
		t.Fatalf("应保留 errcode 85019，实际 %d", wxe.Errcode)
	}
	if !strings.Contains(wxe.Hint, "上传代码") || !strings.Contains(wxe.Hint, "审核通过") {
		t.Fatalf("应给出中文操作顺序建议，实际 %q", wxe.Hint)
	}
	if len(f.releaseRecords(appid)) != 0 {
		t.Fatalf("发布失败时不应写台账")
	}
}

// TestGrayReleaseValidation 验证灰度比例校验：0-100 整数，且 0 必须带 first 开关。
func TestGrayReleaseValidation(t *testing.T) {
	const appid = "wx_gray_validate"
	f := newFixture(t)
	f.app(appid, "灰度参数用例小程序", model.PermissionSetDev)
	f.mock.errcode("/wxa/grayrelease", 0, "ok")

	cases := []struct {
		name string
		req  gen.GrayReleaseRequest
		want string
	}{
		{
			name: "比例小于 0",
			req:  gen.GrayReleaseRequest{GrayPercentage: -1},
			want: "0-100",
		},
		{
			name: "比例大于 100",
			req:  gen.GrayReleaseRequest{GrayPercentage: 101},
			want: "0-100",
		},
		{
			name: "0 比例未指定 first 开关",
			req:  gen.GrayReleaseRequest{GrayPercentage: 0},
			want: "supportDebugerFirst",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f.seed()
			_, err := f.release.GrayRelease(f.ctx, appid, tc.req)
			if !errors.Is(err, core.ErrValidation) {
				t.Fatalf("应返回 ErrValidation，实际 %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("提示应包含 %q，实际 %q", tc.want, err.Error())
			}
		})
	}
	if got := f.mock.callCount("/wxa/grayrelease"); got != 0 {
		t.Fatalf("参数非法时不应调用微信，实际 %d 次", got)
	}
}

// TestGrayReleaseZeroWithDebugerFirst 验证 0 比例 + first 开关可以提交，并写灰度台账。
func TestGrayReleaseZeroWithDebugerFirst(t *testing.T) {
	const appid = "wx_gray_ok"
	f := newFixture(t)
	f.app(appid, "灰度成功小程序", model.PermissionSetDev)
	f.seedAudit(appid, 2001, 0, "4.0.0")
	f.mock.errcode("/wxa/grayrelease", 0, "ok")

	debugerFirst := true
	f.seed()
	result, err := f.release.GrayRelease(f.ctx, appid, gen.GrayReleaseRequest{
		GrayPercentage:      0,
		SupportDebugerFirst: &debugerFirst,
	})
	if err != nil {
		t.Fatalf("GrayRelease 失败: %v", err)
	}
	if !result.Ok || result.Message == nil || !strings.Contains(*result.Message, "只能递增") {
		t.Fatalf("灰度成功提示应说明比例只能递增：%+v", result.Message)
	}

	calls := f.mock.callsTo("/wxa/grayrelease")
	if len(calls) != 1 || calls[0].Method != http.MethodPost {
		t.Fatalf("官方要求 POST /wxa/grayrelease：%+v", calls)
	}
	if !strings.Contains(calls[0].Body, `"gray_percentage":0`) || !strings.Contains(calls[0].Body, `"support_debuger_first":true`) {
		t.Fatalf("请求体字段不对：%s", calls[0].Body)
	}

	records := f.releaseRecords(appid)
	if len(records) != 1 {
		t.Fatalf("应写 1 条发布台账，实际 %d", len(records))
	}
	if records[0].Action != model.ReleaseActionGrayRelease {
		t.Fatalf("台账动作为 grayrelease，实际 %q", records[0].Action)
	}
	if records[0].GrayPercentage == nil || *records[0].GrayPercentage != 0 {
		t.Fatalf("台账应记录灰度比例 0：%+v", records[0].GrayPercentage)
	}
	if records[0].UserVersion != "4.0.0" {
		t.Fatalf("台账版本应取最近一次审核成功的版本，实际 %q", records[0].UserVersion)
	}
	if len(f.operationLogs("grayReleaseApp")) != 1 {
		t.Fatalf("灰度应写 operation_logs(action=grayReleaseApp)")
	}
}

// TestGrayReleaseIncreasingHint 验证 85082（灰度比例只能递增）转成中文建议。
func TestGrayReleaseIncreasingHint(t *testing.T) {
	const appid = "wx_gray_increase"
	f := newFixture(t)
	f.app(appid, "灰度递增用例小程序", model.PermissionSetDev)
	f.mock.errcode("/wxa/grayrelease", 85082, "gray percentage must be bigger")

	f.seed()
	_, err := f.release.GrayRelease(f.ctx, appid, gen.GrayReleaseRequest{GrayPercentage: 30})
	var wxe *core.WeChatError
	if !errors.As(err, &wxe) {
		t.Fatalf("应返回 core.WeChatError，实际 %T: %v", err, err)
	}
	if !strings.Contains(wxe.Hint, "只能递增") || !strings.Contains(wxe.Hint, "85082") {
		t.Fatalf("应给出中文建议，实际 %q", wxe.Hint)
	}
}

// TestGrayPlan 验证灰度计划字段转换（含时间戳）。
func TestGrayPlan(t *testing.T) {
	const appid = "wx_gray_plan"
	f := newFixture(t)
	f.app(appid, "灰度计划小程序", model.PermissionSetDev)
	f.mock.jsonReply("/wxa/getgrayreleaseplan",
		`{"errcode":0,"errmsg":"ok","gray_release_plan":{"status":1,"create_timestamp":1517553721,"gray_percentage":8,"support_debuger_first":true,"support_experiencer_first":false}}`)

	f.seed()
	plan, err := f.release.GrayPlan(f.ctx, appid)
	if err != nil {
		t.Fatalf("GrayPlan 失败: %v", err)
	}
	if plan.Status == nil || *plan.Status != 1 || plan.GrayPercentage == nil || *plan.GrayPercentage != 8 {
		t.Fatalf("灰度计划字段不对：%+v", plan)
	}
	if plan.CreateTimestamp == nil || !plan.CreateTimestamp.Equal(ts(1517553721)) {
		t.Fatalf("创建时间应转成时间戳：%v", plan.CreateTimestamp)
	}
	if plan.SupportDebugerFirst == nil || !*plan.SupportDebugerFirst {
		t.Fatalf("first 开关未回传：%+v", plan)
	}
}

// TestRevertWithoutHistoryExplains87012 验证没有可回退历史版本时给出 87012 的中文解释，
// 并且不会去调用真正的回退接口。
func TestRevertWithoutHistoryExplains87012(t *testing.T) {
	const appid = "wx_revert_empty"
	f := newFixture(t)
	f.app(appid, "无可回退版本小程序", model.PermissionSetDev)
	f.revertRoute(`{"errcode":0,"errmsg":"ok","version_list":[]}`, `{"errcode":0,"errmsg":"ok"}`)

	f.seed()
	_, err := f.release.Revert(f.ctx, appid, nil)
	if !errors.Is(err, core.ErrConflict) {
		t.Fatalf("应返回 ErrConflict，实际 %v", err)
	}
	for _, want := range []string{"87012", "没有可回退的历史版本", "5 个"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("中文解释应包含 %q，实际 %q", want, err.Error())
		}
	}
	historyCalls, realCalls := countRevertCalls(f)
	if historyCalls != 1 || realCalls != 0 {
		t.Fatalf("只应查询历史版本，不应发起回退：history=%d real=%d", historyCalls, realCalls)
	}
	if len(f.releaseRecords(appid)) != 0 {
		t.Fatalf("未回退成功不应写台账")
	}
}

// TestRevertSpecifiedVersionMissing 验证指定版本不在可回退列表时的中文解释。
func TestRevertSpecifiedVersionMissing(t *testing.T) {
	const appid = "wx_revert_missing"
	f := newFixture(t)
	f.app(appid, "指定版本不存在小程序", model.PermissionSetDev)
	f.revertRoute(`{"errcode":0,"version_list":[{"app_version":11,"user_version":"1.0.0","user_desc":"旧版本","commit_time":1700000400}]}`,
		`{"errcode":0,"errmsg":"ok"}`)

	version := int64(99)
	f.seed()
	_, err := f.release.Revert(f.ctx, appid, &gen.RevertRequest{AppVersion: &version})
	if !errors.Is(err, core.ErrConflict) {
		t.Fatalf("应返回 ErrConflict，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "87012") || !strings.Contains(err.Error(), "99") {
		t.Fatalf("应指出具体版本并解释 87012：%q", err.Error())
	}
	if _, realCalls := countRevertCalls(f); realCalls != 0 {
		t.Fatalf("本地校验失败时不应调用回退接口")
	}
}

// TestRevertToPreviousVersion 验证不传 appVersion 时回退到上一个版本并写台账，
// 且 app_version 是 URL 参数（不传时不应出现在 query 里）。
func TestRevertToPreviousVersion(t *testing.T) {
	const appid = "wx_revert_ok"
	f := newFixture(t)
	f.app(appid, "回退成功小程序", model.PermissionSetDev)
	f.revertRoute(`{"errcode":0,"version_list":[{"app_version":11,"user_version":"1.0.0","user_desc":"旧版本","commit_time":1700000400}]}`,
		`{"errcode":0,"errmsg":"ok"}`)

	f.seed()
	result, err := f.release.Revert(f.ctx, appid, &gen.RevertRequest{})
	if err != nil {
		t.Fatalf("Revert 失败: %v", err)
	}
	if !result.Ok || result.Message == nil || !strings.Contains(*result.Message, "上一个线上版本") {
		t.Fatalf("回退提示不对：%+v", result.Message)
	}

	_, realCalls := countRevertCalls(f)
	if realCalls != 1 {
		t.Fatalf("应调用一次回退接口，实际 %d", realCalls)
	}
	for _, c := range f.mock.callsTo("/wxa/revertcoderelease") {
		if c.Query.Get("action") != "get_history_version" && c.Query.Get("app_version") != "" {
			t.Fatalf("不传版本时不应带 app_version，实际 %q", c.Query.Get("app_version"))
		}
	}

	records := f.releaseRecords(appid)
	if len(records) != 1 || records[0].Action != model.ReleaseActionRevert {
		t.Fatalf("应写 1 条 revert 台账：%+v", records)
	}
	if records[0].UserVersion != "1.0.0" {
		t.Fatalf("台账版本应取历史版本号，实际 %q", records[0].UserVersion)
	}
	if len(f.operationLogs("revertRelease")) != 1 {
		t.Fatalf("回退应写 operation_logs(action=revertRelease)")
	}
}

// TestRevertWithExplicitVersion 验证指定版本回退时 app_version 走 URL 参数。
func TestRevertWithExplicitVersion(t *testing.T) {
	const appid = "wx_revert_explicit"
	f := newFixture(t)
	f.app(appid, "指定版本回退小程序", model.PermissionSetDev)
	f.revertRoute(`{"errcode":0,"version_list":[{"app_version":22,"user_version":"2.0.0","user_desc":"待回退版本","commit_time":1700000500}]}`,
		`{"errcode":0,"errmsg":"ok"}`)

	version := int64(22)
	f.seed()
	if _, err := f.release.Revert(f.ctx, appid, &gen.RevertRequest{AppVersion: &version}); err != nil {
		t.Fatalf("Revert 失败: %v", err)
	}
	found := false
	for _, c := range f.mock.callsTo("/wxa/revertcoderelease") {
		if c.Query.Get("action") == "get_history_version" {
			continue
		}
		if c.Query.Get("app_version") == "22" {
			found = true
		}
	}
	if !found {
		t.Fatalf("app_version 应作为 URL 参数传递")
	}
	if records := f.releaseRecords(appid); len(records) != 1 || records[0].UserVersion != "2.0.0" {
		t.Fatalf("台账版本不对：%+v", records)
	}
}

// TestHistoryVersionsAndRevertGray 验证历史版本列表与取消灰度。
func TestHistoryVersionsAndRevertGray(t *testing.T) {
	const appid = "wx_release_misc"
	f := newFixture(t)
	f.app(appid, "历史版本小程序", model.PermissionSetDev)
	f.revertRoute(`{"errcode":0,"version_list":[{"app_version":11,"user_version":"1.0.0","user_desc":"旧版本","commit_time":1700000400},{"app_version":12,"user_version":"1.1.0","user_desc":"更新的旧版本","commit_time":1700000500}]}`,
		`{"errcode":0,"errmsg":"ok"}`)
	f.mock.errcode("/wxa/revertgrayrelease", 0, "ok")

	f.seed()
	versions, err := f.release.HistoryVersions(f.ctx, appid)
	if err != nil {
		t.Fatalf("HistoryVersions 失败: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("应有 2 个历史版本，实际 %d", len(versions))
	}
	if versions[0].AppVersion != 11 || versions[0].UserVersion == nil || *versions[0].UserVersion != "1.0.0" {
		t.Fatalf("历史版本字段不对：%+v", versions[0])
	}
	if versions[0].CommitTime == nil || !versions[0].CommitTime.Equal(ts(1700000400)) {
		t.Fatalf("提交时间未转换：%v", versions[0].CommitTime)
	}

	f.seed()
	result, err := f.release.RevertGray(f.ctx, appid)
	if err != nil {
		t.Fatalf("RevertGray 失败: %v", err)
	}
	if !result.Ok || result.Message == nil || !strings.Contains(*result.Message, "取消分阶段发布") {
		t.Fatalf("取消灰度提示不对：%+v", result.Message)
	}
	if got := f.mock.callCount("/wxa/revertgrayrelease"); got != 1 {
		t.Fatalf("应调用一次取消灰度，实际 %d", got)
	}
	if len(f.operationLogs("revertGrayRelease")) != 1 {
		t.Fatalf("取消灰度应写 operation_logs(action=revertGrayRelease)")
	}
}

// TestVersionInfoAndTrialQRCode 验证版本信息与体验版二维码（二进制 → base64）。
func TestVersionInfoAndTrialQRCode(t *testing.T) {
	const appid = "wx_release_qr"
	f := newFixture(t)
	f.app(appid, "二维码用例小程序", model.PermissionSetDev)
	f.mock.jsonReply("/wxa/getversioninfo",
		`{"errcode":0,"errmsg":"ok","exp_info":{"exp_time":1700000600,"exp_version":"5.0.0","exp_desc":"体验版"},"release_info":{"release_time":1700000700,"release_version":"4.9.0","release_desc":"线上版"}}`)

	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	f.mock.route("/wxa/get_qrcode", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpeg)
	})

	info, err := f.release.VersionInfo(f.ctx, appid)
	if err != nil {
		t.Fatalf("VersionInfo 失败: %v", err)
	}
	if info.ExpVersion == nil || *info.ExpVersion != "5.0.0" || info.ReleaseVersion == nil || *info.ReleaseVersion != "4.9.0" {
		t.Fatalf("版本信息不对：%+v", info)
	}
	if info.ReleaseTime == nil || !info.ReleaseTime.Equal(ts(1700000700)) {
		t.Fatalf("发布时间未转换：%v", info.ReleaseTime)
	}
	if calls := f.mock.callsTo("/wxa/getversioninfo"); len(calls) != 1 || calls[0].Method != http.MethodPost || strings.TrimSpace(calls[0].Body) != "{}" {
		t.Fatalf("getversioninfo 必须 POST 空 JSON {}：%+v", calls)
	}

	path := "pages/index/index?from=1"
	qr, err := f.release.TrialQRCode(f.ctx, appid, gen.GetTrialQRCodeParams{Path: &path})
	if err != nil {
		t.Fatalf("TrialQRCode 失败: %v", err)
	}
	if qr.ContentType != "image/jpeg" {
		t.Fatalf("Content-Type 应为 image/jpeg，实际 %q", qr.ContentType)
	}
	if qr.Base64 != base64.StdEncoding.EncodeToString(jpeg) {
		t.Fatalf("二维码应转成标准 base64")
	}
	calls := f.mock.callsTo("/wxa/get_qrcode")
	if len(calls) != 1 || calls[0].Method != http.MethodGet {
		t.Fatalf("官方要求 GET /wxa/get_qrcode：%+v", calls)
	}
	if calls[0].Query.Get("path") != path {
		t.Fatalf("path 应作为查询参数透传，实际 %q", calls[0].Query.Get("path"))
	}
}

// countRevertCalls 统计回退接口上的「历史版本查询」与「真正回退」次数。
func countRevertCalls(f *fixture) (history, real int) {
	for _, c := range f.mock.callsTo("/wxa/revertcoderelease") {
		if c.Query.Get("action") == "get_history_version" {
			history++
			continue
		}
		real++
	}
	return history, real
}

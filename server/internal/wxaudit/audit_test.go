package wxaudit_test

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// TestSyncAuditStatusPersistsRecords 验证审核状态同步：调用 GET get_latest_auditstatus、
// auditid 兼容 number/string、截图字段兼容 ScreenShot/screenshot 并落库。
func TestSyncAuditStatusPersistsRecords(t *testing.T) {
	const appid = "wx_audit_sync_app"

	cases := []struct {
		name       string
		body       string
		wantAudit  int64
		wantStatus int
		wantVer    string
		wantDesc   string
		wantMedia  []string
		wantSubmit int64
	}{
		{
			name:       "大写 ScreenShot（官方示例口径）",
			body:       `{"errcode":0,"errmsg":"ok","auditid":"1234567890","status":0,"ScreenShot":"media_shot_1|media_shot_2","user_version":"1.2.0","user_desc":"首个版本","submit_audit_time":1700000000}`,
			wantAudit:  1234567890,
			wantStatus: 0,
			wantVer:    "1.2.0",
			wantDesc:   "首个版本",
			wantMedia:  []string{"media_shot_1", "media_shot_2"},
			wantSubmit: 1700000000,
		},
		{
			name:       "小写 screenshot（官方字段表口径）",
			body:       `{"errcode":0,"errmsg":"ok","auditid":1234567891,"status":2,"screenshot":"media_shot_9","user_version":"1.2.1","user_desc":"第二个版本","submit_audit_time":1700000100}`,
			wantAudit:  1234567891,
			wantStatus: 2,
			wantVer:    "1.2.1",
			wantDesc:   "第二个版本",
			wantMedia:  []string{"media_shot_9"},
			wantSubmit: 1700000100,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.app(appid, "同步用例小程序", model.PermissionSetDev)
			f.mock.jsonReply("/wxa/get_latest_auditstatus", tc.body)

			// 共享测试库可能被并发跑的其它测试包清表：允许重试，直到台账确实落库。
			var summary *gen.SyncSummary
			var err error
			for attempt := 0; attempt < 3; attempt++ {
				f.seed()
				summary, err = f.audit.Sync(f.ctx, appids(appid))
				if err == nil && len(f.auditRecords(appid)) == 1 {
					break
				}
				t.Logf("第 %d 次同步后台账为空（共享测试库可能被并发清表），重试", attempt+1)
			}
			if err != nil {
				t.Fatalf("Sync 失败: %v", err)
			}
			if summary.Total != 1 || summary.Succeeded != 1 || summary.Failed != 0 {
				t.Fatalf("同步汇总不对：%+v", summary)
			}
			if summary.Details == nil || len(*summary.Details) != 1 || !(*summary.Details)[0].Ok {
				t.Fatalf("同步明细不对：%+v", summary.Details)
			}

			// 官方要求 GET，且令牌作为 access_token 传递。
			calls := f.mock.callsTo("/wxa/get_latest_auditstatus")
			if len(calls) < 1 {
				t.Fatalf("应调用 get_latest_auditstatus")
			}
			if calls[0].Method != http.MethodGet {
				t.Fatalf("官方要求 GET，实际 %s", calls[0].Method)
			}
			if got := calls[0].Query.Get("access_token"); got != "token_"+appid {
				t.Fatalf("access_token 应为预置令牌，实际 %q", got)
			}

			rows := f.auditRecords(appid)
			if len(rows) != 1 {
				t.Fatalf("审核台账应有 1 条，实际 %d 条", len(rows))
			}
			rec := rows[0]
			if rec.AuditID != tc.wantAudit {
				t.Fatalf("auditid 应为 %d，实际 %d", tc.wantAudit, rec.AuditID)
			}
			if rec.Status != tc.wantStatus {
				t.Fatalf("status 应为 %d，实际 %d", tc.wantStatus, rec.Status)
			}
			if rec.UserVersion != tc.wantVer || rec.UserDesc != tc.wantDesc {
				t.Fatalf("版本/说明未落库：%q / %q", rec.UserVersion, rec.UserDesc)
			}
			if strings.Join(rec.ScreenshotMediaIDs, ",") != strings.Join(tc.wantMedia, ",") {
				t.Fatalf("截图 mediaid 应为 %v，实际 %v", tc.wantMedia, rec.ScreenshotMediaIDs)
			}
			if rec.Source != model.AuditSourcePoll {
				t.Fatalf("Source 应为 poll，实际 %q", rec.Source)
			}
			if rec.SubmitTime == nil || !rec.SubmitTime.Equal(ts(tc.wantSubmit)) {
				t.Fatalf("提交时间应为 %d，实际 %v", tc.wantSubmit, rec.SubmitTime)
			}
		})
	}
}

// TestSyncAuditStatusKeepsGoingOnFailure 验证单个小程序失败不中断整体同步。
func TestSyncAuditStatusKeepsGoingOnFailure(t *testing.T) {
	const good, bad = "wx_audit_sync_good", "wx_audit_sync_bad"
	f := newFixture(t)
	f.app(good, "正常小程序", model.PermissionSetDev)
	f.app(bad, "异常小程序", model.PermissionSetDev)

	// 令牌里带着 appid，模拟服务端据此区分两个小程序（请求本身不带 appid）。
	f.mock.route("/wxa/get_latest_auditstatus", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("access_token") == "token_"+bad {
			_, _ = io.WriteString(w, `{"errcode":85012,"errmsg":"invalid audit id"}`)
			return
		}
		_, _ = io.WriteString(w, `{"errcode":0,"errmsg":"ok","auditid":42,"status":2,"user_version":"1.0.0"}`)
	})

	f.seed()
	summary, err := f.audit.Sync(f.ctx, appids(good, bad))
	if err != nil {
		t.Fatalf("Sync 失败: %v", err)
	}
	if summary.Total != 2 || summary.Succeeded != 1 || summary.Failed != 1 {
		t.Fatalf("汇总应为 2/1/1，实际 %+v", summary)
	}
	var failedDetail *gen.SyncDetail
	for i := range *summary.Details {
		if (*summary.Details)[i].Key == bad {
			failedDetail = &(*summary.Details)[i]
		}
	}
	if failedDetail == nil || failedDetail.Ok {
		t.Fatalf("失败小程序应出现在明细里且 Ok=false：%+v", summary.Details)
	}
	if failedDetail.Errcode == nil || *failedDetail.Errcode != 85012 {
		t.Fatalf("失败明细应带 errcode 85012：%+v", failedDetail)
	}
	if len(f.auditRecords(good)) != 1 {
		t.Fatalf("正常小程序应落库 1 条")
	}
	if len(f.auditRecords(bad)) != 0 {
		t.Fatalf("异常小程序不应落库")
	}
}

// TestOnAuditResultBackfillsAuditID 验证事件推送没有 auditid 时通过回查补齐，
// 且以事件结论（fail）为准、保留事件里的驳回原因与截图。
func TestOnAuditResultBackfillsAuditID(t *testing.T) {
	const appid = "wx_audit_event_app"
	f := newFixture(t)
	f.app(appid, "事件用例小程序", model.PermissionSetDev)
	// 回查到的审核单仍显示「审核中」，事件才是最终结论。
	f.mock.jsonReply("/wxa/get_latest_auditstatus",
		`{"errcode":0,"errmsg":"ok","auditid":987654321,"status":2,"ScreenShot":"","user_version":"2.0.0","user_desc":"事件版本","submit_audit_time":1700000200}`)

	f.seed()
	if err := f.audit.OnAuditResult(f.ctx, appid, "weapp_audit_fail", "驳回原因：类目不符", "shot_a|shot_b", 1700000300); err != nil {
		t.Fatalf("OnAuditResult 不应返回错误: %v", err)
	}

	rows := f.auditRecords(appid)
	if len(rows) != 1 {
		t.Fatalf("应落库 1 条审核记录，实际 %d", len(rows))
	}
	rec := rows[0]
	if rec.AuditID != 987654321 {
		t.Fatalf("auditid 应由回查补齐，实际 %d", rec.AuditID)
	}
	if rec.Status != 1 {
		t.Fatalf("事件结论应为「审核被拒绝」(1)，实际 %d", rec.Status)
	}
	if rec.Reason != "驳回原因：类目不符" {
		t.Fatalf("驳回原因应取事件内容，实际 %q", rec.Reason)
	}
	if strings.Join(rec.ScreenshotMediaIDs, ",") != "shot_a,shot_b" {
		t.Fatalf("截图应按 | 拆分，实际 %v", rec.ScreenshotMediaIDs)
	}
	if rec.Source != model.AuditSourceEvent {
		t.Fatalf("Source 应为 event，实际 %q", rec.Source)
	}
	if rec.StatusTime == nil || !rec.StatusTime.Equal(ts(1700000300)) {
		t.Fatalf("状态时间应取事件时间：%v", rec.StatusTime)
	}
	if rec.UserVersion != "2.0.0" {
		t.Fatalf("版本应取回查结果，实际 %q", rec.UserVersion)
	}
}

// TestOnAuditResultDoesNotBlockCallback 验证回查失败时不写 audit_id=0、不阻塞回调。
func TestOnAuditResultDoesNotBlockCallback(t *testing.T) {
	const appid = "wx_audit_event_fail"
	f := newFixture(t)
	f.app(appid, "事件回查失败小程序", model.PermissionSetDev)
	f.mock.errcode("/wxa/get_latest_auditstatus", 40001, "invalid credential")

	f.seed()
	if err := f.audit.OnAuditResult(f.ctx, appid, "weapp_audit_success", "", "", 1700000400); err != nil {
		t.Fatalf("回查失败也必须返回 nil（不阻塞回调）：%v", err)
	}
	if rows := f.auditRecords(appid); len(rows) != 0 {
		t.Fatalf("回查失败时不应写台账（尤其不能写 audit_id=0），实际 %d 条", len(rows))
	}

	// 未知事件直接忽略，且不调用微信。
	before := f.mock.callCount("/wxa/get_latest_auditstatus")
	f.seed()
	if err := f.audit.OnAuditResult(f.ctx, appid, "weapp_audit_unknown", "", "", 1700000400); err != nil {
		t.Fatalf("未知事件应忽略: %v", err)
	}
	if got := f.mock.callCount("/wxa/get_latest_auditstatus"); got != before {
		t.Fatalf("未知事件不应调用微信接口，调用数 %d → %d", before, got)
	}
}

// TestAuditListAndHistory 验证台账列表分页、状态过滤与历史查询的 404 语义。
func TestAuditListAndHistory(t *testing.T) {
	const appid = "wx_audit_list_app"
	f := newFixture(t)
	f.app(appid, "列表用例小程序", model.PermissionSetDev)
	f.seedAudit(appid, 1, 0, "1.0.0")
	f.seedAudit(appid, 2, 1, "1.0.1")

	status := gen.AuditStatus(1)
	f.seed()
	resp, err := f.audit.List(f.ctx, gen.ListAuditsParams{Appid: strPtr(appid), Status: &status})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 {
		t.Fatalf("按状态过滤应有 1 条：%+v", resp)
	}
	if resp.Items[0].AuditId != 2 || resp.Items[0].UserVersion == nil || *resp.Items[0].UserVersion != "1.0.1" {
		t.Fatalf("列表内容不对：%+v", resp.Items[0])
	}
	if resp.Items[0].Source == nil || *resp.Items[0].Source != gen.AuditRecordSource(model.AuditSourceAPI) {
		t.Fatalf("Source 未回传：%+v", resp.Items[0].Source)
	}

	f.seed()
	all, err := f.audit.List(f.ctx, gen.ListAuditsParams{})
	if err != nil {
		t.Fatalf("List 全部失败: %v", err)
	}
	if all.Total != 2 || all.Page != 1 || all.PageSize != 20 {
		t.Fatalf("默认分页应为 1/20 且共 2 条：%+v", all)
	}

	f.seed()
	history, err := f.audit.History(f.ctx, appid)
	if err != nil {
		t.Fatalf("History 失败: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("历史应有 2 条，实际 %d", len(history))
	}

	// 未登记的小程序 → 404 语义。
	f.seed()
	if _, err := f.audit.History(f.ctx, "wx_not_registered"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("未登记小程序应返回 ErrNotFound，实际 %v", err)
	}
}

// TestUndoAuditDailyLimitConflict 验证撤回次数超限在本地就被拦下（不消耗微信调用）。
func TestUndoAuditDailyLimitConflict(t *testing.T) {
	const appid = "wx_audit_undo_limit"
	f := newFixture(t)
	f.app(appid, "撤回限流小程序", model.PermissionSetDev)
	for i := 0; i < 5; i++ {
		if err := f.env.Repos.UndoQuota.Record(f.ctx, appid, time.Now()); err != nil {
			t.Fatalf("写入撤回用量失败: %v", err)
		}
	}

	f.seed()
	_, err := f.audit.Undo(f.ctx, appid)
	if !errors.Is(err, core.ErrConflict) {
		t.Fatalf("超限应返回 ErrConflict，实际 %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"87013", "每天 5 次", "次日"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("冲突提示应包含 %q，实际 %q", want, msg)
		}
	}
	if got := f.mock.callCount("/wxa/undocodeaudit"); got != 0 {
		t.Fatalf("超限时不应调用微信撤回接口，实际调用 %d 次", got)
	}
}

// TestUndoAuditSuccessRecordsUsageAndStatus 验证撤回成功：记账、把本地审核记录标记为已撤回、写操作审计。
func TestUndoAuditSuccessRecordsUsageAndStatus(t *testing.T) {
	const appid = "wx_audit_undo_ok"
	f := newFixture(t)
	f.app(appid, "撤回成功小程序", model.PermissionSetDev)
	f.seedAudit(appid, 6666, 2, "3.0.0")
	f.mock.errcode("/wxa/undocodeaudit", 0, "ok")

	f.seed()
	result, err := f.audit.Undo(f.ctx, appid)
	if err != nil {
		t.Fatalf("Undo 失败: %v", err)
	}
	if !result.Ok || result.Appid != appid {
		t.Fatalf("撤回结果不对：%+v", result)
	}
	if result.Message == nil || !strings.Contains(*result.Message, "已撤回审核") {
		t.Fatalf("撤回结果应有中文说明：%+v", result.Message)
	}
	calls := f.mock.callsTo("/wxa/undocodeaudit")
	if len(calls) != 1 || calls[0].Method != http.MethodGet {
		t.Fatalf("官方要求 GET /wxa/undocodeaudit：%+v", calls)
	}

	today, err := f.env.Repos.UndoQuota.CountToday(f.ctx, appid)
	if err != nil {
		t.Fatalf("统计撤回用量失败: %v", err)
	}
	if today != 1 {
		t.Fatalf("撤回成功后本地用量应为 1，实际 %d", today)
	}
	rows := f.auditRecords(appid)
	if len(rows) != 1 || rows[0].Status != 3 {
		t.Fatalf("本地审核记录应被标记为已撤回(3)：%+v", rows)
	}
	if len(f.operationLogs("undoAudit")) != 1 {
		t.Fatalf("撤回应写 operation_logs(action=undoAudit)")
	}
}

// TestSpeedUpQuotaExhaustedHint 验证加急额度用尽（89405）转成中文动作建议。
func TestSpeedUpQuotaExhaustedHint(t *testing.T) {
	const appid = "wx_audit_speedup"
	f := newFixture(t)
	f.app(appid, "加急用例小程序", model.PermissionSetDev)
	f.mock.errcode("/wxa/speedupaudit", 89405, "speedup quota exhausted")

	f.seed()
	_, err := f.audit.SpeedUp(f.ctx, appid, gen.SpeedUpRequest{AuditId: 123})
	if err == nil {
		t.Fatalf("额度用尽应返回错误")
	}
	var wxe *core.WeChatError
	if !errors.As(err, &wxe) {
		t.Fatalf("应返回 core.WeChatError，实际 %T: %v", err, err)
	}
	if wxe.Errcode != 89405 {
		t.Fatalf("应保留 errcode 89405，实际 %d", wxe.Errcode)
	}
	if wxe.Class != model.ClassRateLimited {
		t.Fatalf("89405 应归类为 rate_limited，实际 %s", wxe.Class)
	}
	if !strings.Contains(wxe.Hint, "本月加急额度已用完") || !strings.Contains(wxe.Hint, "审核管理页") {
		t.Fatalf("应有中文动作建议，实际 %q", wxe.Hint)
	}
	calls := f.mock.callsTo("/wxa/speedupaudit")
	if len(calls) != 1 || calls[0].Method != http.MethodPost {
		t.Fatalf("官方要求 POST /wxa/speedupaudit：%+v", calls)
	}
	if !strings.Contains(calls[0].Body, `"auditid":123`) {
		t.Fatalf("加急请求体应带 auditid，实际 %s", calls[0].Body)
	}
}

// TestSpeedUpValidation 验证 auditid 非法时本地拦截。
func TestSpeedUpValidation(t *testing.T) {
	const appid = "wx_audit_speedup_invalid"
	f := newFixture(t)
	f.app(appid, "加急参数用例小程序", model.PermissionSetDev)

	f.seed()
	if _, err := f.audit.SpeedUp(f.ctx, appid, gen.SpeedUpRequest{AuditId: 0}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("auditId=0 应返回 ErrValidation，实际 %v", err)
	}
	if got := f.mock.callCount("/wxa/speedupaudit"); got != 0 {
		t.Fatalf("参数非法时不应调用微信，实际 %d 次", got)
	}
}

// TestQuotaCachesAndFallsBack 验证额度查询落库缓存、可从 Platform.CachedQuota 读回、查询失败时回退缓存。
func TestQuotaCachesAndFallsBack(t *testing.T) {
	const appid = "wx_audit_quota"
	f := newFixture(t)
	f.app(appid, "额度用例小程序", model.PermissionSetDev)
	f.mock.jsonReply("/wxa/queryquota", `{"errcode":0,"errmsg":"ok","rest":7,"limit":50,"speedup_rest":2,"speedup_limit":5}`)

	f.seed()
	quota, err := f.audit.Quota(f.ctx, appid)
	if err != nil {
		t.Fatalf("Quota 失败: %v", err)
	}
	if quota.Rest == nil || *quota.Rest != 7 || quota.Limit == nil || *quota.Limit != 50 {
		t.Fatalf("提审额度不对：%+v", quota)
	}
	if quota.SpeedupRest == nil || *quota.SpeedupRest != 2 || quota.SpeedupLimit == nil || *quota.SpeedupLimit != 5 {
		t.Fatalf("加急额度不对：%+v", quota)
	}
	if quota.QueriedAt == nil {
		t.Fatalf("应带查询时间")
	}
	calls := f.mock.callsTo("/wxa/queryquota")
	if len(calls) != 1 || calls[0].Method != http.MethodGet {
		t.Fatalf("官方要求 GET /wxa/queryquota：%+v", calls)
	}

	cached := f.cachedQuotaStable(appid)
	if cached == nil || cached.Rest == nil || *cached.Rest != 7 || *cached.SpeedupRest != 2 {
		t.Fatalf("额度应可从 Platform.CachedQuota 读回：%+v", cached)
	}

	// 查询失败 → 回退返回缓存值（额度为服务商级，页面不应空白）。
	f.mock.errcode("/wxa/queryquota", 45009, "api freq out of limit")
	f.seed()
	fallback, err := f.audit.Quota(f.ctx, appid)
	if err != nil {
		t.Fatalf("查询失败时应回退缓存值而不是报错: %v", err)
	}
	if fallback.Rest == nil || *fallback.Rest != 7 {
		t.Fatalf("回退值应来自缓存：%+v", fallback)
	}
}

// TestCredentialsGate 验证凭据门禁：未注入微信能力时返回 ErrNotConfigured。
func TestCredentialsGate(t *testing.T) {
	f := newFixture(t)
	f.app("wx_audit_gate", "门禁用例小程序", model.PermissionSetDev)
	f.seed()
	// 清掉微信能力，模拟「凭据未配置」（此后不能再调用 seed()：它会写令牌）。
	f.env.Wx = nil
	f.env.Tokens = nil

	if _, err := f.audit.Sync(f.ctx, appids("wx_audit_gate")); !errors.Is(err, core.ErrNotConfigured) {
		t.Fatalf("Sync 应返回 ErrNotConfigured，实际 %v", err)
	}
	if _, err := f.release.VersionInfo(f.ctx, "wx_audit_gate"); !errors.Is(err, core.ErrNotConfigured) {
		t.Fatalf("VersionInfo 应返回 ErrNotConfigured，实际 %v", err)
	}
	if _, err := f.preflight.Run(f.ctx, gen.PreflightRequest{
		Purpose:   gen.PreflightRequestPurposeRelease,
		Selection: appids("wx_audit_gate"),
	}); !errors.Is(err, core.ErrNotConfigured) {
		t.Fatalf("Preflight.Run 应返回 ErrNotConfigured，实际 %v", err)
	}
	// 只读本地台账不依赖微信凭据。
	if _, err := f.audit.List(f.ctx, gen.ListAuditsParams{}); err != nil {
		t.Fatalf("List 是本地查询，不应受凭据影响: %v", err)
	}
}

// strPtr 返回字符串指针。
func strPtr(v string) *string { return &v }

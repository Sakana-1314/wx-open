package wxjob

import (
	"strings"
	"testing"
	"time"

	"wx-platform/server/internal/batch"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

// commitPayload 构造 commit 作业的载荷（含模板与版本号模板）。
func commitPayload(templateID int64) model.JSONMap {
	return model.JSONMap{
		payloadKeyCommit: map[string]any{
			"template_id":          templateID,
			"ext_template":         "",
			"ext_overrides":        map[string]any{},
			"user_version_pattern": "{{date}}-{{seq}}",
			"user_desc_pattern":    "自动上传",
		},
	}
}

// auditPayload 在 commit 载荷上补提审配置。
func auditPayload(templateID int64, profileID uint) model.JSONMap {
	payload := commitPayload(templateID)
	payload[payloadKeyAudit] = map[string]any{
		"profile_id":           profileID,
		"version_desc_pattern": "",
		"privacy_api_not_use":  nil,
		"order_path":           "",
	}
	return payload
}

// TestCommitExecutorSuccessAndRateLimit commit 执行器：成功留档 + 9402202 分类为 rate_limited。
func TestCommitExecutorSuccessAndRateLimit(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	tpl := app.seedTemplate(6001, 0)
	job, items := app.seedJob(model.JobTypeCommit, appid, commitPayload(tpl.TemplateID))
	item := items[model.StepCommit]
	exec := NewCommitExecutor(app.svc)

	stub.json(endpointCommit, `{"errcode":9402202,"errmsg":"concurrent limit"}`)
	res := exec.Execute(app.ctx(), job, item)
	if res.Outcome != batch.OutcomeFailed {
		t.Fatalf("9402202 应为失败，实际 %s", res.Outcome)
	}
	if res.Class != model.ClassRateLimited {
		t.Fatalf("9402202 应分类为 rate_limited（不要重试），实际 %s", res.Class)
	}
	containsAll(t, res.Note, "串行")

	stub.json(endpointCommit, `{"errcode":0,"errmsg":"ok"}`)
	res = exec.Execute(app.ctx(), job, item)
	if res.Outcome != batch.OutcomeSucceeded {
		t.Fatalf("上传成功应返回 succeeded，实际 %s（%s）", res.Outcome, res.Note)
	}
	if res.UserVersion == "" || !strings.HasPrefix(res.UserVersion, "20") {
		t.Fatalf("应回填渲染后的 user_version，实际 %q", res.UserVersion)
	}
	if jsonInt64Of(res.Request["template_id"]) != tpl.TemplateID {
		t.Fatalf("请求留档应含 template_id，实际 %#v", res.Request)
	}
	if _, ok := res.Request["ext_json"].(string); !ok {
		t.Fatalf("请求留档应含字符串化的 ext_json，实际 %#v", res.Request)
	}
	// 上传成功绝不能顺带提审（官方 61039）。
	if stub.callsTo(endpointSubmitAudit) != 0 {
		t.Fatalf("上传代码成功后不得顺带提审")
	}
	// 真实请求体与预览一致。
	body := stub.bodyTo(endpointCommit)
	if jsonInt64Of(body["template_id"]) != tpl.TemplateID || body["ext_json"] == nil {
		t.Fatalf("发给微信的请求体不符：%#v", body)
	}
}

// TestCommitExecutorPermanentCode commit 永久失败码（9402203）给中文原因且不重试。
func TestCommitExecutorPermanentCode(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	tpl := app.seedTemplate(6002, 0)
	job, items := app.seedJob(model.JobTypeCommit, appid, commitPayload(tpl.TemplateID))

	stub.json(endpointCommit, `{"errcode":9402203,"errmsg":"standard template error"}`)
	res := NewCommitExecutor(app.svc).Execute(app.ctx(), job, items[model.StepCommit])
	if res.Class != model.ClassPermanent {
		t.Fatalf("9402203 应分类为 permanent，实际 %s", res.Class)
	}
	containsAll(t, res.Note, "标准模板", "9402203")
}

// TestPrivacyCheckWaitingThenTimeout 隐私检测：61039 → waiting 并累计等待，超上限 → failed。
func TestPrivacyCheckWaitingThenTimeout(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	app.setSetting(model.SettingPrivacyCheckMaxWait, "45")

	job, items := app.seedJob(model.JobTypeSubmitAudit, appid, commitPayload(0))
	item := items[model.StepPrivacyCheck]
	exec := NewPrivacyCheckExecutor(app.svc)
	stub.json(endpointPrivacyCheck, `{"errcode":61039,"errmsg":"privacy check not finish"}`)

	res := exec.Execute(app.ctx(), job, item)
	if res.Outcome != batch.OutcomeWaiting {
		t.Fatalf("61039 应为 waiting，实际 %s（%s）", res.Outcome, res.Note)
	}
	if res.RetryAfter != 30*time.Second {
		t.Fatalf("等待间隔应为 30 秒切片，实际 %s", res.RetryAfter)
	}
	if jsonIntOf(res.Response[keyPrivacyWaitSeconds]) != 30 {
		t.Fatalf("返回值应带累计等待秒数，实际 %#v", res.Response)
	}
	// 累计等待必须写回子项（否则进程重启 / 重新领取后计数会丢）。
	stored := app.reloadItem(item.ID)
	if jsonIntOf(stored.Response[keyPrivacyWaitSeconds]) != 30 {
		t.Fatalf("累计等待应写回 batch_job_items.response，实际 %#v", stored.Response)
	}

	// 第二轮：带上累计值再次执行，超过 45 秒上限 → 永久失败并给出 61039 说明。
	item.Response = res.Response
	res = exec.Execute(app.ctx(), job, item)
	if res.Outcome != batch.OutcomeFailed {
		t.Fatalf("超过等待上限应失败，实际 %s", res.Outcome)
	}
	if res.Class != model.ClassPermanent {
		t.Fatalf("超时失败不应无限重试，实际分类 %s", res.Class)
	}
	containsAll(t, res.Note, "61039", "隐私检测任务长时间未结束")
}

// TestPrivacyCheckWithoutAuthList 返回 without_auth_list / without_conf_list → 永久失败（61040）。
func TestPrivacyCheckWithoutAuthList(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	job, items := app.seedJob(model.JobTypeSubmitAudit, appid, commitPayload(0))

	stub.json(endpointPrivacyCheck, `{"errcode":0,"errmsg":"ok","without_auth_list":["getLocation"],"without_conf_list":["chooseAddress"]}`)
	res := NewPrivacyCheckExecutor(app.svc).Execute(app.ctx(), job, items[model.StepPrivacyCheck])
	if res.Outcome != batch.OutcomeFailed || res.Class != model.ClassPermanent {
		t.Fatalf("隐私接口未授权/未配置应永久失败，实际 %s / %s", res.Outcome, res.Class)
	}
	containsAll(t, res.Note, "61040", "getLocation", "chooseAddress", "privacy_api_not_use")
}

// TestSubmitAuditRequiresCommit 提审前置预检：三条条件（本作业内成功 commit / 30 天内历史成功 commit /
// direct_commit 来源）满足任一即放行；都不满足才失败，且文案必须说明是平台侧预检。
func TestSubmitAuditRequiresCommit(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	profile := app.seedProfile(model.JSONMap{"item_list": []any{validAuditItem()}})
	tpl := app.seedTemplate(7002, 0)
	stub.json("/wxa/get_category", categoryStubBody)
	stub.json(endpointSubmitAudit, `{"errcode":0,"errmsg":"ok","auditid":88990011}`)

	job, items := app.seedJob(model.JobTypeSubmitAudit, appid, auditPayload(tpl.TemplateID, profile.ID))
	item := items[model.StepSubmitAudit]
	exec := NewSubmitAuditExecutor(app.svc)

	// ---- 1) 任何地方都没有成功上传记录 → 平台侧预检失败（不调用微信）----
	res := exec.Execute(app.ctx(), job, item)
	if res.Outcome != batch.OutcomeFailed || res.Class != model.ClassPermanent || res.Errcode != 85086 {
		t.Fatalf("缺少上传记录应 85086 永久失败，实际 outcome=%s class=%s errcode=%d", res.Outcome, res.Class, res.Errcode)
	}
	containsAll(t, res.Errmsg, "本平台未找到该小程序成功的「上传代码」记录", "directCommit", "平台侧预检")
	if stub.callsTo(endpointSubmitAudit) != 0 {
		t.Fatalf("前置不满足时不得调用提审接口")
	}

	// ---- 2) 历史作业里有一条 40 天前的成功 commit → 超出 30 天窗口，不放行 ----
	oldJob, oldItems := app.seedJob(model.JobTypeCommit, appid, nil)
	if err := app.env.Repos.JobItems.UpdateFields(app.ctx(), oldItems[model.StepCommit].ID, map[string]any{
		"status":      model.ItemStatusSucceeded,
		"finished_at": time.Now().Add(-40 * 24 * time.Hour),
	}); err != nil {
		t.Fatalf("预置历史 commit 失败: %v", err)
	}
	_ = oldJob
	res = exec.Execute(app.ctx(), job, item)
	if res.Outcome != batch.OutcomeFailed || res.Errcode != 85086 {
		t.Fatalf("40 天前的上传记录不应放行，实际 outcome=%s errcode=%d", res.Outcome, res.Errcode)
	}

	// ---- 3) 历史作业里有一条最近的成功 commit（跨作业：先上传、再单独提审）→ 放行 ----
	if err := app.env.Repos.JobItems.UpdateFields(app.ctx(), oldItems[model.StepCommit].ID, map[string]any{
		"status":      model.ItemStatusSucceeded,
		"finished_at": time.Now().Add(-time.Hour),
		"attempt":     1,
	}); err != nil {
		t.Fatalf("更新历史 commit 失败: %v", err)
	}
	res = exec.Execute(app.ctx(), job, item)
	if res.Outcome != batch.OutcomeSucceeded {
		t.Fatalf("跨作业存在成功上传记录时应放行提审，实际 %s（%s）", res.Outcome, res.Note)
	}
	if res.WxAuditID != 88990011 {
		t.Fatalf("应回填审核单号，实际 %d", res.WxAuditID)
	}
	if stub.callsTo(endpointSubmitAudit) != 1 {
		t.Fatalf("应调用一次提审接口，实际 %d 次", stub.callsTo(endpointSubmitAudit))
	}
	// 提审成功必须写审核台账（供 GET /audits?appid= 查到，status=2 审核中、Source=api）。
	list, total, err := app.env.Repos.Audits.List(app.ctx(), appid, nil, 1, 10)
	if err != nil {
		t.Fatalf("读取审核台账失败: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("应恰好 1 条审核台账，实际 total=%d %+v", total, list)
	}
	if list[0].AuditID != 88990011 || list[0].Status != repo.AuditStatusAuditing || list[0].Source != model.AuditSourceAPI {
		t.Fatalf("审核台账不符：%+v", list[0])
	}
	if list[0].SubmitTime == nil {
		t.Fatalf("审核台账应记录 SubmitTime")
	}

	// ---- 4) 代码由 CI 直传（direct_commit）→ 跳过上传记录要求 ----
	ciAppid := appidOf(t.Name() + "ci")
	app.seedAuthorizer(ciAppid, func(a *model.Authorizer) { a.CodeSource = model.CodeSourceDirectCommit })
	app.seedToken(ciAppid)
	ciJob, ciItems := app.seedJob(model.JobTypeSubmitAudit, ciAppid, auditPayload(tpl.TemplateID, profile.ID))
	res = exec.Execute(app.ctx(), ciJob, ciItems[model.StepSubmitAudit])
	if res.Outcome != batch.OutcomeSucceeded {
		t.Fatalf("direct_commit 来源应可跳过上传前置，实际 %s（%s）", res.Outcome, res.Note)
	}
}

// TestSubmitAuditIgnoresDryRunCommit 试运行（dry_run）的上传作业从未真的上传过代码，
// 即使子项是 succeeded + 有 finished_at，也不能当作可提审的版本。
func TestSubmitAuditIgnoresDryRunCommit(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	tpl := app.seedTemplate(7003, 0)
	profile := app.seedProfile(model.JSONMap{"item_list": []any{validAuditItem()}})
	stub.json("/wxa/get_category", categoryStubBody)
	stub.json(endpointSubmitAudit, `{"errcode":0,"errmsg":"ok","auditid":1}`)

	dryJob := &model.BatchJob{
		ID: "job-dry-run-0001", Type: model.JobTypeCommit, Status: model.JobStatusSucceeded,
		DryRun: true, Total: 1,
	}
	now := time.Now()
	dryItems := []model.BatchJobItem{{
		JobID: dryJob.ID, Step: model.StepCommit, Appid: appid,
		Status: model.ItemStatusSucceeded, FinishedAt: &now,
		Response: model.JSONMap{"dry_run": true},
	}}
	if err := app.env.Repos.Jobs.Create(app.ctx(), dryJob, dryItems); err != nil {
		t.Fatalf("预置试运行作业失败: %v", err)
	}

	job, items := app.seedJob(model.JobTypeSubmitAudit, appid, auditPayload(tpl.TemplateID, profile.ID))
	res := NewSubmitAuditExecutor(app.svc).Execute(app.ctx(), job, items[model.StepSubmitAudit])
	if res.Outcome != batch.OutcomeFailed || res.Errcode != 85086 {
		t.Fatalf("试运行记录不应放行提审，实际 outcome=%s errcode=%d", res.Outcome, res.Errcode)
	}
	if stub.callsTo(endpointSubmitAudit) != 0 {
		t.Fatalf("前置不满足时不得调用提审接口")
	}
}

// TestSubmitAuditQuotaExhausted 额度为 0 时提审执行器返回 Fatal=true 且信息含 85085。
func TestSubmitAuditQuotaExhausted(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	tpl := app.seedTemplate(7001, 0)
	profile := app.seedProfile(model.JSONMap{"item_list": []any{validAuditItem()}})
	stub.json("/wxa/get_category", categoryStubBody)
	stub.json(endpointSubmitAudit, `{"errcode":0,"errmsg":"ok","auditid":1}`)

	// 额度缓存：剩余 0（服务商级、旗下小程序共用）。
	app.setSetting("audit_quota_rest", "0")
	app.setSetting("audit_quota_limit", "10")
	app.setSetting("audit_quota_queried_at", "1700000000")

	job, items := app.seedJob(model.JobTypePipeline, appid, auditPayload(tpl.TemplateID, profile.ID))
	if err := app.env.Repos.JobItems.UpdateFields(app.ctx(), items[model.StepCommit].ID,
		map[string]any{"status": model.ItemStatusSucceeded, "user_version": "2024-01-01-1"}); err != nil {
		t.Fatalf("预置成功 commit 子项失败: %v", err)
	}
	_, reloaded := app.loadJob(job.ID)

	res := NewSubmitAuditExecutor(app.svc).Execute(app.ctx(), job, reloaded[model.StepSubmitAudit])
	if !res.Fatal {
		t.Fatalf("额度耗尽必须返回 Fatal=true 暂停整个作业，实际 %+v", res)
	}
	containsAll(t, res.FatalNote+res.Note, "85085", "额度")
	if res.Outcome != batch.OutcomeFailed {
		t.Fatalf("额度耗尽应为失败，实际 %s", res.Outcome)
	}
	if stub.callsTo(endpointSubmitAudit) != 0 {
		t.Fatalf("本地已判定额度耗尽时不应再调用提审接口")
	}
}

// TestSubmitAuditCategoryMissing 类目未在小程序上配置好 → 85008 永久失败（提审前拦截）。
func TestSubmitAuditCategoryMissing(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	profile := app.seedProfile(model.JSONMap{"item_list": []any{validAuditItem()}})
	// 小程序上只配置了别的类目。
	stub.json("/wxa/get_category", `{"errcode":0,"errmsg":"ok","category_list":[{"first_class":"工具","second_class":"效率","first_id":900,"second_id":901}]}`)

	job, items := app.seedJob(model.JobTypeSubmitAudit, appid, auditPayload(0, profile.ID))
	if err := app.env.Repos.Authorizers.UpdateFields(app.ctx(), appid, map[string]any{"code_source": model.CodeSourceDirectCommit}); err != nil {
		t.Fatalf("更新代码来源失败: %v", err)
	}
	item := items[model.StepSubmitAudit]

	res := NewSubmitAuditExecutor(app.svc).Execute(app.ctx(), job, item)
	if res.Outcome != batch.OutcomeFailed || res.Errcode != 85008 || res.Class != model.ClassPermanent {
		t.Fatalf("类目缺失应 85008 永久失败，实际 errcode=%d class=%s outcome=%s", res.Errcode, res.Class, res.Outcome)
	}
	containsAll(t, res.Note, "类目", "85008")
	if stub.callsTo(endpointSubmitAudit) != 0 {
		t.Fatalf("类目校验不通过时不得调用提审接口")
	}
}

// TestReleaseWaitsForAuditPass 审核未通过（或还没有审核记录）时发布返回 waiting。
func TestReleaseWaitsForAuditPass(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	job, items := app.seedJob(model.JobTypeRelease, appid, nil)
	item := items[model.StepRelease]
	exec := NewReleaseExecutor(app.svc)

	res := exec.Execute(app.ctx(), job, item)
	if res.Outcome != batch.OutcomeWaiting {
		t.Fatalf("尚无审核通过记录时应等待，实际 %s（%s）", res.Outcome, res.Note)
	}
	if res.RetryAfter != 60*time.Second {
		t.Fatalf("发布等待间隔应为 60 秒，实际 %s", res.RetryAfter)
	}
	containsAll(t, res.Note, "等待审核通过")
	if stub.callsTo(endpointRelease) != 0 {
		t.Fatalf("审核未通过时不得调用发布接口")
	}

	// 审核被拒 + 已超过等待上限 → 失败并提示去审核管理页查看拒绝原因。
	now := time.Now()
	if err := app.env.Repos.Audits.Upsert(app.ctx(), &model.AuditRecord{
		Appid:      appid,
		AuditID:    11223344,
		Status:     1,
		Reason:     "1:账号信息不符合规范",
		SubmitTime: &now,
		Source:     model.AuditSourceAPI,
	}); err != nil {
		t.Fatalf("写入审核台账失败: %v", err)
	}
	app.setSetting(model.SettingAuditResultMaxWait, "60")
	item.Response = res.Response
	res = exec.Execute(app.ctx(), job, item)
	if res.Outcome != batch.OutcomeFailed || res.Class != model.ClassPermanent {
		t.Fatalf("超过等待上限应永久失败，实际 %s / %s", res.Outcome, res.Class)
	}
	containsAll(t, res.Note, "审核管理", "账号信息不符合规范")
}

// TestReleaseSucceedsAfterAuditPassed 审核通过后发布成功并写发布台账。
func TestReleaseSucceedsAfterAuditPassed(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	stub.json(endpointRelease, `{"errcode":0,"errmsg":"ok"}`)

	now := time.Now()
	if err := app.env.Repos.Audits.Upsert(app.ctx(), &model.AuditRecord{
		Appid:       appid,
		AuditID:     55667788,
		UserVersion: "2024-01-01-1",
		Status:      0,
		SubmitTime:  &now,
		Source:      model.AuditSourceAPI,
	}); err != nil {
		t.Fatalf("写入审核台账失败: %v", err)
	}

	job, items := app.seedJob(model.JobTypeRelease, appid, nil)
	res := NewReleaseExecutor(app.svc).Execute(app.ctx(), job, items[model.StepRelease])
	if res.Outcome != batch.OutcomeSucceeded {
		t.Fatalf("审核通过后应发布成功，实际 %s（%s）", res.Outcome, res.Note)
	}
	list, _, err := app.env.Repos.Releases.List(app.ctx(), appid, 1, 10)
	if err != nil {
		t.Fatalf("读取发布台账失败: %v", err)
	}
	if len(list) != 1 || list[0].Action != model.ReleaseActionRelease {
		t.Fatalf("应写入 release 发布记录，实际 %+v", list)
	}
}

// TestReleaseFailureCodes 85019/85020 永久失败并给出中文解释。
func TestReleaseFailureCodes(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	now := time.Now()
	if err := app.env.Repos.Audits.Upsert(app.ctx(), &model.AuditRecord{
		Appid: appid, AuditID: 99887766, Status: 0, SubmitTime: &now, Source: model.AuditSourceAPI,
	}); err != nil {
		t.Fatalf("写入审核台账失败: %v", err)
	}
	stub.json(endpointRelease, `{"errcode":85019,"errmsg":"no audit version"}`)

	job, items := app.seedJob(model.JobTypeRelease, appid, nil)
	res := NewReleaseExecutor(app.svc).Execute(app.ctx(), job, items[model.StepRelease])
	if res.Outcome != batch.OutcomeFailed || res.Class != model.ClassPermanent {
		t.Fatalf("85019 应永久失败，实际 %s / %s", res.Outcome, res.Class)
	}
	containsAll(t, res.Note, "85019", "没有审核版本")
}

// TestSingleStepExecutorDispatcher 单步执行器：未支持的类型返回 skipped，toggle_visit 正常执行。
func TestSingleStepExecutorDispatcher(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	exec := NewSingleStepExecutor(app.svc)

	// set_domain 等类型交给专用页面。
	job, items := app.seedJob(model.JobTypeSetDomain, appid, nil)
	res := exec.Execute(app.ctx(), job, items[model.StepSingle])
	if res.Outcome != batch.OutcomeSkipped {
		t.Fatalf("set_domain 应返回 skipped，实际 %s", res.Outcome)
	}
	containsAll(t, res.Note, "专用页面")

	// toggle_visit：服务状态取契约字段 pauseService（落库为载荷 pause_service）。
	stub.json(endpointVisitStatus, `{"errcode":0,"errmsg":"ok"}`)
	job, items = app.seedJob(model.JobTypeToggleVisit, appid, model.JSONMap{payloadKeyPauseService: true})
	res = exec.Execute(app.ctx(), job, items[model.StepSingle])
	if res.Outcome != batch.OutcomeSucceeded {
		t.Fatalf("toggle_visit 应成功，实际 %s（%s）", res.Outcome, res.Note)
	}
	if action, _ := stub.bodyTo(endpointVisitStatus)["action"].(string); action != "close" {
		t.Fatalf("pauseService=true 应传入 action=close，实际 %#v", stub.bodyTo(endpointVisitStatus))
	}

	// 未指定 pauseService → 永久失败并提示，不得猜默认值、不得调用微信。
	before := stub.callsTo(endpointVisitStatus)
	job, items = app.seedJob(model.JobTypeToggleVisit, appid, nil)
	res = exec.Execute(app.ctx(), job, items[model.StepSingle])
	if res.Outcome != batch.OutcomeFailed || res.Class != model.ClassPermanent {
		t.Fatalf("未指定服务状态应永久失败，实际 %s / %s", res.Outcome, res.Class)
	}
	containsAll(t, res.Note, "未指定服务状态（pauseService）")
	if stub.callsTo(endpointVisitStatus) != before {
		t.Fatalf("未指定服务状态时不得调用微信接口")
	}
}

// TestSingleStepSyncAuditStatus sync_audit_status 把最新审核状态写入台账。
func TestSingleStepSyncAuditStatus(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	stub.json(endpointAuditStatus, `{"errcode":0,"errmsg":"ok","auditid":66778899,"status":2,"user_version":"1.0.1","reason":"","submit_audit_time":1700000000}`)

	job, items := app.seedJob(model.JobTypeSyncAuditStatus, appid, nil)
	res := NewSingleStepExecutor(app.svc).Execute(app.ctx(), job, items[model.StepSingle])
	if res.Outcome != batch.OutcomeSucceeded {
		t.Fatalf("同步审核状态应成功，实际 %s（%s）", res.Outcome, res.Note)
	}
	rec, err := app.env.Repos.Audits.LatestByApp(app.ctx(), appid)
	if err != nil {
		t.Fatalf("读取审核台账失败: %v", err)
	}
	if rec.AuditID != 66778899 || rec.Status != 2 || rec.Source != model.AuditSourcePoll {
		t.Fatalf("审核台账不符：%+v", rec)
	}
}

// TestWeChatErrorMapping *wxapi.APIError → core.WeChatError（保留 errcode 与分类）。
func TestWeChatErrorMapping(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	tpl := app.seedTemplate(6003, 0)
	job, items := app.seedJob(model.JobTypeCommit, appid, commitPayload(tpl.TemplateID))
	stub.json(endpointCommit, `{"errcode":45009,"errmsg":"api freq out of limit"}`)

	res := NewCommitExecutor(app.svc).Execute(app.ctx(), job, items[model.StepCommit])
	if res.Errcode != 45009 || res.Class != model.ClassRateLimited {
		t.Fatalf("应保留 errcode 与分类，实际 errcode=%d class=%s", res.Errcode, res.Class)
	}
	containsAll(t, res.Note, "天级别频率限制")
}

// TestFailureClassificationForMissingAuthorizer 小程序没有授权记录 → 永久失败（不重试）。
func TestFailureClassificationForMissingAuthorizer(t *testing.T) {
	app := newTestApp(t)
	app.withStub()
	job, items := app.seedJob(model.JobTypeCommit, "wxnotauthorized0001", commitPayload(1))

	res := NewCommitExecutor(app.svc).Execute(app.ctx(), job, items[model.StepCommit])
	if res.Outcome != batch.OutcomeFailed || res.Class != model.ClassPermanent {
		t.Fatalf("缺少授权记录应永久失败，实际 %s / %s", res.Outcome, res.Class)
	}
	containsAll(t, res.Note, "授权")
}

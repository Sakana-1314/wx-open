package wxjob

import (
	"context"
	"fmt"
	"testing"
	"time"

	"wx-platform/server/internal/batch"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

// lockNameForTest 每次调用都生成唯一锁名：MySQL GET_LOCK 的锁在会话存续期间一直有效，
// 同一进程内重复跑同一个用例（go test -count=N）时会与上一个会话的锁冲突。
func lockNameForTest(appid string) string {
	return fmt.Sprintf("wxjob_e2e_%s_%d", appid, time.Now().UnixNano())
}

// TestEngineEndToEndCommitJob 让真实的 batch.Engine 通过 EngineStore + CommitExecutor 跑完一个作业：
// 覆盖「创建 → 启动 → 领取 → 执行 → 回写 → 汇总落终态」的整条链路（假微信服务，不依赖真实微信）。
func TestEngineEndToEndCommitJob(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	tpl := app.seedTemplate(8001, 0)
	stub.json(endpointCommit, `{"errcode":0,"errmsg":"ok"}`)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	engine := batch.New(NewEngineStore(app.env), batch.Options{
		Concurrency:  func() int { return 1 },
		MaxAttempts:  func() int { return 2 },
		PollInterval: 30 * time.Millisecond,
		LockName:     lockNameForTest(appid),
		Logger:       func(format string, args ...any) { t.Logf("[engine] "+format, args...) },
	})
	// 与 service.Container.AttachEngine 一致：注册全部执行器后再启动引擎。
	engine.Register(NewCommitExecutor(app.svc))
	engine.Register(NewPrivacyCheckExecutor(app.svc))
	engine.Register(NewSubmitAuditExecutor(app.svc))
	engine.Register(NewReleaseExecutor(app.svc))
	engine.Register(NewSingleStepExecutor(app.svc))
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("引擎启动失败: %v", err)
	}
	app.svc.AttachEngine(engine)

	job, err := app.svc.Create(ctx, gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}-{{seq}}")},
	}, "tester")
	if err != nil {
		t.Fatalf("创建作业失败: %v", err)
	}
	if _, err := app.svc.Start(ctx, job.Id); err != nil {
		t.Fatalf("启动作业失败: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	var final *gen.Job
	for time.Now().Before(deadline) {
		got, gerr := app.svc.Get(ctx, job.Id)
		if gerr != nil {
			t.Fatalf("读取作业失败: %v", gerr)
		}
		if model.JobStatus(got.Status).Terminal() {
			final = got
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if final == nil {
		t.Fatalf("作业未在超时时间内结束")
	}
	if final.Status != gen.JobStatus(model.JobStatusSucceeded) {
		t.Fatalf("作业应执行成功，实际 %s（errorNote=%v）", final.Status, final.ErrorNote)
	}
	if final.Succeeded != 1 || final.Failed != 0 || final.Skipped != 0 {
		t.Fatalf("计数不符：succeeded=%d failed=%d skipped=%d", final.Succeeded, final.Failed, final.Skipped)
	}
	if stub.callsTo(endpointCommit) != 1 {
		t.Fatalf("应恰好调用一次上传代码接口，实际 %d 次", stub.callsTo(endpointCommit))
	}

	items, err := app.env.Repos.JobItems.AllByJob(ctx, job.Id)
	if err != nil {
		t.Fatalf("读取子项失败: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("应有 1 个子项，实际 %d", len(items))
	}
	item := items[0]
	if item.Status != model.ItemStatusSucceeded {
		t.Fatalf("子项应成功，实际 %s（errmsg=%s）", item.Status, item.Errmsg)
	}
	if item.UserVersion == "" {
		t.Fatalf("成功后应回填 user_version")
	}
	if item.Attempt != 1 {
		t.Fatalf("应只尝试一次，实际 %d", item.Attempt)
	}
	if item.Request == nil || item.Request["ext_json"] == nil {
		t.Fatalf("应留档请求体，实际 %#v", item.Request)
	}
}

// TestEngineEndToEndPipeline 全流程（pipeline）作业：commit → privacy_check → submit_audit → release。
//
// 同时验证三件事：
//   - 步骤门控：微信接口的调用顺序必须是 上传代码 → 隐私检测 → 提审 → 发布（否则会触发官方 61039）；
//   - waiting 的落地：审核中的版本会让 release 进入 waiting，等审核通过（这里由测试模拟审核结果回调）后自动继续；
//   - 台账：提审写 audit_records、发布写 release_records。
func TestEngineEndToEndPipeline(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	tpl := app.seedTemplate(8002, 0)
	profile := app.seedProfile(model.JSONMap{"item_list": []any{validAuditItem()}})

	stub.json(endpointCommit, `{"errcode":0,"errmsg":"ok"}`)
	stub.json(endpointPrivacyCheck, `{"errcode":0,"errmsg":"ok"}`)
	stub.json("/wxa/get_category", categoryStubBody)
	stub.json(endpointSubmitAudit, `{"errcode":0,"errmsg":"ok","auditid":123456}`)
	stub.json(endpointRelease, `{"errcode":0,"errmsg":"ok"}`)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	engine := batch.New(NewEngineStore(app.env), batch.Options{
		Concurrency:  func() int { return 2 },
		MaxAttempts:  func() int { return 2 },
		PollInterval: 30 * time.Millisecond,
		LockName:     lockNameForTest(appid),
		Logger:       func(format string, args ...any) { t.Logf("[engine] "+format, args...) },
	})
	engine.Register(NewCommitExecutor(app.svc))
	engine.Register(NewPrivacyCheckExecutor(app.svc))
	engine.Register(NewSubmitAuditExecutor(app.svc))
	engine.Register(NewReleaseExecutor(app.svc))
	engine.Register(NewSingleStepExecutor(app.svc))
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("引擎启动失败: %v", err)
	}
	app.svc.AttachEngine(engine)

	job, err := app.svc.Create(ctx, gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypePipeline),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}-{{seq}}")},
		Audit:     &gen.AuditOptions{AuditProfileId: int64Ptr(int64(profile.ID))},
	}, "tester")
	if err != nil {
		t.Fatalf("创建作业失败: %v", err)
	}
	if jointed := job.Total; jointed != 4 {
		t.Fatalf("pipeline 应有 4 个子项，实际 %d", jointed)
	}
	if _, err := app.svc.Start(ctx, job.Id); err != nil {
		t.Fatalf("启动作业失败: %v", err)
	}

	// 等待作业结束，同时模拟「微信审核结果回调」：审核中 → 审核通过，并让等待项立即到期。
	deadline := time.Now().Add(20 * time.Second)
	var final *gen.Job
	for time.Now().Before(deadline) {
		items, ierr := app.env.Repos.JobItems.AllByJob(ctx, job.Id)
		if ierr != nil {
			t.Fatalf("读取子项失败: %v", ierr)
		}
		approveAudit := false
		for i := range items {
			if items[i].Step == model.StepSubmitAudit && items[i].Status == model.ItemStatusSucceeded {
				approveAudit = true
			}
			if items[i].Step == model.StepRelease && items[i].Status == model.ItemStatusWaiting {
				// 模拟等待到期（审核结果推送到达）。
				if uerr := app.env.Repos.JobItems.UpdateFields(ctx, items[i].ID,
					map[string]any{"next_run_at": time.Now().Add(-time.Second)}); uerr != nil {
					t.Fatalf("推进等待项失败: %v", uerr)
				}
			}
		}
		if approveAudit {
			rec, rerr := app.env.Repos.Audits.LatestByApp(ctx, appid)
			if rerr == nil && rec.Status == repo.AuditStatusAuditing {
				now := time.Now()
				rec.Status = 0
				rec.StatusTime = &now
				if uerr := app.env.Repos.Audits.Upsert(ctx, rec); uerr != nil {
					t.Fatalf("模拟审核通过失败: %v", uerr)
				}
			}
		}

		got, gerr := app.svc.Get(ctx, job.Id)
		if gerr != nil {
			t.Fatalf("读取作业失败: %v", gerr)
		}
		if model.JobStatus(got.Status).Terminal() {
			final = got
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if final == nil {
		t.Fatalf("作业未在超时时间内结束")
	}
	if final.Status != gen.JobStatus(model.JobStatusSucceeded) {
		t.Fatalf("全流程作业应成功，实际 %s（errorNote=%v）", final.Status, final.ErrorNote)
	}
	if final.Succeeded != 4 || final.Failed != 0 {
		t.Fatalf("应 4 步全部成功，实际 succeeded=%d failed=%d skipped=%d", final.Succeeded, final.Failed, final.Skipped)
	}

	// 调用顺序必须是 commit → privacy_check → submit_audit → release。
	order := []string{endpointCommit, endpointPrivacyCheck, endpointSubmitAudit, endpointRelease}
	last := -1
	for _, path := range order {
		idx := stub.firstCallIndex(path)
		if idx < 0 {
			t.Fatalf("未调用 %s（实际调用：%v）", path, stub.calls)
		}
		if idx < last {
			t.Fatalf("步骤顺序错误：%s 在更早的步骤之前被调用（实际顺序 %v）", path, stub.calls)
		}
		last = idx
	}

	// 台账落库。
	rec, err := app.env.Repos.Audits.LatestByApp(ctx, appid)
	if err != nil {
		t.Fatalf("读取审核台账失败: %v", err)
	}
	if rec.AuditID != 123456 || rec.UserVersion == "" {
		t.Fatalf("审核台账不符：%+v", rec)
	}
	releases, _, err := app.env.Repos.Releases.List(ctx, appid, 1, 10)
	if err != nil {
		t.Fatalf("读取发布台账失败: %v", err)
	}
	if len(releases) != 1 || releases[0].Action != model.ReleaseActionRelease {
		t.Fatalf("应写入发布台账，实际 %+v", releases)
	}
	if releases[0].UserVersion == "" {
		t.Fatalf("发布台账应带版本号：%+v", releases[0])
	}
	if final.FinishedAt == nil {
		t.Fatalf("终态作业应记录 finishedAt")
	}
}

// TestEngineEndToEndCommitThenSeparateAudit 回归用例（对应端到端暴露的缺陷）：
// 先跑一个**上传代码作业**，成功后再单独创建一个**提审作业**，提审必须能真正打到微信
// （跨作业的历史成功 commit 记录要能放行平台侧前置预检）。
func TestEngineEndToEndCommitThenSeparateAudit(t *testing.T) {
	app := newTestApp(t)
	stub := app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	app.seedToken(appid)
	tpl := app.seedTemplate(8003, 0)
	profile := app.seedProfile(model.JSONMap{"item_list": []any{validAuditItem()}})

	stub.json(endpointCommit, `{"errcode":0,"errmsg":"ok"}`)
	stub.json(endpointPrivacyCheck, `{"errcode":0,"errmsg":"ok"}`)
	stub.json("/wxa/get_category", categoryStubBody)
	stub.json(endpointSubmitAudit, `{"errcode":0,"errmsg":"ok","auditid":555001}`)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	engine := batch.New(NewEngineStore(app.env), batch.Options{
		Concurrency:  func() int { return 2 },
		MaxAttempts:  func() int { return 2 },
		PollInterval: 30 * time.Millisecond,
		LockName:     lockNameForTest(appid),
		Logger:       func(format string, args ...any) { t.Logf("[engine] "+format, args...) },
	})
	engine.Register(NewCommitExecutor(app.svc))
	engine.Register(NewPrivacyCheckExecutor(app.svc))
	engine.Register(NewSubmitAuditExecutor(app.svc))
	engine.Register(NewReleaseExecutor(app.svc))
	engine.Register(NewSingleStepExecutor(app.svc))
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("引擎启动失败: %v", err)
	}
	app.svc.AttachEngine(engine)

	// ① 上传代码作业。
	commitJob, err := app.svc.Create(ctx, gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}-{{seq}}")},
	}, "tester")
	if err != nil {
		t.Fatalf("创建上传代码作业失败: %v", err)
	}
	if _, err := app.svc.Start(ctx, commitJob.Id); err != nil {
		t.Fatalf("启动上传代码作业失败: %v", err)
	}
	waitJobTerminal(t, app, commitJob.Id, 15*time.Second)
	final := jobStatus(t, app, commitJob.Id)
	if final.Status != gen.JobStatus(model.JobStatusSucceeded) {
		t.Fatalf("上传代码作业应成功，实际 %s（errorNote=%v）", final.Status, final.ErrorNote)
	}

	// ② 单独创建一个提审作业（跨作业用法）。
	auditJob, err := app.svc.Create(ctx, gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeSubmitAudit),
		Selection: appidSelection(appid),
		Audit:     &gen.AuditOptions{AuditProfileId: int64Ptr(int64(profile.ID))},
	}, "tester")
	if err != nil {
		t.Fatalf("创建提审作业失败: %v", err)
	}
	if _, err := app.svc.Start(ctx, auditJob.Id); err != nil {
		t.Fatalf("启动提审作业失败: %v", err)
	}
	waitJobTerminal(t, app, auditJob.Id, 15*time.Second)
	final = jobStatus(t, app, auditJob.Id)
	if final.Status != gen.JobStatus(model.JobStatusSucceeded) {
		items, _ := app.env.Repos.JobItems.AllByJob(ctx, auditJob.Id)
		t.Fatalf("提审作业应成功（跨作业上传记录应放行），实际 %s；子项=%+v", final.Status, items)
	}
	if stub.callsTo(endpointSubmitAudit) != 1 {
		t.Fatalf("提审必须真的调用微信接口一次，实际 %d 次（调用记录 %v）",
			stub.callsTo(endpointSubmitAudit), stub.calls)
	}

	// ③ 回填核对：auditid 既要落到子项（引擎按 Result.WxAuditID 写 wx_audit_id），也要落 audit_records。
	auditItems, err := app.env.Repos.JobItems.AllByJob(ctx, auditJob.Id)
	if err != nil {
		t.Fatalf("读取提审作业子项失败: %v", err)
	}
	seen := map[model.JobStep]model.BatchJobItem{}
	for i := range auditItems {
		seen[auditItems[i].Step] = auditItems[i]
	}
	if it, ok := seen[model.StepPrivacyCheck]; !ok || it.Status != model.ItemStatusSucceeded {
		t.Fatalf("隐私检测子项应成功，实际 %+v", it)
	}
	submitItem, ok := seen[model.StepSubmitAudit]
	if !ok {
		t.Fatalf("提审作业应有 submit_audit 子项，实际 %+v", auditItems)
	}
	if submitItem.Status != model.ItemStatusSucceeded || submitItem.WxAuditID != 555001 {
		t.Fatalf("提审子项应成功并回填 wx_audit_id=555001，实际 status=%s wxAuditId=%d",
			submitItem.Status, submitItem.WxAuditID)
	}

	// ④ 审核台账可查到该审核单且状态为「审核中」（GET /audits?appid= 的数据来源）。
	list, _, err := app.env.Repos.Audits.List(ctx, appid, nil, 1, 10)
	if err != nil {
		t.Fatalf("读取审核台账失败: %v", err)
	}
	if len(list) != 1 || list[0].AuditID != 555001 || list[0].Status != repo.AuditStatusAuditing {
		t.Fatalf("审核台账不符：%+v", list)
	}
	if list[0].UserVersion == "" {
		t.Fatalf("审核台账应带版本号（取自上传成功的 user_version）：%+v", list[0])
	}
}

// waitJobTerminal 轮询等待作业进入终态。
func waitJobTerminal(t *testing.T, app *testApp, jobID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if model.JobStatus(jobStatus(t, app, jobID).Status).Terminal() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("作业 %s 未在 %s 内结束", jobID, timeout)
}

// jobStatus 读取作业状态。
func jobStatus(t *testing.T, app *testApp, jobID string) *gen.Job {
	t.Helper()
	got, err := app.svc.Get(context.Background(), jobID)
	if err != nil {
		t.Fatalf("读取作业 %s 失败: %v", jobID, err)
	}
	return got
}

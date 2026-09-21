package wxjob

import (
	"errors"
	"testing"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// TestCreateMarksInvalidItemsSkipped 无效项必须落库为 skipped（写明原因），而不是被丢弃。
func TestCreateMarksInvalidItemsSkipped(t *testing.T) {
	app := newTestApp(t)
	good := appidOf(t.Name() + "good")
	bad := appidOf(t.Name() + "bad")
	app.seedAuthorizer(good)
	app.seedAuthorizer(bad, func(a *model.Authorizer) { a.FuncInfo = nil }) // 缺开发权限集 18
	tpl := app.seedTemplate(5001, 0)

	job, err := app.svc.Create(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(good, bad),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}-{{seq}}")},
		Note:      ptr("批量上传"),
	}, "tester")
	if err != nil {
		t.Fatalf("创建作业失败: %v", err)
	}
	if job.Total != 2 || job.Skipped != 1 {
		t.Fatalf("期望 total=2 skipped=1，实际 total=%d skipped=%d", job.Total, job.Skipped)
	}
	if job.Status != gen.JobStatus(model.JobStatusPending) {
		t.Fatalf("作业应处于 pending（等用户点启动），实际 %s", job.Status)
	}
	if job.CreatedBy == nil || *job.CreatedBy != "tester" {
		t.Fatalf("应记录创建人，实际 %v", job.CreatedBy)
	}

	items, err := app.env.Repos.JobItems.AllByJob(app.ctx(), job.Id)
	if err != nil {
		t.Fatalf("读取子项失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("期望 2 个子项，实际 %d", len(items))
	}
	var skipped, pending *model.BatchJobItem
	for i := range items {
		switch items[i].Appid {
		case bad:
			skipped = &items[i]
		case good:
			pending = &items[i]
		}
	}
	if skipped == nil || pending == nil {
		t.Fatalf("两个小程序的子项都应落库：%+v", items)
	}
	if skipped.Status != model.ItemStatusSkipped {
		t.Fatalf("无效项应落库为 skipped，实际 %s", skipped.Status)
	}
	containsAll(t, skipped.Errmsg, "权限集 18")
	if skipped.ErrorClass != model.ClassPermanent {
		t.Fatalf("无效项的分类应为 permanent，实际 %s", skipped.ErrorClass)
	}
	if pending.Status != model.ItemStatusPending {
		t.Fatalf("有效项应为 pending，实际 %s", pending.Status)
	}

	// 载荷必须保存足够复现的信息（从库里读回，验证 JSON 列往返后依然可解析）。
	stored, err := app.env.Repos.Jobs.Get(app.ctx(), job.Id)
	if err != nil {
		t.Fatalf("读取作业失败: %v", err)
	}
	commit, ok := stored.Payload[payloadKeyCommit].(map[string]any)
	if !ok || jsonInt64Of(commit["template_id"]) != tpl.TemplateID {
		t.Fatalf("载荷应保存 commit 参数，实际 %#v", stored.Payload[payloadKeyCommit])
	}
	if pattern, _ := commit["user_version_pattern"].(string); pattern != "{{date}}-{{seq}}" {
		t.Fatalf("载荷应保存版本号模板，实际 %q", pattern)
	}
	if appids := payloadAppids(stored.Payload); len(appids) != 2 {
		t.Fatalf("载荷应保存目标 appid 列表，实际 %#v", stored.Payload[payloadKeyAppids])
	}
	if stored.Payload[payloadKeySelection] == nil {
		t.Fatalf("载荷应保存选择条件，实际 %#v", stored.Payload)
	}
}

// TestCreateAllInvalidReturnsValidation 全部无效时返回 core.ErrValidation 并附第一条原因。
func TestCreateAllInvalidReturnsValidation(t *testing.T) {
	app := newTestApp(t)
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid, func(a *model.Authorizer) { a.FuncInfo = nil })
	tpl := app.seedTemplate(5002, 0)

	_, err := app.svc.Create(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}")},
	}, "tester")
	if !errors.Is(err, core.ErrValidation) {
		t.Fatalf("期望 core.ErrValidation，实际 %v", err)
	}
	containsAll(t, err.Error(), "权限集 18")

	// 未创建任何作业。
	jobs, total, lerr := app.env.Repos.Jobs.List(app.ctx(), "", "", 1, 10)
	if lerr != nil {
		t.Fatalf("查询作业失败: %v", lerr)
	}
	if total != 0 || len(jobs) != 0 {
		t.Fatalf("全部无效时不应落库作业，实际 %d 条", total)
	}
}

// TestCreateInFlightConflict 在途冲突：同 appid 已有未结束的 commit 子项时，新作业该项标记 skipped。
func TestCreateInFlightConflict(t *testing.T) {
	app := newTestApp(t)
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	tpl := app.seedTemplate(5003, 0)

	// 预置另一个在途作业的 commit 子项（模拟上一次上传还在执行）。
	prior := &model.BatchJob{ID: "job-prior-0001", Type: model.JobTypeCommit, Status: model.JobStatusRunning, Total: 1}
	priorItems := []model.BatchJobItem{{
		JobID:  prior.ID,
		Step:   model.StepCommit,
		Appid:  appid,
		Status: model.ItemStatusPending,
	}}
	if err := app.env.Repos.Jobs.Create(app.ctx(), prior, priorItems); err != nil {
		t.Fatalf("预置在途作业失败: %v", err)
	}
	_, priorByStep := app.loadJob(prior.ID)

	job, err := app.svc.Create(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}")},
	}, "tester")
	if err != nil {
		t.Fatalf("创建作业失败: %v", err)
	}
	if job.Skipped != 1 {
		t.Fatalf("在途冲突项应被标记 skipped，实际 skipped=%d", job.Skipped)
	}
	items, err := app.env.Repos.JobItems.AllByJob(app.ctx(), job.Id)
	if err != nil {
		t.Fatalf("读取子项失败: %v", err)
	}
	if len(items) != 1 || items[0].Status != model.ItemStatusSkipped {
		t.Fatalf("子项应为 skipped，实际 %+v", items)
	}
	containsAll(t, items[0].Errmsg, "在途")

	// 在途子项被取消后不再阻塞新作业（引擎取消作业时会把未开始项置为 canceled）。
	if err := app.env.Repos.JobItems.UpdateFields(app.ctx(), priorByStep[model.StepCommit].ID, map[string]any{"status": model.ItemStatusCanceled}); err != nil {
		t.Fatalf("取消预置在途子项失败: %v", err)
	}
	job2, err := app.svc.Create(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}")},
	}, "tester")
	if err != nil {
		t.Fatalf("再次创建作业失败: %v", err)
	}
	if job2.Skipped != 0 || job2.Total != 1 {
		t.Fatalf("在途项取消后不应再冲突，实际 total=%d skipped=%d", job2.Total, job2.Skipped)
	}
}

// TestCreateDryRunOnlyPersists 试运行只落库不执行：作业直接落终态，子项为 succeeded 且带计划请求体。
func TestCreateDryRunOnlyPersists(t *testing.T) {
	app := newTestApp(t)
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	tpl := app.seedTemplate(5004, 0)

	job, err := app.svc.Create(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}")},
		DryRun:    boolPtr(true),
	}, "tester")
	if err != nil {
		t.Fatalf("创建试运行作业失败: %v", err)
	}
	if !job.DryRun {
		t.Fatalf("应标记为试运行")
	}
	if job.Status != gen.JobStatus(model.JobStatusSucceeded) {
		t.Fatalf("试运行作业应直接落 succeeded（引擎只扫 pending/running），实际 %s", job.Status)
	}
	if job.Succeeded != 1 {
		t.Fatalf("试运行子项应记成功，实际 succeeded=%d", job.Succeeded)
	}
	items, err := app.env.Repos.JobItems.AllByJob(app.ctx(), job.Id)
	if err != nil {
		t.Fatalf("读取子项失败: %v", err)
	}
	if len(items) != 1 || items[0].Status != model.ItemStatusSucceeded {
		t.Fatalf("试运行子项应为 succeeded，实际 %+v", items)
	}
	if jsonInt64Of(items[0].Request["template_id"]) != tpl.TemplateID {
		t.Fatalf("试运行子项应保留计划请求体，实际 %#v", items[0].Request)
	}
}

// TestEngineControlRequiresEngine 未注入引擎时控制类方法返回明确错误。
func TestEngineControlRequiresEngine(t *testing.T) {
	app := newTestApp(t)
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	tpl := app.seedTemplate(5005, 0)

	job, err := app.svc.Create(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}")},
	}, "tester")
	if err != nil {
		t.Fatalf("创建作业失败: %v", err)
	}

	_, err = app.svc.Start(app.ctx(), job.Id)
	if !errors.Is(err, ErrEngineNotRunning) {
		t.Fatalf("期望 ErrEngineNotRunning，实际 %v", err)
	}
	containsAll(t, err.Error(), "作业引擎未启动", "单实例锁")
	if !errors.Is(err, core.ErrConflict) {
		t.Fatalf("应同时命中 core.ErrConflict 以便 handler 映射 409，实际 %v", err)
	}

	// 不存在的作业 → 404 语义。
	if _, err := app.svc.Get(app.ctx(), "no-such-job"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("期望 core.ErrNotFound，实际 %v", err)
	}
}

// TestListAndItems 列表与明细（含昵称）。
func TestListAndItems(t *testing.T) {
	app := newTestApp(t)
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	tpl := app.seedTemplate(5006, 0)

	job, err := app.svc.Create(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}")},
	}, "tester")
	if err != nil {
		t.Fatalf("创建作业失败: %v", err)
	}

	jobType := gen.JobType(model.JobTypeCommit)
	list, err := app.svc.List(app.ctx(), gen.ListJobsParams{Type: &jobType})
	if err != nil {
		t.Fatalf("查询列表失败: %v", err)
	}
	if list.Total != 1 || len(list.Items) != 1 || list.Items[0].Id != job.Id {
		t.Fatalf("列表结果不符：%+v", list)
	}
	detail, err := app.svc.Get(app.ctx(), job.Id)
	if err != nil {
		t.Fatalf("查询作业详情失败: %v", err)
	}
	if detail.Pending == nil || *detail.Pending != 1 {
		t.Fatalf("详情应带 pending 统计，实际 %+v", detail.Pending)
	}
	if detail.Skipped != 0 || detail.Total != 1 {
		t.Fatalf("详情计数不符：total=%d skipped=%d", detail.Total, detail.Skipped)
	}

	status := gen.JobItemStatus(model.ItemStatusPending)
	items, err := app.svc.Items(app.ctx(), job.Id, gen.ListJobItemsParams{Status: &status})
	if err != nil {
		t.Fatalf("查询明细失败: %v", err)
	}
	if items.Total != 1 || len(items.Items) != 1 {
		t.Fatalf("明细结果不符：%+v", items)
	}
	if items.Items[0].NickName == nil || *items.Items[0].NickName != "测试小程序" {
		t.Fatalf("明细应带小程序昵称，实际 %v", items.Items[0].NickName)
	}
	if items.Items[0].Step != gen.JobStep(model.StepCommit) {
		t.Fatalf("步骤应为 commit，实际 %s", items.Items[0].Step)
	}
}

// boolPtr 返回 bool 指针。
func boolPtr(v bool) *bool { return &v }

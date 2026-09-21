package wxjob

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"wx-platform/server/internal/batch"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

// TestEngineStoreNoItemConversion repo 的 ErrNotFound 必须转成 batch.ErrNoItem，
// 否则引擎会把「本轮没有可领取的子项」当成仓储故障并中断推进。
func TestEngineStoreNoItemConversion(t *testing.T) {
	app := newTestApp(t)
	store := NewEngineStore(app.env)

	_, err := store.ClaimNextPending(app.ctx(), "no-such-job", model.StepCommit)
	if !errors.Is(err, batch.ErrNoItem) {
		t.Fatalf("期望 batch.ErrNoItem，实际 %v", err)
	}
	// 反证：repo 自己返回的是 ErrNotFound，适配层做了转换。
	if _, rawErr := app.env.Repos.JobItems.ClaimNextPending(app.ctx(), "no-such-job", model.StepCommit); !errors.Is(rawErr, repo.ErrNotFound) {
		t.Fatalf("repo 层应返回 ErrNotFound，实际 %v", rawErr)
	}
}

// TestEngineStoreLifecycle 引擎 Store 的领取 / 回写 / 提升等待 / 复位 / 重试 / 取消。
func TestEngineStoreLifecycle(t *testing.T) {
	app := newTestApp(t)
	store := NewEngineStore(app.env)
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	job, items := app.seedJob(model.JobTypeSubmitAudit, appid, nil)

	// GetJob / RunningJobs。
	got, err := store.GetJob(app.ctx(), job.ID)
	if err != nil {
		t.Fatalf("GetJob 失败: %v", err)
	}
	if got.ID != job.ID || got.Status != model.JobStatusRunning {
		t.Fatalf("GetJob 结果不符：%+v", got)
	}
	running, err := store.RunningJobs(app.ctx())
	if err != nil {
		t.Fatalf("RunningJobs 失败: %v", err)
	}
	if len(running) != 1 || running[0].ID != job.ID {
		t.Fatalf("RunningJobs 结果不符：%+v", running)
	}

	// ClaimNextPending：领取即置为 running。
	claimed, err := store.ClaimNextPending(app.ctx(), job.ID, model.StepPrivacyCheck)
	if err != nil {
		t.Fatalf("领取子项失败: %v", err)
	}
	if claimed.ID != items[model.StepPrivacyCheck].ID || claimed.Status != model.ItemStatusRunning {
		t.Fatalf("领取结果不符：%+v", claimed)
	}
	// 同一步骤再次领取 → 没有可领取项。
	if _, err := store.ClaimNextPending(app.ctx(), job.ID, model.StepPrivacyCheck); !errors.Is(err, batch.ErrNoItem) {
		t.Fatalf("重复领取应返回 ErrNoItem，实际 %v", err)
	}

	// UpdateItem 回写结果与累计等待（引擎用 response 保存 waiting 时的自有信息）。
	if err := store.UpdateItem(app.ctx(), claimed.ID, map[string]any{
		"status":   model.ItemStatusWaiting,
		"response": model.JSONMap{"privacy_wait_seconds": 30},
	}); err != nil {
		t.Fatalf("回写子项失败: %v", err)
	}
	stored := app.reloadItem(claimed.ID)
	if stored.Status != model.ItemStatusWaiting || jsonIntOf(stored.Response["privacy_wait_seconds"]) != 30 {
		t.Fatalf("回写结果不符：%+v", stored)
	}

	// PromoteDueWaiting：把到期的 waiting 项改回 pending。
	if err := app.env.Repos.JobItems.UpdateFields(app.ctx(), claimed.ID,
		map[string]any{"next_run_at": time.Now().Add(-time.Minute)}); err != nil {
		t.Fatalf("更新 next_run_at 失败: %v", err)
	}
	n, err := store.PromoteDueWaiting(app.ctx(), job.ID, model.StepPrivacyCheck)
	if err != nil {
		t.Fatalf("提升等待项失败: %v", err)
	}
	if n != 1 || app.reloadItem(claimed.ID).Status != model.ItemStatusPending {
		t.Fatalf("提升等待项结果不符：n=%d status=%s", n, app.reloadItem(claimed.ID).Status)
	}

	// ResetRunning：进程重启时把残留 running 复位为 pending。
	if err := store.UpdateItem(app.ctx(), claimed.ID, map[string]any{"status": model.ItemStatusRunning}); err != nil {
		t.Fatalf("回写子项失败: %v", err)
	}
	if n, err := store.ResetRunning(app.ctx()); err != nil || n != 1 {
		t.Fatalf("复位残留执行中状态结果不符：n=%d err=%v", n, err)
	}
	if app.reloadItem(claimed.ID).Status != model.ItemStatusPending {
		t.Fatalf("复位后应为 pending")
	}

	// RetryFailed / CancelPending。
	if err := store.UpdateItem(app.ctx(), claimed.ID, map[string]any{"status": model.ItemStatusFailed}); err != nil {
		t.Fatalf("回写子项失败: %v", err)
	}
	if n, err := store.RetryFailed(app.ctx(), job.ID); err != nil || n != 1 {
		t.Fatalf("重试失败项结果不符：n=%d err=%v", n, err)
	}
	// 此时两个子项都是待执行（上一个用例把它重置为 pending），取消应覆盖两者。
	if n, err := store.CancelPending(app.ctx(), job.ID); err != nil || n != 2 {
		t.Fatalf("取消未执行项结果不符：n=%d err=%v", n, err)
	}
	afterCancel, err := store.ItemsByJob(app.ctx(), job.ID)
	if err != nil {
		t.Fatalf("ItemsByJob 失败: %v", err)
	}
	for i := range afterCancel {
		if afterCancel[i].Status != model.ItemStatusCanceled {
			t.Fatalf("取消后所有未执行项应为 canceled，实际 %+v", afterCancel[i])
		}
	}
	itemsByJob, err := store.ItemsByJob(app.ctx(), job.ID)
	if err != nil {
		t.Fatalf("ItemsByJob 失败: %v", err)
	}
	if len(itemsByJob) != len(model.JobTypeSubmitAudit.Steps()) {
		t.Fatalf("子项数量不符：%d", len(itemsByJob))
	}

	// UpdateJob：引擎汇总结果时写回计数与终态。
	if err := store.UpdateJob(app.ctx(), job.ID, map[string]any{
		"status":    model.JobStatusSucceeded,
		"succeeded": 2,
		"total":     2,
	}); err != nil {
		t.Fatalf("UpdateJob 失败: %v", err)
	}
	final := app.reloadJob(t, job.ID)
	if final.Status != model.JobStatusSucceeded || final.Succeeded != 2 {
		t.Fatalf("UpdateJob 结果不符：%+v", final)
	}
}

// TestEngineStoreTryLock 单实例锁适配（repo 用 MySQL GET_LOCK 实现，非 MySQL 环境视为可用）。
func TestEngineStoreTryLock(t *testing.T) {
	app := newTestApp(t)
	store := NewEngineStore(app.env)
	name := fmt.Sprintf("wx_platform_job_engine_test_%d", time.Now().UnixNano())
	ok, err := store.TryLock(app.ctx(), name)
	if err != nil {
		t.Fatalf("TryLock 失败: %v", err)
	}
	if !ok {
		t.Fatalf("首次申请实例锁应成功")
	}
	// 本实例重复申请同名锁是幂等的。
	if ok, err := store.TryLock(app.ctx(), name); err != nil || !ok {
		t.Fatalf("重复申请同名锁应幂等成功：ok=%v err=%v", ok, err)
	}
}

// reloadJob 重新读取作业。
func (a *testApp) reloadJob(t *testing.T, id string) *model.BatchJob {
	t.Helper()
	job, err := a.env.Repos.Jobs.Get(a.ctx(), id)
	if err != nil {
		t.Fatalf("读取作业失败: %v", err)
	}
	return job
}

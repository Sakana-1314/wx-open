package wxjob

import (
	"context"
	"errors"

	"wx-platform/server/internal/batch"
	"wx-platform/server/internal/core"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

// EngineStore 把 repo 的作业仓储适配成 batch.Store（引擎唯一的持久化入口）。
//
// 关键语义转换：repo 用 repo.ErrNotFound 表示「没有记录」，而引擎用 batch.ErrNoItem
// 表示「本轮没有可领取的子项」，因此 ClaimNextPending 必须做这次转换，
// 否则引擎会把「无活可干」当成仓储故障而退出推进循环。
type EngineStore struct {
	env *core.Env
}

// NewEngineStore 构造引擎仓储适配（由 cmd/server 装配引擎时调用）。
func NewEngineStore(env *core.Env) *EngineStore { return &EngineStore{env: env} }

// 编译期断言：EngineStore 必须完整实现 batch.Store。
var _ batch.Store = (*EngineStore)(nil)

// GetJob 读取作业。
func (e *EngineStore) GetJob(ctx context.Context, id string) (*model.BatchJob, error) {
	job, err := e.env.Repos.Jobs.Get(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, core.NotFound("作业不存在（id=%s）", id)
		}
		return nil, core.Internal(err)
	}
	return job, nil
}

// UpdateJob 局部更新作业（status / error_note / counters / finished_at 等）。
func (e *EngineStore) UpdateJob(ctx context.Context, id string, fields map[string]any) error {
	if err := e.env.Repos.Jobs.UpdateFields(ctx, id, fields); err != nil {
		return core.Internal(err)
	}
	return nil
}

// RunningJobs 返回全部在途作业（pending + running），供调度循环扫描。
func (e *EngineStore) RunningJobs(ctx context.Context) ([]model.BatchJob, error) {
	jobs, err := e.env.Repos.Jobs.Running(ctx)
	if err != nil {
		return nil, core.Internal(err)
	}
	return jobs, nil
}

// ItemsByJob 返回作业的全部子项。
func (e *EngineStore) ItemsByJob(ctx context.Context, jobID string) ([]model.BatchJobItem, error) {
	items, err := e.env.Repos.JobItems.AllByJob(ctx, jobID)
	if err != nil {
		return nil, core.Internal(err)
	}
	return items, nil
}

// ClaimNextPending 原子领取指定步骤的下一个待执行子项；没有可领取项时返回 batch.ErrNoItem。
func (e *EngineStore) ClaimNextPending(ctx context.Context, jobID string, step model.JobStep) (*model.BatchJobItem, error) {
	item, err := e.env.Repos.JobItems.ClaimNextPending(ctx, jobID, step)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, batch.ErrNoItem
		}
		return nil, core.Internal(err)
	}
	return item, nil
}

// PromoteDueWaiting 把到期的 waiting 子项改回 pending，返回提升条数。
func (e *EngineStore) PromoteDueWaiting(ctx context.Context, jobID string, step model.JobStep) (int64, error) {
	n, err := e.env.Repos.JobItems.PromoteDueWaiting(ctx, jobID, step)
	if err != nil {
		return 0, core.Internal(err)
	}
	return n, nil
}

// UpdateItem 局部更新子项（引擎回写结果用）。
func (e *EngineStore) UpdateItem(ctx context.Context, id uint, fields map[string]any) error {
	if err := e.env.Repos.JobItems.UpdateFields(ctx, id, fields); err != nil {
		return core.Internal(err)
	}
	return nil
}

// CancelPending 取消作业中尚未执行的子项（pending/waiting → canceled）。
func (e *EngineStore) CancelPending(ctx context.Context, jobID string) (int64, error) {
	n, err := e.env.Repos.JobItems.CancelPending(ctx, jobID)
	if err != nil {
		return 0, core.Internal(err)
	}
	return n, nil
}

// RetryFailed 把失败子项重置为 pending，返回重置条数。
func (e *EngineStore) RetryFailed(ctx context.Context, jobID string) (int64, error) {
	n, err := e.env.Repos.JobItems.RetryFailed(ctx, jobID)
	if err != nil {
		return 0, core.Internal(err)
	}
	return n, nil
}

// ResetRunning 进程启动时把残留的 running 子项复位为 pending（断点续跑）。
func (e *EngineStore) ResetRunning(ctx context.Context) (int64, error) {
	n, err := e.env.Repos.JobItems.ResetRunningToPending(ctx)
	if err != nil {
		return 0, core.Internal(err)
	}
	return n, nil
}

// TryLock 尝试获取单实例排他锁（repo 用 MySQL GET_LOCK 实现）。
func (e *EngineStore) TryLock(ctx context.Context, name string) (bool, error) {
	ok, err := e.env.Repos.Jobs.TryLock(ctx, name)
	if err != nil {
		return false, core.Internal(err)
	}
	return ok, nil
}

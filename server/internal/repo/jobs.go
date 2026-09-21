package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"wx-platform/server/internal/model"
)

// itemBatchSize 子项批量写入的分批大小。
const itemBatchSize = 200

// Jobs 批量作业仓储。
type Jobs struct {
	db *gorm.DB

	// 已持有的实例锁：GET_LOCK 的锁属于「会话」，
	// 必须把专用连接一直攥在手里，否则连接被连接池回收（SetConnMaxLifetime）时锁会静默失效。
	lockMu sync.Mutex
	locks  map[string]*sql.Conn
}

// NewJobs 构造 Jobs 仓储。
func NewJobs(db *gorm.DB) *Jobs { return &Jobs{db: db, locks: make(map[string]*sql.Conn)} }

// Create 事务内创建作业与其全部子项：任一子项写失败则整单回滚，避免出现「有作业无子项」的僵尸作业。
// 未显式设置的状态按 pending 补齐（作业与子项），Total 为 0 且传入了子项时以子项数量补齐。
func (r *Jobs) Create(ctx context.Context, job *model.BatchJob, items []model.BatchJobItem) error {
	if job == nil || job.ID == "" {
		return errors.New("作业 ID 不能为空")
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if job.Status == "" {
			job.Status = model.JobStatusPending
		}
		if job.Total == 0 && len(items) > 0 {
			job.Total = len(items)
		}
		if cerr := tx.Create(job).Error; cerr != nil {
			return fmt.Errorf("创建作业失败: %w", cerr)
		}
		if len(items) == 0 {
			return nil
		}
		rows := make([]model.BatchJobItem, 0, len(items))
		for _, it := range items {
			it.ID = 0
			if it.JobID == "" {
				it.JobID = job.ID
			}
			if it.Status == "" {
				it.Status = model.ItemStatusPending
			}
			rows = append(rows, it)
		}
		if cerr := tx.CreateInBatches(rows, itemBatchSize).Error; cerr != nil {
			return fmt.Errorf("创建作业子项失败: %w", cerr)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// Get 按作业 ID 读取作业；不存在返回 ErrNotFound。
func (r *Jobs) Get(ctx context.Context, id string) (*model.BatchJob, error) {
	var job model.BatchJob
	err := r.db.WithContext(ctx).Take(&job, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询作业失败: %w", err)
	}
	return &job, nil
}

// List 分页查询作业；jobType / status 为空表示不过滤，同时返回总数。
// 索引依据：batch_jobs 上 type、status 各有单列索引，按创建时间倒序取最新作业。
func (r *Jobs) List(ctx context.Context, jobType model.JobType, status model.JobStatus, page, pageSize int) ([]model.BatchJob, int64, error) {
	items := make([]model.BatchJob, 0)
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.BatchJob{})
		if jobType != "" {
			q = q.Where("type = ?", jobType)
		}
		if status != "" {
			q = q.Where("status = ?", status)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计作业失败: %w", err)
	}
	limit, offset := paginate(page, pageSize)
	if err := base().Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("查询作业列表失败: %w", err)
	}
	return items, total, nil
}

// Recent 返回最近创建的若干条作业（首页「最近作业」卡片）。
func (r *Jobs) Recent(ctx context.Context, limit int) ([]model.BatchJob, error) {
	items := make([]model.BatchJob, 0)
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	if err := r.db.WithContext(ctx).Order("created_at DESC, id DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询最近作业失败: %w", err)
	}
	return items, nil
}

// UpdateFields 局部更新作业（如 status / note / started_at / finished_at / runner_id）。
func (r *Jobs) UpdateFields(ctx context.Context, id string, fields map[string]any) error {
	if id == "" {
		return errors.New("作业 ID 不能为空")
	}
	if err := updateWhere(r.db.WithContext(ctx), &model.BatchJob{}, "id = ?", []any{id}, fields); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("更新作业失败: %w", err)
	}
	return nil
}

// UpdateCounters 依据子项状态重算 succeeded / failed / skipped / total。
//
// 口径：total = 全部子项数；succeeded / failed / skipped 分别按同名状态计数。
// canceled 子项计入 total，但不计入 succeeded/failed/skipped（作业终态由 service 判定）。
func (r *Jobs) UpdateCounters(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []struct {
			Status model.JobItemStatus
			N      int64
		}
		if err := tx.Model(&model.BatchJobItem{}).
			Select("status AS status, COUNT(*) AS n").
			Where("job_id = ?", id).
			Group("status").Scan(&rows).Error; err != nil {
			return fmt.Errorf("统计作业子项状态失败: %w", err)
		}
		var total, succeeded, failed, skipped int64
		for _, row := range rows {
			total += row.N
			switch row.Status {
			case model.ItemStatusSucceeded:
				succeeded = row.N
			case model.ItemStatusFailed:
				failed = row.N
			case model.ItemStatusSkipped:
				skipped = row.N
			}
		}
		fields := map[string]any{
			"total":     total,
			"succeeded": succeeded,
			"failed":    failed,
			"skipped":   skipped,
		}
		if err := updateWhere(tx, &model.BatchJob{}, "id = ?", []any{id}, fields); err != nil {
			if errors.Is(err, ErrNotFound) {
				return ErrNotFound
			}
			return fmt.Errorf("更新作业计数失败: %w", err)
		}
		return nil
	})
}

// Running 返回全部在途作业（status IN (pending, running)），用于启动时续跑。
// 索引依据：batch_jobs.status 单列索引（IN 等值集合命中），按创建时间升序续跑，保证先提交的先跑完。
func (r *Jobs) Running(ctx context.Context) ([]model.BatchJob, error) {
	items := make([]model.BatchJob, 0)
	if err := r.db.WithContext(ctx).
		Where("status IN ?", []model.JobStatus{model.JobStatusPending, model.JobStatusRunning}).
		Order("created_at ASC, id ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询在途作业失败: %w", err)
	}
	return items, nil
}

// CountByStatus 按作业状态统计（看板用）。
func (r *Jobs) CountByStatus(ctx context.Context) (map[model.JobStatus]int64, error) {
	var rows []struct {
		Status model.JobStatus
		N      int64
	}
	if err := r.db.WithContext(ctx).Model(&model.BatchJob{}).
		Select("status AS status, COUNT(*) AS n").
		Group("status").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("按状态统计作业失败: %w", err)
	}
	out := make(map[model.JobStatus]int64, len(rows))
	for _, row := range rows {
		out[row.Status] = row.N
	}
	return out, nil
}

// HasActiveItem 检查同一 appid 是否已存在未结束的指定步骤子项（提审 / 发布前的在途冲突检查）。
//
// 索引依据：batch_job_items.appid 有单列索引（复合索引 idx_item_job_step_appid 以 job_id 打头，
// 无法用于纯 appid 过滤），因此这里靠 appid 单列索引先把结果集收敛到该小程序。
// 只按子项状态判断即可：作业被取消时相应子项会被置为 canceled，不会误判为在途。
func (r *Jobs) HasActiveItem(ctx context.Context, appid string, steps []model.JobStep) (bool, error) {
	if appid == "" || len(steps) == 0 {
		return false, nil
	}
	var n int64
	err := r.db.WithContext(ctx).Model(&model.BatchJobItem{}).
		Where("appid = ? AND step IN ?", appid, steps).
		Where("status IN ?", []model.JobItemStatus{
			model.ItemStatusPending, model.ItemStatusWaiting, model.ItemStatusRunning,
		}).
		Limit(1).Count(&n).Error
	if err != nil {
		return false, fmt.Errorf("检查在途作业冲突失败: %w", err)
	}
	return n > 0, nil
}

// TryLock 用 MySQL 的 SELECT GET_LOCK(name, 0) 获取单实例排他锁（调度器只在拿到锁的实例上跑）。
//
// 返回值：(true, nil) 拿到锁或当前环境不支持排他锁；(false, nil) 已被其它实例占用；查询失败返回 error。
// 非 MySQL（例如测试用 SQLite）或服务器不支持 GET_LOCK 时按「不可用」处理并返回 (true, nil)，
// 避免非 MySQL 环境因此启动不了。
//
// 实现要点：GET_LOCK 的锁属于「会话」，因此这里从连接池单独取一条专用连接（sql.Conn）执行，
// 并把该连接一直持有在 r.locks 里 —— 若只是用连接池跑一次 SELECT，
// 连接一旦被归还/按 SetConnMaxLifetime 回收，锁就会静默丢失，其它实例会同时抢到锁。
// 本实例重复调用同一个 name 直接返回 true（幂等）；锁在进程退出、连接断开时由 MySQL 自动释放，
// 因此没有提供 Unlock（单实例门闸的语义就是「进程活着就一直持有」）。
func (r *Jobs) TryLock(ctx context.Context, name string) (bool, error) {
	if strings.TrimSpace(name) == "" {
		return false, errors.New("锁名称不能为空")
	}
	if r.db == nil || r.db.Dialector == nil || r.db.Dialector.Name() != "mysql" {
		return true, nil // 非 MySQL 环境不支持 GET_LOCK：视为可用
	}

	r.lockMu.Lock()
	defer r.lockMu.Unlock()
	if r.locks == nil {
		r.locks = make(map[string]*sql.Conn)
	}
	if _, held := r.locks[name]; held {
		return true, nil // 本实例已持有，重复调用幂等
	}

	sqlDB, err := r.db.DB()
	if err != nil {
		return false, fmt.Errorf("获取数据库连接池失败: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return false, fmt.Errorf("获取专用连接失败: %w", err)
	}

	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 0)", name).Scan(&got); err != nil {
		_ = conn.Close()
		if isLockFunctionUnavailable(err) {
			return true, nil // 服务器没有 GET_LOCK 函数：视为不可用
		}
		return false, fmt.Errorf("申请实例锁 %s 失败: %w", name, err)
	}
	if !got.Valid {
		// GET_LOCK 返回 NULL 表示函数不可用（参数非法 / 内部错误），按不可用处理。
		_ = conn.Close()
		return true, nil
	}
	if got.Int64 != 1 {
		_ = conn.Close()
		return false, nil // 已被其它实例占用
	}

	// 保住这条连接：锁随会话存续，会话随进程退出而结束。
	r.locks[name] = conn
	return true, nil
}

// JobItems 作业子项仓储（worker pool 的取任务入口）。
type JobItems struct{ db *gorm.DB }

// NewJobItems 构造 JobItems 仓储。
func NewJobItems(db *gorm.DB) *JobItems { return &JobItems{db: db} }

// ListByJob 分页查询作业子项；jobID / status / appid 为空表示不过滤，同时返回总数。
func (r *JobItems) ListByJob(ctx context.Context, jobID string, status model.JobItemStatus, appid string, page, pageSize int) ([]model.BatchJobItem, int64, error) {
	items := make([]model.BatchJobItem, 0)
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.BatchJobItem{})
		if jobID != "" {
			q = q.Where("job_id = ?", jobID)
		}
		if status != "" {
			q = q.Where("status = ?", status)
		}
		if appid != "" {
			q = q.Where("appid = ?", appid)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计作业子项失败: %w", err)
	}
	limit, offset := paginate(page, pageSize)
	if err := base().Order("id ASC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("查询作业子项失败: %w", err)
	}
	return items, total, nil
}

// AllByJob 返回作业的全部子项（导出明细 / 汇总统计用）。
func (r *JobItems) AllByJob(ctx context.Context, jobID string) ([]model.BatchJobItem, error) {
	items := make([]model.BatchJobItem, 0)
	if err := r.db.WithContext(ctx).Where("job_id = ?", jobID).Order("id ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询作业全部子项失败: %w", err)
	}
	return items, nil
}

// Get 按主键读取子项；不存在返回 ErrNotFound。
func (r *JobItems) Get(ctx context.Context, id uint) (*model.BatchJobItem, error) {
	var it model.BatchJobItem
	err := r.db.WithContext(ctx).Take(&it, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询作业子项失败: %w", err)
	}
	return &it, nil
}

// UpdateFields 局部更新子项（status / errcode / errmsg / error_class / response / next_run_at 等）。
func (r *JobItems) UpdateFields(ctx context.Context, id uint, fields map[string]any) error {
	if err := updateWhere(r.db.WithContext(ctx), &model.BatchJobItem{}, "id = ?", []any{id}, fields); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("更新作业子项失败: %w", err)
	}
	return nil
}

// ClaimNext 原子领取下一个可执行子项（status=pending 且 next_run_at 为空或已到期），无可用返回 ErrNotFound。
//
// 并发安全实现方式：单事务内 `SELECT ... FOR UPDATE`（行级排他锁）+ 条件 UPDATE。
//  1. 加锁读是「当前读」，会读到其它事务已提交的最新版本：第二个 worker 拿到锁后会重新读，
//     此时该行 status 已是 running，因而不会重复领取同一行；
//  2. 锁随事务提交/回滚释放，领取动作与状态流转在同一事务里完成，不存在「读了没改」的窗口；
//  3. UPDATE 再带 `status = pending` 条件，即使隔离级别下出现意外读也不会覆盖他人已领取的行；
//  4. 死锁（1213）/锁等待超时（1205）自动退避重试，避免 worker 因偶发锁冲突退出。
//
// 索引依据：复合唯一索引 idx_item_job_step_appid(job_id, step, appid) 的前缀 job_id 命中过滤，
// 再按主键 id ASC 顺序扫描，天然满足「同一作业内先来先领」。
func (r *JobItems) ClaimNext(ctx context.Context, jobID string) (*model.BatchJobItem, error) {
	if jobID == "" {
		return nil, errors.New("作业 ID 不能为空")
	}
	return r.claimWithRetry(ctx, jobID, "")
}

// ClaimNextPending 原子领取指定步骤的下一个可执行子项（status=pending 且 next_run_at 为空或已到期），
// 无可用项返回 ErrNotFound；step 为空表示不限定步骤（等价于 ClaimNext）。
//
// 与 ClaimNext 共用同一套「事务 + SELECT ... FOR UPDATE + 条件 UPDATE」实现，
// 只是多一个 step 等值条件（由唯一索引 idx_item_job_step_appid 的 (job_id, step) 前缀参与过滤），
// pipeline 作业因此可以让「上传代码」「提审」「发布」各有一组 worker 并行推进，互不抢对方的步骤。
func (r *JobItems) ClaimNextPending(ctx context.Context, jobID string, step model.JobStep) (*model.BatchJobItem, error) {
	if jobID == "" {
		return nil, errors.New("作业 ID 不能为空")
	}
	return r.claimWithRetry(ctx, jobID, step)
}

// claimWithRetry 带锁冲突重试的领取入口（step 为空表示任意步骤）。
func (r *JobItems) claimWithRetry(ctx context.Context, jobID string, step model.JobStep) (*model.BatchJobItem, error) {
	const maxTries = 3
	var lastErr error
	for i := 0; i < maxTries; i++ {
		item, err := r.claimOnce(ctx, jobID, step)
		if err == nil {
			return item, nil
		}
		if errors.Is(err, ErrNotFound) || !isRetryableLockError(err) {
			return nil, err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("领取作业子项被取消: %w", ctx.Err())
		case <-time.After(time.Duration(i+1) * 20 * time.Millisecond):
		}
	}
	return nil, fmt.Errorf("领取作业子项失败（锁冲突重试 %d 次）: %w", maxTries, lastErr)
}

// claimOnce 单次领取尝试（一个事务内完成「加锁读 + 置为 running」）。
func (r *JobItems) claimOnce(ctx context.Context, jobID string, step model.JobStep) (*model.BatchJobItem, error) {
	var claimed *model.BatchJobItem
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var item model.BatchJobItem
		q := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("job_id = ? AND status = ?", jobID, model.ItemStatusPending).
			Where("next_run_at IS NULL OR next_run_at <= ?", now)
		if step != "" {
			q = q.Where("step = ?", step)
		}
		qerr := q.Order("id ASC").Take(&item).Error
		if errors.Is(qerr, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if qerr != nil {
			return fmt.Errorf("锁定待执行子项失败: %w", qerr)
		}

		res := tx.Model(&model.BatchJobItem{}).
			Where("id = ? AND status = ?", item.ID, model.ItemStatusPending).
			Updates(map[string]any{
				"status":     model.ItemStatusRunning,
				"started_at": now,
				"updated_at": now,
			})
		if res.Error != nil {
			return fmt.Errorf("领取子项失败: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			// 持锁期间状态被其它路径改写（理论上不会发生）：本次视为无可领取项。
			return ErrNotFound
		}
		item.Status = model.ItemStatusRunning
		item.StartedAt = &now
		item.UpdatedAt = now
		claimed = &item
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

// PromoteDueWaiting 把该作业该步骤中 next_run_at 已到期（<= now）或为空的 waiting 子项改回 pending，
// 返回受影响条数；step 为空表示不限定步骤。
//
// 用途：提审 / 发布前的等待（如等隐私接口检测任务结束）用 waiting + next_run_at 表示「稍后再试」，
// 调度器每次 tick 先 PromoteDueWaiting 再 ClaimNextPending，就能把到期的等待项重新投入执行。
// next_run_at 保持原值不清空：它已经 <= now，不影响下一次领取（ClaimNext 同样接受已到期的 next_run_at）。
func (r *JobItems) PromoteDueWaiting(ctx context.Context, jobID string, step model.JobStep) (int64, error) {
	if jobID == "" {
		return 0, errors.New("作业 ID 不能为空")
	}
	q := r.db.WithContext(ctx).Model(&model.BatchJobItem{}).
		Where("job_id = ? AND status = ?", jobID, model.ItemStatusWaiting).
		Where("next_run_at IS NULL OR next_run_at <= ?", time.Now())
	if step != "" {
		q = q.Where("step = ?", step)
	}
	res := q.Update("status", model.ItemStatusPending)
	if res.Error != nil {
		return 0, fmt.Errorf("提升到期等待子项失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// ResetRunningToPending 进程启动时把残留的 running 子项复位为 pending，并清空 next_run_at 使其立即可领。
// 返回复位条数。
func (r *JobItems) ResetRunningToPending(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).Model(&model.BatchJobItem{}).
		Where("status = ?", model.ItemStatusRunning).
		Updates(map[string]any{
			"status":      model.ItemStatusPending,
			"next_run_at": nil,
		})
	if res.Error != nil {
		return 0, fmt.Errorf("复位运行中子项失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// RetryFailed 把作业内失败子项重置为 pending（保留 attempt，便于观察重试历史）；返回重置条数。
func (r *JobItems) RetryFailed(ctx context.Context, jobID string) (int64, error) {
	res := r.db.WithContext(ctx).Model(&model.BatchJobItem{}).
		Where("job_id = ? AND status = ?", jobID, model.ItemStatusFailed).
		Updates(map[string]any{
			"status":      model.ItemStatusPending,
			"next_run_at": nil,
		})
	if res.Error != nil {
		return 0, fmt.Errorf("重试失败子项失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// CancelPending 取消作业中尚未执行的子项（pending / waiting → canceled）；返回取消条数。
func (r *JobItems) CancelPending(ctx context.Context, jobID string) (int64, error) {
	res := r.db.WithContext(ctx).Model(&model.BatchJobItem{}).
		Where("job_id = ?", jobID).
		Where("status IN ?", []model.JobItemStatus{model.ItemStatusPending, model.ItemStatusWaiting}).
		Update("status", model.ItemStatusCanceled)
	if res.Error != nil {
		return 0, fmt.Errorf("取消待执行子项失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// CountByStatus 按状态统计指定作业（或全部作业，jobID 为空时）的子项数。
func (r *JobItems) CountByStatus(ctx context.Context, jobID string) (map[model.JobItemStatus]int64, error) {
	rows, err := r.groupCount(ctx, "job_id = ?", jobID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// CountsByApp 按状态统计某小程序的全部子项数（小程序详情页进度条）。
func (r *JobItems) CountsByApp(ctx context.Context, appid string) (map[model.JobItemStatus]int64, error) {
	return r.groupCount(ctx, "appid = ?", appid)
}

// groupCount 按 status 分组计数，cond 为可选过滤条件。
func (r *JobItems) groupCount(ctx context.Context, cond string, value string) (map[model.JobItemStatus]int64, error) {
	var rows []struct {
		Status model.JobItemStatus
		N      int64
	}
	q := r.db.WithContext(ctx).Model(&model.BatchJobItem{}).
		Select("status AS status, COUNT(*) AS n")
	if value != "" {
		q = q.Where(cond, value)
	}
	if err := q.Group("status").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("按状态统计作业子项失败: %w", err)
	}
	out := make(map[model.JobItemStatus]int64, len(rows))
	for _, row := range rows {
		out[row.Status] = row.N
	}
	return out, nil
}

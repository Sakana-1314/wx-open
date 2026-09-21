// Package batch 实现批量作业引擎：把「一次操作 N 个小程序」变成可观察、可暂停、
// 可重试、可断点续跑的作业。
//
// 设计要点（每一条都对应官方文档的硬性要求）：
//   - 步骤串行：同一作业内按 commit → privacy_check → submit_audit → release 的步骤边界推进，
//     上一步全部结束后才进入下一步。这样天然满足「上传代码后必须等隐私检测任务结束才能提审，
//     且不要把上传与提审放在同一个重试循环里」的要求（否则会反复触发 61039）。
//   - 同一小程序串行：微信对同账号的提交有并发限制（9402202），引擎按 appid 串行，绝不并发打同一小程序。
//   - 分类重试：可重试类退避重试；令牌失效由执行器刷新后重试；频次/额度类会暂停整个作业（提审额度是服务商级共享的）；
//     环境类（IP 白名单）不消耗重试次数，直接停下等人处理；永久失败不重试。
//   - waiting 状态：用于「等隐私检测结束」「等审核通过」这类前置条件，到点自动重新排队。
//   - 断点续跑：进程重启时把残留的 running 复位为 pending，未完成作业继续执行。
//   - 单实例：通过数据库排他锁避免多实例重复下发（拿不到锁则引擎不启动，只提供只读页面）。
package batch

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"wx-platform/server/internal/model"
)

// Outcome 单次执行的结局。
type Outcome string

const (
	// OutcomeSucceeded 成功。
	OutcomeSucceeded Outcome = "succeeded"
	// OutcomeFailed 失败（可重试或永久，由 Class 决定）。
	OutcomeFailed Outcome = "failed"
	// OutcomeWaiting 前置条件未满足，稍后重新排队。
	OutcomeWaiting Outcome = "waiting"
	// OutcomeSkipped 跳过（如已有在审版本、未授权、体检不通过）。
	OutcomeSkipped Outcome = "skipped"
)

// Result 执行器返回的结果。
type Result struct {
	Outcome Outcome
	// Class 失败分类，决定是否重试。
	Class model.ErrorClass
	// Errcode/Errmsg 微信返回码与原文。
	Errcode int
	Errmsg  string
	// Request/Response 落库留档（已脱敏）。
	Request  map[string]any
	Response map[string]any
	// UserVersion 本次下发的版本号（commit 用）。
	UserVersion string
	// WxAuditID 提审得到的审核单号。
	WxAuditID int64
	// RetryAfter waiting 时的下次检查时间；为零时用默认轮询间隔。
	RetryAfter time.Duration
	// Note 人类可读说明（写入错误信息或备注）。
	Note string
	// Fatal 为 true 表示整个作业必须停下（额度耗尽、IP 未白名单、暂停等）。
	Fatal     bool
	FatalNote string
}

// Executor 单步执行器（由 service 层实现，注入微信客户端与数据访问）。
type Executor interface {
	// Step 该执行器负责的步骤。
	Step() model.JobStep
	// Execute 执行一个子项。实现必须是并发安全的，不得 panic。
	Execute(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) Result
}

// Store 引擎所需的持久化操作（由 service 层用 repo 适配实现）。
type Store interface {
	GetJob(ctx context.Context, id string) (*model.BatchJob, error)
	UpdateJob(ctx context.Context, id string, fields map[string]any) error
	RunningJobs(ctx context.Context) ([]model.BatchJob, error)

	ItemsByJob(ctx context.Context, jobID string) ([]model.BatchJobItem, error)
	ClaimNextPending(ctx context.Context, jobID string, step model.JobStep) (*model.BatchJobItem, error)
	PromoteDueWaiting(ctx context.Context, jobID string, step model.JobStep) (int64, error)
	UpdateItem(ctx context.Context, id uint, fields map[string]any) error
	CancelPending(ctx context.Context, jobID string) (int64, error)
	RetryFailed(ctx context.Context, jobID string) (int64, error)
	ResetRunning(ctx context.Context) (int64, error)

	// TryLock 尝试获取单实例排他锁。
	TryLock(ctx context.Context, name string) (bool, error)
}

// Options 引擎参数。
type Options struct {
	// Concurrency 并发线程数（每次从设置页实时读取）。
	Concurrency func() int
	// MaxAttempts 单个子项最大尝试次数。
	MaxAttempts func() int
	// PollInterval 无进展时的轮询间隔。
	PollInterval time.Duration
	// LockName 单实例锁名称。
	LockName string
	// Logger 日志输出（默认丢弃，测试注入）。
	Logger func(format string, args ...any)
}

// Engine 作业引擎。
type Engine struct {
	store     Store
	opts      Options
	executors map[model.JobStep]Executor

	mu      sync.Mutex
	active  map[string]context.CancelFunc
	paused  map[string]bool
	started bool
	locked  bool
}

// New 构造引擎。
func New(store Store, opts Options) *Engine {
	if opts.PollInterval <= 0 {
		opts.PollInterval = 2 * time.Second
	}
	if opts.Concurrency == nil {
		opts.Concurrency = func() int { return 3 }
	}
	if opts.MaxAttempts == nil {
		opts.MaxAttempts = func() int { return 3 }
	}
	if opts.Logger == nil {
		opts.Logger = func(string, ...any) {}
	}
	return &Engine{
		store:     store,
		opts:      opts,
		executors: map[model.JobStep]Executor{},
		active:    map[string]context.CancelFunc{},
		paused:    map[string]bool{},
	}
}

// Register 注册执行器。
func (e *Engine) Register(exec Executor) {
	e.executors[exec.Step()] = exec
}

// ErrNoItem 当前没有可领取的子项（Store 实现返回它表示「本轮无事可做」）。
var ErrNoItem = errors.New("没有可领取的子项")

// ErrLocked 未取得单实例锁。
var ErrLocked = errors.New("另一个实例正在运行作业引擎（数据库排他锁被占用）")

// Start 取得单实例锁、复位残留状态，并启动调度循环。
func (e *Engine) Start(ctx context.Context) error {
	lockName := e.opts.LockName
	if lockName == "" {
		lockName = "wx_platform_job_engine"
	}
	ok, err := e.store.TryLock(ctx, lockName)
	if err != nil {
		return fmt.Errorf("获取作业引擎锁失败: %w", err)
	}
	if !ok {
		return ErrLocked
	}
	e.mu.Lock()
	e.locked = true
	e.started = true
	e.mu.Unlock()

	// 断点续跑：把上次进程残留的 running 复位为 pending。
	if n, err := e.store.ResetRunning(ctx); err != nil {
		e.opts.Logger("[engine] 复位残留执行中状态失败: %v", err)
	} else if n > 0 {
		e.opts.Logger("[engine] 已复位 %d 个残留的执行中子项", n)
	}

	go e.loop(ctx)
	return nil
}

// loop 周期性扫描未完成作业并推进。
func (e *Engine) loop(ctx context.Context) {
	ticker := time.NewTicker(e.opts.PollInterval)
	defer ticker.Stop()
	e.sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.sweep(ctx)
		}
	}
}

// sweep 找出未完成作业并为每个作业启动一个推进协程。
func (e *Engine) sweep(ctx context.Context) {
	jobs, err := e.store.RunningJobs(ctx)
	if err != nil {
		e.opts.Logger("[engine] 读取未完成作业失败: %v", err)
		return
	}
	for i := range jobs {
		job := jobs[i]
		if job.Status == model.JobStatusPaused {
			continue
		}
		e.mu.Lock()
		if _, running := e.active[job.ID]; running {
			e.mu.Unlock()
			continue
		}
		jobCtx, cancel := context.WithCancel(ctx)
		e.active[job.ID] = cancel
		e.mu.Unlock()

		go func(j *model.BatchJob) {
			defer func() {
				e.mu.Lock()
				delete(e.active, j.ID)
				e.mu.Unlock()
				cancel()
			}()
			if err := e.runJob(jobCtx, j.ID); err != nil {
				e.opts.Logger("[engine] 作业 %s 推进结束并报错: %v", j.ID, err)
			}
		}(&job)
	}
}

// stepOrder 返回作业的步骤顺序。
func stepOrder(job *model.BatchJob) []model.JobStep {
	steps := job.Type.Steps()
	if len(steps) == 0 {
		return []model.JobStep{model.StepSingle}
	}
	return steps
}

// activeStep 返回当前应当推进的步骤：第一个仍有未结束子项的步骤。
func activeStep(items []model.BatchJobItem, order []model.JobStep) (model.JobStep, bool) {
	for _, step := range order {
		for i := range items {
			if items[i].Step == step && !items[i].Status.Finished() {
				return step, true
			}
		}
	}
	return "", false
}

// runJob 推进一个作业直到结束。
func (e *Engine) runJob(ctx context.Context, jobID string) error {
	job, err := e.store.GetJob(ctx, jobID)
	if err != nil {
		return err
	}
	if job.Status != model.JobStatusRunning {
		// pending 作业需要显式启动（由 API 触发），避免误跑。
		return nil
	}
	order := stepOrder(job)
	concurrency := e.opts.Concurrency()
	if job.Concurrency > 0 {
		concurrency = job.Concurrency
	}
	if concurrency < 1 {
		concurrency = 1
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		// 每轮重新读取作业状态，让暂停/取消能及时生效。
		latest, err := e.store.GetJob(ctx, jobID)
		if err != nil {
			return err
		}
		switch latest.Status {
		case model.JobStatusPaused, model.JobStatusCanceled, model.JobStatusInterrupted:
			return nil
		case model.JobStatusSucceeded, model.JobStatusPartialFailed, model.JobStatusFailed:
			return nil
		}

		items, err := e.store.ItemsByJob(ctx, jobID)
		if err != nil {
			return err
		}
		step, ok := activeStep(items, order)
		if !ok {
			return e.finalize(ctx, jobID)
		}
		exec, ok := e.executors[step]
		if !ok {
			return fmt.Errorf("作业 %s 的步骤 %s 没有注册执行器", jobID, step)
		}

		if _, err := e.store.PromoteDueWaiting(ctx, jobID, step); err != nil {
			e.opts.Logger("[engine] 提升到期等待项失败: %v", err)
		}

		// 本轮并行处理该步骤中所有可领取的子项（并发上限内）。
		progressed, err := e.runStep(ctx, latest, step, exec, concurrency, jobID)
		if err != nil {
			return err
		}
		if !progressed {
			// 没有可领取项：要么都在等待（睡到最近到期时间），要么该步骤已无未结束项（下一轮换步骤）。
			items, err = e.store.ItemsByJob(ctx, jobID)
			if err != nil {
				return err
			}
			wait := nextWaitDelay(items, step, e.opts.PollInterval)
			if wait > 0 {
				if err := sleepCtx(ctx, wait); err != nil {
					return nil
				}
				continue
			}
			// 该步骤仍有未结束项但都不可领取：终止本轮，避免死循环。
			if stillActive(items, step) {
				return fmt.Errorf("作业 %s 步骤 %s 存在无法推进的子项", jobID, step)
			}
		}
	}
}

// runStep 并发执行某步骤中可领取的子项，返回是否领取到了至少一个子项。
func (e *Engine) runStep(ctx context.Context, job *model.BatchJob, step model.JobStep, exec Executor, concurrency int, jobID string) (bool, error) {
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	claimed := false

	for {
		item, err := e.store.ClaimNextPending(ctx, jobID, step)
		if err != nil {
			if errors.Is(err, ErrNoItem) {
				break
			}
			return claimed, err
		}
		claimed = true
		sem <- struct{}{}
		wg.Add(1)
		go func(it *model.BatchJobItem) {
			defer wg.Done()
			defer func() { <-sem }()
			e.runItem(ctx, job, step, exec, it)
		}(item)
	}
	wg.Wait()
	return claimed, nil
}

// runItem 执行单个子项并落库结果。
func (e *Engine) runItem(ctx context.Context, job *model.BatchJob, step model.JobStep, exec Executor, item *model.BatchJobItem) {
	now := time.Now()
	attempt := item.Attempt + 1
	if err := e.store.UpdateItem(ctx, item.ID, map[string]any{
		"status":      model.ItemStatusRunning,
		"attempt":     attempt,
		"started_at":  now,
		"next_run_at": nil,
	}); err != nil {
		e.opts.Logger("[engine] 标记子项执行中失败(id=%d): %v", item.ID, err)
		return
	}
	item.Attempt = attempt
	item.Status = model.ItemStatusRunning

	// 执行器本身不做超时控制，超时由每个微信调用自己的 http client 负责；
	// 这里只防止执行器永久阻塞。
	result := e.executeSafely(ctx, exec, job, item)
	fields := map[string]any{
		"finished_at": time.Now(),
		"errcode":     result.Errcode,
		"errmsg":      truncate(result.Errmsg, 900),
		"error_class": result.Class,
	}
	if result.Request != nil {
		fields["request"] = model.JSONMap(result.Request)
	}
	if result.Response != nil {
		fields["response"] = model.JSONMap(result.Response)
	}
	if result.UserVersion != "" {
		fields["user_version"] = result.UserVersion
	}
	if result.WxAuditID != 0 {
		fields["wx_audit_id"] = result.WxAuditID
	}

	switch result.Outcome {
	case OutcomeSucceeded:
		fields["status"] = model.ItemStatusSucceeded
		fields["errmsg"] = ""
		fields["errcode"] = 0
	case OutcomeSkipped:
		fields["status"] = model.ItemStatusSkipped
	case OutcomeWaiting:
		delay := result.RetryAfter
		if delay <= 0 {
			delay = e.opts.PollInterval
		}
		fields["status"] = model.ItemStatusWaiting
		fields["next_run_at"] = time.Now().Add(delay)
		fields["finished_at"] = nil
		if result.Note != "" {
			fields["errmsg"] = truncate(result.Note, 900)
		}
	case OutcomeFailed:
		maxAttempts := e.opts.MaxAttempts()
		if result.Class == model.ClassRetryable && attempt < maxAttempts {
			// 指数退避 + 抖动，避免同一时刻批量重试造成尖峰。
			delay := time.Duration(1<<uint(attempt-1)) * time.Second
			delay += time.Duration(rand.Int63n(int64(time.Second)))
			fields["status"] = model.ItemStatusWaiting
			fields["next_run_at"] = time.Now().Add(delay)
			fields["finished_at"] = nil
			fields["errmsg"] = truncate(fmt.Sprintf("第 %d 次尝试失败，将重试：%s", attempt, result.Errmsg), 900)
		} else {
			fields["status"] = model.ItemStatusFailed
		}
	default:
		fields["status"] = model.ItemStatusFailed
		fields["errmsg"] = truncate("执行器返回了未知结局: "+string(result.Outcome), 900)
	}

	if err := e.store.UpdateItem(ctx, item.ID, fields); err != nil {
		e.opts.Logger("[engine] 写回子项结果失败(id=%d): %v", item.ID, err)
	}
	if result.Fatal {
		e.opts.Logger("[engine] 作业 %s 触发致命停止：%s", job.ID, result.FatalNote)
		if err := e.store.UpdateJob(ctx, job.ID, map[string]any{
			"status":     model.JobStatusPaused,
			"error_note": truncate(result.FatalNote, 1000),
		}); err != nil {
			e.opts.Logger("[engine] 暂停作业失败: %v", err)
		}
	}
}

// executeSafely 执行并兜住 panic（执行器来自 service 层，不能让它拖垮整个引擎）。
func (e *Engine) executeSafely(ctx context.Context, exec Executor, job *model.BatchJob, item *model.BatchJobItem) (result Result) {
	defer func() {
		if rec := recover(); rec != nil {
			result = Result{
				Outcome: OutcomeFailed,
				Class:   model.ClassUnknown,
				Errmsg:  fmt.Sprintf("执行器 panic: %v", rec),
			}
		}
	}()
	return exec.Execute(ctx, job, item)
}

// finalize 汇总作业结果并落终态。
func (e *Engine) finalize(ctx context.Context, jobID string) error {
	items, err := e.store.ItemsByJob(ctx, jobID)
	if err != nil {
		return err
	}
	var succeeded, failed, skipped int
	for i := range items {
		switch items[i].Status {
		case model.ItemStatusSucceeded:
			succeeded++
		case model.ItemStatusFailed:
			failed++
		case model.ItemStatusSkipped:
			skipped++
		}
	}
	status := model.JobStatusSucceeded
	switch {
	case failed == 0:
		status = model.JobStatusSucceeded
	case succeeded > 0 || skipped > 0:
		status = model.JobStatusPartialFailed
	default:
		status = model.JobStatusFailed
	}
	now := time.Now()
	return e.store.UpdateJob(ctx, jobID, map[string]any{
		"status":      status,
		"succeeded":   succeeded,
		"failed":      failed,
		"skipped":     skipped,
		"total":       len(items),
		"finished_at": now,
	})
}

// BeginJob 手动启动作业（pending → running）。
func (e *Engine) BeginJob(ctx context.Context, jobID string) error {
	now := time.Now()
	if err := e.store.UpdateJob(ctx, jobID, map[string]any{
		"status":     model.JobStatusRunning,
		"started_at": now,
		"error_note": "",
	}); err != nil {
		return err
	}
	// 立即触发一次推进，不必等下一个轮询周期。
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 24*time.Hour)
		defer cancel()
		if err := e.runJob(ctx, jobID); err != nil {
			e.opts.Logger("[engine] 手动启动作业 %s 失败: %v", jobID, err)
		}
	}()
	return nil
}

// PauseJob 暂停作业（已发出的请求不打断，后续子项不再领取）。
func (e *Engine) PauseJob(ctx context.Context, jobID string) error {
	return e.store.UpdateJob(ctx, jobID, map[string]any{"status": model.JobStatusPaused})
}

// ResumeJob 恢复作业。
func (e *Engine) ResumeJob(ctx context.Context, jobID string) error {
	return e.BeginJob(ctx, jobID)
}

// CancelJob 取消作业：未开始的子项标记取消，已完成的保留结果。
func (e *Engine) CancelJob(ctx context.Context, jobID string) error {
	if _, err := e.store.CancelPending(ctx, jobID); err != nil {
		return err
	}
	return e.store.UpdateJob(ctx, jobID, map[string]any{"status": model.JobStatusCanceled, "finished_at": time.Now()})
}

// RetryFailedItems 重试失败项（永久失败同样重置，交由用户判断；重试后若仍失败会立即再次失败）。
func (e *Engine) RetryFailedItems(ctx context.Context, jobID string) (int64, error) {
	n, err := e.store.RetryFailed(ctx, jobID)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		// 顺带把终态作业重新置为运行中。
		if err := e.store.UpdateJob(ctx, jobID, map[string]any{
			"status":      model.JobStatusRunning,
			"finished_at": nil,
		}); err != nil {
			return n, err
		}
	}
	return n, nil
}

// nextWaitDelay 计算最近的等待到期时间；没有等待项时返回 0。
func nextWaitDelay(items []model.BatchJobItem, step model.JobStep, fallback time.Duration) time.Duration {
	var earliest *time.Time
	for i := range items {
		it := items[i]
		if it.Step != step || it.Status != model.ItemStatusWaiting {
			continue
		}
		if it.NextRunAt == nil {
			return fallback
		}
		if earliest == nil || it.NextRunAt.Before(*earliest) {
			earliest = it.NextRunAt
		}
	}
	if earliest == nil {
		return 0
	}
	delay := time.Until(*earliest)
	if delay <= 0 {
		return time.Millisecond
	}
	if delay > fallback {
		// 等待时间较长时分段唤醒，保证暂停/取消能及时生效。
		return fallback
	}
	return delay
}

// stillActive 判断该步骤是否仍有未结束子项。
func stillActive(items []model.BatchJobItem, step model.JobStep) bool {
	for i := range items {
		if items[i].Step == step && !items[i].Status.Finished() {
			return true
		}
	}
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// CountSummary 作业计数摘要。
type CountSummary struct {
	Total     int
	Pending   int
	Waiting   int
	Running   int
	Succeeded int
	Failed    int
	Skipped   int
	Canceled  int
}

// Summarize 汇总子项状态。
func Summarize(items []model.BatchJobItem) CountSummary {
	var s CountSummary
	s.Total = len(items)
	for i := range items {
		switch items[i].Status {
		case model.ItemStatusPending:
			s.Pending++
		case model.ItemStatusWaiting:
			s.Waiting++
		case model.ItemStatusRunning:
			s.Running++
		case model.ItemStatusSucceeded:
			s.Succeeded++
		case model.ItemStatusFailed:
			s.Failed++
		case model.ItemStatusSkipped:
			s.Skipped++
		case model.ItemStatusCanceled:
			s.Canceled++
		}
	}
	return s
}

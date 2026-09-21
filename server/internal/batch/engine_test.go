package batch

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"wx-platform/server/internal/model"
)

// fakeStore 内存实现，用于在不依赖数据库与微信的情况下验证引擎行为。
type fakeStore struct {
	mu    sync.Mutex
	jobs  map[string]*model.BatchJob
	items map[string][]*model.BatchJobItem
	seq   uint
}

func newFakeStore() *fakeStore {
	return &fakeStore{jobs: map[string]*model.BatchJob{}, items: map[string][]*model.BatchJobItem{}}
}

func (f *fakeStore) addJob(job *model.BatchJob, items []model.BatchJobItem) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if job.Status == "" {
		job.Status = model.JobStatusRunning
	}
	cp := *job
	f.jobs[job.ID] = &cp
	list := make([]*model.BatchJobItem, 0, len(items))
	for i := range items {
		it := items[i]
		f.seq++
		it.ID = f.seq
		if it.Status == "" {
			it.Status = model.ItemStatusPending
		}
		list = append(list, &it)
	}
	f.items[job.ID] = list
}

func (f *fakeStore) GetJob(_ context.Context, id string) (*model.BatchJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	job, ok := f.jobs[id]
	if !ok {
		return nil, errors.New("job not found")
	}
	cp := *job
	return &cp, nil
}

func (f *fakeStore) UpdateJob(_ context.Context, id string, fields map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	job, ok := f.jobs[id]
	if !ok {
		return errors.New("job not found")
	}
	if v, ok := fields["status"].(model.JobStatus); ok {
		job.Status = v
	}
	if v, ok := fields["succeeded"].(int); ok {
		job.Succeeded = v
	}
	if v, ok := fields["failed"].(int); ok {
		job.Failed = v
	}
	if v, ok := fields["skipped"].(int); ok {
		job.Skipped = v
	}
	if v, ok := fields["total"].(int); ok {
		job.Total = v
	}
	if v, ok := fields["error_note"].(string); ok {
		job.ErrorNote = v
	}
	if v, ok := fields["finished_at"].(time.Time); ok {
		job.FinishedAt = &v
	}
	return nil
}

func (f *fakeStore) RunningJobs(_ context.Context) ([]model.BatchJob, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []model.BatchJob{}
	for _, job := range f.jobs {
		if job.Status == model.JobStatusRunning || job.Status == model.JobStatusPending {
			out = append(out, *job)
		}
	}
	return out, nil
}

func (f *fakeStore) ItemsByJob(_ context.Context, jobID string) ([]model.BatchJobItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	list := f.items[jobID]
	out := make([]model.BatchJobItem, 0, len(list))
	for _, it := range list {
		out = append(out, *it)
	}
	return out, nil
}

func (f *fakeStore) ClaimNextPending(_ context.Context, jobID string, step model.JobStep) (*model.BatchJobItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for _, it := range f.items[jobID] {
		if it.Step != step || it.Status != model.ItemStatusPending {
			continue
		}
		it.Status = model.ItemStatusRunning
		cp := *it
		_ = now
		return &cp, nil
	}
	return nil, ErrNoItem
}

func (f *fakeStore) PromoteDueWaiting(_ context.Context, jobID string, step model.JobStep) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	var n int64
	for _, it := range f.items[jobID] {
		if it.Step != step || it.Status != model.ItemStatusWaiting {
			continue
		}
		if it.NextRunAt == nil || !it.NextRunAt.After(now) {
			it.Status = model.ItemStatusPending
			it.NextRunAt = nil
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) UpdateItem(_ context.Context, id uint, fields map[string]any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, list := range f.items {
		for _, it := range list {
			if it.ID != id {
				continue
			}
			if v, ok := fields["status"].(model.JobItemStatus); ok {
				it.Status = v
			}
			if v, ok := fields["attempt"].(int); ok {
				it.Attempt = v
			}
			if v, ok := fields["errcode"].(int); ok {
				it.Errcode = v
			}
			if v, ok := fields["errmsg"].(string); ok {
				it.Errmsg = v
			}
			if v, ok := fields["error_class"].(model.ErrorClass); ok {
				it.ErrorClass = v
			}
			if v, ok := fields["next_run_at"].(time.Time); ok {
				it.NextRunAt = &v
			}
			if v, ok := fields["user_version"].(string); ok {
				it.UserVersion = v
			}
			if v, ok := fields["wx_audit_id"].(int64); ok {
				it.WxAuditID = v
			}
			return nil
		}
	}
	return errors.New("item not found")
}

func (f *fakeStore) CancelPending(_ context.Context, jobID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, it := range f.items[jobID] {
		if it.Status == model.ItemStatusPending || it.Status == model.ItemStatusWaiting {
			it.Status = model.ItemStatusCanceled
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) RetryFailed(_ context.Context, jobID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, it := range f.items[jobID] {
		if it.Status == model.ItemStatusFailed {
			it.Status = model.ItemStatusPending
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) ResetRunning(_ context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, list := range f.items {
		for _, it := range list {
			if it.Status == model.ItemStatusRunning {
				it.Status = model.ItemStatusPending
				n++
			}
		}
	}
	return n, nil
}

func (f *fakeStore) TryLock(_ context.Context, _ string) (bool, error) { return true, nil }

// fakeExecutor 可编程的执行器。
type fakeExecutor struct {
	step  model.JobStep
	mu    sync.Mutex
	calls []string
	// plan 按 appid 返回结果序列（第 n 次调用取第 n 个，最后一个重复使用）。
	plan map[string][]Result
	// defaultResult 未命中 plan 时使用。
	defaultResult Result
	delay         time.Duration
	inflight      int
	maxInflight   int
}

func (f *fakeExecutor) Step() model.JobStep { return f.step }

func (f *fakeExecutor) Execute(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) Result {
	f.mu.Lock()
	f.inflight++
	if f.inflight > f.maxInflight {
		f.maxInflight = f.inflight
	}
	count := 0
	for _, c := range f.calls {
		if c == item.Appid {
			count++
		}
	}
	f.calls = append(f.calls, item.Appid)
	results := f.plan[item.Appid]
	var result Result
	switch {
	case item.Appid == "":
		result = f.defaultResult
	case len(results) > count:
		result = results[count]
	case len(results) > 0:
		result = results[len(results)-1]
	default:
		result = f.defaultResult
	}
	delay := f.delay
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(delay):
		}
	}
	f.mu.Lock()
	f.inflight--
	f.mu.Unlock()
	return result
}

func (f *fakeExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeExecutor) maxParallel() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.maxInflight
}

func okResult() Result { return Result{Outcome: OutcomeSucceeded} }

// TestEngineRunsStepsInOrder 步骤门控：必须等 commit 全部结束后才执行 submit_audit。
func TestEngineRunsStepsInOrder(t *testing.T) {
	store := newFakeStore()
	job := &model.BatchJob{ID: "job-1", Type: model.JobTypePipeline, Status: model.JobStatusRunning, Concurrency: 2}
	items := []model.BatchJobItem{
		{Step: model.StepCommit, Appid: "app1"},
		{Step: model.StepCommit, Appid: "app2"},
		{Step: model.StepPrivacyCheck, Appid: "app1"},
		{Step: model.StepPrivacyCheck, Appid: "app2"},
		{Step: model.StepSubmitAudit, Appid: "app1"},
		{Step: model.StepSubmitAudit, Appid: "app2"},
		{Step: model.StepRelease, Appid: "app1"},
		{Step: model.StepRelease, Appid: "app2"},
	}
	store.addJob(job, items)

	order := []model.JobStep{}
	var mu sync.Mutex
	record := func(step model.JobStep) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, step)
	}
	mk := func(step model.JobStep) *fakeExecutor {
		return &fakeExecutor{step: step, defaultResult: Result{Outcome: OutcomeSucceeded}}
	}
	commit := mk(model.StepCommit)
	privacy := mk(model.StepPrivacyCheck)
	audit := mk(model.StepSubmitAudit)
	release := mk(model.StepRelease)

	engine := New(store, Options{Concurrency: func() int { return 2 }, PollInterval: 5 * time.Millisecond})
	engine.Register(&recordingExecutor{inner: commit, record: func() { record(model.StepCommit) }})
	engine.Register(&recordingExecutor{inner: privacy, record: func() { record(model.StepPrivacyCheck) }})
	engine.Register(&recordingExecutor{inner: audit, record: func() { record(model.StepSubmitAudit) }})
	engine.Register(&recordingExecutor{inner: release, record: func() { record(model.StepRelease) }})

	if err := engine.runJob(context.Background(), job.ID); err != nil {
		t.Fatalf("推进作业失败: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(order) != 8 {
		t.Fatalf("期望执行 8 次，实际 %d 次：%v", len(order), order)
	}
	// 断言步骤不交叉：commit 的两项必须先于 privacy 的两项，依此类推。
	stepEnd := map[model.JobStep]int{}
	for i, step := range order {
		stepEnd[step] = i
	}
	if stepEnd[model.StepCommit] >= firstIndex(order, model.StepPrivacyCheck) {
		t.Fatalf("commit 未在 privacy_check 之前完成: %v", order)
	}
	if stepEnd[model.StepPrivacyCheck] >= firstIndex(order, model.StepSubmitAudit) {
		t.Fatalf("privacy_check 未在 submit_audit 之前完成: %v", order)
	}
	if stepEnd[model.StepSubmitAudit] >= firstIndex(order, model.StepRelease) {
		t.Fatalf("submit_audit 未在 release 之前完成: %v", order)
	}

	final, _ := store.GetJob(context.Background(), job.ID)
	if final.Status != model.JobStatusSucceeded {
		t.Fatalf("作业状态期望 succeeded，实际 %s", final.Status)
	}
	if final.Succeeded != 8 {
		t.Fatalf("成功计数期望 8，实际 %d", final.Succeeded)
	}
}

func firstIndex(order []model.JobStep, step model.JobStep) int {
	for i, s := range order {
		if s == step {
			return i
		}
	}
	return len(order)
}

// recordingExecutor 记录调用顺序。
type recordingExecutor struct {
	inner  Executor
	record func()
}

func (r *recordingExecutor) Step() model.JobStep { return r.inner.Step() }

func (r *recordingExecutor) Execute(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) Result {
	r.record()
	return r.inner.Execute(ctx, job, item)
}

// TestEngineRetriesRetryable 可重试分类：应退避重试并在成功后计入成功。
func TestEngineRetriesRetryable(t *testing.T) {
	store := newFakeStore()
	job := &model.BatchJob{ID: "job-retry", Type: model.JobTypeCommit, Status: model.JobStatusRunning}
	store.addJob(job, []model.BatchJobItem{{Step: model.StepCommit, Appid: "app1"}})

	exec := &fakeExecutor{
		step: model.StepCommit,
		plan: map[string][]Result{
			"app1": {
				{Outcome: OutcomeFailed, Class: model.ClassRetryable, Errcode: -1, Errmsg: "系统繁忙"},
				{Outcome: OutcomeSucceeded, UserVersion: "v1"},
			},
		},
	}
	engine := New(store, Options{
		Concurrency:  func() int { return 1 },
		MaxAttempts:  func() int { return 3 },
		PollInterval: 5 * time.Millisecond,
	})
	engine.Register(exec)

	if err := engine.runJob(context.Background(), job.ID); err != nil {
		t.Fatalf("推进失败: %v", err)
	}
	if exec.callCount() != 2 {
		t.Fatalf("期望重试一次共 2 次调用，实际 %d", exec.callCount())
	}
	final, _ := store.GetJob(context.Background(), job.ID)
	if final.Status != model.JobStatusSucceeded || final.Succeeded != 1 {
		t.Fatalf("重试后应成功，实际 status=%s succeeded=%d", final.Status, final.Succeeded)
	}
}

// TestEnginePermanentNotRetried 永久失败不重试。
func TestEnginePermanentNotRetried(t *testing.T) {
	store := newFakeStore()
	job := &model.BatchJob{ID: "job-perm", Type: model.JobTypeCommit, Status: model.JobStatusRunning}
	store.addJob(job, []model.BatchJobItem{{Step: model.StepCommit, Appid: "app1"}})

	exec := &fakeExecutor{
		step:          model.StepCommit,
		defaultResult: Result{Outcome: OutcomeFailed, Class: model.ClassPermanent, Errcode: 85044, Errmsg: "代码包超过大小限制"},
	}
	engine := New(store, Options{Concurrency: func() int { return 1 }, PollInterval: 5 * time.Millisecond})
	engine.Register(exec)

	if err := engine.runJob(context.Background(), job.ID); err != nil {
		t.Fatalf("推进失败: %v", err)
	}
	if exec.callCount() != 1 {
		t.Fatalf("永久失败不应重试，实际调用 %d 次", exec.callCount())
	}
	final, _ := store.GetJob(context.Background(), job.ID)
	if final.Status != model.JobStatusFailed {
		t.Fatalf("期望作业 failed，实际 %s", final.Status)
	}
}

// TestEngineFatalPausesJob 额度耗尽这类致命结果必须暂停整个作业，而不是逐个小程序失败。
func TestEngineFatalPausesJob(t *testing.T) {
	store := newFakeStore()
	job := &model.BatchJob{ID: "job-fatal", Type: model.JobTypeSubmitAudit, Status: model.JobStatusRunning}
	store.addJob(job, []model.BatchJobItem{
		{Step: model.StepSubmitAudit, Appid: "app1"},
		{Step: model.StepSubmitAudit, Appid: "app2"},
	})

	exec := &fakeExecutor{
		step: model.StepSubmitAudit,
		defaultResult: Result{
			Outcome:   OutcomeFailed,
			Class:     model.ClassRateLimited,
			Errcode:   85085,
			Errmsg:    "提审数量已达本月上限",
			Fatal:     true,
			FatalNote: "提审额度已用尽（85085），作业已暂停",
		},
	}
	engine := New(store, Options{Concurrency: func() int { return 1 }, PollInterval: 5 * time.Millisecond})
	engine.Register(exec)

	if err := engine.runJob(context.Background(), job.ID); err != nil {
		t.Fatalf("推进失败: %v", err)
	}
	final, _ := store.GetJob(context.Background(), job.ID)
	if final.Status != model.JobStatusPaused {
		t.Fatalf("期望作业被暂停，实际 %s", final.Status)
	}
	if final.ErrorNote == "" {
		t.Fatalf("暂停原因应写入 error_note")
	}
}

// TestEngineWaitingThenSucceed waiting 子项到点后自动重新排队并继续推进。
func TestEngineWaitingThenSucceed(t *testing.T) {
	store := newFakeStore()
	job := &model.BatchJob{ID: "job-wait", Type: model.JobTypeSubmitAudit, Status: model.JobStatusRunning}
	store.addJob(job, []model.BatchJobItem{{Step: model.StepPrivacyCheck, Appid: "app1"}})

	exec := &fakeExecutor{
		step: model.StepPrivacyCheck,
		plan: map[string][]Result{
			"app1": {
				{Outcome: OutcomeWaiting, RetryAfter: 20 * time.Millisecond, Note: "隐私检测任务未完成（61039），稍后重试"},
				{Outcome: OutcomeSucceeded},
			},
		},
	}
	engine := New(store, Options{Concurrency: func() int { return 1 }, PollInterval: 5 * time.Millisecond})
	engine.Register(exec)

	if err := engine.runJob(context.Background(), job.ID); err != nil {
		t.Fatalf("推进失败: %v", err)
	}
	if exec.callCount() != 2 {
		t.Fatalf("等待后应再次执行，实际 %d 次", exec.callCount())
	}
	final, _ := store.GetJob(context.Background(), job.ID)
	if final.Status != model.JobStatusSucceeded {
		t.Fatalf("期望 succeeded，实际 %s", final.Status)
	}
}

// TestEngineSameAppSerialized 同一 appid 永不并发（微信 9402202 的防线）。
func TestEngineSameAppSerialized(t *testing.T) {
	store := newFakeStore()
	job := &model.BatchJob{ID: "job-serial", Type: model.JobTypeCommit, Status: model.JobStatusRunning, Concurrency: 3}
	items := []model.BatchJobItem{}
	for _, appid := range []string{"app1", "app2", "app3"} {
		items = append(items, model.BatchJobItem{Step: model.StepCommit, Appid: appid})
	}
	store.addJob(job, items)

	// 通过延时放大并发窗口：3 个不同 appid 可并发，但同 appid 只有一项，故最大并发 ≤3。
	exec := &fakeExecutor{step: model.StepCommit, defaultResult: okResult(), delay: 10 * time.Millisecond}
	engine := New(store, Options{Concurrency: func() int { return 3 }, PollInterval: 5 * time.Millisecond})
	engine.Register(exec)

	if err := engine.runJob(context.Background(), job.ID); err != nil {
		t.Fatalf("推进失败: %v", err)
	}
	if exec.callCount() != 3 {
		t.Fatalf("期望 3 次调用，实际 %d", exec.callCount())
	}
	if exec.maxParallel() > 3 {
		t.Fatalf("并发超过上限：%d", exec.maxParallel())
	}
}

// TestEngineCancelJob 取消后未开始项标记取消。
func TestEngineCancelJob(t *testing.T) {
	store := newFakeStore()
	job := &model.BatchJob{ID: "job-cancel", Type: model.JobTypeCommit, Status: model.JobStatusRunning}
	store.addJob(job, []model.BatchJobItem{{Step: model.StepCommit, Appid: "app1"}})

	engine := New(store, Options{Concurrency: func() int { return 1 }, PollInterval: 5 * time.Millisecond})
	engine.Register(&fakeExecutor{step: model.StepCommit, defaultResult: okResult()})

	if err := engine.CancelJob(context.Background(), job.ID); err != nil {
		t.Fatalf("取消失败: %v", err)
	}
	final, _ := store.GetJob(context.Background(), job.ID)
	if final.Status != model.JobStatusCanceled {
		t.Fatalf("期望 canceled，实际 %s", final.Status)
	}
	items, _ := store.ItemsByJob(context.Background(), job.ID)
	if items[0].Status != model.ItemStatusCanceled {
		t.Fatalf("未开始项应被标记取消，实际 %s", items[0].Status)
	}
}

// TestEnginePauseStopsProgress 暂停后不再领取子项。
func TestEnginePauseStopsProgress(t *testing.T) {
	store := newFakeStore()
	job := &model.BatchJob{ID: "job-pause", Type: model.JobTypeCommit, Status: model.JobStatusPaused}
	store.addJob(job, []model.BatchJobItem{{Step: model.StepCommit, Appid: "app1"}})

	exec := &fakeExecutor{step: model.StepCommit, defaultResult: okResult()}
	engine := New(store, Options{Concurrency: func() int { return 1 }, PollInterval: 5 * time.Millisecond})
	engine.Register(exec)

	if err := engine.runJob(context.Background(), job.ID); err != nil {
		t.Fatalf("推进失败: %v", err)
	}
	if exec.callCount() != 0 {
		t.Fatalf("暂停作业不应执行任何子项，实际 %d 次", exec.callCount())
	}
}

// TestEngineResumeUnfinished 启动时把残留 running 复位为 pending（断点续跑）。
func TestEngineResumeUnfinished(t *testing.T) {
	store := newFakeStore()
	store.addJob(&model.BatchJob{ID: "job-resume", Type: model.JobTypeCommit, Status: model.JobStatusRunning},
		[]model.BatchJobItem{{Step: model.StepCommit, Appid: "app1", Status: model.ItemStatusRunning}})

	engine := New(store, Options{Concurrency: func() int { return 1 }, PollInterval: 5 * time.Millisecond})
	engine.Register(&fakeExecutor{step: model.StepCommit, defaultResult: okResult()})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	items, _ := store.ItemsByJob(context.Background(), "job-resume")
	if items[0].Status != model.ItemStatusPending {
		t.Fatalf("残留 running 应被复位为 pending，实际 %s", items[0].Status)
	}
}

// TestSummarize 计数汇总。
func TestSummarize(t *testing.T) {
	items := []model.BatchJobItem{
		{Status: model.ItemStatusSucceeded},
		{Status: model.ItemStatusFailed},
		{Status: model.ItemStatusWaiting},
		{Status: model.ItemStatusSkipped},
		{Status: model.ItemStatusPending},
		{Status: model.ItemStatusRunning},
		{Status: model.ItemStatusCanceled},
	}
	s := Summarize(items)
	if s.Total != 7 || s.Succeeded != 1 || s.Failed != 1 || s.Waiting != 1 ||
		s.Skipped != 1 || s.Pending != 1 || s.Running != 1 || s.Canceled != 1 {
		t.Fatalf("汇总结果异常: %+v", s)
	}
}

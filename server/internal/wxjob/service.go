package wxjob

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"wx-platform/server/internal/batch"
	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

// ErrEngineNotRunning 作业引擎未注入（未取得单实例锁或服务刚启动）。
//
// 同时命中 core.ErrConflict，handler 会映射成 409，页面上直接展示 message。
var ErrEngineNotRunning = fmt.Errorf("%w: 作业引擎未启动（可能未取得单实例锁）：批量作业的推进与控制都依赖引擎，"+
	"请确认本实例拿到了数据库排他锁（多实例部署时只有一个实例会启动引擎），或稍后重试", core.ErrConflict)

// conflictNote 在途冲突的提示（同一小程序提交有并发限制）。
const conflictNote = "该小程序已有在途作业，请等待其完成或先暂停该作业后再重试（同一小程序的上传/提审有并发限制，官方 9402202）"

// 分页默认值与上限（与 repo 层口径一致；repo 未回传生效值，这里用于响应回显）。
const (
	defaultPageSize = 20
	maxPageSize     = 200
)

// JobService 批量作业服务：作业的创建 / 预览 / 查询 / 控制。
type JobService struct {
	env *core.Env
	// engine 由 service.Container 在启动期通过 AttachEngine 注入，之后只读。
	engine *batch.Engine
}

// NewJobService 构造批量作业服务。
//
// 微信调用日志由 wxapi.Client 的 Logger（service.WxCallRecorder）统一落 api_call_logs，
// 因此这里不需要额外的 recorder 依赖。
func NewJobService(env *core.Env) *JobService {
	return &JobService{env: env}
}

// AttachEngine 注入作业引擎（由 service.Container 在注册执行器后调用）。
func (s *JobService) AttachEngine(e *batch.Engine) { s.engine = e }

// repos 返回数据访问聚合（env 未初始化时返回 nil）。
func (s *JobService) repos() *core.Repos {
	if s.env == nil {
		return nil
	}
	return s.env.Repos
}

// settings 读取运行参数服务。
func (s *JobService) settings() *core.SettingService {
	if s.env == nil {
		return nil
	}
	return s.env.Settings
}

// settingInt 读取整数运行参数；缺失或非法时用默认值（且保证为正数）。
func (s *JobService) settingInt(key string, def int) int {
	if svc := s.settings(); svc != nil {
		if v := svc.GetInt(key, def); v > 0 {
			return v
		}
	}
	return def
}

// Preview 逐项算出最终请求体（dry-run，**不调用微信**），并给出每项的校验结果。
//
// 校验覆盖：模板存在且为普通模板、ext_json 渲染与解析、user_version ≤64 字符、
// 提审项类目字段与标题/标签长度、小程序是否已授权 + 启用 + 具备开发权限集（18）。
func (s *JobService) Preview(ctx context.Context, req gen.JobCreateRequest) (*gen.JobPreviewResponse, error) {
	_, planned, err := s.buildPlan(ctx, req, SingleOptions{})
	if err != nil {
		return nil, err
	}
	out := &gen.JobPreviewResponse{Items: make([]gen.JobPreviewItem, 0, len(planned))}
	for i := range planned {
		it := planned[i]
		item := gen.JobPreviewItem{
			Appid:    it.Appid,
			Endpoint: it.endpoint(),
			Payload:  it.payload(),
			Valid:    it.valid(),
			Problems: []string{},
		}
		if it.NickName != "" {
			nick := it.NickName
			item.NickName = &nick
		}
		item.Problems = append(item.Problems, it.Problems...)
		if item.Valid {
			out.ValidCount++
		} else {
			out.InvalidCount++
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

// Create 创建批量作业。全部无效时返回 core.Validation 并附第一条失败原因。
//
// 单步作业（撤回/加急/回退/服务状态）的附加参数请用 CreateSingle。
func (s *JobService) Create(ctx context.Context, req gen.JobCreateRequest, actor string) (*gen.Job, error) {
	return s.create(ctx, req, SingleOptions{}, actor)
}

// CreateSingle 创建带附加参数的单步作业（契约的 JobCreateRequest 没有这些字段）。
func (s *JobService) CreateSingle(ctx context.Context, req gen.JobCreateRequest, single SingleOptions, actor string) (*gen.Job, error) {
	return s.create(ctx, req, single, actor)
}

// create 创建作业的公共实现。
func (s *JobService) create(ctx context.Context, req gen.JobCreateRequest, single SingleOptions, actor string) (*gen.Job, error) {
	if s.repos() == nil {
		return nil, core.Internal(errors.New("数据访问未初始化"))
	}
	pc, planned, err := s.buildPlan(ctx, req, single)
	if err != nil {
		return nil, err
	}

	validCount := 0
	firstProblem := ""
	for i := range planned {
		if planned[i].valid() {
			validCount++
			continue
		}
		if firstProblem == "" && len(planned[i].Problems) > 0 {
			firstProblem = fmt.Sprintf("%s：%s", planned[i].Appid, planned[i].Problems[0])
		}
	}
	if validCount == 0 {
		return nil, core.Validation("全部 %d 个小程序都没有通过校验，作业未创建：%s", len(planned), firstProblem)
	}

	jobID, err := newJobID()
	if err != nil {
		return nil, core.Internal(err)
	}
	dryRun := req.DryRun != nil && *req.DryRun
	now := time.Now()

	appids := make([]string, 0, len(planned))
	for i := range planned {
		appids = append(appids, planned[i].Appid)
	}
	job := &model.BatchJob{
		ID:          jobID,
		Type:        pc.jobType,
		Status:      model.JobStatusPending,
		DryRun:      dryRun,
		Concurrency: concurrencyOf(req),
		Payload:     pc.opts.toPayload(pc.jobType, appids, req.Selection),
		Note:        derefString(req.Note),
		CreatedBy:   actor,
	}
	if dryRun {
		// 试运行只落库不执行：直接落终态，引擎只扫描 pending/running，不会捡起它。
		job.Status = model.JobStatusSucceeded
		job.FinishedAt = &now
	}

	conflicts := conflictStepsFor(pc.jobType)
	items := make([]model.BatchJobItem, 0, len(planned))
	for i := range planned {
		it := planned[i]
		status := model.ItemStatusPending
		reason := ""
		class := model.ErrorClass("")
		switch {
		case !it.valid():
			status = model.ItemStatusSkipped
			reason = truncateErrmsg(strings.Join(it.Problems, "；"))
			class = model.ClassPermanent
		case len(conflicts) > 0:
			busy, cerr := s.repos().Jobs.HasActiveItem(ctx, it.Appid, conflicts)
			if cerr != nil {
				return nil, core.Internal(cerr)
			}
			if busy {
				status = model.ItemStatusSkipped
				reason = conflictNote
				class = model.ClassPermanent
			}
		}

		for _, st := range it.Steps {
			item := model.BatchJobItem{
				JobID:       jobID,
				Step:        st.Step,
				Appid:       it.Appid,
				Status:      status,
				Request:     st.Payload,
				ErrorClass:  class,
				Errmsg:      reason,
				UserVersion: jsonStringOf(st.Payload["user_version"]),
			}
			if dryRun {
				if status == model.ItemStatusPending {
					item.Status = model.ItemStatusSucceeded
					item.Errmsg = "试运行：仅落库校验，未调用微信接口"
					item.ErrorClass = ""
					item.Response = model.JSONMap{"dry_run": true, "endpoint": st.Endpoint,
						"note": "试运行不调用微信；正式创建作业后会按此请求体执行"}
				}
			}
			items = append(items, item)
		}
	}

	job.Total = len(items)

	if err := s.repos().Jobs.Create(ctx, job, items); err != nil {
		return nil, core.Internal(err)
	}
	// 计数立刻与子项对齐（引擎最终汇总时会再算一次）：无效项 / 在途冲突项都是 skipped，
	// 若不在创建时算一遍，详情页会显示「total=2 但 skipped=0」。
	if err := s.repos().Jobs.UpdateCounters(ctx, jobID); err != nil {
		return nil, core.Internal(err)
	}
	job.Total, job.Succeeded, job.Failed, job.Skipped = countItems(items)
	return s.toGenJob(job, items), nil
}

// countItems 按状态统计子项（口径与 repo.Jobs.UpdateCounters 一致）。
func countItems(items []model.BatchJobItem) (total, succeeded, failed, skipped int) {
	total = len(items)
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
	return total, succeeded, failed, skipped
}

// List 分页查询作业。
func (s *JobService) List(ctx context.Context, params gen.ListJobsParams) (*gen.JobListResponse, error) {
	if s.repos() == nil {
		return nil, core.Internal(errors.New("数据访问未初始化"))
	}
	page, pageSize := pageOf(params.Page), pageSizeOf(params.PageSize)
	var jobType model.JobType
	if params.Type != nil {
		jobType = model.JobType(*params.Type)
	}
	var status model.JobStatus
	if params.Status != nil {
		status = model.JobStatus(*params.Status)
	}
	jobs, total, err := s.repos().Jobs.List(ctx, jobType, status, page, pageSize)
	if err != nil {
		return nil, core.Internal(err)
	}
	out := &gen.JobListResponse{
		Items:    make([]gen.Job, 0, len(jobs)),
		Total:    int(total),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range jobs {
		out.Items = append(out.Items, *s.toGenJob(&jobs[i], nil))
	}
	return out, nil
}

// Get 读取作业详情（含逐状态进度统计）。
func (s *JobService) Get(ctx context.Context, id string) (*gen.Job, error) {
	job, items, err := s.jobWithItems(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.toGenJob(job, items), nil
}

// Items 分页查询作业逐项执行明细（附小程序昵称，便于页面识别）。
func (s *JobService) Items(ctx context.Context, id string, params gen.ListJobItemsParams) (*gen.JobItemListResponse, error) {
	if _, _, err := s.jobWithItems(ctx, id); err != nil {
		return nil, err
	}
	page, pageSize := pageOf(params.Page), pageSizeOf(params.PageSize)
	var status model.JobItemStatus
	if params.Status != nil {
		status = model.JobItemStatus(*params.Status)
	}
	appid := derefString(params.Appid)
	items, total, err := s.repos().JobItems.ListByJob(ctx, id, status, appid, page, pageSize)
	if err != nil {
		return nil, core.Internal(err)
	}
	nicks := s.nickNames(ctx, items)
	out := &gen.JobItemListResponse{
		Items:    make([]gen.JobItem, 0, len(items)),
		Total:    int(total),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range items {
		out.Items = append(out.Items, toGenJobItem(&items[i], nicks[items[i].Appid]))
	}
	return out, nil
}

// Start 启动作业（pending → running）。已结束/已在执行的作业会被拒绝。
func (s *JobService) Start(ctx context.Context, id string) (*gen.Job, error) {
	job, _, err := s.jobWithItems(ctx, id)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case model.JobStatusPending, model.JobStatusInterrupted:
	case model.JobStatusRunning:
		return nil, core.Conflict("作业已在执行中，无需重复启动")
	case model.JobStatusPaused:
		return nil, core.Conflict("作业处于暂停状态：请使用「恢复」继续执行")
	default:
		return nil, core.Conflict("作业已结束（%s），不能启动：如需重跑请新建作业", job.Status)
	}
	if s.engine == nil {
		return nil, ErrEngineNotRunning
	}
	if err := s.engine.BeginJob(ctx, id); err != nil {
		return nil, core.Internal(err)
	}
	return s.Get(ctx, id)
}

// Pause 暂停作业（已发出的请求不打断，后续子项不再领取）。
func (s *JobService) Pause(ctx context.Context, id string) (*gen.Job, error) {
	job, _, err := s.jobWithItems(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.Status.Terminal() {
		return nil, core.Conflict("作业已结束（%s），无法暂停", job.Status)
	}
	if s.engine == nil {
		return nil, ErrEngineNotRunning
	}
	if err := s.engine.PauseJob(ctx, id); err != nil {
		return nil, core.Internal(err)
	}
	return s.Get(ctx, id)
}

// Resume 恢复暂停的作业。
func (s *JobService) Resume(ctx context.Context, id string) (*gen.Job, error) {
	job, _, err := s.jobWithItems(ctx, id)
	if err != nil {
		return nil, err
	}
	switch job.Status {
	case model.JobStatusPaused, model.JobStatusInterrupted:
	case model.JobStatusRunning:
		return nil, core.Conflict("作业已在执行中，无需恢复")
	default:
		return nil, core.Conflict("作业状态为 %s，不能恢复：只有暂停/中断的作业可以恢复", job.Status)
	}
	if s.engine == nil {
		return nil, ErrEngineNotRunning
	}
	if err := s.engine.ResumeJob(ctx, id); err != nil {
		return nil, core.Internal(err)
	}
	return s.Get(ctx, id)
}

// Cancel 取消作业：未开始的子项标记取消，已完成的保留结果。
func (s *JobService) Cancel(ctx context.Context, id string) (*gen.Job, error) {
	job, _, err := s.jobWithItems(ctx, id)
	if err != nil {
		return nil, err
	}
	if job.Status == model.JobStatusCanceled {
		return s.Get(ctx, id) // 幂等：重复取消直接返回当前状态
	}
	if job.Status.Terminal() {
		return nil, core.Conflict("作业已结束（%s），无法取消", job.Status)
	}
	if s.engine == nil {
		return nil, ErrEngineNotRunning
	}
	if err := s.engine.CancelJob(ctx, id); err != nil {
		return nil, core.Internal(err)
	}
	return s.Get(ctx, id)
}

// RetryFailed 重试失败项；重试后作业重新置为 running（由引擎完成）。
func (s *JobService) RetryFailed(ctx context.Context, id string) (*gen.Job, error) {
	if _, _, err := s.jobWithItems(ctx, id); err != nil {
		return nil, err
	}
	if s.engine == nil {
		return nil, ErrEngineNotRunning
	}
	n, err := s.engine.RetryFailedItems(ctx, id)
	if err != nil {
		return nil, core.Internal(err)
	}
	if n == 0 {
		return nil, core.Conflict("该作业没有失败的子项可重试")
	}
	return s.Get(ctx, id)
}

// jobWithItems 读取作业与其全部子项（不存在返回 core.NotFound）。
func (s *JobService) jobWithItems(ctx context.Context, id string) (*model.BatchJob, []model.BatchJobItem, error) {
	if s.repos() == nil {
		return nil, nil, core.Internal(errors.New("数据访问未初始化"))
	}
	if strings.TrimSpace(id) == "" {
		return nil, nil, core.Validation("作业 ID 不能为空")
	}
	job, err := s.repos().Jobs.Get(ctx, id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, nil, core.NotFound("作业不存在（id=%s）", id)
		}
		return nil, nil, core.Internal(err)
	}
	items, err := s.repos().JobItems.AllByJob(ctx, id)
	if err != nil {
		return nil, nil, core.Internal(err)
	}
	return job, items, nil
}

// nickNames 批量取小程序昵称（明细列表展示用）。
func (s *JobService) nickNames(ctx context.Context, items []model.BatchJobItem) map[string]string {
	if len(items) == 0 || s.repos() == nil {
		return nil
	}
	seen := map[string]bool{}
	appids := make([]string, 0, len(items))
	for i := range items {
		if items[i].Appid == "" || seen[items[i].Appid] {
			continue
		}
		seen[items[i].Appid] = true
		appids = append(appids, items[i].Appid)
	}
	list, err := s.repos().Authorizers.ByAppids(ctx, appids)
	if err != nil {
		return nil
	}
	out := make(map[string]string, len(list))
	for i := range list {
		out[list[i].Appid] = list[i].NickName
	}
	return out
}

// toGenJob 把模型转成契约类型；items 非 nil 时一并算出 pending/running/waiting。
func (s *JobService) toGenJob(job *model.BatchJob, items []model.BatchJobItem) *gen.Job {
	out := &gen.Job{
		Id:         job.ID,
		Type:       gen.JobType(job.Type),
		Status:     gen.JobStatus(job.Status),
		DryRun:     job.DryRun,
		Total:      job.Total,
		Succeeded:  job.Succeeded,
		Failed:     job.Failed,
		Skipped:    job.Skipped,
		CreatedAt:  job.CreatedAt,
		StartedAt:  job.StartedAt,
		FinishedAt: job.FinishedAt,
	}
	if job.Concurrency > 0 {
		v := job.Concurrency
		out.Concurrency = &v
	}
	if job.Payload != nil {
		payload := map[string]any(job.Payload)
		out.Payload = &payload
	}
	if strings.TrimSpace(job.Note) != "" {
		note := job.Note
		out.Note = &note
	}
	if strings.TrimSpace(job.ErrorNote) != "" {
		note := job.ErrorNote
		out.ErrorNote = &note
	}
	if strings.TrimSpace(job.CreatedBy) != "" {
		by := job.CreatedBy
		out.CreatedBy = &by
	}
	if items != nil {
		summary := batch.Summarize(items)
		pending, running, waiting := summary.Pending, summary.Running, summary.Waiting
		out.Pending = &pending
		out.Running = &running
		out.Waiting = &waiting
	}
	return out
}

// toGenJobItem 把子项转成契约类型。
func toGenJobItem(item *model.BatchJobItem, nickName string) gen.JobItem {
	out := gen.JobItem{
		Id:         int64(item.ID),
		JobId:      item.JobID,
		Step:       gen.JobStep(item.Step),
		Appid:      item.Appid,
		Status:     gen.JobItemStatus(item.Status),
		Attempt:    item.Attempt,
		NextRunAt:  item.NextRunAt,
		StartedAt:  item.StartedAt,
		FinishedAt: item.FinishedAt,
	}
	if strings.TrimSpace(item.UserVersion) != "" {
		v := item.UserVersion
		out.UserVersion = &v
	}
	if item.WxAuditID != 0 {
		v := item.WxAuditID
		out.WxAuditId = &v
	}
	if item.Errcode != 0 {
		v := item.Errcode
		out.Errcode = &v
	}
	if strings.TrimSpace(item.Errmsg) != "" {
		v := item.Errmsg
		out.Errmsg = &v
	}
	if strings.TrimSpace(string(item.ErrorClass)) != "" {
		v := gen.ErrorClass(item.ErrorClass)
		out.ErrorClass = &v
	}
	if item.Request != nil {
		v := map[string]any(item.Request)
		out.Request = &v
	}
	if item.Response != nil {
		v := map[string]any(item.Response)
		out.Response = &v
	}
	if nickName != "" {
		v := nickName
		out.NickName = &v
	}
	return out
}

// concurrencyOf 归一化并发参数（0 表示用平台设置）。
func concurrencyOf(req gen.JobCreateRequest) int {
	if req.Concurrency == nil || *req.Concurrency <= 0 {
		return 0
	}
	if *req.Concurrency > 8 {
		return 8
	}
	return *req.Concurrency
}

// conflictStepsFor 返回该作业类型需要在创建时做在途冲突检查的步骤。
//
// commit / submit_audit 必须检查（同一小程序提交并发限制，9402202）；release 同样检查，
// 避免同一个小程序的两个作业同时发布。privacy_check 只是等待步骤，不参与冲突判断。
func conflictStepsFor(t model.JobType) []model.JobStep {
	switch t {
	case model.JobTypeCommit:
		return []model.JobStep{model.StepCommit}
	case model.JobTypeSubmitAudit:
		return []model.JobStep{model.StepSubmitAudit}
	case model.JobTypeRelease:
		return []model.JobStep{model.StepRelease}
	case model.JobTypePipeline:
		return []model.JobStep{model.StepCommit, model.StepSubmitAudit, model.StepRelease}
	}
	return nil
}

// pageOf / pageSizeOf 归一化分页参数（与 repo 层口径一致）。
func pageOf(page *gen.Page) int {
	if page == nil || *page < 1 {
		return 1
	}
	return *page
}

func pageSizeOf(pageSize *gen.PageSize) int {
	if pageSize == nil || *pageSize < 1 {
		return defaultPageSize
	}
	if *pageSize > maxPageSize {
		return maxPageSize
	}
	return *pageSize
}

// newJobID 生成 UUIDv4 形式的作业 ID（batch_jobs.id 长度 36）。
func newJobID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("生成作业 ID 失败: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// truncateErrmsg 按 batch_job_items.errmsg 的容量截断（引擎侧按 900 截断，这里保持一致）。
func truncateErrmsg(s string) string {
	const limit = 900
	if len(s) <= limit {
		return s
	}
	return s[:limit]
}

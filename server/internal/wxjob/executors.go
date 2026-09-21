package wxjob

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"wx-platform/server/internal/batch"
	"wx-platform/server/internal/core"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
	"wx-platform/server/internal/wxapi"
)

// 默认等待上限（与 config / core.SettingService 的兜底值一致）。
const (
	defaultPrivacyCheckMaxWait = 600    // 隐私检测最长等待：10 分钟
	defaultAuditResultMaxWait  = 604800 // 审核结果最长等待：7 天
)

// 等待轮询间隔（切片后重新排队，保证暂停/取消能及时生效）。
const (
	privacyCheckInterval = 30 * time.Second // 官方建议约 1 分钟后重试，取 30 秒切片
	auditResultInterval  = 60 * time.Second
)

// 等待累计秒数写在子项 response 里的键名（引擎会用返回值覆盖 response，两者必须一致）。
const (
	keyPrivacyWaitSeconds = "privacy_wait_seconds"
	keyReleaseWaitSeconds = "release_wait_seconds"
)

// 撤回审核的官方额度（每账号每天 ≤5 次、每月 ≤10 次，超限 87013）。
const (
	maxUndoPerDay   = 5
	maxUndoPerMonth = 10
)

// errWeChatNotReady 微信能力未注入。
var errWeChatNotReady = errors.New("微信能力未注入（未配置第三方平台凭据）：请先在 server/.env 配置 " +
	"WX_COMPONENT_APPID / WX_COMPONENT_APPSECRET / VERIFY_TOKEN / ENCODING_AES_KEY 后重启服务")

// ToWeChatError 把微信客户端返回的错误统一转成 core.WeChatError
// （Class=model.ClassifyErrcode、Text=model.ErrcodeText、Hint=model.ErrcodeRule 的建议）。
//
// 第二个返回值为 false 表示 err 不是微信侧错误（数据库、令牌管理器等）。
func ToWeChatError(err error) (*core.WeChatError, bool) {
	apiErr, ok := wxapi.IsAPIError(err)
	if !ok {
		return nil, false
	}
	class, text, hint, _ := model.ErrcodeRule(apiErr.Errcode)
	return &core.WeChatError{
		Endpoint: apiErr.Endpoint,
		Method:   apiErr.Method,
		Appid:    apiErr.Appid,
		Errcode:  apiErr.Errcode,
		Errmsg:   apiErr.Errmsg,
		Class:    class,
		Text:     text,
		Hint:     hint,
	}, true
}

// failureResult 依据错误类型生成失败结果（保留 errcode、分类与中文解释）。
func failureResult(err error, fallback model.ErrorClass, note string) batch.Result {
	if wc, ok := ToWeChatError(err); ok {
		return batch.Result{
			Outcome: batch.OutcomeFailed,
			Class:   wc.Class,
			Errcode: wc.Errcode,
			Errmsg:  wc.Errmsg,
			Note:    joinNote(wc.Text, wc.Hint, note),
		}
	}
	class := fallback
	switch {
	case errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrValidation), errors.Is(err, repo.ErrNotFound):
		class = model.ClassPermanent
	case errors.Is(err, core.ErrNotConfigured), errors.Is(err, errWeChatNotReady):
		class = model.ClassEnvironment
	}
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	return batch.Result{Outcome: batch.OutcomeFailed, Class: class, Errmsg: truncateErrmsg(msg), Note: joinNote(msg, note)}
}

// permanentResult 永久失败（重试无意义，必须改配置或改数据）。
func permanentResult(err error) batch.Result {
	res := failureResult(err, model.ClassPermanent, "")
	res.Class = model.ClassPermanent
	return res
}

// skippedResult 跳过（如已有在审版本）。
func skippedResult(note string) batch.Result {
	return batch.Result{Outcome: batch.OutcomeSkipped, Class: model.ClassPermanent, Note: note}
}

// joinNote 拼接非空片段。
func joinNote(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			out = append(out, strings.TrimSpace(part))
		}
	}
	return strings.Join(out, "；")
}

// errcodeLabel 返回「中文说明（errcode N）」：作业明细里既能读中文，也能对照官方返回码。
func errcodeLabel(code int) string {
	return fmt.Sprintf("%s（errcode %d）", model.ErrcodeText(code), code)
}

// hintOf 返回返回码的处置建议。
func hintOf(code int) string {
	_, _, hint, _ := model.ErrcodeRule(code)
	return hint
}

// ---- 公共前置条件 ----

// readyForWx 检查依赖是否齐备；微信能力未注入时返回「环境类」失败（重试无意义，等人配置凭据）。
func (s *JobService) readyForWx() (batch.Result, bool) {
	if s.repos() == nil {
		return permanentResult(core.Internal(errors.New("数据访问未初始化"))), false
	}
	if s.env.Wx == nil || s.env.Tokens == nil {
		return batch.Result{
			Outcome: batch.OutcomeFailed,
			Class:   model.ClassEnvironment,
			Errmsg:  errWeChatNotReady.Error(),
			Note:    errWeChatNotReady.Error(),
		}, false
	}
	return batch.Result{}, true
}

// authorizer 读取小程序的授权记录（缺失视为永久失败）。
func (s *JobService) authorizer(ctx context.Context, appid string) (*model.Authorizer, error) {
	if s.repos() == nil {
		return nil, core.Internal(errors.New("数据访问未初始化"))
	}
	a, err := s.repos().Authorizers.Get(ctx, appid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, core.NotFound("小程序 %s 没有授权记录：请先在「小程序」页完成授权", appid)
		}
		return nil, core.Internal(err)
	}
	return a, nil
}

// authorizerToken 取授权方令牌（自动刷新；授权方接口一律用它，用错会报 61014）。
func (s *JobService) authorizerToken(ctx context.Context, appid string) (string, error) {
	if s.env == nil || s.env.Tokens == nil {
		return "", errWeChatNotReady
	}
	token, err := s.env.Tokens.AuthorizerToken(ctx, appid)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token) == "" {
		return "", errors.New("授权方令牌为空：请让商家重新授权，或执行「重新拉取令牌」")
	}
	return token, nil
}

// planFromJob 依据作业载荷重建参数上下文（执行器路径：不依赖内存状态，重启后可复现）。
func (s *JobService) planFromJob(ctx context.Context, job *model.BatchJob) (*planContext, error) {
	if job == nil {
		return nil, core.Internal(errors.New("作业为空"))
	}
	pc := &planContext{
		repos:      s.repos(),
		jobType:    model.JobType(job.Type),
		globalVars: s.globalExtVars(ctx),
		tplCache:   map[int64]*model.CodeTemplate{},
		tplErr:     map[int64]error{},
	}
	opts, err := optionsFromPayload(job.Payload)
	if err != nil {
		return nil, err
	}
	pc.opts = opts
	return pc, nil
}

// recordItemResponse 把「等待累计」这类自有信息写回子项 response。
//
// 引擎随后会用执行器返回的 Response 覆盖该字段，因此返回值里必须包含同样的键。
func (s *JobService) recordItemResponse(ctx context.Context, itemID uint, resp model.JSONMap) error {
	if s.repos() == nil || itemID == 0 {
		return nil
	}
	return s.repos().JobItems.UpdateFields(ctx, itemID, map[string]any{"response": resp})
}

// jobHasStep 判断作业类型是否包含某步骤。
func jobHasStep(job *model.BatchJob, step model.JobStep) bool {
	if job == nil {
		return false
	}
	for _, s := range model.JobType(job.Type).Steps() {
		if s == step {
			return true
		}
	}
	return false
}

// 编译期断言：五个执行器都必须实现 batch.Executor。
var (
	_ batch.Executor = (*CommitExecutor)(nil)
	_ batch.Executor = (*PrivacyCheckExecutor)(nil)
	_ batch.Executor = (*SubmitAuditExecutor)(nil)
	_ batch.Executor = (*ReleaseExecutor)(nil)
	_ batch.Executor = (*SingleStepExecutor)(nil)
)

// ---- 上传代码 ----

// CommitExecutor 执行 /wxa/commit（上传代码并生成体验版）。
type CommitExecutor struct{ svc *JobService }

// NewCommitExecutor 构造上传代码执行器。
func NewCommitExecutor(svc *JobService) *CommitExecutor { return &CommitExecutor{svc: svc} }

// Step 该执行器负责的步骤。
func (e *CommitExecutor) Step() model.JobStep { return model.StepCommit }

// Execute 执行一个子项。
func (e *CommitExecutor) Execute(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	return e.svc.runCommit(ctx, job, item)
}

func (s *JobService) runCommit(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	if res, ok := s.readyForWx(); !ok {
		return res
	}
	pc, err := s.planFromJob(ctx, job)
	if err != nil {
		return permanentResult(err)
	}
	a, err := s.authorizer(ctx, item.Appid)
	if err != nil {
		return permanentResult(err)
	}
	if problems := authorizerProblems(a); len(problems) > 0 {
		return permanentResult(errors.New(strings.Join(problems, "；")))
	}

	_, req, problems := pc.commitStep(ctx, a, seqOf(job.Payload, item.Appid))
	if len(problems) > 0 {
		return permanentResult(fmt.Errorf("上传代码参数校验失败：%s", strings.Join(problems, "；")))
	}

	token, err := s.authorizerToken(ctx, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}
	if err := s.env.Wx.Commit(ctx, token, item.Appid, req.toAPI()); err != nil {
		return commitFailure(err)
	}
	return batch.Result{
		Outcome:     batch.OutcomeSucceeded,
		Request:     req.toMap(),
		Response:    model.JSONMap{"errcode": 0, "errmsg": "ok", "note": "上传成功，已生成体验版；必须等隐私检测任务结束后再提审（官方 61039）"},
		UserVersion: req.UserVersion,
	}
}

// commitFailure 按官方要求对 /wxa/commit 的失败分类。
//
//   - 9402202：同一小程序提交过频（并发限制），标 rate_limited 且**不重试**，提示串行；
//   - 85013/85014/85043-85048/85310-85312/9402203：永久失败，按 model.ErrcodeText 给中文原因；
//   - 42001/40001/40014：令牌失效，标 token_expired 并交给用户重试（平台会自动刷新令牌）。
//
// 这里**不发起提审**：上传后必须等隐私检测任务结束，否则会反复触发 61039（由引擎的步骤门控保证顺序）。
func commitFailure(err error) batch.Result {
	res := failureResult(err, model.ClassRetryable, "")
	apiErr, ok := wxapi.IsAPIError(err)
	if !ok {
		return res
	}
	switch {
	case apiErr.Errcode == 9402202:
		res.Class = model.ClassRateLimited
		res.Note = joinNote(errcodeLabel(9402202),
			"同一小程序的上传/提审必须串行：请等上一次操作完成后再试，并避免把上传代码与提审放进同一个重试循环（官方 61039）")
	case isPermanentCommitCode(apiErr.Errcode):
		res.Class = model.ClassPermanent
		res.Note = joinNote(errcodeLabel(apiErr.Errcode), hintOf(apiErr.Errcode))
	case isTokenExpiredCode(apiErr.Errcode):
		res.Class = model.ClassTokenExpired
		res.Note = joinNote(errcodeLabel(apiErr.Errcode),
			"授权方令牌已失效：平台会自动刷新令牌，请稍后重试；若持续失败请让商家重新授权")
	}
	return res
}

// isPermanentCommitCode 判断上传代码的永久失败码（重试无意义，必须改配置/模板）。
func isPermanentCommitCode(code int) bool {
	switch {
	case code == 85013 || code == 85014 || code == 9402203:
		return true
	case code >= 85043 && code <= 85048:
		return true
	case code >= 85310 && code <= 85312:
		return true
	}
	return false
}

// isTokenExpiredCode 判断令牌失效码。
func isTokenExpiredCode(code int) bool { return code == 42001 || code == 40001 || code == 40014 }

// ---- 隐私检测 ----

// PrivacyCheckExecutor 执行 /wxa/security/get_code_privacy_info（提审前置的隐私接口检测）。
type PrivacyCheckExecutor struct{ svc *JobService }

// NewPrivacyCheckExecutor 构造隐私检测执行器。
func NewPrivacyCheckExecutor(svc *JobService) *PrivacyCheckExecutor {
	return &PrivacyCheckExecutor{svc: svc}
}

// Step 该执行器负责的步骤。
func (e *PrivacyCheckExecutor) Step() model.JobStep { return model.StepPrivacyCheck }

// Execute 执行一个子项。
func (e *PrivacyCheckExecutor) Execute(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	return e.svc.runPrivacyCheck(ctx, job, item)
}

func (s *JobService) runPrivacyCheck(ctx context.Context, _ *model.BatchJob, item *model.BatchJobItem) batch.Result {
	if res, ok := s.readyForWx(); !ok {
		return res
	}
	token, err := s.authorizerToken(ctx, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}
	info, err := s.env.Wx.GetCodePrivacyInfo(ctx, token, item.Appid)
	if err != nil {
		if apiErr, ok := wxapi.IsAPIError(err); ok && apiErr.Errcode == 61039 {
			return s.privacyWaiting(ctx, item)
		}
		res := failureResult(err, model.ClassRetryable, "")
		if apiErr, ok := wxapi.IsAPIError(err); ok && apiErr.Errcode == 61040 {
			res.Class = model.ClassPermanent
			res.Note = joinNote(model.ErrcodeText(61040),
				"请在 ext_json 里声明 requiredPrivateInfos 并先在小程序侧申请对应隐私接口权限；"+
					"若确认不使用隐私接口，可在提审配置里勾选 privacy_api_not_use")
		}
		return res
	}

	waited := jsonIntOf(item.Response[keyPrivacyWaitSeconds])
	resp := model.JSONMap{
		"without_auth_list":   info.WithoutAuthList,
		"without_conf_list":   info.WithoutConfList,
		keyPrivacyWaitSeconds: waited,
	}
	if len(info.WithoutAuthList) > 0 || len(info.WithoutConfList) > 0 {
		_ = s.recordItemResponse(ctx, item.ID, resp)
		return batch.Result{
			Outcome:  batch.OutcomeFailed,
			Class:    model.ClassPermanent,
			Errcode:  61040,
			Errmsg:   model.ErrcodeText(61040),
			Response: resp,
			Note: fmt.Sprintf("代码使用了没有权限或未在 ext_json 声明的隐私接口（官方 61040）：未授权 %s；未配置 %s。"+
				"请为小程序申请该隐私接口权限，或在 ext_json 里声明 requiredPrivateInfos；"+
				"若确认不使用隐私接口，可在提审配置里勾选 privacy_api_not_use。",
				listText(info.WithoutAuthList), listText(info.WithoutConfList)),
		}
	}
	return batch.Result{Outcome: batch.OutcomeSucceeded, Response: resp}
}

// privacyWaiting 处理 61039（检测任务未完成）：累计等待时长，超上限则永久失败。
func (s *JobService) privacyWaiting(ctx context.Context, item *model.BatchJobItem) batch.Result {
	maxWait := s.settingInt(model.SettingPrivacyCheckMaxWait, defaultPrivacyCheckMaxWait)
	waited := jsonIntOf(item.Response[keyPrivacyWaitSeconds]) + int(privacyCheckInterval.Seconds())
	resp := model.JSONMap{
		"errcode":             61039,
		"errmsg":              model.ErrcodeText(61039),
		keyPrivacyWaitSeconds: waited,
		"max_wait_seconds":    maxWait,
	}
	if err := s.recordItemResponse(ctx, item.ID, resp); err != nil {
		resp["record_error"] = err.Error()
	}
	if waited > maxWait {
		return batch.Result{
			Outcome:  batch.OutcomeFailed,
			Class:    model.ClassPermanent,
			Errcode:  61039,
			Errmsg:   model.ErrcodeText(61039),
			Response: resp,
			Note: fmt.Sprintf("隐私检测任务长时间未结束（已等待 %d 秒，上限 %d 秒）：请勿与上传代码一起重试（官方 61039 说明：提交代码后需等检测任务结束才能提审，"+
				"把上传与提审放进同一个重试循环只会反复触发 61039）。建议稍后在作业详情页点「重试失败项」，或重新提审。", waited, maxWait),
		}
	}
	return batch.Result{
		Outcome:    batch.OutcomeWaiting,
		Class:      model.ClassRetryable,
		Errcode:    61039,
		Errmsg:     model.ErrcodeText(61039),
		RetryAfter: privacyCheckInterval,
		Response:   resp,
		Note: fmt.Sprintf("隐私接口检测任务尚未完成（61039），已等待 %d 秒（上限 %d 秒），%d 秒后自动重试",
			waited, maxWait, int(privacyCheckInterval.Seconds())),
	}
}

// ---- 提审 ----

// SubmitAuditExecutor 执行 /wxa/submit_audit（提交代码审核）。
type SubmitAuditExecutor struct{ svc *JobService }

// NewSubmitAuditExecutor 构造提审执行器。
func NewSubmitAuditExecutor(svc *JobService) *SubmitAuditExecutor {
	return &SubmitAuditExecutor{svc: svc}
}

// Step 该执行器负责的步骤。
func (e *SubmitAuditExecutor) Step() model.JobStep { return model.StepSubmitAudit }

// Execute 执行一个子项。
func (e *SubmitAuditExecutor) Execute(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	return e.svc.runSubmitAudit(ctx, job, item)
}

func (s *JobService) runSubmitAudit(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	if res, ok := s.readyForWx(); !ok {
		return res
	}
	pc, err := s.planFromJob(ctx, job)
	if err != nil {
		return permanentResult(err)
	}
	a, err := s.authorizer(ctx, item.Appid)
	if err != nil {
		return permanentResult(err)
	}
	if problems := authorizerProblems(a); len(problems) > 0 {
		return permanentResult(errors.New(strings.Join(problems, "；")))
	}

	// 前置：本作业内必须已成功上传代码（官方 85086；direct_commit 来源可跳过）。
	commitItem, blocked := s.requireCommit(ctx, job, a, item.Appid)
	if blocked != nil {
		return *blocked
	}

	_, req, problems := pc.submitAuditStep(ctx, a, seqOf(job.Payload, item.Appid))
	if len(problems) > 0 {
		return permanentResult(fmt.Errorf("提审参数校验失败：%s", strings.Join(problems, "；")))
	}

	// 额度是服务商级、旗下小程序共用：先看本地缓存，避免对每个小程序都白跑一次微信调用。
	if res, ok := s.quotaGuard(ctx); !ok {
		return res
	}

	token, err := s.authorizerToken(ctx, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}

	// 类目必须已在该小程序上配置好（官方 85008），getAllCategoryName 是唯一权威来源。
	cats, err := s.env.Wx.GetAllCategoryName(ctx, token, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}
	if missing := missingCategories(req.Items, cats); len(missing) > 0 {
		return batch.Result{
			Outcome: batch.OutcomeFailed,
			Class:   model.ClassPermanent,
			Errcode: 85008,
			Errmsg:  model.ErrcodeText(85008),
			Note: "提审配置里的类目在该小程序上不存在或未通过审核（官方 85008）：" + strings.Join(missing, "、") +
				"；请在小程序侧添加该类目并等审核通过，或改用 getAllCategoryName 返回的类目（一级/二级/三级名称与 id 都要一致）。",
		}
	}

	resp, err := s.env.Wx.SubmitAudit(ctx, token, item.Appid, req.toAPI())
	if err != nil {
		return submitAuditFailure(err)
	}

	response := model.JSONMap{"auditid": resp.AuditID, "errcode": 0, "errmsg": "ok"}
	if note := s.recordAuditSubmit(ctx, item.Appid, resp.AuditID, commitItem); note != "" {
		response["audit_record_note"] = note
	}
	return batch.Result{
		Outcome:   batch.OutcomeSucceeded,
		WxAuditID: resp.AuditID,
		Request:   req.toMap(),
		Response:  response,
	}
}

// quotaGuard 提审前的额度检查（服务商级共享）。
//
// Rest == nil 表示尚未探测过额度，此时不拦截（由微信侧 85085 兜底）。
// 额度为 0 时返回 Fatal=true：额度是服务商级共享的，必须暂停整个作业而不是逐个小程序失败。
func (s *JobService) quotaGuard(ctx context.Context) (batch.Result, bool) {
	if s.env == nil || s.env.Platform == nil {
		return batch.Result{}, true
	}
	quota := s.env.Platform.CachedQuota(ctx)
	if quota == nil || quota.Rest == nil || *quota.Rest > 0 {
		return batch.Result{}, true
	}
	limit := "未知"
	if quota.Limit != nil {
		limit = fmt.Sprintf("%d", *quota.Limit)
	}
	note := fmt.Sprintf("提审额度已用尽（85085）：本月剩余 %d（上限 %s），该额度为服务商级、旗下小程序共用；"+
		"请在「小程序服务商助手」申请临时额度后，回到本页「恢复」作业。已暂停本作业，避免对每个小程序都白跑一次微信调用。",
		*quota.Rest, limit)
	return batch.Result{
		Outcome:   batch.OutcomeFailed,
		Class:     model.ClassRateLimited,
		Errcode:   85085,
		Errmsg:    model.ErrcodeText(85085),
		Note:      note,
		Fatal:     true,
		FatalNote: note,
	}, false
}

// submitAuditFailure 提审失败分类：额度耗尽 → 暂停整个作业；已有在审版本 → 跳过该项。
func submitAuditFailure(err error) batch.Result {
	res := failureResult(err, model.ClassRetryable, "")
	apiErr, ok := wxapi.IsAPIError(err)
	if !ok {
		return res
	}
	switch apiErr.Errcode {
	case 85085:
		res.Class = model.ClassRateLimited
		res.Fatal = true
		res.FatalNote = "提审额度已用尽（85085）：服务商本月提审额度已用完，旗下小程序共用；" +
			"请在「小程序服务商助手」申请临时额度后回到本页「恢复」作业（已暂停本作业）"
		res.Note = res.FatalNote
	case 85009:
		return skippedResult("该小程序已有正在审核的版本（85009）：请等待审核完成或先撤回审核；本项已跳过，不消耗提审额度")
	case 85086:
		res.Class = model.ClassPermanent
		res.Note = joinNote(errcodeLabel(85086), "请先执行上传代码作业，或改用 pipeline 作业类型")
	case 61040:
		res.Class = model.ClassPermanent
		res.Note = joinNote(errcodeLabel(61040),
			"请在 ext_json 里声明 requiredPrivateInfos 并申请对应权限，或声明 privacy_api_not_use")
	}
	return res
}

// recordAuditSubmit 提审成功后写审核台账（status=2 审核中，Source=api）；失败只返回说明，不影响提审结果。
func (s *JobService) recordAuditSubmit(ctx context.Context, appid string, auditID int64, commitItem *model.BatchJobItem) string {
	if s.repos() == nil || auditID == 0 {
		return ""
	}
	now := time.Now()
	rec := &model.AuditRecord{
		Appid:      appid,
		AuditID:    auditID,
		Status:     repo.AuditStatusAuditing,
		SubmitTime: &now,
		Source:     model.AuditSourceAPI,
	}
	if commitItem != nil {
		rec.UserVersion = commitItem.UserVersion
		rec.UserDesc = jsonStringOf(commitItem.Request["user_desc"])
	}
	if err := s.repos().Audits.Upsert(ctx, rec); err != nil {
		return "审核台账写入失败：" + err.Error()
	}
	return ""
}

// missingCategories 返回「配置里存在、但该小程序没有」的类目（官方 85008）。
func missingCategories(items []auditItem, cats []wxapi.CategoryName) []string {
	var missing []string
	for _, it := range items {
		found := false
		for _, c := range cats {
			if c.FirstID != it.FirstID || c.SecondID != it.SecondID {
				continue
			}
			if it.ThirdID != 0 && c.ThirdID != it.ThirdID {
				continue
			}
			found = true
			break
		}
		if !found {
			missing = append(missing, fmt.Sprintf("%s/%s（first_id=%d, second_id=%d）",
				it.FirstClass, it.SecondClass, it.FirstID, it.SecondID))
		}
	}
	return missing
}

// crossJobCommitWindow 跨作业寻找「成功上传代码」记录的时间窗口（30 天）。
//
// 太久的记录不能当作有效版本：微信侧最后一次上传才是会被提审的版本，
// 因此只用最近 30 天内的成功记录放行，避免拿几个月前的老记录放行提审。
const crossJobCommitWindow = 30 * 24 * time.Hour

// requireCommit 提审前的「上传代码」前置校验。
//
// 三条条件满足**任一**即放行（「先跑上传作业、再单独跑提审作业」是真实用法，只看本作业会误拦）：
//  1. 本作业内存在 step=commit 且 status=succeeded 的子项；
//  2. 该 appid 在本平台历史上有成功的 commit 记录（只认最近 30 天，见 crossJobCommitWindow）；
//  3. authorizer.CodeSource == model.CodeSourceDirectCommit（代码由 CI 直传，不走模板库）。
//
// **门控强度**：作业自身包含上传步骤时（pipeline）只认条件 1 —— 本次上传失败就不该去提审
// 上一版历史代码（否则会把「想发新版本」变成「把旧版本再提审一遍」）；只有单独提审作业
// （不包含 commit 步骤）才用条件 2/3 兜底，以支持「先跑上传作业、再单独跑提审作业」的用法。
//
// 三条都不满足才失败，且 errmsg 明确写出这是**平台侧预检**（微信侧尚未被调用），
// 避免用户误以为是微信返回的 85086。
func (s *JobService) requireCommit(ctx context.Context, job *model.BatchJob, a *model.Authorizer, appid string) (*model.BatchJobItem, *batch.Result) {
	// 条件 3：CI 直传。
	if a != nil && a.CodeSource == model.CodeSourceDirectCommit {
		return nil, nil
	}
	// 条件 1：本作业内已成功上传。
	item, err := s.inJobItem(ctx, job, appid, model.StepCommit)
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, repo.ErrNotFound) {
		res := failureResult(core.Internal(err), model.ClassRetryable, "")
		return nil, &res
	}
	// pipeline 这类「自己负责上传」的作业不允许用历史记录兜底。
	if jobContainsStep(job, model.StepCommit) {
		res := batch.Result{
			Outcome: batch.OutcomeFailed,
			Class:   model.ClassPermanent,
			Errcode: 85086,
			Errmsg: "本次作业的上传代码步骤没有成功，因此不执行提审（不会去提审历史版本）：" +
				"请在上传步骤修复失败项后重试（本项由平台侧预检拦截，未调用微信提审接口）",
			Note: "流水线作业的上传步骤未成功；为避免把旧版本重新提审，提审子项被跳过为失败。",
		}
		return nil, &res
	}
	// 条件 2：单独提审作业允许用最近一次成功上传兜底（先跑上传作业、再跑提审作业的用法）。
	if item, err := s.historicalCommit(ctx, appid); err == nil {
		return item, nil
	} else if !errors.Is(err, repo.ErrNotFound) {
		res := failureResult(core.Internal(err), model.ClassRetryable, "")
		return nil, &res
	}

	res := batch.Result{
		Outcome: batch.OutcomeFailed,
		Class:   model.ClassPermanent,
		Errcode: 85086,
		Errmsg: "本平台未找到该小程序成功的「上传代码」记录：请先执行批量上传代码作业" +
			"（微信侧会在未上传时返回 85086）；若代码由 CI 用 directCommit 直传，" +
			"请把该小程序的代码来源改为「CI 直传」（本项由平台侧预检拦截，未调用微信提审接口）",
		Note: fmt.Sprintf("上传代码前置预检未通过：本作业（%s）内没有成功的 commit 子项，"+
			"且平台在最近 %d 天内也没有该小程序的成功上传记录（batch_job_items 中 step=commit 且 status=succeeded），"+
			"因此没有可提审的代码版本。请先执行上传代码作业，或用 pipeline 作业把「上传 → 隐私检测 → 提审」串起来，"+
			"或把代码来源标记为 direct_commit（CI 直传）。", job.ID, int(crossJobCommitWindow.Hours()/24)),
	}
	return nil, &res
}

// jobContainsStep 判断作业类型自身是否包含某个步骤（用于区分「流水线作业」与「独立单步作业」）。
func jobContainsStep(job *model.BatchJob, step model.JobStep) bool {
	if job == nil {
		return false
	}
	for _, s := range job.Type.Steps() {
		if s == step {
			return true
		}
	}
	return false
}

// inJobItem 在本作业内查找指定步骤的成功子项；不存在返回 repo.ErrNotFound。
func (s *JobService) inJobItem(ctx context.Context, job *model.BatchJob, appid string, step model.JobStep) (*model.BatchJobItem, error) {
	if s.repos() == nil {
		return nil, errors.New("数据访问未初始化")
	}
	if job == nil {
		return nil, errors.New("作业为空")
	}
	items, err := s.repos().JobItems.AllByJob(ctx, job.ID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].Step == step && items[i].Appid == appid && items[i].Status == model.ItemStatusSucceeded {
			return &items[i], nil
		}
	}
	return nil, repo.ErrNotFound
}

// historicalCommit 跨作业查该 appid 最近一次成功上传代码的记录（只认 30 天内），没有则返回 repo.ErrNotFound。
//
// 为什么直连 env.DB：repo.JobItems 目前只有「按作业查」的方法，缺「按 appid + step 取最近一条」的查询；
// 与 internal/wxaudit 对 env.DB 的用法保持一致（业务读仍走 Repos，这里只补一个窄查询）。
func (s *JobService) historicalCommit(ctx context.Context, appid string) (*model.BatchJobItem, error) {
	if s.env == nil || s.env.DB == nil {
		return nil, repo.ErrNotFound
	}
	since := time.Now().Add(-crossJobCommitWindow)
	var item model.BatchJobItem
	err := s.env.DB.WithContext(ctx).Model(&model.BatchJobItem{}).
		Select("batch_job_items.*").
		// 试运行（dry_run）从来没有真的上传过代码，不能当作可提审的版本。
		Joins("JOIN batch_jobs ON batch_jobs.id = batch_job_items.job_id").
		Where("batch_jobs.dry_run = ?", false).
		Where("batch_job_items.appid = ? AND batch_job_items.step = ? AND batch_job_items.status = ?",
			appid, model.StepCommit, model.ItemStatusSucceeded).
		Where("batch_job_items.finished_at IS NOT NULL AND batch_job_items.finished_at >= ?", since).
		Order("batch_job_items.finished_at DESC, batch_job_items.id DESC").
		Take(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repo.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// requireInJobItem 要求本作业内存在指定步骤的成功子项；不满足时返回带中文说明的失败结果。
func (s *JobService) requireInJobItem(ctx context.Context, job *model.BatchJob, appid string, step model.JobStep) (*model.BatchJobItem, *batch.Result) {
	switch item, err := s.inJobItem(ctx, job, appid, step); {
	case err == nil:
		return item, nil
	case !errors.Is(err, repo.ErrNotFound):
		res := failureResult(core.Internal(err), model.ClassRetryable, "")
		return nil, &res
	}

	if step == model.StepSubmitAudit {
		res := batch.Result{
			Outcome: batch.OutcomeFailed,
			Class:   model.ClassPermanent,
			Errcode: 85019,
			Errmsg: "本作业内没有「提审」成功的记录（官方 85019：没有审核版本）：发布必须发生在审核通过之后。" +
				"请使用 pipeline 作业类型把「上传代码 → 隐私检测 → 提审 → 审核通过 → 发布」串成一条流水线，" +
				"或先单独执行提审作业并等审核通过后再发布。",
			Note: "平台侧预检（未调用微信）：本作业内没有成功的 submit_audit 子项。",
		}
		return nil, &res
	}

	res := batch.Result{
		Outcome: batch.OutcomeFailed,
		Class:   model.ClassPermanent,
		Errcode: 85086,
		Errmsg: fmt.Sprintf("本作业内没有「上传代码」成功的记录（官方 85086：%s）：请先执行上传代码作业，"+
			"或使用 pipeline 作业类型把「上传代码 → 隐私检测 → 提审」串起来；"+
			"若代码由 CI 直传（miniprogram-ci directCommit），请先在「小程序」页把该小程序的代码来源标记为 direct_commit。",
			model.ErrcodeText(85086)),
	}
	return nil, &res
}

// ---- 发布 ----

// ReleaseExecutor 执行 /wxa/release 或 /wxa/grayrelease（发布 / 分阶段发布）。
type ReleaseExecutor struct{ svc *JobService }

// NewReleaseExecutor 构造发布执行器。
func NewReleaseExecutor(svc *JobService) *ReleaseExecutor { return &ReleaseExecutor{svc: svc} }

// Step 该执行器负责的步骤。
func (e *ReleaseExecutor) Step() model.JobStep { return model.StepRelease }

// Execute 执行一个子项。
func (e *ReleaseExecutor) Execute(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	return e.svc.runRelease(ctx, job, item)
}

func (s *JobService) runRelease(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	if res, ok := s.readyForWx(); !ok {
		return res
	}
	pc, err := s.planFromJob(ctx, job)
	if err != nil {
		return permanentResult(err)
	}
	a, err := s.authorizer(ctx, item.Appid)
	if err != nil {
		return permanentResult(err)
	}
	if problems := authorizerProblems(a); len(problems) > 0 {
		return permanentResult(errors.New(strings.Join(problems, "；")))
	}

	// pipeline 作业要求本作业内已成功提审；单独的发布作业则以审核台账为准（否则永远无法发布）。
	if jobHasStep(job, model.StepSubmitAudit) {
		if _, blocked := s.requireInJobItem(ctx, job, item.Appid, model.StepSubmitAudit); blocked != nil {
			return *blocked
		}
	}

	rec, aerr := s.repos().Audits.LatestByApp(ctx, item.Appid)
	found := aerr == nil
	if aerr != nil && !errors.Is(aerr, repo.ErrNotFound) {
		return failureResult(core.Internal(aerr), model.ClassRetryable, "")
	}
	if !found || rec.Status != 0 {
		// 审核未通过（或仍在审核中）→ 等下一轮；超过上限仍不通过则失败并提示去看拒绝原因。
		return s.releaseWaiting(ctx, item, rec)
	}

	token, err := s.authorizerToken(ctx, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}
	version := rec.UserVersion

	if gray := pc.opts.release.GrayPercentage; gray != nil {
		if *gray < 0 || *gray > 100 {
			return batch.Result{
				Outcome: batch.OutcomeFailed, Class: model.ClassPermanent, Errcode: 85081,
				Errmsg: model.ErrcodeText(85081),
				Note:   "灰度比例必须是 0-100 的整数（官方 85081）；比例只能递增（85082）",
			}
		}
		req := wxapi.GrayReleaseRequest{
			GrayPercentage:          *gray,
			SupportDebugerFirst:     pc.opts.release.SupportDebugerFirst,
			SupportExperiencerFirst: pc.opts.release.SupportExperiencerFirst,
		}
		if err := s.env.Wx.GrayRelease(ctx, token, item.Appid, req); err != nil {
			return releaseFailure(err)
		}
		note := s.recordRelease(ctx, item.Appid, model.ReleaseActionGrayRelease, version, gray)
		response := model.JSONMap{
			"errcode": 0, "errmsg": "ok",
			"gray_percentage":           *gray,
			"support_debuger_first":     req.SupportDebugerFirst,
			"support_experiencer_first": req.SupportExperiencerFirst,
		}
		if note != "" {
			response["release_record_note"] = note
		}
		return batch.Result{
			Outcome:  batch.OutcomeSucceeded,
			Request:  map[string]any{"gray_percentage": *gray, "support_debuger_first": req.SupportDebugerFirst, "support_experiencer_first": req.SupportExperiencerFirst},
			Response: response,
		}
	}

	if err := s.env.Wx.Release(ctx, token, item.Appid); err != nil {
		return releaseFailure(err)
	}
	note := s.recordRelease(ctx, item.Appid, model.ReleaseActionRelease, version, nil)
	response := model.JSONMap{"errcode": 0, "errmsg": "ok", "note": "发布成功，用户已可访问正式版本"}
	if note != "" {
		response["release_record_note"] = note
	}
	return batch.Result{Outcome: batch.OutcomeSucceeded, Request: map[string]any{}, Response: response}
}

// releaseWaiting 审核未通过（或在审核中）时的等待与超时处理。
func (s *JobService) releaseWaiting(ctx context.Context, item *model.BatchJobItem, rec *model.AuditRecord) batch.Result {
	maxWait := s.settingInt(model.SettingAuditResultMaxWait, defaultAuditResultMaxWait)
	waited := jsonIntOf(item.Response[keyReleaseWaitSeconds]) + int(auditResultInterval.Seconds())

	statusText := "尚无审核记录"
	reason := ""
	if rec != nil {
		statusText = model.AuditStatusText(rec.Status)
		reason = strings.TrimSpace(rec.Reason)
	}
	resp := model.JSONMap{
		keyReleaseWaitSeconds: waited,
		"max_wait_seconds":    maxWait,
		"audit_status_text":   statusText,
	}
	if reason != "" {
		resp["reason"] = truncateErrmsg(reason)
	}
	if err := s.recordItemResponse(ctx, item.ID, resp); err != nil {
		resp["record_error"] = err.Error()
	}

	if waited > maxWait {
		note := fmt.Sprintf("等待审核通过已超过 %d 秒（上限 %d 秒）仍未通过（当前审核状态：%s）", waited, maxWait, statusText)
		if reason != "" {
			note += "；微信给出的原因：" + truncateErrmsg(reason)
		}
		note += "。请到「审核管理」页查看审核状态与拒绝原因；需要重新提审时请修改内容后新建提审作业。"
		return batch.Result{
			Outcome: batch.OutcomeFailed, Class: model.ClassPermanent, Errcode: 85080,
			Errmsg: model.ErrcodeText(85080), Response: resp, Note: note,
		}
	}

	note := fmt.Sprintf("等待审核通过：当前审核状态为「%s」，%d 秒后自动重试，审核通过后会自动发布",
		statusText, int(auditResultInterval.Seconds()))
	if reason != "" {
		note = fmt.Sprintf("等待审核通过：最近一次提审结果为「%s」（原因：%s）；%s；若确认不再重试可直接取消作业",
			statusText, truncateErrmsg(reason), note)
	}
	return batch.Result{
		Outcome:    batch.OutcomeWaiting,
		Class:      model.ClassRetryable,
		RetryAfter: auditResultInterval,
		Response:   resp,
		Note:       note,
	}
}

// recordRelease 写发布台账（失败只返回说明，不影响发布结果）。
func (s *JobService) recordRelease(ctx context.Context, appid string, action model.ReleaseAction, version string, gray *int) string {
	if s.repos() == nil {
		return ""
	}
	now := time.Now()
	rec := &model.ReleaseRecord{
		Appid:       appid,
		Action:      action,
		UserVersion: version,
		ReleaseTime: &now,
	}
	if gray != nil {
		rec.GrayPercentage = gray
		rec.Note = fmt.Sprintf("分阶段发布：灰度 %d%%", *gray)
	}
	if err := s.repos().Releases.Create(ctx, rec); err != nil {
		return "发布台账写入失败：" + err.Error()
	}
	return ""
}

// releaseFailure 发布失败分类：85019/85020 永久失败并给出操作顺序说明。
func releaseFailure(err error) batch.Result {
	res := failureResult(err, model.ClassRetryable, "")
	apiErr, ok := wxapi.IsAPIError(err)
	if !ok {
		return res
	}
	switch apiErr.Errcode {
	case 85019, 85020, 85021:
		res.Class = model.ClassPermanent
		res.Note = joinNote(errcodeLabel(apiErr.Errcode),
			"发布必须按「上传代码 → 提审 → 审核通过 → 发布」的顺序进行；若刚审核通过，请稍后重试")
	case 9400001:
		res.Class = model.ClassPermanent
		res.Note = joinNote(errcodeLabel(9400001), "需先在开放平台解绑该开发小程序")
	}
	return res
}

// ---- 单步作业 ----

// SingleStepExecutor 执行 sync_audit_status / undo_audit / speedup_audit / revert / toggle_visit。
//
// 其它单步类型（sync_info / set_domain）返回 skipped，由各自的专用页面执行。
type SingleStepExecutor struct{ svc *JobService }

// NewSingleStepExecutor 构造单步作业执行器。
func NewSingleStepExecutor(svc *JobService) *SingleStepExecutor { return &SingleStepExecutor{svc: svc} }

// Step 该执行器负责的步骤。
func (e *SingleStepExecutor) Step() model.JobStep { return model.StepSingle }

// Execute 执行一个子项。
func (e *SingleStepExecutor) Execute(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	return e.svc.runSingleStep(ctx, job, item)
}

func (s *JobService) runSingleStep(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	switch model.JobType(job.Type) {
	case model.JobTypeSyncAuditStatus:
		return s.runSyncAuditStatus(ctx, item)
	case model.JobTypeUndoAudit:
		return s.runUndoAudit(ctx, item)
	case model.JobTypeSpeedUpAudit:
		return s.runSpeedUpAudit(ctx, job, item)
	case model.JobTypeRevert:
		return s.runRevert(ctx, job, item)
	case model.JobTypeToggleVisit:
		return s.runToggleVisit(ctx, job, item)
	}
	return skippedResult("该作业类型暂由专用页面执行（本执行器只支持 sync_audit_status / undo_audit / " +
		"speedup_audit / revert / toggle_visit）")
}

// runSyncAuditStatus 查最新审核单并 upsert 到审核台账（Source=poll）。
func (s *JobService) runSyncAuditStatus(ctx context.Context, item *model.BatchJobItem) batch.Result {
	if res, ok := s.readyForWx(); !ok {
		return res
	}
	token, err := s.authorizerToken(ctx, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}
	st, err := s.env.Wx.GetLatestAuditStatus(ctx, token, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}
	if st.AuditID == 0 {
		return skippedResult("该小程序还没有审核单（官方 85012）：无需对账")
	}
	now := time.Now()
	rec := &model.AuditRecord{
		Appid:       item.Appid,
		AuditID:     st.AuditID,
		UserVersion: st.UserVersion,
		UserDesc:    st.UserDesc,
		Status:      st.Status,
		Reason:      st.Reason,
		StatusTime:  &now,
		Source:      model.AuditSourcePoll,
	}
	if st.SubmitAuditTime > 0 {
		t := time.Unix(st.SubmitAuditTime, 0)
		rec.SubmitTime = &t
	}
	if shots := firstNonEmpty(st.Screenshot, st.ScreenShotAlt); shots != "" {
		rec.ScreenshotMediaIDs = splitPipes(shots)
	}
	note := ""
	if err := s.repos().Audits.Upsert(ctx, rec); err != nil {
		note = "审核台账写入失败：" + err.Error()
	}
	response := model.JSONMap{"auditid": st.AuditID, "status": st.Status, "status_text": model.AuditStatusText(st.Status)}
	if note != "" {
		response["audit_record_note"] = note
	}
	return batch.Result{
		Outcome:   batch.OutcomeSucceeded,
		WxAuditID: st.AuditID,
		Request:   map[string]any{"auditid": st.AuditID},
		Response:  response,
	}
}

// runUndoAudit 撤回审核（先做本地额度兜底，超限直接失败，避免白跑微信）。
func (s *JobService) runUndoAudit(ctx context.Context, item *model.BatchJobItem) batch.Result {
	if res, ok := s.readyForWx(); !ok {
		return res
	}
	if s.repos() != nil {
		today, err := s.repos().UndoQuota.CountToday(ctx, item.Appid)
		if err != nil {
			return failureResult(core.Internal(err), model.ClassRetryable, "")
		}
		month, err := s.repos().UndoQuota.CountMonth(ctx, item.Appid)
		if err != nil {
			return failureResult(core.Internal(err), model.ClassRetryable, "")
		}
		if today >= maxUndoPerDay || month >= maxUndoPerMonth {
			return batch.Result{
				Outcome: batch.OutcomeFailed, Class: model.ClassPermanent, Errcode: 87013,
				Errmsg: model.ErrcodeText(87013),
				Note: fmt.Sprintf("撤回审核次数已达上限（每天 %d 次 / 每月 %d 次，本地记录：今天 %d 次、本月 %d 次；官方返回 87013）："+
					"每天 0 点恢复额度，请次日再试。", maxUndoPerDay, maxUndoPerMonth, today, month),
			}
		}
	}

	token, err := s.authorizerToken(ctx, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}
	if err := s.env.Wx.UndoCodeAudit(ctx, token, item.Appid); err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}

	note := ""
	if s.repos() != nil {
		if err := s.repos().UndoQuota.Record(ctx, item.Appid, time.Now()); err != nil {
			note = "撤回用量记录失败：" + err.Error()
		}
		// 撤回成功后把最近一条审核单标记为「已撤回」（status=3）。
		if rec, err := s.repos().Audits.LatestByApp(ctx, item.Appid); err == nil {
			now := time.Now()
			rec.Status = 3
			rec.StatusTime = &now
			if uerr := s.repos().Audits.Upsert(ctx, rec); uerr != nil && note == "" {
				note = "审核台账更新失败：" + uerr.Error()
			}
		}
	}
	response := model.JSONMap{"errcode": 0, "errmsg": "ok", "note": "已撤回审核"}
	if note != "" {
		response["audit_record_note"] = note
	}
	return batch.Result{Outcome: batch.OutcomeSucceeded, Request: map[string]any{}, Response: response}
}

// runSpeedUpAudit 加急审核：审核单号优先取作业载荷，其次最近审核记录，再退回 get_latest_auditstatus。
func (s *JobService) runSpeedUpAudit(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	if res, ok := s.readyForWx(); !ok {
		return res
	}
	pc, err := s.planFromJob(ctx, job)
	if err != nil {
		return permanentResult(err)
	}
	token, err := s.authorizerToken(ctx, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}

	// 取参规则：优先用作业载荷里显式给出的审核单号（CreateSingle 旁路），否则用该小程序
	// **最近一次审核单**（env.Repos.Audits.LatestByApp）；没有审核单则失败并说明原因。
	auditID := pc.opts.single.AuditID
	if auditID == 0 && s.repos() != nil {
		rec, rerr := s.repos().Audits.LatestByApp(ctx, item.Appid)
		switch {
		case rerr == nil && rec.AuditID != 0:
			auditID = rec.AuditID
		case rerr != nil && !errors.Is(rerr, repo.ErrNotFound):
			return failureResult(core.Internal(rerr), model.ClassRetryable, "")
		}
	}
	if auditID == 0 {
		return batch.Result{
			Outcome: batch.OutcomeFailed, Class: model.ClassPermanent, Errcode: 85012,
			Errmsg: model.ErrcodeText(85012),
			Note: "没有可加急的审核单（官方 85012：无效的审核 id）：该小程序在平台审核台账里还没有审核记录。" +
				"请先提审（或先执行「审核状态同步」作业把审核单同步进来）后再加急。",
		}
	}

	if err := s.env.Wx.SpeedUpAudit(ctx, token, item.Appid, auditID); err != nil {
		res := failureResult(err, model.ClassRetryable, "")
		if apiErr, ok := wxapi.IsAPIError(err); ok {
			switch apiErr.Errcode {
			case 89405:
				res.Class = model.ClassRateLimited
				res.Fatal = true
				res.FatalNote = "本月加急审核额度已用完（89405）：加急额度为服务商级、旗下小程序共用，" +
					"已暂停本作业；请提高提审质量以获取更多额度后，回到本页「恢复」作业"
				res.Note = res.FatalNote
			case 89404:
				return skippedResult("该审核单已加急成功（89404）：无需重复加急，本项已跳过")
			}
		}
		return res
	}
	return batch.Result{
		Outcome:  batch.OutcomeSucceeded,
		Request:  map[string]any{"auditid": auditID},
		Response: model.JSONMap{"errcode": 0, "errmsg": "ok", "auditid": auditID, "note": "已加急，预计 2-12 小时内审完"},
	}
}

// runRevert 版本回退。
//
// 取参规则：不传 app_version 即回退到**上一个版本**（官方默认行为），传了则回退到指定版本。
// 无上一个线上版本（或该版本已回退过）微信返回 87012，按永久失败处理并给出说明。
func (s *JobService) runRevert(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	if res, ok := s.readyForWx(); !ok {
		return res
	}
	pc, err := s.planFromJob(ctx, job)
	if err != nil {
		return permanentResult(err)
	}
	token, err := s.authorizerToken(ctx, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}
	if err := s.env.Wx.RevertCodeRelease(ctx, token, item.Appid, pc.opts.single.AppVersion); err != nil {
		res := failureResult(err, model.ClassRetryable, "")
		if apiErr, ok := wxapi.IsAPIError(err); ok && apiErr.Errcode == 87012 {
			res.Class = model.ClassPermanent
			res.Note = joinNote(model.ErrcodeText(87012),
				"无上一个线上版本（或该版本已回退过）时会返回 87012：请先在「版本信息」页确认存在可回退的线上版本")
		}
		return res
	}

	target := "上一个版本"
	req := map[string]any{}
	if pc.opts.single.AppVersion != nil {
		target = fmt.Sprintf("指定版本 %d", *pc.opts.single.AppVersion)
		req["app_version"] = *pc.opts.single.AppVersion
	}
	note := s.recordRelease(ctx, item.Appid, model.ReleaseActionRevert, "", nil)
	response := model.JSONMap{"errcode": 0, "errmsg": "ok", "note": "已回退到" + target}
	if note != "" {
		response["release_record_note"] = note
	}
	return batch.Result{Outcome: batch.OutcomeSucceeded, Request: req, Response: response}
}

// runToggleVisit 设置小程序服务状态。
//
// 取参规则（只读作业载荷，绝不猜默认值）：
//   - pause_service（来自契约的 gen.JobCreateRequest.PauseService）：true = 暂停服务（action=close），
//     false = 恢复服务（action=open）；
//   - 兼容旁路 single.visit_action（CreateSingle 传入，仅给契约未覆盖的场景用）；
//   - 两者都没有 → OutcomeFailed（永久失败）并提示「未指定服务状态（pauseService）」。
func (s *JobService) runToggleVisit(ctx context.Context, job *model.BatchJob, item *model.BatchJobItem) batch.Result {
	if res, ok := s.readyForWx(); !ok {
		return res
	}
	pc, err := s.planFromJob(ctx, job)
	if err != nil {
		return permanentResult(err)
	}

	var action string
	switch {
	case pc.opts.pauseService != nil:
		if *pc.opts.pauseService {
			action = "close"
		} else {
			action = "open"
		}
	default:
		action = strings.ToLower(strings.TrimSpace(pc.opts.single.VisitAction))
	}
	if action != "open" && action != "close" {
		return permanentResult(errors.New("未指定服务状态（pauseService）：请在创建 toggle_visit 作业时给出 pauseService" +
			"（true=暂停服务，false=恢复服务）。平台不会猜测默认值，以免误停线上小程序"))
	}

	token, err := s.authorizerToken(ctx, item.Appid)
	if err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}
	if err := s.env.Wx.SetVisitStatus(ctx, token, item.Appid, action); err != nil {
		return failureResult(err, model.ClassRetryable, "")
	}
	text := "已暂停服务（用户不可见）"
	if action == "open" {
		text = "已恢复服务（用户可见）"
	}
	return batch.Result{
		Outcome:  batch.OutcomeSucceeded,
		Request:  map[string]any{"action": action},
		Response: model.JSONMap{"errcode": 0, "errmsg": "ok", "action": action, "note": text},
	}
}

// ---- 小工具 ----

// listText 把字符串列表转成可读文本（空列表给出「无」）。
func listText(values []string) string {
	if len(values) == 0 {
		return "无"
	}
	return strings.Join(values, "、")
}

// firstNonEmpty 返回第一个非空字符串。
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// splitPipes 按 | 拆分截图 mediaid（官方推送用 | 分隔多个截图）。
func splitPipes(raw string) model.StringSlice {
	parts := strings.Split(raw, "|")
	out := make(model.StringSlice, 0, len(parts))
	for _, part := range parts {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

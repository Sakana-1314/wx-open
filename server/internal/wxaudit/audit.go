package wxaudit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

// AuditService 审核单台账与审核动作（撤回、加急、额度）。
type AuditService struct{ base }

// NewAuditService 构造审核服务。
func NewAuditService(env *core.Env) *AuditService { return &AuditService{base: base{env: env}} }

// auditResultHandler 与 callback.BusinessHandler 中审核结果推送方法的签名完全一致。
//
// 审核服务只承担回调业务的一个分支（整接口由 service.CallbackBusiness 组装，
// 其中授权相关分支由 wxauth 实现），因此这里只断言本方法，避免重复实现无关方法。
type auditResultHandler interface {
	OnAuditResult(ctx context.Context, appid, event, reason, screenshot string, eventTime int64) error
}

// 编译期断言：AuditService 可直接挂到回调业务分发器上。
var _ auditResultHandler = (*AuditService)(nil)

// undoHints 撤回审核的中文动作建议（覆盖 model 里的通用建议）。
var undoHints = map[int]string{
	85012: "无效的审核 id（85012）：先执行「同步审核状态」拿到最新审核单再撤回。",
	87013: "撤回次数已达官方上限（每天 5 次、每月 10 次）：每天额度 0 点恢复，请次日再操作。",
	85009: "已有正在审核的版本（85009）：确认审核单确实处于「审核中」再撤回。",
}

// speedUpHints 加急审核的中文动作建议（89401~89405 为官方定义的加急专用错误码）。
var speedUpHints = map[int]string{
	85012: "无效的审核 id（85012）：先执行「同步审核状态」拿到最新 auditid 再加急。",
	89401: "系统不稳定（89401）：稍后重试；加急成功后官方预计 2-12 小时内审完。",
	89402: "该小程序不在待审核队列（89402）：确认审核单已提交且仍处于「审核中」状态。",
	89403: "该审核单不支持加急（89403）：官方不支持此类加急，只能等待常规审核。",
	89404: "该审核单已加急成功（89404）：无需重复加急，避免浪费服务商额度。",
	89405: "本月加急额度已用完（89405）：剩余额度可在审核管理页查看；额度为服务商级、旗下小程序共用。",
}

// List 分页查询审核台账（只读本地库，不调用微信）。
func (s *AuditService) List(ctx context.Context, params gen.ListAuditsParams) (*gen.AuditListResponse, error) {
	if s.env == nil || s.env.Repos == nil {
		return nil, core.Internal(errors.New("服务依赖未初始化"))
	}
	if params.Status != nil && !params.Status.Valid() {
		return nil, core.Validation("status 只能是 0-4（0 审核成功 / 1 被拒绝 / 2 审核中 / 3 已撤回 / 4 延后）")
	}
	page, pageSize := pagination(params.Page, params.PageSize)
	var status *int
	if params.Status != nil {
		status = intPtr(int(*params.Status))
	}
	rows, total, err := s.env.Repos.Audits.List(ctx, derefString(params.Appid), status, page, pageSize)
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]gen.AuditRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, toGenAuditRecord(row))
	}
	return &gen.AuditListResponse{Items: items, Page: page, PageSize: pageSize, Total: int(total)}, nil
}

// Sync 向微信侧核对审核状态并落库（事件推送的兜底对账）。
//
// 对每个目标小程序调用 GET /wxa/get_latest_auditstatus，按 (appid, audit_id) 幂等 Upsert；
// UserVersion / UserDesc / 提交时间一并落库；单个小程序失败只记入 details，不中断整体同步。
func (s *AuditService) Sync(ctx context.Context, sel gen.AppidSelection) (*gen.SyncSummary, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	list, err := core.ResolveSelection(ctx, s.env.Repos, sel)
	if err != nil {
		return nil, mapError(err)
	}

	details := make([]gen.SyncDetail, 0, len(list))
	succeeded, failed := 0, 0
	for i := range list {
		appid := list[i].Appid
		detail := gen.SyncDetail{Key: appid}
		if err := s.syncOne(ctx, appid); err != nil {
			detail.Ok = false
			detail.Errcode, detail.Errmsg = errorFields(err)
			failed++
			log.Printf("[wxaudit] 同步审核状态失败(appid=%s): %v", appid, err)
		} else {
			detail.Ok = true
			succeeded++
		}
		details = append(details, detail)
	}
	log.Printf("[wxaudit] 审核状态同步完成：共 %d 个小程序，成功 %d，失败 %d", len(list), succeeded, failed)
	return &gen.SyncSummary{
		Total:     len(list),
		Succeeded: succeeded,
		Failed:    failed,
		Details:   &details,
	}, nil
}

// syncOne 同步单个小程序的审核状态。
func (s *AuditService) syncOne(ctx context.Context, appid string) error {
	token, err := s.token(ctx, appid)
	if err != nil {
		return err
	}
	st, err := s.env.Wx.GetLatestAuditStatus(ctx, token, appid)
	if err != nil {
		return mapError(err)
	}
	if st == nil {
		return core.Internal(fmt.Errorf("小程序 %s：微信未返回审核状态内容", appid))
	}
	if st.AuditID <= 0 {
		// 该小程序还没有任何审核单（例如从未提审）：不是错误，但没有可落库的主键。
		return core.Validation("小程序 %s：微信未返回 auditid，说明还没有审核单（请先上传代码并提审）", appid)
	}
	if err := s.env.Repos.Audits.Upsert(ctx, auditRecordFromStatus(appid, st, model.AuditSourcePoll)); err != nil {
		return mapError(err)
	}
	return nil
}

// History 返回单个小程序的审核历史（按创建时间倒序，最多 50 条）。
func (s *AuditService) History(ctx context.Context, appid string) ([]gen.AuditRecord, error) {
	if _, err := s.authorizer(ctx, appid); err != nil {
		return nil, err
	}
	rows, err := s.env.Repos.Audits.HistoryByApp(ctx, strings.TrimSpace(appid), historyLimit)
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]gen.AuditRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, toGenAuditRecord(row))
	}
	return items, nil
}

// Undo 撤回代码审核。
//
// 官方限制：单个账号每天最多撤回 5 次、每月最多 10 次（每天额度 0 点生效），
// 超限微信返回 87013；因此先在本地按用量拦截，避免白白占用微信调用。
func (s *AuditService) Undo(ctx context.Context, appid string) (*gen.AuditActionResult, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	appid = a.Appid

	today, err := s.env.Repos.UndoQuota.CountToday(ctx, appid)
	if err != nil {
		return nil, mapError(err)
	}
	month, err := s.env.Repos.UndoQuota.CountMonth(ctx, appid)
	if err != nil {
		return nil, mapError(err)
	}
	if today >= undoDailyLimit {
		return nil, core.Conflict(
			"小程序 %s 今天已撤回 %d 次，已达官方上限（每天 %d 次、每月 %d 次，超限微信返回 87013）：撤回额度每天 0 点恢复，请次日再操作。",
			appid, today, undoDailyLimit, undoMonthlyLimit)
	}
	if month >= undoMonthlyLimit {
		return nil, core.Conflict(
			"小程序 %s 本月已撤回 %d 次，已达官方上限（每月 %d 次，超限微信返回 87013）：下月额度自动恢复。",
			appid, month, undoMonthlyLimit)
	}

	token, err := s.token(ctx, appid)
	if err != nil {
		return nil, err
	}
	// 官方：GET /wxa/undocodeaudit，无请求体。
	if err := s.env.Wx.UndoCodeAudit(ctx, token, appid); err != nil {
		return nil, mapWxError(err, undoHints)
	}

	now := time.Now()
	// 微信侧已生效：本地用量必须记账；记账失败也不回滚操作（否则会诱导重复撤回），只告警。
	if err := s.env.Repos.UndoQuota.Record(ctx, appid, now); err != nil {
		log.Printf("[wxaudit] 撤回成功后记录本地用量失败(appid=%s): %v", appid, err)
	}
	s.markLatestWithdrawn(ctx, appid, now)
	s.logOperation(ctx, actionUndoAudit, targetApp, appid, model.JSONMap{
		"auditId":      s.latestAuditID(ctx, appid),
		"todayUsed":    today + 1,
		"monthUsed":    month + 1,
		"dailyLimit":   undoDailyLimit,
		"monthlyLimit": undoMonthlyLimit,
	})

	return actionOK(appid, fmt.Sprintf(
		"已撤回审核：本账号当天已用 %d/%d 次、本月已用 %d/%d 次；撤回后可重新提审。",
		today+1, undoDailyLimit, month+1, undoMonthlyLimit)), nil
}

// markLatestWithdrawn 把本地最新一条审核记录标记为「已撤回」。
//
// 撤回成功后微信侧最新审核单状态即为 3（已撤回），下一次 Sync 也会得到同样结果；
// 本地还没有任何记录时跳过（由 Sync 补全）。
func (s *AuditService) markLatestWithdrawn(ctx context.Context, appid string, at time.Time) {
	rec, err := s.env.Repos.Audits.LatestByApp(ctx, appid)
	if err != nil {
		if !errors.Is(err, repo.ErrNotFound) {
			log.Printf("[wxaudit] 读取最新审核记录失败(appid=%s): %v", appid, err)
		}
		return
	}
	if rec.Status == auditStatusUndone {
		return
	}
	rec.Status = auditStatusUndone
	statusAt := at
	rec.StatusTime = &statusAt
	if err := s.env.Repos.Audits.Upsert(ctx, rec); err != nil {
		log.Printf("[wxaudit] 更新审核记录为已撤回失败(appid=%s): %v", appid, err)
	}
}

// latestAuditID 读取本地最新审核单号（仅用于操作审计，取不到返回 0）。
func (s *AuditService) latestAuditID(ctx context.Context, appid string) int64 {
	rec, err := s.env.Repos.Audits.LatestByApp(ctx, appid)
	if err != nil || rec == nil {
		return 0
	}
	return rec.AuditID
}

// SpeedUp 加急代码审核。
//
// 官方：POST /wxa/speedupaudit（body：auditid），加急后预计 2-12 小时审完；
// 89401~89405 为加急专用错误码，统一转成中文动作建议。
func (s *AuditService) SpeedUp(ctx context.Context, appid string, req gen.SpeedUpRequest) (*gen.AuditActionResult, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	appid = a.Appid
	if req.AuditId <= 0 {
		return nil, core.Validation("auditId 必须大于 0：请先执行「同步审核状态」拿到最新审核单 auditid 再加急（无效 id 微信返回 85012）")
	}

	token, err := s.token(ctx, appid)
	if err != nil {
		return nil, err
	}
	if err := s.env.Wx.SpeedUpAudit(ctx, token, appid, req.AuditId); err != nil {
		return nil, mapWxError(err, speedUpHints)
	}
	s.logOperation(ctx, actionSpeedUp, targetApp, appid, model.JSONMap{"auditId": req.AuditId})
	return actionOK(appid, fmt.Sprintf(
		"已提交加急审核（auditid=%d）：官方预计 2-12 小时内完成审核，可用「同步审核状态」查看结果。", req.AuditId)), nil
}

// Quota 查询服务商审核额度（提审与加急额度，旗下小程序共用）。
//
// 额度是服务商级的，因此任意一个小程序的令牌都能查到；查询成功后落库缓存，
// 查询失败时回退返回最近一次缓存值（页面不至于完全空白）。
func (s *AuditService) Quota(ctx context.Context, appid string) (*gen.AuditQuota, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return nil, err
	}

	resp, err := s.env.Wx.QueryQuota(ctx, token, a.Appid)
	if err != nil {
		wxe := mapError(err)
		if cached := s.cachedQuota(ctx); cached != nil {
			log.Printf("[wxaudit] 查询服务商额度失败，回退缓存值(appid=%s): %v", a.Appid, err)
			return cached, nil
		}
		return nil, wxe
	}
	s.saveQuota(ctx, resp)
	return quotaFromResponse(resp, time.Now()), nil
}

// OnAuditResult 处理微信推送的审核结果事件（callback.BusinessHandler 的一个分支）。
//
// 事件 XML 里只有 Event / Reason / ScreenShot / 时间，**没有 auditid**，
// 因此必须回查最新审核单才能补齐台账主键（Source=event，保留事件里的驳回原因与截图）。
//
// 回查失败时只写日志并返回 nil：回调不能因为对账失败被阻塞或让微信重推；
// 也绝不写 audit_id=0 的占位记录（既无法幂等，又会与其它小程序的事件记录撞唯一索引）。
// 下一次 Sync 会把这条审核单补全。
func (s *AuditService) OnAuditResult(ctx context.Context, appid, event, reason, screenshot string, eventTime int64) error {
	status, known := auditEventStatus(event)
	if !known {
		log.Printf("[wxaudit] 忽略未处理的审核事件(appid=%s event=%s)", appid, event)
		return nil
	}
	if err := s.ready(); err != nil {
		return err
	}
	appid = strings.TrimSpace(appid)
	if appid == "" {
		return core.Validation("审核结果事件缺少 appid")
	}

	token, err := s.token(ctx, appid)
	if err != nil {
		log.Printf("[wxaudit] 审核结果事件回查 token 失败(appid=%s event=%s)：%v", appid, event, err)
		return nil
	}
	st, err := s.env.Wx.GetLatestAuditStatus(ctx, token, appid)
	if err != nil {
		log.Printf("[wxaudit] 审核结果事件回查最新审核单失败(appid=%s event=%s)：%v", appid, event, err)
		return nil
	}
	if st == nil || st.AuditID <= 0 {
		log.Printf("[wxaudit] 审核结果事件回查未拿到 auditid(appid=%s event=%s)：本次不入库，等待下一次同步", appid, event)
		return nil
	}

	rec := auditRecordFromStatus(appid, st, model.AuditSourceEvent)
	// 事件本身是审核结论，比回查到的状态更权威（回查可能仍显示「审核中」）。
	rec.Status = status
	if strings.TrimSpace(reason) != "" {
		rec.Reason = reason
	}
	if strings.TrimSpace(screenshot) != "" {
		rec.ScreenshotMediaIDs = splitScreenshots(screenshot)
	}
	if at := unixTime(eventTime); at != nil {
		rec.StatusTime = at
	}
	if err := s.env.Repos.Audits.Upsert(ctx, rec); err != nil {
		return mapError(err)
	}
	log.Printf("[wxaudit] 审核结果已入库(appid=%s event=%s auditid=%d status=%d %s)",
		appid, event, rec.AuditID, rec.Status, model.AuditStatusText(rec.Status))
	return nil
}

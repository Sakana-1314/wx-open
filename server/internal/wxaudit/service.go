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
	"wx-platform/server/internal/wxapi"
	"wx-platform/server/internal/wxtoken"
)

// 本文件集中三个服务共用的地基：依赖门禁、令牌获取、错误映射、台账/时间转换与操作日志，
// 避免 audit / release / preflight 各自重复一遍。

// operation_logs.action 取值。
//
// 直接沿用 api/openapi.yaml 的 operationId，便于前端按接口名过滤操作审计。
const (
	actionUndoAudit   = "undoAudit"
	actionSpeedUp     = "speedUpAudit"
	actionRelease     = "releaseApp"
	actionGrayRelease = "grayReleaseApp"
	actionRevertGray  = "revertGrayRelease"
	actionRevert      = "revertRelease"
	actionPreflight   = "runPreflight"
)

// targetApp 操作审计对象类型：单个小程序。
const targetApp = "app"

// 审核状态取值（与 model.AuditStatusText 一致）。
const (
	auditStatusSuccess  = 0 // 审核成功
	auditStatusRejected = 1 // 审核被拒绝
	auditStatusAuditing = 2 // 审核中
	auditStatusUndone   = 3 // 已撤回
	auditStatusDelayed  = 4 // 审核延后
)

// 本地撤回次数上限（官方：单个账号每天最多 5 次、每月最多 10 次，超限 87013）。
const (
	undoDailyLimit   = 5
	undoMonthlyLimit = 10
)

// 分页默认值与上限（与 repo.paginate 保持一致，响应里回显的是生效值）。
const (
	defaultPageSize = 20
	maxPageSize     = 200
)

// historyLimit 审核历史 / 发布版本追溯一次读取的最大条数（不超过 repo 的 maxPageSize）。
const historyLimit = 50

// base 汇总三个服务共用的依赖与工具方法。
type base struct{ env *core.Env }

// ready 依赖门禁：凭据未配置时任何需要调用微信的操作都返回 core.ErrNotConfigured。
func (b base) ready() error {
	if b.env == nil || b.env.Repos == nil {
		return core.Internal(errors.New("服务依赖未初始化"))
	}
	if !b.env.WeChatReady() {
		return core.ErrNotConfigured
	}
	return nil
}

// authorizer 读取单个授权小程序；appid 为空返回参数校验错误，未登记返回 core.ErrNotFound。
func (b base) authorizer(ctx context.Context, appid string) (*model.Authorizer, error) {
	appid = strings.TrimSpace(appid)
	if appid == "" {
		return nil, core.Validation("appid 不能为空")
	}
	a, err := b.env.Repos.Authorizers.Get(ctx, appid)
	if err != nil {
		return nil, mapError(err)
	}
	return a, nil
}

// token 获取授权方令牌（authorizer_access_token，/wxa/* 代码管理接口使用）。
func (b base) token(ctx context.Context, appid string) (string, error) {
	tok, err := b.env.Tokens.AuthorizerToken(ctx, appid)
	if err != nil {
		return "", mapError(err)
	}
	if strings.TrimSpace(tok) == "" {
		return "", core.Internal(fmt.Errorf("未取到小程序 %s 的 authorizer_access_token", appid))
	}
	return tok, nil
}

// cachedQuota 读取最近一次缓存的服务商额度（Platform 未注入时返回 nil）。
func (b base) cachedQuota(ctx context.Context) *gen.AuditQuota {
	if b.env == nil || b.env.Platform == nil {
		return nil
	}
	return b.env.Platform.CachedQuota(ctx)
}

// saveQuota 缓存本次查询到的服务商额度；缓存失败只告警（不影响本次返回）。
func (b base) saveQuota(ctx context.Context, resp *wxapi.QuotaResponse) {
	if b.env == nil || b.env.Platform == nil || resp == nil {
		return
	}
	if err := b.env.Platform.SaveQuota(ctx, resp); err != nil {
		log.Printf("[wxaudit] 缓存服务商额度失败: %v", err)
	}
}

// latestAuditVersion 取本地最近一次「审核成功」的版本号。
//
// 官方语义：发布（含灰度、回退台账）登记的都是「最后一个审核通过的版本」，
// 因此这里按时间倒序找第一条 status=0 的审核记录，而不是简单取最新一条
// （最新一条可能是撤回或审核中的记录）。
func (b base) latestAuditVersion(ctx context.Context, appid string) string {
	rows, err := b.env.Repos.Audits.HistoryByApp(ctx, appid, historyLimit)
	if err != nil {
		log.Printf("[wxaudit] 查询审核历史失败(appid=%s): %v", appid, err)
		return ""
	}
	for i := range rows {
		if rows[i].Status == auditStatusSuccess {
			return rows[i].UserVersion
		}
	}
	return ""
}

// logOperation 写一条操作审计（撤回、加急、发布、灰度、回退、体检）。
//
// 操作日志只用于审计：写失败不能影响已经生效的微信侧状态，因此只记录日志。
func (b base) logOperation(ctx context.Context, action, targetType, targetID string, detail model.JSONMap) {
	if b.env == nil || b.env.Repos == nil || action == "" {
		return
	}
	row := &model.OperationLog{
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     detail,
	}
	if err := b.env.Repos.Operations.Create(ctx, row); err != nil {
		log.Printf("[wxaudit] 写入操作日志失败(action=%s target=%s): %v", action, targetID, err)
	}
}

// mapError 把底层错误统一映射成契约错误：
// 微信侧失败 → core.WeChatError（保留 errcode/分类/官方建议）；未登记记录 → ErrNotFound；
// 凭据未配置 → ErrNotConfigured；已是契约错误则原样返回；其余按内部错误处理。
func mapError(err error) error {
	if err == nil {
		return nil
	}
	var wxe *core.WeChatError
	if errors.As(err, &wxe) {
		return err
	}
	var apiErr *wxapi.APIError
	if errors.As(err, &apiErr) {
		return toWeChatError(apiErr, nil)
	}
	switch {
	case errors.Is(err, repo.ErrNotFound):
		return core.NotFound("记录不存在（%s）", err.Error())
	case errors.Is(err, wxtoken.ErrNotConfigured):
		return core.ErrNotConfigured
	case errors.Is(err, wxtoken.ErrNoTicket):
		// 票据推送未就绪：属于「平台还没配好」，按状态冲突提示，消息里带官方处置建议。
		return fmt.Errorf("%w: %s", core.ErrConflict, err.Error())
	case errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrConflict),
		errors.Is(err, core.ErrValidation), errors.Is(err, core.ErrUnauthorized),
		errors.Is(err, core.ErrNotConfigured):
		return err
	}
	return core.Internal(err)
}

// toWeChatError 把 *wxapi.APIError 转成 core.WeChatError；hints 命中时覆盖官方通用建议。
func toWeChatError(apiErr *wxapi.APIError, hints map[int]string) *core.WeChatError {
	out := &core.WeChatError{
		Endpoint: apiErr.Endpoint,
		Method:   apiErr.Method,
		Appid:    apiErr.Appid,
		Errcode:  apiErr.Errcode,
		Errmsg:   apiErr.Errmsg,
		Class:    apiErr.Class(),
		Text:     model.ErrcodeText(apiErr.Errcode),
		Hint:     errcodeHint(apiErr.Errcode),
	}
	if hint, ok := hints[apiErr.Errcode]; ok && hint != "" {
		out.Hint = hint
	}
	return out
}

// mapWxError 在 mapError 基础上按接口覆盖处置建议（如 89405 的中文动作建议）。
func mapWxError(err error, hints map[int]string) error {
	mapped := mapError(err)
	if len(hints) == 0 {
		return mapped
	}
	var wxe *core.WeChatError
	if errors.As(mapped, &wxe) {
		if hint, ok := hints[wxe.Errcode]; ok && hint != "" {
			wxe.Hint = hint
		}
	}
	return mapped
}

// errcodeHint 取 model 里登记的官方处置建议。
func errcodeHint(errcode int) string {
	_, _, hint, ok := model.ErrcodeRule(errcode)
	if !ok {
		return ""
	}
	return hint
}

// errorFields 从错误里取出微信 errcode 与可读消息（同步结果逐项上报用）。
func errorFields(err error) (*int, *string) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	var wxe *core.WeChatError
	if errors.As(err, &wxe) && wxe.Errcode != 0 {
		code := wxe.Errcode
		return &code, &msg
	}
	return nil, &msg
}

// actionOK 构造成功结果。
func actionOK(appid, message string) *gen.AuditActionResult {
	msg := message
	return &gen.AuditActionResult{Appid: appid, Ok: true, Message: &msg}
}

// pagination 归一化分页参数（与 repo.paginate 口径一致），返回生效后的页码与每页条数。
func pagination(page, pageSize *int) (int, int) {
	p, size := 1, defaultPageSize
	if page != nil && *page > 0 {
		p = *page
	}
	if pageSize != nil && *pageSize > 0 {
		size = *pageSize
	}
	if size > maxPageSize {
		size = maxPageSize
	}
	return p, size
}

// derefString 解引用字符串指针并去掉首尾空白。
func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

// derefBool 解引用布尔指针（nil 视为 false，与官方默认值一致）。
func derefBool(v *bool) bool { return v != nil && *v }

// strPtr 非空字符串才转指针（避免返回空串指针让前端误以为有值）。
func strPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// intPtr 返回 int 指针。
func intPtr(v int) *int { return &v }

// unixTime 把微信返回的秒级时间戳转成 *time.Time；0/负数表示未返回，转 nil。
//
// 兼容毫秒：官方示例里分阶段发布计划的 create_timestamp 出现过毫秒值，
// 超过 1e12 时按毫秒处理，避免前端显示 5 万年后。
func unixTime(ts int64) *time.Time {
	if ts <= 0 {
		return nil
	}
	if ts > 1_000_000_000_000 {
		ts /= 1000
	}
	t := time.Unix(ts, 0)
	return &t
}

// splitScreenshots 把审核驳回截图 mediaid 串按 | 拆成列表。
//
// 官方字段名大小写不一致（字段表截图用 screenshot、示例/推送用 ScreenShot），
// wxapi 两种都解析，调用方只需取非空的那个。
func splitScreenshots(raw string) model.StringSlice {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, "|")
	out := make(model.StringSlice, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// auditRecordFromStatus 把微信返回的审核单状态转成本地台账记录。
//
// statusTime 取「本次观察到该状态的时间」：微信只返回提交时间，没有状态变更时间，
// 因此轮询/事件回查时以观察时刻记录（比留空更能反映台账新鲜度）。
func auditRecordFromStatus(appid string, st *wxapi.AuditStatusResponse, source model.AuditSource) *model.AuditRecord {
	now := time.Now()
	return &model.AuditRecord{
		Appid:              appid,
		AuditID:            st.AuditID,
		UserVersion:        st.UserVersion,
		UserDesc:           st.UserDesc,
		Status:             st.Status,
		Reason:             st.Reason,
		ScreenshotMediaIDs: splitScreenshots(firstNonEmpty(st.Screenshot, st.ScreenShotAlt)),
		SubmitTime:         unixTime(st.SubmitAuditTime),
		StatusTime:         &now,
		Source:             source,
	}
}

// auditEventStatus 把审核结果事件名映射为本地状态。
func auditEventStatus(event string) (int, bool) {
	switch event {
	case "weapp_audit_success":
		return auditStatusSuccess, true
	case "weapp_audit_fail":
		return auditStatusRejected, true
	case "weapp_audit_delay":
		return auditStatusDelayed, true
	}
	return 0, false
}

// toGenAuditRecord 把本地台账转成契约结构。
func toGenAuditRecord(rec model.AuditRecord) gen.AuditRecord {
	out := gen.AuditRecord{
		Appid:      rec.Appid,
		AuditId:    rec.AuditID,
		CreatedAt:  rec.CreatedAt,
		Status:     gen.AuditStatus(rec.Status),
		SubmitTime: rec.SubmitTime,
		StatusTime: rec.StatusTime,
	}
	if rec.UserVersion != "" {
		v := rec.UserVersion
		out.UserVersion = &v
	}
	if rec.UserDesc != "" {
		v := rec.UserDesc
		out.UserDesc = &v
	}
	if rec.Reason != "" {
		v := rec.Reason
		out.Reason = &v
	}
	if len(rec.ScreenshotMediaIDs) > 0 {
		ids := append([]string(nil), rec.ScreenshotMediaIDs...)
		out.ScreenshotMediaIds = &ids
	}
	if rec.Source != "" {
		s := gen.AuditRecordSource(rec.Source)
		out.Source = &s
	}
	return out
}

// toGenReleaseRecord 把本地发布台账转成契约结构。
func toGenReleaseRecord(rec model.ReleaseRecord) gen.ReleaseRecord {
	out := gen.ReleaseRecord{
		Appid:          rec.Appid,
		Action:         gen.ReleaseRecordAction(rec.Action),
		CreatedAt:      rec.CreatedAt,
		GrayPercentage: rec.GrayPercentage,
		ReleaseTime:    rec.ReleaseTime,
		Note:           strPtr(rec.Note),
	}
	if rec.UserVersion != "" {
		v := rec.UserVersion
		out.UserVersion = &v
	}
	return out
}

// quotaFromResponse 把 wxapi 的额度响应转成契约结构。
func quotaFromResponse(resp *wxapi.QuotaResponse, at time.Time) *gen.AuditQuota {
	if resp == nil {
		return nil
	}
	rest, limit := resp.Rest, resp.Limit
	speedupRest, speedupLimit := resp.SpeedupRest, resp.SpeedupLimit
	return &gen.AuditQuota{
		Rest:         &rest,
		Limit:        &limit,
		SpeedupRest:  &speedupRest,
		SpeedupLimit: &speedupLimit,
		QueriedAt:    &at,
	}
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

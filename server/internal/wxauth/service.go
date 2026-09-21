// Package wxauth 实现「授权管理」子系统，供 handler 与回调业务分发直接调用：
//
//   - AuthorizerService：已授权小程序的授权链路（授权链接 → 授权码换令牌 → 事件回调）、
//     信息同步、ext 变量 / 分组标签 / 提审配置等本地维护字段、授权方选项；
//   - TemplateService：代码草稿箱与代码模板库（第三方平台侧，令牌为 component_access_token）；
//   - AppService：单个小程序的域名 / 页面列表 / 服务状态 / 基础库版本（代调用，令牌为 authorizer_access_token）。
//
// 分层约定：
//   - 所有微信调用都经 wxapi，令牌一律经 wxtoken 获取（绝不自己拼 access_token）；
//   - 微信侧失败统一 `wxErr` 包装成 core.WeChatError（保留 errcode 与中文处置建议），
//     本地失败用 core 的哨兵错误（ErrNotFound / ErrConflict / ErrValidation / ErrNotConfigured）；
//   - 关键操作写 operation_logs（授权链接生成、授权完成、取消授权、同步、模板增删、域名与状态变更）。
package wxauth

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
	"wx-platform/server/internal/wxapi"
	"wx-platform/server/internal/wxtoken"
)

// operation_logs.action 的取值（前端按这些值筛选审计日志）。
const (
	actionAuthorizationURL  = "authorizer.authorization_url"
	actionAuthorize         = "authorizer.authorize"
	actionAuthorizedEvent   = "authorizer.authorized_event"
	actionUnauthorized      = "authorizer.unauthorized"
	actionSync              = "authorizer.sync"
	actionSyncMany          = "authorizer.sync_many"
	actionResyncTokens      = "authorizer.resync_tokens"
	actionUpdateAuthorizer  = "authorizer.update"
	actionDeleteAuthorizer  = "authorizer.delete"
	actionGetOption         = "authorizer.get_option"
	actionSetOption         = "authorizer.set_option"
	actionMessageReceived   = "authorizer.message_received"
	actionSyncDrafts        = "template.sync_drafts"
	actionSyncTemplates     = "template.sync_templates"
	actionAddToTemplate     = "template.add"
	actionUpdateTemplate    = "template.update"
	actionDeleteTemplate    = "template.delete"
	actionSetVisitStatus    = "app.set_visit_status"
	actionSetSupportVersion = "app.set_support_version"
	actionApplyDomains      = "app.apply_domains"
)

// targetType 审计日志的目标类型。
const targetAuthorizer = "authorizer"

// base 三个服务的公共依赖（env）与公共辅助方法。
//
// Env 里的 Wx / Tokens 在第三方平台凭据未配置时为 nil，
// 因此所有需要微信能力的入口都必须先过 ensureWeChat（凭据门禁）。
type base struct {
	env *core.Env
}

// newBase 构造公共依赖。
func newBase(env *core.Env) base { return base{env: env} }

// repos 返回数据访问聚合（可能为 nil，调用前用 ensureRepos 判断）。
func (b base) repos() *core.Repos {
	if b.env == nil {
		return nil
	}
	return b.env.Repos
}

// componentAppid 返回第三方平台 appid。
func (b base) componentAppid() string {
	if b.env == nil || b.env.Cfg == nil {
		return ""
	}
	return strings.TrimSpace(b.env.Cfg.ComponentAppID)
}

// publicBaseURL 返回本站公网基址（用于拼接默认的授权回调地址）。
func (b base) publicBaseURL() string {
	if b.env == nil || b.env.Cfg == nil {
		return ""
	}
	return b.env.Cfg.PublicBaseURL
}

// ensureRepos 数据访问不可用时返回内部错误。
func (b base) ensureRepos() error {
	if b.repos() == nil {
		return core.Internal(errors.New("数据访问未初始化"))
	}
	return nil
}

// ensureWeChat 凭据门禁：Wx / Tokens 未注入时一律返回 core.ErrNotConfigured，
// 绝不带着 nil 客户端继续往下走。
func (b base) ensureWeChat() error {
	if b.env == nil || !b.env.WeChatReady() {
		return core.ErrNotConfigured
	}
	return nil
}

// componentToken 取第三方平台令牌（模板库 / 授权方信息接口使用）。
func (b base) componentToken(ctx context.Context) (string, error) {
	if err := b.ensureWeChat(); err != nil {
		return "", err
	}
	if b.componentAppid() == "" {
		return "", core.ErrNotConfigured
	}
	token, err := b.env.Tokens.ComponentToken(ctx)
	if err != nil {
		return "", wxErr("", err)
	}
	return token, nil
}

// authorizerToken 取授权方令牌（/wxa/*、/cgi-bin/wxopen/* 代调用接口使用）。
func (b base) authorizerToken(ctx context.Context, appid string) (string, error) {
	if err := b.ensureWeChat(); err != nil {
		return "", err
	}
	token, err := b.env.Tokens.AuthorizerToken(ctx, appid)
	if err != nil {
		return "", wxErr(appid, err)
	}
	return token, nil
}

// getAuthorizer 读取本地授权方记录；不存在返回 404（各状态都允许读取/编辑）。
func (b base) getAuthorizer(ctx context.Context, appid string) (*model.Authorizer, error) {
	if err := b.ensureRepos(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(appid) == "" {
		return nil, core.Validation("缺少小程序 appid")
	}
	a, err := b.repos().Authorizers.Get(ctx, appid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, core.NotFound("小程序 %s 不存在（或已删除）：请先在「授权管理」同步授权方清单", appid)
		}
		return nil, core.Internal(err)
	}
	return a, nil
}

// requireAuthorized 先过凭据门禁，再读取本地记录并要求处于「已授权」状态。
//
// 它的调用方全是「需要微信能力」的代调用路径（/wxa/*、get_authorizer_option 等），
// 因此凭据缺失必须优先返回 ErrNotConfigured，而不是先报「记录不存在」。
func (b base) requireAuthorized(ctx context.Context, appid string) (*model.Authorizer, error) {
	if err := b.ensureWeChat(); err != nil {
		return nil, err
	}
	a, err := b.getAuthorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	if a.AuthorizationStatus != model.AuthStatusAuthorized {
		return nil, core.Conflict("小程序 %s 当前不是「已授权」状态，无法代调用：需商家重新扫码授权后再操作", appid)
	}
	return a, nil
}

// wxErr 把微信调用失败统一成 core.WeChatError，便于 handler 输出 502 + 具体 errcode。
//
// Class 取 model.ClassifyErrcode，Text/Hint 取 model.ErrcodeRule 的官方说明与处置建议
// （例如 86102 →「域名每月修改次数已用尽（50 次）」），保证前端拿到的是中文可执行文案。
func wxErr(appid string, err error) error {
	if err == nil {
		return nil
	}
	var apiErr *wxapi.APIError
	if errors.As(err, &apiErr) {
		class, text, hint, _ := model.ErrcodeRule(apiErr.Errcode)
		owner := apiErr.Appid
		if owner == "" {
			owner = appid
		}
		return &core.WeChatError{
			Endpoint: apiErr.Endpoint,
			Method:   apiErr.Method,
			Appid:    owner,
			Errcode:  apiErr.Errcode,
			Errmsg:   apiErr.Errmsg,
			Class:    class,
			Text:     text,
			Hint:     hint,
		}
	}
	switch {
	case errors.Is(err, wxtoken.ErrNotConfigured):
		return core.ErrNotConfigured
	case errors.Is(err, wxtoken.ErrNoTicket):
		// 票据问题需要人工在开放平台后台恢复推送，属于「状态冲突」而不是 500。
		return core.Conflict("%s", err.Error())
	case errors.Is(err, repo.ErrNotFound):
		return core.NotFound("%s", err.Error())
	}
	return err
}

// errcodeOf 从错误里取出微信返回码（用于 SyncDetail；本地错误返回 nil）。
func errcodeOf(err error) *int {
	var apiErr *wxapi.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.Errcode
		return &code
	}
	var wErr *core.WeChatError
	if errors.As(err, &wErr) && wErr.Errcode != 0 {
		code := wErr.Errcode
		return &code
	}
	return nil
}

// actorCtxKey 审计日志操作人的 context 键。
type actorCtxKey struct{}

// WithActor 把操作人写进 context（handler 拿到登录用户后可调用，审计日志用它填 actor）。
// 未设置时审计日志的 actor 记为 "system"（回调等无登录用户的路径）。
func WithActor(ctx context.Context, actor string) context.Context {
	if strings.TrimSpace(actor) == "" {
		return ctx
	}
	return context.WithValue(ctx, actorCtxKey{}, strings.TrimSpace(actor))
}

// actorOf 读取操作人。
func actorOf(ctx context.Context) string {
	if v, ok := ctx.Value(actorCtxKey{}).(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return "system"
}

// writeOperation 写关键操作审计日志。
//
// 审计日志写失败不能影响主流程（例如授权已经成功、模板已经删除），因此这里只打日志。
func (b base) writeOperation(ctx context.Context, action, targetType, targetID string, detail model.JSONMap) {
	repos := b.repos()
	if repos == nil {
		return
	}
	if detail == nil {
		detail = model.JSONMap{}
	}
	entry := &model.OperationLog{
		Actor:      actorOf(ctx),
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     detail,
		CreatedAt:  time.Now(),
	}
	if err := repos.Operations.Create(ctx, entry); err != nil {
		log.Printf("[wxauth] 写入操作日志失败（action=%s, target=%s）: %v", action, targetID, err)
	}
}

// ---- 参数与指针小工具（保持 DTO 的「可选字段为指针」语义） ----

// pageOf 归一化页码。
func pageOf(page *int) int {
	if page == nil || *page < 1 {
		return 1
	}
	return *page
}

// pageSizeOf 归一化每页条数（与 repo 的上限 200 保持一致）。
func pageSizeOf(size *int) int {
	if size == nil || *size < 1 {
		return 20
	}
	if *size > 200 {
		return 200
	}
	return *size
}

// derefString 读可选字符串并去空白。
func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func strPtr(v string) *string { return &v }

func intPtr(v int) *int { return &v }

func boolPtr(v bool) *bool { return &v }

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// dedupeStrings 去空白、去重（保持输入顺序），批量域名的入参清洗用。
func dedupeStrings(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

// sameIntSet 判断两个整数集合是否等价（顺序无关；用于比对两次的权限集是否变化）。
func sameIntSet(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[int]int, len(a))
	for _, v := range a {
		seen[v]++
	}
	for _, v := range b {
		seen[v]--
		if seen[v] < 0 {
			return false
		}
	}
	return true
}

// truncate 截断过长字符串（审计日志里保存回调原文用）。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…(已截断)"
}

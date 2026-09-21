// Package service 组装各业务子系统，并向 handler 暴露统一的 Container 入口。
//
// 分层约定（自下而上）：
//
//	model/repo        数据模型与数据访问
//	wxcrypt/wxapi     微信协议：加解密与 HTTP 客户端
//	wxtoken/callback  令牌管理与回调接收
//	core              各服务共用的地基（错误、选择、渲染、运行参数、平台状态）
//	wxauth/wxaudit/wxjob  业务子系统：授权与模板库 / 审核与发布 / 批量作业
//	service           组装（Container）与 handler 适配
//	handler/router    HTTP 编解码与路由
package service

import (
	"context"
	"gorm.io/gorm"

	"wx-platform/server/internal/auth"
	"wx-platform/server/internal/batch"
	"wx-platform/server/internal/callback"
	"wx-platform/server/internal/config"
	"wx-platform/server/internal/core"
	"wx-platform/server/internal/secretbox"
	"wx-platform/server/internal/wxapi"
	"wx-platform/server/internal/wxaudit"
	"wx-platform/server/internal/wxauth"
	"wx-platform/server/internal/wxjob"
	"wx-platform/server/internal/wxtoken"
)

// Container 聚合全部依赖与业务服务，是 handler 的唯一依赖入口。
type Container struct {
	Env *core.Env

	Auth   *AuthService
	Logs   *LogService
	Engine *batch.Engine

	Authorizer *wxauth.AuthorizerService
	Template   *wxauth.TemplateService
	App        *wxauth.AppService

	Audit     *wxaudit.AuditService
	Release   *wxaudit.ReleaseService
	Preflight *wxaudit.PreflightService

	Job *wxjob.JobService
}

// NewContainer 构造容器（第三方平台凭据缺失时微信子系统为 nil，相关接口返回明确错误）。
func NewContainer(db *gorm.DB, cfg *config.Config, box *secretbox.Box, am *auth.Manager, limiter *auth.LoginLimiter) *Container {
	env := core.NewEnv(db, cfg, box)
	return &Container{
		Env:  env,
		Auth: &AuthService{manager: am, limiter: limiter},
		Logs: &LogService{repos: env.Repos},
	}
}

// AttachWeChat 注入微信客户端与令牌管理器，并构造依赖它们的子系统。
func (c *Container) AttachWeChat(client *wxapi.Client, tokens *wxtoken.Manager) {
	if client == nil || tokens == nil {
		return
	}
	c.Env.Wx = client
	c.Env.Tokens = tokens
	c.Env.Platform.AttachTokens(tokens)

	c.Authorizer = wxauth.NewAuthorizerService(c.Env)
	c.Template = wxauth.NewTemplateService(c.Env)
	c.App = wxauth.NewAppService(c.Env)

	c.Audit = wxaudit.NewAuditService(c.Env)
	c.Release = wxaudit.NewReleaseService(c.Env)
	c.Preflight = wxaudit.NewPreflightService(c.Env)

	c.Job = wxjob.NewJobService(c.Env)
}

// AttachEngine 注入作业引擎并注册各步骤执行器。
func (c *Container) AttachEngine(engine *batch.Engine) {
	c.Engine = engine
	if c.Job == nil {
		return
	}
	// 作业服务需要引擎引用来实现开始/暂停/恢复/取消/重试失败项。
	c.Job.AttachEngine(engine)
	engine.Register(wxjob.NewCommitExecutor(c.Job))
	engine.Register(wxjob.NewPrivacyCheckExecutor(c.Job))
	engine.Register(wxjob.NewSubmitAuditExecutor(c.Job))
	engine.Register(wxjob.NewReleaseExecutor(c.Job))
	engine.Register(wxjob.NewSingleStepExecutor(c.Job))
}

// CallbackBusiness 让 callback 包把业务处理分发到对应子系统。
type CallbackBusiness struct {
	Authorizer *wxauth.AuthorizerService
	Audit      *wxaudit.AuditService
}

// NewCallbackBusiness 构造回调业务分发器。
func (c *Container) NewCallbackBusiness() *CallbackBusiness {
	return &CallbackBusiness{Authorizer: c.Authorizer, Audit: c.Audit}
}

// OnAuthorized 授权成功 / 更新授权。
func (b *CallbackBusiness) OnAuthorized(ctx context.Context, appid, authCode, infoType string) error {
	return b.Authorizer.OnAuthorized(ctx, appid, authCode, infoType)
}

// OnUnauthorized 取消授权。
func (b *CallbackBusiness) OnUnauthorized(ctx context.Context, appid string) error {
	return b.Authorizer.OnUnauthorized(ctx, appid)
}

// OnAuditResult 审核结果推送。
func (b *CallbackBusiness) OnAuditResult(ctx context.Context, appid, event, reason, screenshot string, eventTime int64) error {
	return b.Audit.OnAuditResult(ctx, appid, event, reason, screenshot, eventTime)
}

// OnMessage 代收的用户消息（仅审计）。
func (b *CallbackBusiness) OnMessage(ctx context.Context, appid, plainXML string) error {
	return b.Authorizer.OnMessage(ctx, appid, plainXML)
}

// 编译期断言：CallbackBusiness 实现 callback.BusinessHandler。
var _ callback.BusinessHandler = (*CallbackBusiness)(nil)

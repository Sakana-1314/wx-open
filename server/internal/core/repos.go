package core

import (
	"gorm.io/gorm"

	"wx-platform/server/internal/config"
	"wx-platform/server/internal/repo"
	"wx-platform/server/internal/secretbox"
	"wx-platform/server/internal/wxapi"
	"wx-platform/server/internal/wxtoken"
)

// Repos 聚合全部数据访问对象，避免容器字段膨胀。
type Repos struct {
	State       *repo.PlatformState
	Tokens      *repo.Tokens
	Authorizers *repo.Authorizers
	Drafts      *repo.Drafts
	Templates   *repo.Templates
	Profiles    *repo.AuditProfiles
	Jobs        *repo.Jobs
	JobItems    *repo.JobItems
	Audits      *repo.Audits
	Releases    *repo.Releases
	UndoQuota   *repo.UndoQuota
	ApiCalls    *repo.ApiCallLogs
	Callbacks   *repo.CallbackEvents
	Operations  *repo.OperationLogs
	Settings    *repo.Settings
}

// NewRepos 构造数据访问聚合。
func NewRepos(db *gorm.DB) *Repos {
	return &Repos{
		State:       repo.NewPlatformState(db),
		Tokens:      repo.NewTokens(db),
		Authorizers: repo.NewAuthorizers(db),
		Drafts:      repo.NewDrafts(db),
		Templates:   repo.NewTemplates(db),
		Profiles:    repo.NewAuditProfiles(db),
		Jobs:        repo.NewJobs(db),
		JobItems:    repo.NewJobItems(db),
		Audits:      repo.NewAudits(db),
		Releases:    repo.NewReleases(db),
		UndoQuota:   repo.NewUndoQuota(db),
		ApiCalls:    repo.NewApiCallLogs(db),
		Callbacks:   repo.NewCallbackEvents(db),
		Operations:  repo.NewOperationLogs(db),
		Settings:    repo.NewSettings(db),
	}
}

// Env 是所有业务服务的公共依赖集合。
//
// 把依赖收敛到一个结构体，新增服务的构造函数签名保持稳定；Wx/Tokens 在第三方平台
// 凭据未配置时为 nil，相关服务需返回明确的 ErrNotConfigured。
type Env struct {
	// DB 底层连接：仅用于少数需要直接写库的场合（如提审配置 CRUD），业务查询一律走 Repos。
	DB       *gorm.DB
	Cfg      *config.Config
	Repos    *Repos
	Box      *secretbox.Box
	Wx       *wxapi.Client
	Tokens   *wxtoken.Manager
	Settings *SettingService
	Platform *PlatformService
}

// NewEnv 构造依赖集合（不含微信客户端，需配置凭据后由 AttachWeChat 注入）。
func NewEnv(db *gorm.DB, cfg *config.Config, box *secretbox.Box) *Env {
	repos := NewRepos(db)
	env := &Env{DB: db, Cfg: cfg, Repos: repos, Box: box}
	env.Settings = &SettingService{db: db, cfg: cfg}
	env.Platform = &PlatformService{cfg: cfg, repos: repos}
	return env
}

// WeChatReady 报告微信能力是否已注入。
func (e *Env) WeChatReady() bool { return e.Wx != nil && e.Tokens != nil }

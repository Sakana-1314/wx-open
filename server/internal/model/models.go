package model

import (
	"time"
)

// ---- 强类型枚举 ----

// AuthorizationStatus 授权状态。
type AuthorizationStatus string

const (
	// AuthStatusAuthorized 已授权。
	AuthStatusAuthorized AuthorizationStatus = "authorized"
	// AuthStatusUnauthorized 已取消授权（保留历史，不再代调用）。
	AuthStatusUnauthorized AuthorizationStatus = "unauthorized"
)

// CodeSource 代码进入小程序的方式。
type CodeSource string

const (
	// CodeSourceTemplate 平台用模板库批量下发（主线，可批量）。
	CodeSourceTemplate CodeSource = "template"
	// CodeSourceDirectCommit 开发者在开发者工具 / miniprogram-ci 里用 directCommit 直传到该小程序的审核列表。
	CodeSourceDirectCommit CodeSource = "direct_commit"
)

// JobType 作业类型。
type JobType string

const (
	JobTypeCommit          JobType = "commit"
	JobTypeSubmitAudit     JobType = "submit_audit"
	JobTypeRelease         JobType = "release"
	JobTypePipeline        JobType = "pipeline"
	JobTypeSyncInfo        JobType = "sync_info"
	JobTypeSyncAuditStatus JobType = "sync_audit_status"
	JobTypeUndoAudit       JobType = "undo_audit"
	JobTypeSpeedUpAudit    JobType = "speedup_audit"
	JobTypeSetDomain       JobType = "set_domain"
	JobTypeRevert          JobType = "revert"
	JobTypeToggleVisit     JobType = "toggle_visit"
)

// Valid 校验作业类型。
func (t JobType) Valid() bool {
	switch t {
	case JobTypeCommit, JobTypeSubmitAudit, JobTypeRelease, JobTypePipeline,
		JobTypeSyncInfo, JobTypeSyncAuditStatus, JobTypeUndoAudit, JobTypeSpeedUpAudit,
		JobTypeSetDomain, JobTypeRevert, JobTypeToggleVisit:
		return true
	}
	return false
}

// Steps 返回该作业包含的步骤序列。
func (t JobType) Steps() []JobStep {
	switch t {
	case JobTypeCommit:
		return []JobStep{StepCommit}
	case JobTypeSubmitAudit:
		return []JobStep{StepPrivacyCheck, StepSubmitAudit}
	case JobTypeRelease:
		return []JobStep{StepRelease}
	case JobTypePipeline:
		return []JobStep{StepCommit, StepPrivacyCheck, StepSubmitAudit, StepRelease}
	case JobTypeSyncInfo, JobTypeSyncAuditStatus, JobTypeUndoAudit,
		JobTypeSpeedUpAudit, JobTypeSetDomain, JobTypeRevert, JobTypeToggleVisit:
		return []JobStep{StepSingle}
	}
	return nil
}

// JobStatus 作业状态。
type JobStatus string

const (
	JobStatusPending       JobStatus = "pending"
	JobStatusRunning       JobStatus = "running"
	JobStatusPaused        JobStatus = "paused"
	JobStatusSucceeded     JobStatus = "succeeded"
	JobStatusPartialFailed JobStatus = "partial_failed"
	JobStatusFailed        JobStatus = "failed"
	JobStatusCanceled      JobStatus = "canceled"
	JobStatusInterrupted   JobStatus = "interrupted"
)

// Terminal 判断是否为终态。
func (s JobStatus) Terminal() bool {
	switch s {
	case JobStatusSucceeded, JobStatusPartialFailed, JobStatusFailed, JobStatusCanceled:
		return true
	}
	return false
}

// JobStep 作业步骤。
type JobStep string

const (
	// StepSingle 单步作业（同步、撤回、加急等）。
	StepSingle JobStep = "single"
	// StepCommit 上传代码。
	StepCommit JobStep = "commit"
	// StepPrivacyCheck 等待隐私接口检测任务结束（提审前置，避免 61039）。
	StepPrivacyCheck JobStep = "privacy_check"
	// StepSubmitAudit 提审。
	StepSubmitAudit JobStep = "submit_audit"
	// StepRelease 发布。
	StepRelease JobStep = "release"
)

// JobItemStatus 作业子项状态。
type JobItemStatus string

const (
	ItemStatusPending   JobItemStatus = "pending"
	ItemStatusWaiting   JobItemStatus = "waiting"
	ItemStatusRunning   JobItemStatus = "running"
	ItemStatusSucceeded JobItemStatus = "succeeded"
	ItemStatusFailed    JobItemStatus = "failed"
	ItemStatusSkipped   JobItemStatus = "skipped"
	ItemStatusCanceled  JobItemStatus = "canceled"
)

// Finished 判断子项是否已结束。
func (s JobItemStatus) Finished() bool {
	switch s {
	case ItemStatusSucceeded, ItemStatusFailed, ItemStatusSkipped, ItemStatusCanceled:
		return true
	}
	return false
}

// ErrorClass 微信返回码的处置分类。
type ErrorClass string

const (
	// ClassRetryable 可重试（系统繁忙、分钟级限流）。
	ClassRetryable ErrorClass = "retryable"
	// ClassTokenExpired 令牌失效，刷新后可重试一次。
	ClassTokenExpired ErrorClass = "token_expired"
	// ClassRateLimited 频次 / 额度限制，需退避或暂停整个作业。
	ClassRateLimited ErrorClass = "rate_limited"
	// ClassEnvironment 环境问题（IP 白名单等），需人工修复。
	ClassEnvironment ErrorClass = "environment"
	// ClassPermanent 永久失败，重试无意义。
	ClassPermanent ErrorClass = "permanent"
	// ClassUnknown 未归类。
	ClassUnknown ErrorClass = "wechat_unknown"
)

// CallbackKind 回调入口类型。
type CallbackKind string

const (
	// CallbackKindComponent 授权事件接收 URL（票据 + 授权变更）。
	CallbackKindComponent CallbackKind = "component"
	// CallbackKindMessage 消息与事件接收 URL（审核结果 + 代收消息）。
	CallbackKindMessage CallbackKind = "message"
)

// ReleaseAction 发布动作。
type ReleaseAction string

const (
	ReleaseActionRelease     ReleaseAction = "release"
	ReleaseActionGrayRelease ReleaseAction = "grayrelease"
	ReleaseActionRevert      ReleaseAction = "revert"
)

// AuditSource 审核状态来源。
type AuditSource string

const (
	AuditSourceAPI   AuditSource = "api"
	AuditSourceEvent AuditSource = "event"
	AuditSourcePoll  AuditSource = "poll"
)

// TokenScope 令牌归属。
type TokenScope string

const (
	// TokenScopeComponent 第三方平台自身令牌（模板库等接口使用）。
	TokenScopeComponent TokenScope = "component"
	// TokenScopeAuthorizer 授权方令牌（代码管理接口使用）。
	TokenScopeAuthorizer TokenScope = "authorizer"
)

// PermissionSetDev「小程序开发与数据分析」权限集 id：代码上传 / 提审 / 发布全部依赖它，且为互斥权限集。
const PermissionSetDev = 18

// PermissionSetBasicInfo「小程序基本信息管理」权限集 id：类目管理接口使用。
const PermissionSetBasicInfo = 30

// PermissionSetNames 常用权限集 id -> 名称，用于前端提示。
var PermissionSetNames = map[int]string{
	17:  "获取小程序码",
	18:  "小程序开发与数据分析（代码管理必需，互斥）",
	19:  "小程序客服管理",
	25:  "开放平台账号管理（互斥）",
	30:  "小程序基本信息管理（类目）",
	37:  "附近的小程序管理",
	40:  "小程序插件管理（互斥）",
	52:  "小程序直播管理",
	65:  "小程序广告管理",
	76:  "小程序违规与交易投诉管理（互斥）",
	88:  "小程序链接管理",
	119: "小程序支付管理服务（互斥）",
	142: "小程序发货管理服务",
	182: "小程序订单管理服务",
}

// TemplateLibraryLimit 代码模板库数量上限（官方 200）。
const TemplateLibraryLimit = 200

// ---- GORM 模型 ----

// PlatformState 第三方平台运行时状态（单行，主键固定为 1）。密钥不入库，仅存运行时数据。
type PlatformState struct {
	ID                     uint       `gorm:"primaryKey" json:"id"`
	ComponentAppID         string     `gorm:"size:64" json:"componentAppid"`
	VerifyTicket           string     `gorm:"type:text" json:"-"`
	TicketReceivedAt       *time.Time `json:"ticketReceivedAt"`
	ComponentAccessToken   string     `gorm:"type:text" json:"-"`
	ComponentTokenExpireAt *time.Time `json:"componentTokenExpireAt"`
	LastPushOKAt           *time.Time `json:"lastPushOkAt"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

// TableName 指定表名。
func (PlatformState) TableName() string { return "platform_state" }

// Authorizer 已授权小程序。
type Authorizer struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	Appid         string `gorm:"size:64;uniqueIndex" json:"appid"`
	NickName      string `gorm:"size:255" json:"nickName"`
	HeadImg       string `gorm:"size:512" json:"headImg"`
	QrcodeURL     string `gorm:"size:512" json:"qrcodeUrl"`
	UserName      string `gorm:"size:64" json:"userName"`
	Alias         string `gorm:"size:128" json:"alias"`
	PrincipalName string `gorm:"size:255" json:"principalName"`
	Signature     string `gorm:"size:512" json:"signature"`

	ServiceTypeID int `json:"serviceTypeId"`
	VerifyTypeID  int `json:"verifyTypeId"`
	RegisterType  int `json:"registerType"`
	AccountStatus int `json:"accountStatus"`

	BusinessInfo    JSONMap  `gorm:"type:json" json:"businessInfo"`
	FuncInfo        IntSlice `gorm:"type:json" json:"funcInfo"`
	MiniProgramCats JSONMap  `gorm:"type:json" json:"miniProgramCategories"`

	AuthorizationStatus AuthorizationStatus `gorm:"size:32;index" json:"authorizationStatus"`
	AuthorizedAt        *time.Time          `json:"authorizedAt"`
	LastSyncAt          *time.Time          `json:"lastSyncAt"`

	RefreshTokenCipher    string     `gorm:"type:varbinary(512)" json:"-"`
	RefreshTokenUpdatedAt *time.Time `json:"refreshTokenUpdatedAt"`

	CodeSource     CodeSource    `gorm:"size:32" json:"codeSource"`
	Remark         string        `gorm:"size:512" json:"remark"`
	GroupName      string        `gorm:"size:128;index" json:"groupName"`
	Tags           StringSlice   `gorm:"type:json" json:"tags"`
	ExtVars        JSONStringMap `gorm:"type:json" json:"extVars"`
	AuditProfileID *uint         `json:"auditProfileId"`
	AuditOverride  JSONMap       `gorm:"type:json" json:"auditOverride"`
	Enabled        bool          `gorm:"default:true" json:"enabled"`

	PrivacyConfigured *bool      `json:"privacySettingConfigured"`
	DomainSnapshot    JSONMap    `gorm:"type:json" json:"domainSnapshot"`
	LastPreflightAt   *time.Time `json:"lastPreflightAt"`

	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
	DeletedAt *time.Time `gorm:"index" json:"deletedAt"`
}

// TableName 指定表名。
func (Authorizer) TableName() string { return "authorizers" }

// HasDevPermission 判断是否已授权开发权限集（18）。
func (a *Authorizer) HasDevPermission() bool {
	for _, id := range a.FuncInfo {
		if id == PermissionSetDev {
			return true
		}
	}
	return false
}

// TokenCache 令牌 / 票据相关的缓存（重启后仍可复用，减少对微信的调用）。
type TokenCache struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Scope     TokenScope `gorm:"size:32;uniqueIndex:idx_token_scope_appid" json:"scope"`
	Appid     string     `gorm:"size:64;uniqueIndex:idx_token_scope_appid" json:"appid"`
	Token     string     `gorm:"type:text" json:"-"`
	ExpiresAt time.Time  `json:"expiresAt"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// TableName 指定表名。
func (TokenCache) TableName() string { return "token_cache" }

// CodeDraft 草稿箱快照（每个开发小程序只保留最新一份上传记录）。
type CodeDraft struct {
	ID                     uint   `gorm:"primaryKey" json:"id"`
	DraftID                int64  `gorm:"uniqueIndex" json:"draftId"`
	UserVersion            string `gorm:"size:128" json:"userVersion"`
	UserDesc               string `gorm:"size:512" json:"userDesc"`
	SourceMiniProgramAppid string `gorm:"size:64;index" json:"sourceMiniProgramAppid"`
	SourceMiniProgram      string `gorm:"size:255" json:"sourceMiniProgram"`
	Developer              string `gorm:"size:128" json:"developer"`
	CreateTime             int64  `json:"createTime"`
	// autoCreateTime：该列 NOT NULL，调用方漏填时为 0 值会写成 '0000-00-00'，
	// MySQL 8 严格模式直接报 1292（MariaDB 默认容忍，因此本机测不出来）。填零值时由 GORM 补当前时间。
	SyncedAt  time.Time `gorm:"autoCreateTime" json:"syncedAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 指定表名。
func (CodeDraft) TableName() string { return "code_drafts" }

// CodeTemplate 模板库快照（模板不会被覆盖；仅普通模板可用于下发）。
type CodeTemplate struct {
	ID                     uint   `gorm:"primaryKey" json:"id"`
	TemplateID             int64  `gorm:"uniqueIndex" json:"templateId"`
	DraftID                int64  `json:"draftId"`
	TemplateType           int    `json:"templateType"`
	UserVersion            string `gorm:"size:128" json:"userVersion"`
	UserDesc               string `gorm:"size:512" json:"userDesc"`
	SourceMiniProgramAppid string `gorm:"size:64;index" json:"sourceMiniProgramAppid"`
	SourceMiniProgram      string `gorm:"size:255" json:"sourceMiniProgram"`
	Developer              string `gorm:"size:128" json:"developer"`
	CreateTime             int64  `json:"createTime"`
	IsDefault              bool   `json:"isDefault"`
	Note                   string `gorm:"size:512" json:"note"`
	// autoCreateTime：同 CodeDraft.SyncedAt，避免零值写成 '0000-00-00' 被 MySQL 8 严格模式拒绝。
	SyncedAt  time.Time `gorm:"autoCreateTime" json:"syncedAt"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 指定表名。
func (CodeTemplate) TableName() string { return "code_templates" }

// AuditProfile 提审配置模板：item_list 的 6 个类目字段必须取自 getAllCategoryName。
type AuditProfile struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Name             string    `gorm:"size:128;uniqueIndex" json:"name"`
	IsDefault        bool      `json:"isDefault"`
	ItemList         JSONMap   `gorm:"type:json" json:"itemList"`
	VersionDesc      string    `gorm:"size:512" json:"versionDesc"`
	UgcDeclare       JSONMap   `gorm:"type:json" json:"ugcDeclare"`
	PrivacyAPINotUse *bool     `json:"privacyApiNotUse"`
	OrderPath        string    `gorm:"size:512" json:"orderPath"`
	Note             string    `gorm:"size:512" json:"note"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// TableName 指定表名。
func (AuditProfile) TableName() string { return "audit_profiles" }

// BatchJob 批量作业。
type BatchJob struct {
	ID          string     `gorm:"primaryKey;size:36" json:"id"`
	Type        JobType    `gorm:"size:32;index" json:"type"`
	Status      JobStatus  `gorm:"size:32;index" json:"status"`
	DryRun      bool       `json:"dryRun"`
	Total       int        `json:"total"`
	Succeeded   int        `json:"succeeded"`
	Failed      int        `json:"failed"`
	Skipped     int        `json:"skipped"`
	Concurrency int        `json:"concurrency"`
	Payload     JSONMap    `gorm:"type:json" json:"payload"`
	Note        string     `gorm:"size:512" json:"note"`
	ErrorNote   string     `gorm:"size:1024" json:"errorNote"`
	CreatedBy   string     `gorm:"size:64" json:"createdBy"`
	RunnerID    string     `gorm:"size:64" json:"runnerId"`
	StartedAt   *time.Time `json:"startedAt"`
	FinishedAt  *time.Time `json:"finishedAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// TableName 指定表名。
func (BatchJob) TableName() string { return "batch_jobs" }

// BatchJobItem 作业逐项执行明细（step 使 pipeline 的每一步都可独立观察与重试）。
type BatchJobItem struct {
	ID          uint          `gorm:"primaryKey" json:"id"`
	JobID       string        `gorm:"size:36;uniqueIndex:idx_item_job_step_appid;index" json:"jobId"`
	Step        JobStep       `gorm:"size:32;uniqueIndex:idx_item_job_step_appid" json:"step"`
	Appid       string        `gorm:"size:64;uniqueIndex:idx_item_job_step_appid;index" json:"appid"`
	Status      JobItemStatus `gorm:"size:32;index" json:"status"`
	Attempt     int           `json:"attempt"`
	NextRunAt   *time.Time    `json:"nextRunAt"`
	UserVersion string        `gorm:"size:128" json:"userVersion"`
	WxAuditID   int64         `json:"wxAuditId"`
	Errcode     int           `json:"errcode"`
	Errmsg      string        `gorm:"size:1024" json:"errmsg"`
	ErrorClass  ErrorClass    `gorm:"size:32" json:"errorClass"`
	Request     JSONMap       `gorm:"type:json" json:"request"`
	Response    JSONMap       `gorm:"type:json" json:"response"`
	StartedAt   *time.Time    `json:"startedAt"`
	FinishedAt  *time.Time    `json:"finishedAt"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

// TableName 指定表名。
func (BatchJobItem) TableName() string { return "batch_job_items" }

// AuditRecord 审核单台账（一个版本一条）。
type AuditRecord struct {
	ID                 uint        `gorm:"primaryKey" json:"id"`
	Appid              string      `gorm:"size:64;uniqueIndex:idx_audit_appid_auditid" json:"appid"`
	AuditID            int64       `gorm:"uniqueIndex:idx_audit_appid_auditid" json:"auditId"`
	UserVersion        string      `gorm:"size:128" json:"userVersion"`
	UserDesc           string      `gorm:"size:512" json:"userDesc"`
	Status             int         `gorm:"index" json:"status"`
	Reason             string      `gorm:"type:text" json:"reason"`
	ScreenshotMediaIDs StringSlice `gorm:"type:json" json:"screenshotMediaIds"`
	SubmitTime         *time.Time  `json:"submitTime"`
	StatusTime         *time.Time  `json:"statusTime"`
	Source             AuditSource `gorm:"size:32" json:"source"`
	CreatedAt          time.Time   `json:"createdAt"`
	UpdatedAt          time.Time   `json:"updatedAt"`
}

// TableName 指定表名。
func (AuditRecord) TableName() string { return "audit_records" }

// 审核状态取值（官方 submit_audit / get_auditstatus 文档）。
const (
	// AuditStatusSuccess 审核成功（可发布）。
	AuditStatusSuccess = 0
	// AuditStatusRejected 审核被拒绝（reason 含拒绝原因）。
	AuditStatusRejected = 1
	// AuditStatusAuditing 审核中。
	AuditStatusAuditing = 2
	// AuditStatusWithdrawn 已撤回。
	AuditStatusWithdrawn = 3
	// AuditStatusDelayed 审核延后（reason 含延后原因）。
	AuditStatusDelayed = 4
)

// AuditStatusText 返回审核状态中文说明。
func AuditStatusText(status int) string {
	switch status {
	case AuditStatusSuccess:
		return "审核成功"
	case AuditStatusRejected:
		return "审核被拒绝"
	case AuditStatusAuditing:
		return "审核中"
	case AuditStatusWithdrawn:
		return "已撤回"
	case AuditStatusDelayed:
		return "审核延后"
	}
	return "未知状态"
}

// AuditStatusFromEvent 把审核结果推送事件名映射为状态码（官方事件名，没有 wxa_audit_status 这类写法）。
func AuditStatusFromEvent(event string) (int, bool) {
	switch event {
	case "weapp_audit_success":
		return AuditStatusSuccess, true
	case "weapp_audit_fail":
		return AuditStatusRejected, true
	case "weapp_audit_delay":
		return AuditStatusDelayed, true
	}
	return 0, false
}

// ReleaseRecord 发布台账。
type ReleaseRecord struct {
	ID             uint          `gorm:"primaryKey" json:"id"`
	Appid          string        `gorm:"size:64;index" json:"appid"`
	Action         ReleaseAction `gorm:"size:32" json:"action"`
	UserVersion    string        `gorm:"size:128" json:"userVersion"`
	GrayPercentage *int          `json:"grayPercentage"`
	ReleaseTime    *time.Time    `json:"releaseTime"`
	Note           string        `gorm:"size:512" json:"note"`
	CreatedAt      time.Time     `json:"createdAt"`
}

// TableName 指定表名。
func (ReleaseRecord) TableName() string { return "release_records" }

// UndoQuotaUsage 撤回审核的本地用量（每账号每天 ≤5 次、每月 ≤10 次，超限 87013）。
type UndoQuotaUsage struct {
	ID       uint      `gorm:"primaryKey" json:"id"`
	Appid    string    `gorm:"size:64;index" json:"appid"`
	UsedAt   time.Time `json:"usedAt"`
	MonthKey string    `gorm:"size:7;index" json:"monthKey"`
}

// TableName 指定表名。
func (UndoQuotaUsage) TableName() string { return "undo_quota_usage" }

// ApiCallLog 微信接口调用日志（排障核心：含请求/响应与返回码分类）。
type ApiCallLog struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	JobID      string     `gorm:"size:36;index" json:"jobId"`
	Appid      string     `gorm:"size:64;index" json:"appid"`
	Scope      TokenScope `gorm:"size:32" json:"scope"`
	Endpoint   string     `gorm:"size:128;index" json:"endpoint"`
	Method     string     `gorm:"size:8" json:"method"`
	HTTPStatus int        `json:"httpStatus"`
	OK         bool       `json:"ok"`
	Errcode    int        `gorm:"index" json:"errcode"`
	Errmsg     string     `gorm:"size:1024" json:"errmsg"`
	ErrorClass ErrorClass `gorm:"size:32;index" json:"errorClass"`
	DurationMs int64      `json:"durationMs"`
	Request    JSONMap    `gorm:"type:json" json:"request"`
	Response   JSONMap    `gorm:"type:json" json:"response"`
	CreatedAt  time.Time  `gorm:"index" json:"createdAt"`
}

// TableName 指定表名。
func (ApiCallLog) TableName() string { return "api_call_logs" }

// CallbackEvent 回调事件原文与处理结果（幂等与审计依据）。
type CallbackEvent struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	Kind        CallbackKind `gorm:"size:32;index" json:"kind"`
	Appid       string       `gorm:"size:64;index" json:"appid"`
	InfoType    string       `gorm:"size:64;index" json:"infoType"`
	Event       string       `gorm:"size:64" json:"event"`
	Encrypted   string       `gorm:"type:text" json:"-"`
	Decrypted   string       `gorm:"type:text" json:"decrypted"`
	DedupeKey   string       `gorm:"size:64;uniqueIndex" json:"-"`
	SignatureOK bool         `json:"signatureOk"`
	Processed   bool         `json:"processed"`
	ProcessNote string       `gorm:"size:1024" json:"processNote"`
	ReceivedAt  time.Time    `gorm:"index" json:"receivedAt"`
}

// TableName 指定表名。
func (CallbackEvent) TableName() string { return "callback_events" }

// OperationLog 平台操作审计。
type OperationLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Actor      string    `gorm:"size:64" json:"actor"`
	Action     string    `gorm:"size:64;index" json:"action"`
	TargetType string    `gorm:"size:64" json:"targetType"`
	TargetID   string    `gorm:"size:128" json:"targetId"`
	Detail     JSONMap   `gorm:"type:json" json:"detail"`
	IP         string    `gorm:"size:64" json:"ip"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}

// TableName 指定表名。
func (OperationLog) TableName() string { return "operation_logs" }

// RuntimeSetting 可在线调整的运行参数（非密钥）。
type RuntimeSetting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"size:1024" json:"value"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TableName 指定表名。
func (RuntimeSetting) TableName() string { return "runtime_settings" }

// 运行参数键。
const (
	SettingJobConcurrency        = "job_concurrency"
	SettingJobMaxAttempts        = "job_max_attempts"
	SettingWxMaxQps              = "wx_max_qps"
	SettingLogRetentionDays      = "log_retention_days"
	SettingDefaultTemplateID     = "default_template_id"
	SettingDefaultAuditProfileID = "default_audit_profile_id"
	SettingWxRequestTimeout      = "wx_request_timeout_seconds"
	SettingPrivacyCheckMaxWait   = "privacy_check_max_wait_seconds"
	SettingAuditResultMaxWait    = "audit_result_max_wait_seconds"
)

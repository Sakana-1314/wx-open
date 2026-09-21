// Package handler 实现 oapi-codegen 生成的 ServerInterface（HTTP 编解码层）。
//
// 约定：handler 只做参数绑定、调用 service、写响应；业务规则一律在 service/core 层。
// 各分组的端点在 handlers_*.go 中，本文件只放 Server 定义与鉴权/平台/日志/设置等端点。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wx-platform/server/internal/auth"
	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/service"
)

// Server 实现全部 API 端点。
type Server struct {
	svc *service.Container
}

// NewServer 构造 Server。
func NewServer(svc *service.Container) *Server { return &Server{svc: svc} }

// Login 管理员登录，签发 JWT。见 api/openapi.yaml。
func (s *Server) Login(c *gin.Context) {
	var req gen.LoginJSONRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBadRequest(c, "请求体格式错误")
		return
	}
	token, err := s.svc.Auth.Login(req.Username, req.Password)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, gen.LoginResponse{
		Token: token,
		User: gen.UserInfo{
			Username: s.svc.Auth.CurrentUser(),
			UserType: gen.Admin,
		},
	})
}

// GetCurrentUser 返回当前登录用户，用于前端校验令牌有效性。见 api/openapi.yaml。
func (s *Server) GetCurrentUser(c *gin.Context) {
	claims := auth.ClaimsFrom(c)
	if claims == nil {
		writeServiceError(c, core.ErrUnauthorized)
		return
	}
	c.JSON(http.StatusOK, gen.UserInfo{Username: claims.Subject, UserType: gen.Admin})
}

// ListApiCallLogs 见 api/openapi.yaml。
func (s *Server) ListApiCallLogs(c *gin.Context, params gen.ListApiCallLogsParams) {
	res, err := s.svc.Logs.ApiCalls(c.Request.Context(), params)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// ListCallbackEvents 见 api/openapi.yaml。
func (s *Server) ListCallbackEvents(c *gin.Context, params gen.ListCallbackEventsParams) {
	res, err := s.svc.Logs.Callbacks(c.Request.Context(), params)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// ListOperationLogs 见 api/openapi.yaml。
func (s *Server) ListOperationLogs(c *gin.Context, params gen.ListOperationLogsParams) {
	res, err := s.svc.Logs.Operations(c.Request.Context(), params)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// ListAuditProfiles 提审配置列表（类目/标题/标签/UGC 声明等，供批量提审复用）。见 api/openapi.yaml。
func (s *Server) ListAuditProfiles(c *gin.Context) {
	res, err := s.svc.Audit.ListProfiles(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// CreateAuditProfile 新建提审配置。见 api/openapi.yaml。
func (s *Server) CreateAuditProfile(c *gin.Context) {
	var req gen.AuditProfileUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBadRequest(c, "请求体格式错误")
		return
	}
	res, err := s.svc.Audit.CreateProfile(c.Request.Context(), req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, res)
}

// UpdateAuditProfile 编辑提审配置。见 api/openapi.yaml。
func (s *Server) UpdateAuditProfile(c *gin.Context, id int64) {
	var req gen.AuditProfileUpsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBadRequest(c, "请求体格式错误")
		return
	}
	res, err := s.svc.Audit.UpdateProfile(c.Request.Context(), id, req)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// DeleteAuditProfile 删除提审配置（被引用时返回 409）。见 api/openapi.yaml。
func (s *Server) DeleteAuditProfile(c *gin.Context, id int64) {
	if err := s.svc.Audit.DeleteProfile(c.Request.Context(), id); err != nil {
		writeServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// EnsurePushTicket 调用 api_start_push_ticket 恢复票据推送。见 api/openapi.yaml。
func (s *Server) EnsurePushTicket(c *gin.Context) {
	if err := s.svc.Env.Platform.EnsurePushTicket(c.Request.Context()); err != nil {
		writeServiceError(c, err)
		return
	}
	noContent(c)
}

// GetSettings 见 api/openapi.yaml。
func (s *Server) GetSettings(c *gin.Context) {
	res, err := s.svc.Env.Settings.Get()
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// UpdateSettings 见 api/openapi.yaml。
func (s *Server) UpdateSettings(c *gin.Context) {
	var req gen.SettingsUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBadRequest(c, "请求体格式错误")
		return
	}
	res, err := s.svc.Env.Settings.Update(core.SettingsPatch{
		JobConcurrency:             req.JobConcurrency,
		JobMaxAttempts:             req.JobMaxAttempts,
		WxMaxQps:                   req.WxMaxQps,
		LogRetentionDays:           req.LogRetentionDays,
		DefaultTemplateID:          req.DefaultTemplateId,
		DefaultAuditProfileID:      req.DefaultAuditProfileId,
		WxRequestTimeoutSeconds:    req.WxRequestTimeoutSeconds,
		PrivacyCheckMaxWaitSeconds: req.PrivacyCheckMaxWaitSeconds,
		AuditResultMaxWaitSeconds:  req.AuditResultMaxWaitSeconds,
	})
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetPlatformStatus 见 api/openapi.yaml。
func (s *Server) GetPlatformStatus(c *gin.Context) {
	res, err := s.svc.Env.Platform.Status(c.Request.Context())
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

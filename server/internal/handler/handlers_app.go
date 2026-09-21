package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wx-platform/server/internal/gen"
)

// 本文件实现「单个小程序的域名 / 页面 / 服务状态 / 基础库」端点。

// ApplyDomains 批量配置小程序服务器域名 / 业务域名。见 api/openapi.yaml。
func (s *Server) ApplyDomains(c *gin.Context) {
	var req gen.DomainApplyRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.App.ApplyDomains(c.Request.Context(), req)
	callAndRespond(c, res, err, http.StatusOK)
}

// ListCodePages 已上传代码的页面列表（供提审配置选地址）。见 api/openapi.yaml。
func (s *Server) ListCodePages(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.App.Pages(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// GetSupportVersion 最低基础库版本与用户占比。见 api/openapi.yaml。
func (s *Server) GetSupportVersion(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.App.SupportVersion(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// SetSupportVersion 设置最低基础库版本。见 api/openapi.yaml。
func (s *Server) SetSupportVersion(c *gin.Context, appid gen.Appid) {
	var req gen.SupportVersionUpdateRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.App.SetSupportVersion(c.Request.Context(), string(appid), req)
	callAndRespond(c, res, err, http.StatusOK)
}

// GetVisitStatus 查询小程序服务状态。见 api/openapi.yaml。
func (s *Server) GetVisitStatus(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.App.VisitStatus(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// SetVisitStatus 暂停 / 恢复服务。见 api/openapi.yaml。
func (s *Server) SetVisitStatus(c *gin.Context, appid gen.Appid) {
	var req gen.VisitStatusUpdateRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.App.SetVisitStatus(c.Request.Context(), string(appid), req)
	callAndRespond(c, res, err, http.StatusOK)
}

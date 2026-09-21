package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wx-platform/server/internal/gen"
)

// 本文件实现「审核管理」与「发布管理」组的端点。

// ListAudits 审核单列表。见 api/openapi.yaml。
func (s *Server) ListAudits(c *gin.Context, params gen.ListAuditsParams) {
	res, err := s.svc.Audit.List(c.Request.Context(), params)
	callAndRespond(c, res, err, http.StatusOK)
}

// SyncAudits 向微信侧核对审核状态（事件推送的兜底对账）。见 api/openapi.yaml。
func (s *Server) SyncAudits(c *gin.Context) {
	var req gen.AppidSelection
	if !bindOptionalJSON(c, &req) {
		return
	}
	res, err := s.svc.Audit.Sync(c.Request.Context(), req)
	callAndRespond(c, res, err, http.StatusOK)
}

// ListAuditHistory 单个小程序的审核历史。见 api/openapi.yaml。
func (s *Server) ListAuditHistory(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.Audit.History(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// GetAuditQuota 查询提审与加急额度（旗下小程序共用）。见 api/openapi.yaml。
func (s *Server) GetAuditQuota(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.Audit.Quota(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// SpeedUpAudit 加急审核。见 api/openapi.yaml。
func (s *Server) SpeedUpAudit(c *gin.Context, appid gen.Appid) {
	var req gen.SpeedUpRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.Audit.SpeedUp(c.Request.Context(), string(appid), req)
	callAndRespond(c, res, err, http.StatusOK)
}

// UndoAudit 撤回审核（每天 ≤5 次、每月 ≤10 次）。见 api/openapi.yaml。
func (s *Server) UndoAudit(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.Audit.Undo(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// ListReleases 发布台账。见 api/openapi.yaml。
func (s *Server) ListReleases(c *gin.Context, params gen.ListReleasesParams) {
	res, err := s.svc.Release.List(c.Request.Context(), params)
	callAndRespond(c, res, err, http.StatusOK)
}

// GetVersionInfo 体验版与线上版信息。见 api/openapi.yaml。
func (s *Server) GetVersionInfo(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.Release.VersionInfo(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// ReleaseApp 发布最后一个审核通过的版本（全量、立即生效）。见 api/openapi.yaml。
func (s *Server) ReleaseApp(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.Release.Release(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// GrayReleaseApp 分阶段发布（灰度比例只能递增）。见 api/openapi.yaml。
func (s *Server) GrayReleaseApp(c *gin.Context, appid gen.Appid) {
	var req gen.GrayReleaseRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.Release.GrayRelease(c.Request.Context(), string(appid), req)
	callAndRespond(c, res, err, http.StatusOK)
}

// GetGrayReleasePlan 查询分阶段发布计划。见 api/openapi.yaml。
func (s *Server) GetGrayReleasePlan(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.Release.GrayPlan(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// RevertGrayRelease 取消分阶段发布。见 api/openapi.yaml。
func (s *Server) RevertGrayRelease(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.Release.RevertGray(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// ListRevertVersions 可回退的历史版本（最多最近 5 个）。见 api/openapi.yaml。
func (s *Server) ListRevertVersions(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.Release.HistoryVersions(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// RevertRelease 版本回退。见 api/openapi.yaml。
func (s *Server) RevertRelease(c *gin.Context, appid gen.Appid) {
	var req gen.RevertRequest
	if !bindOptionalJSON(c, &req) {
		return
	}
	res, err := s.svc.Release.Revert(c.Request.Context(), string(appid), &req)
	callAndRespond(c, res, err, http.StatusOK)
}

// GetTrialQRCode 体验版二维码（返回 base64 图片）。见 api/openapi.yaml。
func (s *Server) GetTrialQRCode(c *gin.Context, appid gen.Appid, params gen.GetTrialQRCodeParams) {
	res, err := s.svc.Release.TrialQRCode(c.Request.Context(), string(appid), params)
	callAndRespond(c, res, err, http.StatusOK)
}

// 编译期断言：Server 必须实现全部契约端点。
var _ gen.ServerInterface = (*Server)(nil)

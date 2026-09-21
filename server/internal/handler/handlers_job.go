package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wx-platform/server/internal/gen"
)

// 本文件实现「批量作业」与「前置体检」组的端点。

// PreviewJob 预览批量作业的逐项最终请求体（不调用微信）。见 api/openapi.yaml。
func (s *Server) PreviewJob(c *gin.Context) {
	var req gen.JobCreateRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.Job.Preview(c.Request.Context(), req)
	callAndRespond(c, res, err, http.StatusOK)
}

// CreateJob 创建批量作业。见 api/openapi.yaml。
func (s *Server) CreateJob(c *gin.Context) {
	var req gen.JobCreateRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.Job.Create(c.Request.Context(), req, actorOf(c))
	callAndRespond(c, res, err, http.StatusCreated)
}

// ListJobs 作业列表。见 api/openapi.yaml。
func (s *Server) ListJobs(c *gin.Context, params gen.ListJobsParams) {
	res, err := s.svc.Job.List(c.Request.Context(), params)
	callAndRespond(c, res, err, http.StatusOK)
}

// GetJob 作业详情（含进度统计）。见 api/openapi.yaml。
func (s *Server) GetJob(c *gin.Context, id string) {
	res, err := s.svc.Job.Get(c.Request.Context(), id)
	callAndRespond(c, res, err, http.StatusOK)
}

// ListJobItems 作业逐项执行明细。见 api/openapi.yaml。
func (s *Server) ListJobItems(c *gin.Context, id string, params gen.ListJobItemsParams) {
	res, err := s.svc.Job.Items(c.Request.Context(), id, params)
	callAndRespond(c, res, err, http.StatusOK)
}

// StartJob 启动作业。见 api/openapi.yaml。
func (s *Server) StartJob(c *gin.Context, id gen.JobId) {
	res, err := s.svc.Job.Start(c.Request.Context(), string(id))
	callAndRespond(c, res, err, http.StatusOK)
}

// PauseJob 暂停作业。见 api/openapi.yaml。
func (s *Server) PauseJob(c *gin.Context, id gen.JobId) {
	res, err := s.svc.Job.Pause(c.Request.Context(), string(id))
	callAndRespond(c, res, err, http.StatusOK)
}

// ResumeJob 恢复作业。见 api/openapi.yaml。
func (s *Server) ResumeJob(c *gin.Context, id gen.JobId) {
	res, err := s.svc.Job.Resume(c.Request.Context(), string(id))
	callAndRespond(c, res, err, http.StatusOK)
}

// CancelJob 取消作业（未开始项标记取消）。见 api/openapi.yaml。
func (s *Server) CancelJob(c *gin.Context, id gen.JobId) {
	res, err := s.svc.Job.Cancel(c.Request.Context(), string(id))
	callAndRespond(c, res, err, http.StatusOK)
}

// RetryFailedJobItems 重试失败项。见 api/openapi.yaml。
func (s *Server) RetryFailedJobItems(c *gin.Context, id gen.JobId) {
	res, err := s.svc.Job.RetryFailed(c.Request.Context(), string(id))
	callAndRespond(c, res, err, http.StatusOK)
}

// RunPreflight 对目标小程序逐项体检。见 api/openapi.yaml。
func (s *Server) RunPreflight(c *gin.Context) {
	var req gen.PreflightRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.Preflight.Run(c.Request.Context(), req)
	callAndRespond(c, res, err, http.StatusOK)
}

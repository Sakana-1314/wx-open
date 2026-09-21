package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"wx-platform/server/internal/gen"
)

// 本文件实现「小程序管理」与「代码模板库」两组的端点，只做参数绑定与响应写出。

// ListAuthorizers 小程序列表（分页 + 筛选）。见 api/openapi.yaml。
func (s *Server) ListAuthorizers(c *gin.Context, params gen.ListAuthorizersParams) {
	res, err := s.svc.Authorizer.List(c.Request.Context(), params)
	callAndRespond(c, res, err, http.StatusOK)
}

// GetAuthorizer 小程序详情（含权限集、类目与域名快照）。见 api/openapi.yaml。
func (s *Server) GetAuthorizer(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.Authorizer.Get(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// UpdateAuthorizer 编辑备注 / 分组 / 标签 / 启停 / ext 变量 / 提审配置。见 api/openapi.yaml。
func (s *Server) UpdateAuthorizer(c *gin.Context, appid gen.Appid) {
	var req gen.AuthorizerUpdateRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.Authorizer.Update(c.Request.Context(), string(appid), req)
	callAndRespond(c, res, err, http.StatusOK)
}

// DeleteAuthorizer 删除未授权的小程序记录。见 api/openapi.yaml。
func (s *Server) DeleteAuthorizer(c *gin.Context, appid gen.Appid) {
	if err := s.svc.Authorizer.Delete(c.Request.Context(), string(appid)); err != nil {
		writeServiceError(c, err)
		return
	}
	noContent(c)
}

// SyncAuthorizer 同步单个小程序信息。见 api/openapi.yaml。
func (s *Server) SyncAuthorizer(c *gin.Context, appid gen.Appid) {
	res, err := s.svc.Authorizer.Sync(c.Request.Context(), string(appid))
	callAndRespond(c, res, err, http.StatusOK)
}

// SyncAuthorizers 从微信侧同步授权方清单与信息（全量或指定）。见 api/openapi.yaml。
func (s *Server) SyncAuthorizers(c *gin.Context) {
	var req gen.AppidSelection
	if !bindOptionalJSON(c, &req) {
		return
	}
	res, err := s.svc.Authorizer.SyncMany(c.Request.Context(), req)
	callAndRespond(c, res, err, http.StatusOK)
}

// ResyncAuthorizerTokens 重新拉取全部 refresh_token（官方恢复路径）。见 api/openapi.yaml。
func (s *Server) ResyncAuthorizerTokens(c *gin.Context) {
	res, err := s.svc.Authorizer.ResyncTokens(c.Request.Context())
	callAndRespond(c, res, err, http.StatusOK)
}

// GetAuthorizationUrl 生成预授权码与授权链接。见 api/openapi.yaml。
func (s *Server) GetAuthorizationUrl(c *gin.Context, params gen.GetAuthorizationUrlParams) {
	res, err := s.svc.Authorizer.AuthorizationURL(c.Request.Context(), params)
	callAndRespond(c, res, err, http.StatusOK)
}

// CompleteAuthorize 用授权码换取令牌并登记小程序。见 api/openapi.yaml。
func (s *Server) CompleteAuthorize(c *gin.Context) {
	var req gen.AuthorizeRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.Authorizer.CompleteAuthorize(c.Request.Context(), req)
	callAndRespond(c, res, err, http.StatusOK)
}

// GetAuthorizerOption 读取授权方选项。见 api/openapi.yaml。
func (s *Server) GetAuthorizerOption(c *gin.Context, appid gen.Appid, params gen.GetAuthorizerOptionParams) {
	res, err := s.svc.Authorizer.GetOption(c.Request.Context(), string(appid), params)
	callAndRespond(c, res, err, http.StatusOK)
}

// SetAuthorizerOption 设置授权方选项。见 api/openapi.yaml。
func (s *Server) SetAuthorizerOption(c *gin.Context, appid gen.Appid) {
	var req gen.AuthorizerOptionUpdateRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.Authorizer.SetOption(c.Request.Context(), string(appid), req)
	callAndRespond(c, res, err, http.StatusOK)
}

// ListDrafts 代码草稿箱列表（component_access_token）。见 api/openapi.yaml。
func (s *Server) ListDrafts(c *gin.Context) {
	res, err := s.svc.Template.ListDrafts(c.Request.Context())
	callAndRespond(c, res, err, http.StatusOK)
}

// SyncDrafts 从微信侧同步草稿箱。见 api/openapi.yaml。
func (s *Server) SyncDrafts(c *gin.Context) {
	res, err := s.svc.Template.SyncDrafts(c.Request.Context())
	callAndRespond(c, res, err, http.StatusOK)
}

// AddDraftToTemplate 将草稿添加到模板库（上限 200，超限报 85065）。见 api/openapi.yaml。
func (s *Server) AddDraftToTemplate(c *gin.Context, draftId int64) {
	var req gen.AddToTemplateRequest
	if !bindOptionalJSON(c, &req) {
		return
	}
	res, err := s.svc.Template.AddDraftToTemplate(c.Request.Context(), draftId, &req)
	callAndRespond(c, res, err, http.StatusOK)
}

// ListTemplates 代码模板库列表。见 api/openapi.yaml。
func (s *Server) ListTemplates(c *gin.Context, params gen.ListTemplatesParams) {
	res, err := s.svc.Template.ListTemplates(c.Request.Context(), params)
	callAndRespond(c, res, err, http.StatusOK)
}

// SyncTemplates 从微信侧同步模板库。见 api/openapi.yaml。
func (s *Server) SyncTemplates(c *gin.Context) {
	res, err := s.svc.Template.SyncTemplates(c.Request.Context())
	callAndRespond(c, res, err, http.StatusOK)
}

// UpdateTemplate 标记默认模板 / 备注。见 api/openapi.yaml。
func (s *Server) UpdateTemplate(c *gin.Context, templateId int64) {
	var req gen.TemplateUpdateRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := s.svc.Template.UpdateTemplate(c.Request.Context(), templateId, req)
	callAndRespond(c, res, err, http.StatusOK)
}

// DeleteTemplate 删除代码模板。见 api/openapi.yaml。
func (s *Server) DeleteTemplate(c *gin.Context, templateId int64) {
	if err := s.svc.Template.DeleteTemplate(c.Request.Context(), templateId); err != nil {
		writeServiceError(c, err)
		return
	}
	noContent(c)
}

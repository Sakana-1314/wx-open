package wxauth

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
	"wx-platform/server/internal/wxapi"
)

// TemplateService 代码草稿箱与代码模板库（第三方平台侧，令牌统一为 component_access_token）。
//
// 官方要点：
//   - 草稿箱是「当下状态」，每次同步整体替换；模板一旦添加就不会被覆盖，只能删除；
//   - addtotemplate **不返回 template_id**，必须再调 gettemplatelist 才能拿到新模板；
//   - 模板库上限 200 个（超限报 85065）；删除时模板不存在返回 85064。
type TemplateService struct {
	base
}

// NewTemplateService 构造模板库服务。
func NewTemplateService(env *core.Env) *TemplateService {
	return &TemplateService{base: newBase(env)}
}

// ListDrafts 草稿箱列表。
//
// 优先读本地快照（微信接口是 GET 但没必要每次列表都打微信）；
// 库为空时（首次部署或刚清库）自动同步一次，免得前端看到空列表以为没有草稿。
func (s *TemplateService) ListDrafts(ctx context.Context) (*gen.DraftListResponse, error) {
	if err := s.ensureRepos(); err != nil {
		return nil, err
	}
	items, err := s.repos().Drafts.List(ctx)
	if err != nil {
		return nil, core.Internal(err)
	}
	if len(items) == 0 {
		if err := s.ensureWeChat(); err != nil {
			return nil, err
		}
		if _, err := s.SyncDrafts(ctx); err != nil {
			return nil, err
		}
		if items, err = s.repos().Drafts.List(ctx); err != nil {
			return nil, core.Internal(err)
		}
	}
	out := &gen.DraftListResponse{Items: make([]gen.CodeDraft, 0, len(items))}
	for i := range items {
		out.Items = append(out.Items, toCodeDraft(&items[i]))
	}
	return out, nil
}

// SyncDrafts 从微信侧同步草稿箱并整体替换本地快照。
func (s *TemplateService) SyncDrafts(ctx context.Context) (*gen.SyncSummary, error) {
	if err := s.ensureRepos(); err != nil {
		return nil, err
	}
	token, err := s.componentToken(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.env.Wx.GetTemplateDraftList(ctx, token)
	if err != nil {
		return nil, wxErr("", err)
	}
	now := time.Now()
	rows := make([]model.CodeDraft, 0, len(list))
	details := make([]gen.SyncDetail, 0, len(list))
	for _, d := range list {
		rows = append(rows, model.CodeDraft{
			DraftID:                d.DraftID,
			UserVersion:            d.UserVersion,
			UserDesc:               d.UserDesc,
			SourceMiniProgramAppid: d.SourceMiniProgramAppid,
			SourceMiniProgram:      d.SourceMiniProgram,
			Developer:              d.Developer,
			CreateTime:             d.CreateTime,
			SyncedAt:               now,
		})
		details = append(details, gen.SyncDetail{Key: strconv.FormatInt(d.DraftID, 10), Ok: true})
	}
	if err := s.repos().Drafts.ReplaceAll(ctx, rows); err != nil {
		return nil, core.Internal(err)
	}
	summary := &gen.SyncSummary{
		Total:     len(rows),
		Succeeded: len(rows),
		Details:   &details,
	}
	s.writeOperation(ctx, actionSyncDrafts, "code_draft", "", model.JSONMap{"total": summary.Total})
	return summary, nil
}

// AddDraftToTemplate 把草稿添加到代码模板库，并返回同步后的模板列表。
//
// 官方接口不返回 template_id，因此这里必须再拉一次 gettemplatelist（否则前端拿不到新模板）。
func (s *TemplateService) AddDraftToTemplate(ctx context.Context, draftID int64, req *gen.AddToTemplateRequest) (*gen.TemplateListResponse, error) {
	if err := s.ensureRepos(); err != nil {
		return nil, err
	}
	if draftID <= 0 {
		return nil, core.Validation("draftId 必须为正整数")
	}
	templateType := 0
	if req != nil && req.TemplateType != nil {
		templateType = int(*req.TemplateType)
		if !gen.AddToTemplateRequestTemplateType(templateType).Valid() {
			return nil, core.Validation("templateType 只能是 0（普通模板，唯一可用）或 1（标准模板，官方已下架）")
		}
	}
	if _, err := s.repos().Drafts.Get(ctx, draftID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, core.NotFound("草稿 %d 不存在：请先同步草稿箱（草稿在微信侧保留时间有限）", draftID)
		}
		return nil, core.Internal(err)
	}
	// 提前检查上限：模板库满时微信会返回 85065，这里先给出可执行的中文提示。
	count, err := s.repos().Templates.Count(ctx)
	if err != nil {
		return nil, core.Internal(err)
	}
	if count >= model.TemplateLibraryLimit {
		return nil, core.Conflict("模板库已达上限 %d 个（官方限制）：请先删除不再使用的模板（85065 表示模板库已满）再添加新模板", model.TemplateLibraryLimit)
	}

	token, err := s.componentToken(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.env.Wx.AddToTemplate(ctx, token, draftID, templateType); err != nil {
		return nil, wxErr("", err)
	}
	// 官方：addtotemplate 不返回 template_id，必须重新拉取模板列表才能拿到新模板。
	if _, err := s.SyncTemplates(ctx); err != nil {
		return nil, err
	}
	s.writeOperation(ctx, actionAddToTemplate, "code_draft", strconv.FormatInt(draftID, 10), model.JSONMap{
		"templateType": templateType,
	})
	return s.ListTemplates(ctx, gen.ListTemplatesParams{})
}

// ListTemplates 代码模板库列表（templateType 为 nil 表示全部）。
func (s *TemplateService) ListTemplates(ctx context.Context, params gen.ListTemplatesParams) (*gen.TemplateListResponse, error) {
	if err := s.ensureRepos(); err != nil {
		return nil, err
	}
	var filter *int
	if params.TemplateType != nil {
		if !params.TemplateType.Valid() {
			return nil, core.Validation("templateType 只能是 0（普通模板，唯一可用）或 1（标准模板，官方已下架）")
		}
		value := int(*params.TemplateType)
		filter = &value
	}
	items, err := s.repos().Templates.List(ctx, filter)
	if err != nil {
		return nil, core.Internal(err)
	}
	if len(items) == 0 {
		// 本地模板库为空时自动同步一次（首次部署 / 刚清库）。
		total, err := s.repos().Templates.Count(ctx)
		if err != nil {
			return nil, core.Internal(err)
		}
		if total == 0 {
			if err := s.ensureWeChat(); err != nil {
				return nil, err
			}
			if _, err := s.SyncTemplates(ctx); err != nil {
				return nil, err
			}
			if items, err = s.repos().Templates.List(ctx, filter); err != nil {
				return nil, core.Internal(err)
			}
		}
	}
	out := &gen.TemplateListResponse{
		Items: make([]gen.CodeTemplate, 0, len(items)),
		Limit: model.TemplateLibraryLimit,
	}
	for i := range items {
		out.Items = append(out.Items, toCodeTemplate(&items[i]))
	}
	return out, nil
}

// SyncTemplates 从微信侧同步模板库并整体替换本地快照（保留本地的默认标记与备注）。
func (s *TemplateService) SyncTemplates(ctx context.Context) (*gen.SyncSummary, error) {
	if err := s.ensureRepos(); err != nil {
		return nil, err
	}
	token, err := s.componentToken(ctx)
	if err != nil {
		return nil, err
	}
	list, err := s.env.Wx.GetTemplateList(ctx, token, nil)
	if err != nil {
		return nil, wxErr("", err)
	}
	now := time.Now()
	rows := make([]model.CodeTemplate, 0, len(list))
	details := make([]gen.SyncDetail, 0, len(list))
	for _, t := range list {
		rows = append(rows, model.CodeTemplate{
			TemplateID:             t.TemplateID,
			DraftID:                t.DraftID,
			TemplateType:           t.TemplateType,
			UserVersion:            t.UserVersion,
			UserDesc:               t.UserDesc,
			SourceMiniProgramAppid: t.SourceMiniProgramAppid,
			SourceMiniProgram:      t.SourceMiniProgram,
			Developer:              t.Developer,
			CreateTime:             t.CreateTime,
			SyncedAt:               now,
		})
		details = append(details, gen.SyncDetail{Key: strconv.FormatInt(t.TemplateID, 10), Ok: true})
	}
	if err := s.repos().Templates.ReplaceAll(ctx, rows); err != nil {
		return nil, core.Internal(err)
	}
	summary := &gen.SyncSummary{
		Total:     len(rows),
		Succeeded: len(rows),
		Details:   &details,
	}
	s.writeOperation(ctx, actionSyncTemplates, "code_template", "", model.JSONMap{"total": summary.Total})
	return summary, nil
}

// UpdateTemplate 更新模板的本地维护字段（默认模板 / 运维备注）。
//
// 只改本地：微信侧模板内容不可修改（只能删掉重新从草稿添加）。
func (s *TemplateService) UpdateTemplate(ctx context.Context, templateID int64, req gen.TemplateUpdateRequest) (*gen.CodeTemplate, error) {
	if err := s.ensureRepos(); err != nil {
		return nil, err
	}
	if templateID <= 0 {
		return nil, core.Validation("templateId 必须为正整数")
	}
	if _, err := s.repos().Templates.Get(ctx, templateID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, core.NotFound("模板 %d 不存在：请先同步模板库", templateID)
		}
		return nil, core.Internal(err)
	}

	changed := make([]string, 0, 2)
	if req.Note != nil {
		if err := s.repos().Templates.UpdateNote(ctx, templateID, strings.TrimSpace(*req.Note)); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return nil, core.NotFound("模板 %d 不存在：请先同步模板库", templateID)
			}
			return nil, core.Internal(err)
		}
		changed = append(changed, "note")
	}
	if req.IsDefault != nil && *req.IsDefault {
		// 默认模板全局唯一，由 repo 在事务里保证（先清后置）。
		if err := s.repos().Templates.SetDefault(ctx, templateID); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return nil, core.NotFound("模板 %d 不存在：请先同步模板库", templateID)
			}
			return nil, core.Internal(err)
		}
		changed = append(changed, "isDefault")
	}
	// isDefault=false 是「取消默认」：平台不提供「没有默认模板」的状态，
	// 直接改会把默认模板留空（批量下发无从取默认），因此这里忽略，由前端改为把另一个模板设为默认。

	updated, err := s.repos().Templates.Get(ctx, templateID)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, core.NotFound("模板 %d 不存在：请先同步模板库", templateID)
		}
		return nil, core.Internal(err)
	}
	if len(changed) > 0 {
		s.writeOperation(ctx, actionUpdateTemplate, "code_template", strconv.FormatInt(templateID, 10), model.JSONMap{"fields": changed})
	}
	out := toCodeTemplate(updated)
	return &out, nil
}

// DeleteTemplate 删除代码模板：先删微信侧（85064 视为已不存在，继续删本地），再删本地快照。
func (s *TemplateService) DeleteTemplate(ctx context.Context, templateID int64) error {
	if err := s.ensureRepos(); err != nil {
		return err
	}
	if templateID <= 0 {
		return core.Validation("templateId 必须为正整数")
	}
	if _, err := s.repos().Templates.Get(ctx, templateID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return core.NotFound("模板 %d 不存在：请先同步模板库", templateID)
		}
		return core.Internal(err)
	}
	token, err := s.componentToken(ctx)
	if err != nil {
		return err
	}
	if err := s.env.Wx.DeleteTemplate(ctx, token, templateID); err != nil {
		if apiErr, ok := wxapi.IsAPIError(err); !ok || apiErr.Errcode != 85064 {
			// 85064「找不到模板」说明微信侧已经不存在，继续删本地即可。
			return wxErr("", err)
		}
	}
	if err := s.repos().Templates.Delete(ctx, templateID); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return core.NotFound("模板 %d 不存在：请先同步模板库", templateID)
		}
		return core.Internal(err)
	}
	s.writeOperation(ctx, actionDeleteTemplate, "code_template", strconv.FormatInt(templateID, 10), nil)
	return nil
}

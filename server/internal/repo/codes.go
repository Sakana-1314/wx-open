package repo

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"wx-platform/server/internal/model"
)

// Drafts 草稿箱快照仓储（草稿箱是「当下状态」，每次同步整体替换）。
type Drafts struct{ db *gorm.DB }

// NewDrafts 构造 Drafts 仓储。
func NewDrafts(db *gorm.DB) *Drafts { return &Drafts{db: db} }

// ReplaceAll 事务内整体替换草稿箱快照（先清空再写入）。
// 事务保证「清空后写入失败」不会留下空草稿箱。
func (r *Drafts) ReplaceAll(ctx context.Context, items []model.CodeDraft) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := clearTable(tx, &model.CodeDraft{}); err != nil {
			return fmt.Errorf("清空草稿箱失败: %w", err)
		}
		rows := make([]model.CodeDraft, 0, len(items))
		seen := make(map[int64]struct{}, len(items))
		for _, it := range items {
			if it.DraftID == 0 {
				continue
			}
			if _, dup := seen[it.DraftID]; dup {
				continue // draft_id 唯一索引：同一 draft 只保留第一条
			}
			seen[it.DraftID] = struct{}{}
			it.ID = 0
			rows = append(rows, it)
		}
		if len(rows) == 0 {
			return nil
		}
		if err := tx.CreateInBatches(rows, 200).Error; err != nil {
			return fmt.Errorf("写入草稿箱失败: %w", err)
		}
		return nil
	})
}

// List 返回全部草稿（按创建时间倒序，最新的在最前）。
func (r *Drafts) List(ctx context.Context) ([]model.CodeDraft, error) {
	items := make([]model.CodeDraft, 0)
	if err := r.db.WithContext(ctx).Order("create_time DESC, id DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询草稿箱失败: %w", err)
	}
	return items, nil
}

// Get 按 draft_id 读取单条草稿；不存在返回 ErrNotFound。
func (r *Drafts) Get(ctx context.Context, draftID int64) (*model.CodeDraft, error) {
	var d model.CodeDraft
	err := r.db.WithContext(ctx).Where("draft_id = ?", draftID).Take(&d).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询草稿失败: %w", err)
	}
	return &d, nil
}

// Templates 模板库快照仓储（模板不会被覆盖，只有普通模板可下发）。
type Templates struct{ db *gorm.DB }

// NewTemplates 构造 Templates 仓储。
func NewTemplates(db *gorm.DB) *Templates { return &Templates{db: db} }

// ReplaceAll 事务内整体替换模板库快照，并保留本地维护字段。
//
// 保留规则：按 template_id 匹配旧的 is_default 与 note —— 微信 gettemplatelist 不返回这两个字段，
// 整体替换时若直接覆盖会把「默认模板」和运维备注清空。
func (r *Templates) ReplaceAll(ctx context.Context, items []model.CodeTemplate) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		type local struct {
			TemplateID int64
			IsDefault  bool
			Note       string
		}
		var locals []local
		if err := tx.Model(&model.CodeTemplate{}).
			Select("template_id, is_default, note").Scan(&locals).Error; err != nil {
			return fmt.Errorf("读取模板本地字段失败: %w", err)
		}
		preserved := make(map[int64]local, len(locals))
		for _, l := range locals {
			preserved[l.TemplateID] = l
		}

		if err := clearTable(tx, &model.CodeTemplate{}); err != nil {
			return fmt.Errorf("清空模板库失败: %w", err)
		}

		rows := make([]model.CodeTemplate, 0, len(items))
		seen := make(map[int64]struct{}, len(items))
		for _, it := range items {
			if it.TemplateID == 0 {
				continue
			}
			if _, dup := seen[it.TemplateID]; dup {
				continue // template_id 唯一索引：同一模板只保留第一条
			}
			seen[it.TemplateID] = struct{}{}
			it.ID = 0
			if l, ok := preserved[it.TemplateID]; ok {
				it.IsDefault = l.IsDefault
				it.Note = l.Note
			}
			rows = append(rows, it)
		}
		if len(rows) == 0 {
			return nil
		}
		if err := tx.CreateInBatches(rows, 200).Error; err != nil {
			return fmt.Errorf("写入模板库失败: %w", err)
		}
		return nil
	})
}

// List 返回模板列表；templateType 为 nil 表示全部（0=普通模板，1=标准模板）。
func (r *Templates) List(ctx context.Context, templateType *int) ([]model.CodeTemplate, error) {
	items := make([]model.CodeTemplate, 0)
	q := r.db.WithContext(ctx).Model(&model.CodeTemplate{})
	if templateType != nil {
		q = q.Where("template_type = ?", *templateType)
	}
	if err := q.Order("template_id DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询模板库失败: %w", err)
	}
	return items, nil
}

// Get 按 template_id 读取模板；不存在返回 ErrNotFound。
func (r *Templates) Get(ctx context.Context, templateID int64) (*model.CodeTemplate, error) {
	var t model.CodeTemplate
	err := r.db.WithContext(ctx).Where("template_id = ?", templateID).Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询模板失败: %w", err)
	}
	return &t, nil
}

// Delete 删除本地模板快照记录（不影响微信侧模板库）；不存在返回 ErrNotFound。
func (r *Templates) Delete(ctx context.Context, templateID int64) error {
	res := r.db.WithContext(ctx).Where("template_id = ?", templateID).Delete(&model.CodeTemplate{})
	if res.Error != nil {
		return fmt.Errorf("删除模板失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDefault 设置默认模板（事务内先全部置 false，再置该条 true，保证全局只有一个默认）。
// 目标模板不存在返回 ErrNotFound。
func (r *Templates) SetDefault(ctx context.Context, templateID int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var n int64
		if err := tx.Model(&model.CodeTemplate{}).Where("template_id = ?", templateID).Count(&n).Error; err != nil {
			return fmt.Errorf("检查模板是否存在失败: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		if err := tx.Model(&model.CodeTemplate{}).
			Where("is_default = ?", true).
			Update("is_default", false).Error; err != nil {
			return fmt.Errorf("清除原默认模板失败: %w", err)
		}
		if err := tx.Model(&model.CodeTemplate{}).
			Where("template_id = ?", templateID).
			Update("is_default", true).Error; err != nil {
			return fmt.Errorf("设置默认模板失败: %w", err)
		}
		return nil
	})
}

// UpdateNote 更新模板运维备注；模板不存在返回 ErrNotFound。
func (r *Templates) UpdateNote(ctx context.Context, templateID int64, note string) error {
	if err := updateWhere(r.db.WithContext(ctx), &model.CodeTemplate{},
		"template_id = ?", []any{templateID}, map[string]any{"note": note}); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("更新模板备注失败: %w", err)
	}
	return nil
}

// Count 返回本地模板库数量（用于与官方上限 model.TemplateLibraryLimit 比较）。
func (r *Templates) Count(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.CodeTemplate{}).Count(&n).Error; err != nil {
		return 0, fmt.Errorf("统计模板数量失败: %w", err)
	}
	return n, nil
}

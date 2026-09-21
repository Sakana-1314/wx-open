package repo

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"wx-platform/server/internal/model"
)

// AuditProfiles 提审配置仓储。
type AuditProfiles struct{ db *gorm.DB }

// NewAuditProfiles 构造 AuditProfiles 仓储。
func NewAuditProfiles(db *gorm.DB) *AuditProfiles { return &AuditProfiles{db: db} }

// List 返回全部提审配置（默认配置排最前）。
func (r *AuditProfiles) List(ctx context.Context) ([]model.AuditProfile, error) {
	items := make([]model.AuditProfile, 0)
	if err := r.db.WithContext(ctx).Order("is_default DESC, id ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询提审配置列表失败: %w", err)
	}
	return items, nil
}

// Get 按主键读取提审配置；不存在返回 ErrNotFound。
func (r *AuditProfiles) Get(ctx context.Context, id uint) (*model.AuditProfile, error) {
	var p model.AuditProfile
	err := r.db.WithContext(ctx).Take(&p, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询提审配置失败: %w", err)
	}
	return &p, nil
}

// GetDefault 返回默认提审配置；无默认配置返回 ErrNotFound。
func (r *AuditProfiles) GetDefault(ctx context.Context) (*model.AuditProfile, error) {
	var p model.AuditProfile
	err := r.db.WithContext(ctx).Where("is_default = ?", true).Order("id ASC").Take(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询默认提审配置失败: %w", err)
	}
	return &p, nil
}

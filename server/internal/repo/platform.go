package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"wx-platform/server/internal/model"
)

// PlatformState 第三方平台运行时状态（单行表，主键固定 1）。
type PlatformState struct{ db *gorm.DB }

// NewPlatformState 构造 PlatformState 仓储。
func NewPlatformState(db *gorm.DB) *PlatformState { return &PlatformState{db: db} }

// Get 读取平台状态；主键 1 不存在时创建后再返回。
//
// 并发安全：多个实例同时启动时可能同时进入创建分支，此时后写的一方拿到 1062，
// 这里回退为重新读取，保证调用方一定能拿到唯一那行。
func (r *PlatformState) Get(ctx context.Context) (*model.PlatformState, error) {
	var st model.PlatformState
	err := r.db.WithContext(ctx).First(&st, 1).Error
	if err == nil {
		return &st, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("读取平台状态失败: %w", err)
	}

	st = model.PlatformState{ID: 1}
	if cerr := r.db.WithContext(ctx).Create(&st).Error; cerr != nil {
		var again model.PlatformState
		if rerr := r.db.WithContext(ctx).First(&again, 1).Error; rerr == nil {
			return &again, nil
		}
		return nil, fmt.Errorf("初始化平台状态失败: %w", cerr)
	}
	return &st, nil
}

// SaveTicket 保存最新的 component_verify_ticket 与接收时间（票据 12 小时内有效）。
func (r *PlatformState) SaveTicket(ctx context.Context, ticket string, at time.Time) error {
	if _, err := r.Get(ctx); err != nil {
		return err
	}
	fields := map[string]any{
		"verify_ticket":      ticket,
		"ticket_received_at": at,
	}
	if err := updateWhere(r.db.WithContext(ctx), &model.PlatformState{}, "id = ?", []any{1}, fields); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("保存 component_verify_ticket 失败: %w", err)
	}
	return nil
}

// SaveComponentToken 保存 component_access_token 与过期时间。
func (r *PlatformState) SaveComponentToken(ctx context.Context, token string, expiresAt time.Time) error {
	if _, err := r.Get(ctx); err != nil {
		return err
	}
	fields := map[string]any{
		"component_access_token":    token,
		"component_token_expire_at": expiresAt,
	}
	if err := updateWhere(r.db.WithContext(ctx), &model.PlatformState{}, "id = ?", []any{1}, fields); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("保存 component_access_token 失败: %w", err)
	}
	return nil
}

// MarkPushOK 记录最近一次票据推送成功时间（用于判断推送链路是否健康）。
func (r *PlatformState) MarkPushOK(ctx context.Context, at time.Time) error {
	if _, err := r.Get(ctx); err != nil {
		return err
	}
	if err := updateWhere(r.db.WithContext(ctx), &model.PlatformState{}, "id = ?", []any{1},
		map[string]any{"last_push_ok_at": at}); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("记录票据推送时间失败: %w", err)
	}
	return nil
}

// Tokens 令牌缓存仓储（重启后可复用，减少对微信的取令牌调用）。
type Tokens struct{ db *gorm.DB }

// NewTokens 构造 Tokens 仓储。
func NewTokens(db *gorm.DB) *Tokens { return &Tokens{db: db} }

// Get 按 (scope, appid) 读取令牌；无记录返回 ErrNotFound。
// 索引依据：唯一索引 idx_token_scope_appid(scope, appid)，等值命中。
func (r *Tokens) Get(ctx context.Context, scope model.TokenScope, appid string) (*model.TokenCache, error) {
	var tc model.TokenCache
	err := r.db.WithContext(ctx).
		Where("scope = ? AND appid = ?", scope, appid).
		Take(&tc).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("读取令牌缓存失败: %w", err)
	}
	return &tc, nil
}

// Put 写入令牌（按 scope+appid upsert）。
//
// 依赖唯一索引 idx_token_scope_appid 触发 ON DUPLICATE KEY UPDATE，
// 因此并发刷新同一令牌也不会产生重复行。
func (r *Tokens) Put(ctx context.Context, scope model.TokenScope, appid, token string, expiresAt time.Time) error {
	row := model.TokenCache{Scope: scope, Appid: appid, Token: token, ExpiresAt: expiresAt}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "scope"}, {Name: "appid"}},
		DoUpdates: clause.Assignments(map[string]any{
			"token":      token,
			"expires_at": expiresAt,
			"updated_at": time.Now(),
		}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("写入令牌缓存失败: %w", err)
	}
	return nil
}

// Delete 删除令牌缓存（令牌被判失效时调用）。幂等：记录本就不存在时返回 nil。
func (r *Tokens) Delete(ctx context.Context, scope model.TokenScope, appid string) error {
	err := r.db.WithContext(ctx).
		Where("scope = ? AND appid = ?", scope, appid).
		Delete(&model.TokenCache{}).Error
	if err != nil {
		return fmt.Errorf("删除令牌缓存失败: %w", err)
	}
	return nil
}

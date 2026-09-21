package repo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"wx-platform/server/internal/model"
)

// AuthorizerFilter 授权小程序列表筛选条件（零值表示不过滤）。
type AuthorizerFilter struct {
	Keyword          string
	Status           string // ""=全部
	GroupName        string
	Tag              string
	HasDevPermission *bool
	IncludeDisabled  bool
	Page             int
	PageSize         int
}

// Authorizers 授权小程序仓储。
//
// 注意：model.Authorizer.DeletedAt 是普通 *time.Time（不是 gorm.DeletedAt），
// GORM 不会自动过滤，因此本仓储所有查询都显式带 deleted_at IS NULL。
type Authorizers struct{ db *gorm.DB }

// NewAuthorizers 构造 Authorizers 仓储。
func NewAuthorizers(db *gorm.DB) *Authorizers { return &Authorizers{db: db} }

// applyFilter 把筛选条件套到查询上（List / Count 共用，保证 total 与列表口径一致）。
func applyAuthorizerFilter(db *gorm.DB, f AuthorizerFilter) *gorm.DB {
	if !f.IncludeDisabled {
		db = db.Where("enabled = ?", true)
	}
	if f.Status != "" {
		db = db.Where("authorization_status = ?", f.Status)
	}
	if f.GroupName != "" {
		db = db.Where("group_name = ?", f.GroupName)
	}
	if kw := strings.TrimSpace(f.Keyword); kw != "" {
		like := "%" + kw + "%"
		db = db.Where("(appid LIKE ? OR nick_name LIKE ? OR alias LIKE ? OR user_name LIKE ? OR principal_name LIKE ?)",
			like, like, like, like, like)
	}
	if f.Tag != "" {
		// tags 是 JSON 数组列：JSON_CONTAINS(tags, '"标签"') 命中即包含该标签。
		db = db.Where("JSON_CONTAINS(tags, JSON_QUOTE(?))", f.Tag)
	}
	if f.HasDevPermission != nil {
		// func_info 是权限集 id 的 JSON 数组（开发权限集 = 18）。
		dev := fmt.Sprintf("%d", model.PermissionSetDev)
		if *f.HasDevPermission {
			db = db.Where("JSON_CONTAINS(func_info, ?)", dev)
		} else {
			db = db.Where("func_info IS NULL OR JSON_CONTAINS(func_info, ?) = 0", dev)
		}
	}
	return db
}

// List 分页查询授权小程序，同时返回符合筛选条件的总数。
// 索引依据：authorization_status / group_name 各有单列索引；tag 与权限集过滤走 JSON 函数（无法走索引，仅在已收敛结果集上执行）。
func (r *Authorizers) List(ctx context.Context, f AuthorizerFilter) ([]model.Authorizer, int64, error) {
	items := make([]model.Authorizer, 0)
	base := func() *gorm.DB {
		return applyAuthorizerFilter(
			r.db.WithContext(ctx).Model(&model.Authorizer{}).Where("deleted_at IS NULL"), f)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计授权小程序失败: %w", err)
	}
	limit, offset := paginate(f.Page, f.PageSize)
	if err := base().Order("id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("查询授权小程序列表失败: %w", err)
	}
	return items, total, nil
}

// Get 按 appid 读取单个授权小程序（不含已软删）。
func (r *Authorizers) Get(ctx context.Context, appid string) (*model.Authorizer, error) {
	var a model.Authorizer
	err := r.db.WithContext(ctx).
		Where("appid = ? AND deleted_at IS NULL", appid).Take(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询授权小程序失败: %w", err)
	}
	return &a, nil
}

// GetByID 按主键读取授权小程序（含已软删，便于详情页展示删除态）。
func (r *Authorizers) GetByID(ctx context.Context, id uint) (*model.Authorizer, error) {
	var a model.Authorizer
	err := r.db.WithContext(ctx).Take(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("按 ID 查询授权小程序失败: %w", err)
	}
	return &a, nil
}

// Upsert 按 appid 写入授权小程序：存在则更新，不存在则新增。
//
// 本地维护字段不会被外部同步覆盖：
//   - refresh_token 密文与刷新时间（只由 UpdateRefreshToken 写入）；
//   - 软删标记 deleted_at（是否恢复由业务显式决定，避免同步任务复活已删除的小程序，
//     需要恢复时调用 UpdateFields(appid, map[string]any{"deleted_at": nil})）。
func (r *Authorizers) Upsert(ctx context.Context, a *model.Authorizer) error {
	if a == nil || a.Appid == "" {
		return errors.New("授权小程序 appid 不能为空")
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.Authorizer
		err := tx.Select("id", "created_at", "deleted_at", "refresh_token_cipher", "refresh_token_updated_at").
			Where("appid = ?", a.Appid).Take(&existing).Error
		switch {
		case err == nil:
			a.ID = existing.ID
			a.CreatedAt = existing.CreatedAt
			a.DeletedAt = existing.DeletedAt
			if a.RefreshTokenCipher == "" {
				a.RefreshTokenCipher = existing.RefreshTokenCipher
			}
			if a.RefreshTokenUpdatedAt == nil {
				a.RefreshTokenUpdatedAt = existing.RefreshTokenUpdatedAt
			}
			if uerr := tx.Save(a).Error; uerr != nil {
				return fmt.Errorf("更新授权小程序失败: %w", uerr)
			}
			return nil
		case errors.Is(err, gorm.ErrRecordNotFound):
			// model.Authorizer.Enabled 带 `default:true`：GORM 插入时会把零值替换成数据库默认值
			// （连结构体字段都会被改写成 true），因此先记住调用方想要的启用状态，插入后按需显式覆盖。
			wantEnabled := a.Enabled
			if cerr := tx.Create(a).Error; cerr != nil {
				return fmt.Errorf("新增授权小程序失败: %w", cerr)
			}
			if !wantEnabled {
				if uerr := tx.Model(&model.Authorizer{}).
					Where("id = ?", a.ID).UpdateColumn("enabled", false).Error; uerr != nil {
					return fmt.Errorf("写入停用状态失败: %w", uerr)
				}
				a.Enabled = false // 保持入参结构与库内一致
			}
			return nil
		default:
			return fmt.Errorf("查询授权小程序失败: %w", err)
		}
	})
	return err
}

// UpdateFields 局部更新授权小程序（只更新传入字段；未变化时不报错，记录不存在返回 ErrNotFound）。
//
// 这里刻意不过滤 deleted_at：调用方可用 map[string]any{"deleted_at": nil} 显式恢复软删记录。
func (r *Authorizers) UpdateFields(ctx context.Context, appid string, fields map[string]any) error {
	if appid == "" {
		return errors.New("授权小程序 appid 不能为空")
	}
	if err := updateWhere(r.db.WithContext(ctx), &model.Authorizer{}, "appid = ?", []any{appid}, fields); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("更新授权小程序失败: %w", err)
	}
	return nil
}

// UpdateRefreshToken 单独保存 refresh_token 密文与刷新时间（密文由 secretbox 加密，仓库层不接触明文）。
func (r *Authorizers) UpdateRefreshToken(ctx context.Context, appid string, cipher []byte, at time.Time) error {
	fields := map[string]any{
		"refresh_token_cipher":     cipher,
		"refresh_token_updated_at": at,
	}
	if err := updateWhere(r.db.WithContext(ctx), &model.Authorizer{}, "appid = ?", []any{appid}, fields); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("保存 refresh_token 失败: %w", err)
	}
	return nil
}

// MarkUnauthorized 把小程序标记为已取消授权（保留历史数据，不再代调用）。
func (r *Authorizers) MarkUnauthorized(ctx context.Context, appid string, at time.Time) error {
	fields := map[string]any{
		"authorization_status": model.AuthStatusUnauthorized,
		"updated_at":           at,
	}
	if err := updateWhere(r.db.WithContext(ctx), &model.Authorizer{}, "appid = ?", []any{appid}, fields); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("标记取消授权失败: %w", err)
	}
	return nil
}

// SoftDelete 软删除授权小程序（仅置 deleted_at，物理数据保留供审计）。
func (r *Authorizers) SoftDelete(ctx context.Context, appid string) error {
	fields := map[string]any{"deleted_at": time.Now()}
	if err := updateWhere(r.db.WithContext(ctx), &model.Authorizer{}, "appid = ?", []any{appid}, fields); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("删除授权小程序失败: %w", err)
	}
	return nil
}

// All 返回全部未软删的授权小程序（供同步、看板使用）。
func (r *Authorizers) All(ctx context.Context) ([]model.Authorizer, error) {
	items := make([]model.Authorizer, 0)
	if err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").Order("id ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询全部授权小程序失败: %w", err)
	}
	return items, nil
}

// ByAppids 按 appid 集合批量查询（未软删），返回顺序按 id。
func (r *Authorizers) ByAppids(ctx context.Context, appids []string) ([]model.Authorizer, error) {
	items := make([]model.Authorizer, 0)
	if len(appids) == 0 {
		return items, nil
	}
	if err := r.db.WithContext(ctx).
		Where("appid IN ?", appids).Where("deleted_at IS NULL").
		Order("id ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("按 appid 批量查询授权小程序失败: %w", err)
	}
	return items, nil
}

// ActiveAppids 返回可代调用的小程序 appid（已授权 + enabled + 未软删）。
func (r *Authorizers) ActiveAppids(ctx context.Context) ([]string, error) {
	appids := make([]string, 0)
	if err := r.db.WithContext(ctx).Model(&model.Authorizer{}).
		Where("authorization_status = ? AND enabled = ? AND deleted_at IS NULL",
			model.AuthStatusAuthorized, true).
		Order("id ASC").Pluck("appid", &appids).Error; err != nil {
		return nil, fmt.Errorf("查询可用小程序 appid 失败: %w", err)
	}
	return appids, nil
}

// CountByStatus 按授权状态统计（未软删），用于首页看板。
func (r *Authorizers) CountByStatus(ctx context.Context) (map[model.AuthorizationStatus]int64, error) {
	var rows []struct {
		Status model.AuthorizationStatus
		N      int64
	}
	if err := r.db.WithContext(ctx).Model(&model.Authorizer{}).
		Select("authorization_status AS status, COUNT(*) AS n").
		Where("deleted_at IS NULL").
		Group("authorization_status").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("按状态统计授权小程序失败: %w", err)
	}
	out := make(map[model.AuthorizationStatus]int64, len(rows))
	for _, row := range rows {
		out[row.Status] = row.N
	}
	return out, nil
}

// UpdatePreflightSnapshot 保存提审前置检查快照（隐私接口是否已配置 + 域名快照）。
// domains 传 nil 表示「本次未取到域名信息」，会覆盖旧快照为 NULL，由调用方决定是否调用。
func (r *Authorizers) UpdatePreflightSnapshot(ctx context.Context, appid string, privacyConfigured bool, domains model.JSONMap, at time.Time) error {
	fields := map[string]any{
		"privacy_configured": privacyConfigured,
		"domain_snapshot":    domains,
		"last_preflight_at":  at,
	}
	if err := updateWhere(r.db.WithContext(ctx), &model.Authorizer{}, "appid = ?", []any{appid}, fields); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("保存提审前置快照失败: %w", err)
	}
	return nil
}

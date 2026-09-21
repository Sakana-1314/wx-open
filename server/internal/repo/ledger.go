package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wx-platform/server/internal/model"
)

// AuditStatusAuditing 审核中（对应微信 audit_status=2，语义见 model.AuditStatusText）。
const AuditStatusAuditing = 2

// Audits 审核单台账仓储。
type Audits struct{ db *gorm.DB }

// NewAudits 构造 Audits 仓储。
func NewAudits(db *gorm.DB) *Audits { return &Audits{db: db} }

// Upsert 按 (appid, audit_id) 写入审核单台账（API 查询、审核事件、轮询结果共用，保证幂等）。
//
// 实现说明：先按 (appid, audit_id) 查，命中则全字段更新（保留本地首次入库时间），
// 未命中则插入；若插入撞上 audit_id 唯一索引（同一审核单换了 appid 上报），
// 退化为按 audit_id 更新，避免把事件记录整条丢掉。
func (r *Audits) Upsert(ctx context.Context, rec *model.AuditRecord) error {
	if rec == nil || rec.Appid == "" {
		return errors.New("审核台账 appid 不能为空")
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing model.AuditRecord
		qerr := tx.Select("id", "created_at").Where("appid = ? AND audit_id = ?", rec.Appid, rec.AuditID).
			Take(&existing).Error
		switch {
		case qerr == nil:
			rec.ID = existing.ID
			rec.CreatedAt = existing.CreatedAt
			if serr := tx.Save(rec).Error; serr != nil {
				return fmt.Errorf("更新审核台账失败: %w", serr)
			}
			return nil
		case !errors.Is(qerr, gorm.ErrRecordNotFound):
			return fmt.Errorf("查询审核台账失败: %w", qerr)
		}

		if cerr := tx.Create(rec).Error; cerr != nil {
			if !isDuplicateKey(cerr) {
				return fmt.Errorf("新增审核台账失败: %w", cerr)
			}
			var byAudit model.AuditRecord
			if ferr := tx.Where("audit_id = ?", rec.AuditID).Take(&byAudit).Error; ferr != nil {
				return fmt.Errorf("按 audit_id 查询审核台账失败: %w", ferr)
			}
			rec.ID = byAudit.ID
			rec.CreatedAt = byAudit.CreatedAt
			if uerr := tx.Save(rec).Error; uerr != nil {
				return fmt.Errorf("更新审核台账失败: %w", uerr)
			}
		}
		return nil
	})
}

// List 分页查询审核台账；appid 为空表示全部，status 为 nil 表示全部状态，同时返回总数。
// 索引依据：audit_records.appid 与 status 各有单列索引；按创建时间倒序。
func (r *Audits) List(ctx context.Context, appid string, status *int, page, pageSize int) ([]model.AuditRecord, int64, error) {
	items := make([]model.AuditRecord, 0)
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.AuditRecord{})
		if appid != "" {
			q = q.Where("appid = ?", appid)
		}
		if status != nil {
			q = q.Where("status = ?", *status)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计审核台账失败: %w", err)
	}
	limit, offset := paginate(page, pageSize)
	if err := base().Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("查询审核台账失败: %w", err)
	}
	return items, total, nil
}

// HistoryByApp 返回某小程序的审核历史（按创建时间倒序，最多 limit 条）。
func (r *Audits) HistoryByApp(ctx context.Context, appid string, limit int) ([]model.AuditRecord, error) {
	items := make([]model.AuditRecord, 0)
	if limit <= 0 {
		limit = defaultPageSize
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	if err := r.db.WithContext(ctx).
		Where("appid = ?", appid).
		Order("created_at DESC, id DESC").Limit(limit).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询审核历史失败: %w", err)
	}
	return items, nil
}

// LatestByApp 返回某小程序最近一条审核记录；无记录返回 ErrNotFound。
func (r *Audits) LatestByApp(ctx context.Context, appid string) (*model.AuditRecord, error) {
	var rec model.AuditRecord
	err := r.db.WithContext(ctx).
		Where("appid = ?", appid).
		Order("created_at DESC, id DESC").Take(&rec).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询最新审核记录失败: %w", err)
	}
	return &rec, nil
}

// PendingAppids 返回仍处于「审核中」的小程序 appid 去重列表（审核结果轮询用）。
// 索引依据：audit_records.status 单列索引 + appid 单列索引，DISTINCT 后由 SQL 层去重。
func (r *Audits) PendingAppids(ctx context.Context) ([]string, error) {
	appids := make([]string, 0)
	if err := r.db.WithContext(ctx).Model(&model.AuditRecord{}).
		Distinct().Where("status = ?", AuditStatusAuditing).
		Order("appid ASC").Pluck("appid", &appids).Error; err != nil {
		return nil, fmt.Errorf("查询审核中的小程序失败: %w", err)
	}
	return appids, nil
}

// Releases 发布台账仓储。
type Releases struct{ db *gorm.DB }

// NewReleases 构造 Releases 仓储。
func NewReleases(db *gorm.DB) *Releases { return &Releases{db: db} }

// Create 新增发布记录（发布/灰度/回退共用，仅追加不修改）。
func (r *Releases) Create(ctx context.Context, rec *model.ReleaseRecord) error {
	if rec == nil {
		return errors.New("发布记录为空")
	}
	if err := r.db.WithContext(ctx).Create(rec).Error; err != nil {
		return fmt.Errorf("新增发布记录失败: %w", err)
	}
	return nil
}

// List 分页查询发布记录；appid 为空表示全部，同时返回总数。
// 索引依据：release_records.appid 单列索引，按 id 倒序即发布先后倒序。
func (r *Releases) List(ctx context.Context, appid string, page, pageSize int) ([]model.ReleaseRecord, int64, error) {
	items := make([]model.ReleaseRecord, 0)
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.ReleaseRecord{})
		if appid != "" {
			q = q.Where("appid = ?", appid)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计发布记录失败: %w", err)
	}
	limit, offset := paginate(page, pageSize)
	if err := base().Order("id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("查询发布记录失败: %w", err)
	}
	return items, total, nil
}

// UndoQuota 撤回审核的本地用量台账（每账号每天 ≤5 次、每月 ≤10 次，超限微信返回 87013）。
type UndoQuota struct{ db *gorm.DB }

// NewUndoQuota 构造 UndoQuota 仓储。
func NewUndoQuota(db *gorm.DB) *UndoQuota { return &UndoQuota{db: db} }

// CountToday 统计某小程序当天已使用的撤回次数（本地时区 0 点起算）。
func (r *UndoQuota) CountToday(ctx context.Context, appid string) (int64, error) {
	start := startOfDay(time.Now())
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.UndoQuotaUsage{}).
		Where("appid = ? AND used_at >= ? AND used_at < ?", appid, start, start.AddDate(0, 0, 1)).
		Count(&n).Error; err != nil {
		return 0, fmt.Errorf("统计当日撤回用量失败: %w", err)
	}
	return n, nil
}

// CountMonth 统计某小程序本月已使用的撤回次数。
// 索引依据：month_key（YYYY-MM）单列索引，比按 used_at 做范围比较更稳定，不受跨时区影响。
func (r *UndoQuota) CountMonth(ctx context.Context, appid string) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).Model(&model.UndoQuotaUsage{}).
		Where("appid = ? AND month_key = ?", appid, monthKey(time.Now())).
		Count(&n).Error; err != nil {
		return 0, fmt.Errorf("统计当月撤回用量失败: %w", err)
	}
	return n, nil
}

// Record 记录一次撤回用量（MonthKey 由 at 推导，保证与 CountMonth 口径一致）。
func (r *UndoQuota) Record(ctx context.Context, appid string, at time.Time) error {
	row := model.UndoQuotaUsage{Appid: appid, UsedAt: at, MonthKey: monthKey(at)}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("记录撤回用量失败: %w", err)
	}
	return nil
}

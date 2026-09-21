package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"wx-platform/server/internal/model"
)

// ApiCallLogFilter 接口调用日志筛选条件（零值表示不过滤）。
type ApiCallLogFilter struct {
	Appid    string
	JobID    string
	Endpoint string
	Errcode  *int
	Page     int
	PageSize int
}

// ApiCallLogs 微信接口调用日志仓储（排障核心）。
type ApiCallLogs struct{ db *gorm.DB }

// NewApiCallLogs 构造 ApiCallLogs 仓储。
func NewApiCallLogs(db *gorm.DB) *ApiCallLogs { return &ApiCallLogs{db: db} }

// Create 写入一条接口调用日志（CreatedAt 由 GORM 自动填充）。
func (r *ApiCallLogs) Create(ctx context.Context, log *model.ApiCallLog) error {
	if log == nil {
		return errors.New("接口调用日志为空")
	}
	if log.CreatedAt.IsZero() {
		log.CreatedAt = time.Now()
	}
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("写入接口调用日志失败: %w", err)
	}
	return nil
}

// List 分页查询接口调用日志，同时返回总数。
// 索引依据：appid / job_id / endpoint / errcode / created_at 均有单列索引，
// 因此「某小程序某接口近 N 条」这类排障查询都能走索引；列表按时间倒序。
func (r *ApiCallLogs) List(ctx context.Context, f ApiCallLogFilter) ([]model.ApiCallLog, int64, error) {
	items := make([]model.ApiCallLog, 0)
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.ApiCallLog{})
		if f.Appid != "" {
			q = q.Where("appid = ?", f.Appid)
		}
		if f.JobID != "" {
			q = q.Where("job_id = ?", f.JobID)
		}
		if f.Endpoint != "" {
			q = q.Where("endpoint = ?", f.Endpoint)
		}
		if f.Errcode != nil {
			q = q.Where("errcode = ?", *f.Errcode)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计接口调用日志失败: %w", err)
	}
	limit, offset := paginate(f.Page, f.PageSize)
	if err := base().Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("查询接口调用日志失败: %w", err)
	}
	return items, total, nil
}

// PruneBefore 物理清理指定时间点之前的日志，返回删除条数（定时清理任务调用）。
// 索引依据：api_call_logs.created_at 单列索引。
func (r *ApiCallLogs) PruneBefore(ctx context.Context, before time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&model.ApiCallLog{})
	if res.Error != nil {
		return 0, fmt.Errorf("清理接口调用日志失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// CallbackEvents 回调事件仓储（幂等与审计依据）。
type CallbackEvents struct{ db *gorm.DB }

// NewCallbackEvents 构造 CallbackEvents 仓储。
func NewCallbackEvents(db *gorm.DB) *CallbackEvents { return &CallbackEvents{db: db} }

// CreateIfAbsent 以 dedupe_key 做幂等写入：首次写入返回 (event, true, nil)，
// 已存在则返回库里那条 (existing, false, nil)，微信重复推送不会重复入库。
//
// 实现：先直接插入，撞唯一索引（1062）说明已存在，再按 dedupe_key 读回旧记录 ——
// 比「先查再插」更能抵御并发重复推送（两个 goroutine 同时到达也只有一个能插入成功）。
// dedupe_key 为空时无法保证幂等，直接报错，由调用方补齐。
func (r *CallbackEvents) CreateIfAbsent(ctx context.Context, ev *model.CallbackEvent) (*model.CallbackEvent, bool, error) {
	if ev == nil {
		return nil, false, errors.New("回调事件为空")
	}
	if ev.DedupeKey == "" {
		return nil, false, errors.New("回调事件缺少 dedupe_key，无法保证幂等")
	}
	if ev.ReceivedAt.IsZero() {
		ev.ReceivedAt = time.Now()
	}

	if err := r.db.WithContext(ctx).Create(ev).Error; err == nil {
		return ev, true, nil
	} else if !isDuplicateKey(err) {
		return nil, false, fmt.Errorf("写入回调事件失败: %w", err)
	}

	var existing model.CallbackEvent
	err := r.db.WithContext(ctx).Where("dedupe_key = ?", ev.DedupeKey).Take(&existing).Error
	if err != nil {
		// 已插入但读不回来（例如并发事务尚未提交）：返回错误让微信按重试策略重推。
		return nil, false, fmt.Errorf("读取已存在的回调事件失败: %w", err)
	}
	return &existing, false, nil
}

// List 分页查询回调事件；kind / appid / infoType 为空表示不过滤，同时返回总数。
// 索引依据：callback_events.kind / appid / info_type / received_at 均有单列索引。
func (r *CallbackEvents) List(ctx context.Context, kind model.CallbackKind, appid, infoType string, page, pageSize int) ([]model.CallbackEvent, int64, error) {
	items := make([]model.CallbackEvent, 0)
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.CallbackEvent{})
		if kind != "" {
			q = q.Where("kind = ?", kind)
		}
		if appid != "" {
			q = q.Where("appid = ?", appid)
		}
		if infoType != "" {
			q = q.Where("info_type = ?", infoType)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计回调事件失败: %w", err)
	}
	limit, offset := paginate(page, pageSize)
	if err := base().Order("received_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("查询回调事件失败: %w", err)
	}
	return items, total, nil
}

// MarkProcessed 标记回调事件处理结果；事件不存在返回 ErrNotFound。
func (r *CallbackEvents) MarkProcessed(ctx context.Context, id uint, ok bool, note string) error {
	fields := map[string]any{"processed": ok, "process_note": note}
	if err := updateWhere(r.db.WithContext(ctx), &model.CallbackEvent{}, "id = ?", []any{id}, fields); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("标记回调事件处理结果失败: %w", err)
	}
	return nil
}

// PruneBefore 物理清理指定时间点之前的回调事件，返回删除条数。
// 索引依据：callback_events.received_at 单列索引。
func (r *CallbackEvents) PruneBefore(ctx context.Context, before time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("received_at < ?", before).Delete(&model.CallbackEvent{})
	if res.Error != nil {
		return 0, fmt.Errorf("清理回调事件失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// OperationLogs 平台操作审计仓储。
type OperationLogs struct{ db *gorm.DB }

// NewOperationLogs 构造 OperationLogs 仓储。
func NewOperationLogs(db *gorm.DB) *OperationLogs { return &OperationLogs{db: db} }

// Create 写入一条操作审计日志。
func (r *OperationLogs) Create(ctx context.Context, log *model.OperationLog) error {
	if log == nil {
		return errors.New("操作日志为空")
	}
	if err := r.db.WithContext(ctx).Create(log).Error; err != nil {
		return fmt.Errorf("写入操作日志失败: %w", err)
	}
	return nil
}

// List 分页查询操作日志；action 为空表示全部，按时间倒序，同时返回总数。
// 索引依据：operation_logs.action / created_at 单列索引。
func (r *OperationLogs) List(ctx context.Context, action string, page, pageSize int) ([]model.OperationLog, int64, error) {
	items := make([]model.OperationLog, 0)
	base := func() *gorm.DB {
		q := r.db.WithContext(ctx).Model(&model.OperationLog{})
		if action != "" {
			q = q.Where("action = ?", action)
		}
		return q
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("统计操作日志失败: %w", err)
	}
	limit, offset := paginate(page, pageSize)
	if err := base().Order("created_at DESC, id DESC").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, 0, fmt.Errorf("查询操作日志失败: %w", err)
	}
	return items, total, nil
}

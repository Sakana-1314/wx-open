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

// settingsKeyColumn 「key」是 MySQL / MariaDB 保留字，
// 必须交给 GORM 用 clause.Column 生成带反引号的列名，否则 SQL 报语法错误。
var settingsKeyColumn = clause.Column{Name: "key"}

// Settings 运行参数仓储（可在线调整的非密钥参数）。
type Settings struct{ db *gorm.DB }

// NewSettings 构造 Settings 仓储。
func NewSettings(db *gorm.DB) *Settings { return &Settings{db: db} }

// All 返回全部运行参数（key -> value），键按字典序，便于设置页稳定展示。
func (r *Settings) All(ctx context.Context) (map[string]string, error) {
	rows := make([]model.RuntimeSetting, 0)
	if err := r.db.WithContext(ctx).
		Order(clause.OrderByColumn{Column: settingsKeyColumn}).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("查询运行参数失败: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		out[row.Key] = row.Value
	}
	return out, nil
}

// Put 写入运行参数（按主键 key upsert）。
func (r *Settings) Put(ctx context.Context, key, value string) error {
	if key == "" {
		return errors.New("运行参数键不能为空")
	}
	row := model.RuntimeSetting{Key: key, Value: value}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{settingsKeyColumn},
		DoUpdates: clause.Assignments(map[string]any{
			"value":      value,
			"updated_at": time.Now(),
		}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("写入运行参数 %s 失败: %w", key, err)
	}
	return nil
}

// Get 读取单个运行参数；键不存在返回 ErrNotFound。
func (r *Settings) Get(ctx context.Context, key string) (string, error) {
	var row model.RuntimeSetting
	err := r.db.WithContext(ctx).
		Where(clause.Eq{Column: settingsKeyColumn, Value: key}).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("读取运行参数 %s 失败: %w", key, err)
	}
	return row.Value, nil
}

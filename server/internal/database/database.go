// Package database 负责 MySQL 连接、表结构迁移与运行参数初始化。
package database

import (
	"fmt"
	"log"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"wx-platform/server/internal/config"
	"wx-platform/server/internal/model"
)

// Connect 建立 GORM MySQL 连接并执行 AutoMigrate。
func Connect(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("获取底层连接失败: %w", err)
	}
	// 作业引擎并发写 + 回调写入：池不宜过小，避免排队超时。
	sqlDB.SetMaxOpenConns(32)
	sqlDB.SetMaxIdleConns(8)
	sqlDB.SetConnMaxLifetime(time.Hour)

	if err := db.AutoMigrate(models()...); err != nil {
		return nil, fmt.Errorf("自动迁移失败: %w", err)
	}
	return db, nil
}

// models 返回需要迁移的全部模型（按依赖顺序）。
func models() []any {
	return []any{
		&model.PlatformState{},
		&model.Authorizer{},
		&model.TokenCache{},
		&model.CodeDraft{},
		&model.CodeTemplate{},
		&model.AuditProfile{},
		&model.BatchJob{},
		&model.BatchJobItem{},
		&model.AuditRecord{},
		&model.ReleaseRecord{},
		&model.UndoQuotaUsage{},
		&model.ApiCallLog{},
		&model.CallbackEvent{},
		&model.OperationLog{},
		&model.RuntimeSetting{},
	}
}

// EnsurePlatformState 保证 platform_state 存在唯一一行（主键 1）。
func EnsurePlatformState(db *gorm.DB, componentAppID string) error {
	var state model.PlatformState
	err := db.First(&state, 1).Error
	if err == nil {
		if state.ComponentAppID != componentAppID {
			state.ComponentAppID = componentAppID
			if err := db.Save(&state).Error; err != nil {
				return fmt.Errorf("更新平台状态失败: %w", err)
			}
		}
		return nil
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("读取平台状态失败: %w", err)
	}
	state = model.PlatformState{ID: 1, ComponentAppID: componentAppID}
	if err := db.Create(&state).Error; err != nil {
		return fmt.Errorf("初始化平台状态失败: %w", err)
	}
	return nil
}

// SeedRuntimeSettings 写入默认运行参数（仅在键缺失时）。
func SeedRuntimeSettings(db *gorm.DB, cfg *config.Config) error {
	defaults := map[string]string{
		model.SettingJobConcurrency:        fmt.Sprintf("%d", cfg.JobConcurrency),
		model.SettingJobMaxAttempts:        fmt.Sprintf("%d", cfg.JobMaxAttempts),
		model.SettingWxMaxQps:              fmt.Sprintf("%d", cfg.WxMaxQps),
		model.SettingLogRetentionDays:      fmt.Sprintf("%d", cfg.LogRetentionDays),
		model.SettingDefaultTemplateID:     "",
		model.SettingDefaultAuditProfileID: "",
		model.SettingWxRequestTimeout:      fmt.Sprintf("%d", int(cfg.WxRequestTimeout.Seconds())),
		model.SettingPrivacyCheckMaxWait:   fmt.Sprintf("%d", int(cfg.PrivacyCheckMaxWait.Seconds())),
		model.SettingAuditResultMaxWait:    fmt.Sprintf("%d", int(cfg.AuditResultMaxWait.Seconds())),
	}
	for key, value := range defaults {
		var existing model.RuntimeSetting
		err := db.First(&existing, "`key` = ?", key).Error
		if err == nil {
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return fmt.Errorf("读取运行参数 %s 失败: %w", key, err)
		}
		if err := db.Create(&model.RuntimeSetting{Key: key, Value: value}).Error; err != nil {
			return fmt.Errorf("初始化运行参数 %s 失败: %w", key, err)
		}
	}
	return nil
}

// SeedDefaultAuditProfile 创建默认提审配置（类目字段留空，需用户按 getAllCategoryName 填写）。
func SeedDefaultAuditProfile(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.AuditProfile{}).Count(&count).Error; err != nil {
		return fmt.Errorf("统计提审配置失败: %w", err)
	}
	if count > 0 {
		return nil
	}
	profile := model.AuditProfile{
		Name:      "默认提审配置",
		IsDefault: true,
		Note:      "类目字段需按 /wxa/get_category（getAllCategoryName）返回填写：first_class/second_class 为中文名，first_id/second_id 为数字 id",
	}
	if err := db.Create(&profile).Error; err != nil {
		return fmt.Errorf("创建默认提审配置失败: %w", err)
	}
	log.Printf("已创建默认提审配置（id=%d），请在小程序管理页补齐类目信息", profile.ID)
	return nil
}

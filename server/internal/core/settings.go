package core

import (
	"strconv"
	"strings"

	"gorm.io/gorm"

	"wx-platform/server/internal/config"
	"wx-platform/server/internal/model"
)

// Settings 运行参数（非密钥），可在线调整。
type Settings struct {
	JobConcurrency             int    `json:"jobConcurrency"`
	JobMaxAttempts             int    `json:"jobMaxAttempts"`
	WxMaxQps                   int    `json:"wxMaxQps"`
	LogRetentionDays           int    `json:"logRetentionDays"`
	DefaultTemplateID          *int64 `json:"defaultTemplateId"`
	DefaultAuditProfileID      *int64 `json:"defaultAuditProfileId"`
	WxRequestTimeoutSeconds    int    `json:"wxRequestTimeoutSeconds"`
	PrivacyCheckMaxWaitSeconds int    `json:"privacyCheckMaxWaitSeconds"`
	AuditResultMaxWaitSeconds  int    `json:"auditResultMaxWaitSeconds"`
}

// SettingService 读写 runtime_settings。
type SettingService struct {
	db  *gorm.DB
	cfg *config.Config
}

// settingsDefaults 以启动配置作为兜底值。
func (s *SettingService) settingsDefaults() Settings {
	return Settings{
		JobConcurrency:             s.cfg.JobConcurrency,
		JobMaxAttempts:             s.cfg.JobMaxAttempts,
		WxMaxQps:                   s.cfg.WxMaxQps,
		LogRetentionDays:           s.cfg.LogRetentionDays,
		WxRequestTimeoutSeconds:    int(s.cfg.WxRequestTimeout.Seconds()),
		PrivacyCheckMaxWaitSeconds: int(s.cfg.PrivacyCheckMaxWait.Seconds()),
		AuditResultMaxWaitSeconds:  int(s.cfg.AuditResultMaxWait.Seconds()),
	}
}

// Get 读取当前参数（数据库值覆盖启动配置）。
func (s *SettingService) Get() (Settings, error) {
	out := s.settingsDefaults()
	rows := []model.RuntimeSetting{}
	if err := s.db.Find(&rows).Error; err != nil {
		return out, Internal(err)
	}
	for _, row := range rows {
		value := strings.TrimSpace(row.Value)
		switch row.Key {
		case model.SettingJobConcurrency:
			out.JobConcurrency = atoiOr(value, out.JobConcurrency)
		case model.SettingJobMaxAttempts:
			out.JobMaxAttempts = atoiOr(value, out.JobMaxAttempts)
		case model.SettingWxMaxQps:
			out.WxMaxQps = atoiOr(value, out.WxMaxQps)
		case model.SettingLogRetentionDays:
			out.LogRetentionDays = atoiOr(value, out.LogRetentionDays)
		case model.SettingWxRequestTimeout:
			out.WxRequestTimeoutSeconds = atoiOr(value, out.WxRequestTimeoutSeconds)
		case model.SettingPrivacyCheckMaxWait:
			out.PrivacyCheckMaxWaitSeconds = atoiOr(value, out.PrivacyCheckMaxWaitSeconds)
		case model.SettingAuditResultMaxWait:
			out.AuditResultMaxWaitSeconds = atoiOr(value, out.AuditResultMaxWaitSeconds)
		case model.SettingDefaultTemplateID:
			out.DefaultTemplateID = parseInt64Ptr(value)
		case model.SettingDefaultAuditProfileID:
			out.DefaultAuditProfileID = parseInt64Ptr(value)
		}
	}
	return out, nil
}

// GetInt 供其他服务读取单个整数参数。
func (s *SettingService) GetInt(key string, def int) int {
	var row model.RuntimeSetting
	if err := s.db.First(&row, "`key` = ?", key).Error; err != nil {
		return def
	}
	return atoiOr(strings.TrimSpace(row.Value), def)
}

// Update 更新参数，只写显式提供的字段（nil 表示不变）。
func (s *SettingService) Update(in SettingsPatch) (Settings, error) {
	updates := map[string]string{}
	if in.JobConcurrency != nil {
		if err := checkRange("并发数", *in.JobConcurrency, 1, 8); err != nil {
			return Settings{}, err
		}
		updates[model.SettingJobConcurrency] = itoa(*in.JobConcurrency)
	}
	if in.JobMaxAttempts != nil {
		if err := checkRange("最大重试次数", *in.JobMaxAttempts, 1, 8); err != nil {
			return Settings{}, err
		}
		updates[model.SettingJobMaxAttempts] = itoa(*in.JobMaxAttempts)
	}
	if in.WxMaxQps != nil {
		if err := checkRange("微信接口 QPS", *in.WxMaxQps, 1, 50); err != nil {
			return Settings{}, err
		}
		updates[model.SettingWxMaxQps] = itoa(*in.WxMaxQps)
	}
	if in.LogRetentionDays != nil {
		if err := checkRange("日志保留天数", *in.LogRetentionDays, 1, 365); err != nil {
			return Settings{}, err
		}
		updates[model.SettingLogRetentionDays] = itoa(*in.LogRetentionDays)
	}
	if in.WxRequestTimeoutSeconds != nil {
		if err := checkRange("微信请求超时（秒）", *in.WxRequestTimeoutSeconds, 1, 120); err != nil {
			return Settings{}, err
		}
		updates[model.SettingWxRequestTimeout] = itoa(*in.WxRequestTimeoutSeconds)
	}
	if in.PrivacyCheckMaxWaitSeconds != nil {
		if err := checkRange("隐私检测最长等待（秒）", *in.PrivacyCheckMaxWaitSeconds, 10, 3600); err != nil {
			return Settings{}, err
		}
		updates[model.SettingPrivacyCheckMaxWait] = itoa(*in.PrivacyCheckMaxWaitSeconds)
	}
	if in.AuditResultMaxWaitSeconds != nil {
		if err := checkRange("审核结果最长等待（秒）", *in.AuditResultMaxWaitSeconds, 60, 604800); err != nil {
			return Settings{}, err
		}
		updates[model.SettingAuditResultMaxWait] = itoa(*in.AuditResultMaxWaitSeconds)
	}
	if in.DefaultTemplateID != nil {
		updates[model.SettingDefaultTemplateID] = int64PtrToString(in.DefaultTemplateID)
	}
	if in.DefaultAuditProfileID != nil {
		updates[model.SettingDefaultAuditProfileID] = int64PtrToString(in.DefaultAuditProfileID)
	}

	for key, value := range updates {
		row := model.RuntimeSetting{Key: key, Value: value}
		if err := s.db.Save(&row).Error; err != nil {
			return Settings{}, Internal(err)
		}
	}
	return s.Get()
}

// SettingsPatch 局部更新（nil 表示保持原值）。
type SettingsPatch struct {
	JobConcurrency             *int
	JobMaxAttempts             *int
	WxMaxQps                   *int
	LogRetentionDays           *int
	DefaultTemplateID          *int64
	DefaultAuditProfileID      *int64
	WxRequestTimeoutSeconds    *int
	PrivacyCheckMaxWaitSeconds *int
	AuditResultMaxWaitSeconds  *int
}

func checkRange(label string, value, min, max int) error {
	if value < min || value > max {
		return Validation("%s 必须在 %d-%d 之间", label, min, max)
	}
	return nil
}

func atoiOr(value string, def int) int {
	if value == "" {
		return def
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return def
	}
	return n
}

func parseInt64Ptr(value string) *int64 {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func int64PtrToString(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func itoa(v int) string { return strconv.Itoa(v) }

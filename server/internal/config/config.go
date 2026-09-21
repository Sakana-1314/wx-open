// Package config 负责从环境变量（可经 .env 加载）读取类型化配置。
//
// 设计取舍：
//   - JWT_SECRET / ADMIN_PASSWORD 必填，缺失拒绝启动；
//   - 第三方平台凭据（component_appid 等）允许为空：缺失时服务仍可启动，
//     由 platform/status 与前置体检提示「未配置」，任何真实微信调用都会被拦截。
package config

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 应用配置，全部字段类型化。
type Config struct {
	// 服务
	ServerPort     string
	PublicBaseURL  string
	EnableMockWX   bool
	MockWXFailRate int

	// 数据库（MySQL / MariaDB）
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string

	// 平台自身鉴权
	JWTSecret     string
	JWTTokenTTL   time.Duration
	AdminPassword string
	LoginRateMax  int
	LoginRateWin  time.Duration

	// 第三方平台凭据
	ComponentAppID       string
	ComponentAppSecret   string
	ComponentVerifyToken string
	ComponentAESKey      string
	ComponentAESKeyPrev  string
	SecretEncKey         []byte
	SecretEncKeyDerived  bool

	// 微信 API
	WxAPIBase        string
	WxRequestTimeout time.Duration
	WxMaxQps         int

	// 作业引擎
	JobConcurrency   int
	JobMaxAttempts   int
	LogRetentionDays int

	// 隐私检测 / 审核结果等待上限
	PrivacyCheckMaxWait time.Duration
	AuditResultMaxWait  time.Duration
}

// Load 读取并校验配置。
func Load() (*Config, error) {
	cfg := &Config{
		ServerPort:     getEnv("SERVER_PORT", "8091"),
		PublicBaseURL:  strings.TrimRight(getEnv("PUBLIC_BASE_URL", "http://127.0.0.1:8091"), "/"),
		EnableMockWX:   getEnvBool("MOCK_WX", false),
		MockWXFailRate: getEnvInt("MOCK_WX_FAIL_RATE", 0),

		DBHost:     getEnv("DB_HOST", "127.0.0.1"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "wxplatform"),
		DBPassword: getEnv("DB_PASSWORD", "wxplatform_dev_password"),
		DBName:     getEnv("DB_NAME", "wx_platform"),

		ComponentAppID:       os.Getenv("WX_COMPONENT_APPID"),
		ComponentAppSecret:   os.Getenv("WX_COMPONENT_APPSECRET"),
		ComponentVerifyToken: os.Getenv("WX_COMPONENT_VERIFY_TOKEN"),
		ComponentAESKey:      os.Getenv("WX_COMPONENT_ENCODING_AES_KEY"),
		ComponentAESKeyPrev:  os.Getenv("WX_COMPONENT_ENCODING_AES_KEY_PREV"),

		WxAPIBase:        getEnv("WX_API_BASE", "https://api.weixin.qq.com"),
		WxRequestTimeout: time.Duration(getEnvInt("WX_REQUEST_TIMEOUT_SECONDS", 15)) * time.Second,
		WxMaxQps:         getEnvInt("WX_MAX_QPS", 8),

		JobConcurrency:   getEnvInt("JOB_CONCURRENCY", 3),
		JobMaxAttempts:   getEnvInt("JOB_MAX_ATTEMPTS", 3),
		LogRetentionDays: getEnvInt("LOG_RETENTION_DAYS", 30),

		PrivacyCheckMaxWait: time.Duration(getEnvInt("PRIVACY_CHECK_MAX_WAIT_SECONDS", 600)) * time.Second,
		AuditResultMaxWait:  time.Duration(getEnvInt("AUDIT_RESULT_MAX_WAIT_SECONDS", 604800)) * time.Second,

		LoginRateMax: getEnvInt("LOGIN_RATE_MAX", 10),
		LoginRateWin: time.Duration(getEnvInt("LOGIN_RATE_WINDOW_SECONDS", 300)) * time.Second,
	}

	cfg.JWTSecret = os.Getenv("JWT_SECRET")
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("环境变量 JWT_SECRET 未设置，拒绝启动")
	}
	cfg.AdminPassword = os.Getenv("ADMIN_PASSWORD")
	if cfg.AdminPassword == "" {
		return nil, fmt.Errorf("环境变量 ADMIN_PASSWORD 未设置，拒绝启动")
	}

	ttlMinutes := getEnvInt("JWT_TOKEN_TTL_MINUTES", 1440)
	if ttlMinutes <= 0 {
		return nil, fmt.Errorf("JWT_TOKEN_TTL_MINUTES 必须为正整数")
	}
	cfg.JWTTokenTTL = time.Duration(ttlMinutes) * time.Minute

	if err := cfg.resolveSecretKey(); err != nil {
		return nil, err
	}
	if err := cfg.validateRanges(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// resolveSecretKey 解析 refresh_token 落库加密密钥（32 字节）。
// 未配置 SECRET_ENC_KEY 时由 JWT_SECRET 派生，避免因缺少一个变量而无法启动。
func (c *Config) resolveSecretKey() error {
	raw := strings.TrimSpace(os.Getenv("SECRET_ENC_KEY"))
	if raw == "" {
		sum := sha256.Sum256([]byte("wx-platform/secret-enc/" + c.JWTSecret))
		c.SecretEncKey = sum[:]
		c.SecretEncKeyDerived = true
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return fmt.Errorf("SECRET_ENC_KEY 必须是 base64 编码的 32 字节密钥: %w", err)
	}
	if len(key) != 32 {
		return fmt.Errorf("SECRET_ENC_KEY 解码后必须是 32 字节，当前 %d 字节", len(key))
	}
	c.SecretEncKey = key
	return nil
}

// validateRanges 校验取值范围，避免明显错误的配置进入运行时。
func (c *Config) validateRanges() error {
	if c.JobConcurrency < 1 || c.JobConcurrency > 8 {
		return fmt.Errorf("JOB_CONCURRENCY 必须在 1-8 之间")
	}
	if c.JobMaxAttempts < 1 || c.JobMaxAttempts > 8 {
		return fmt.Errorf("JOB_MAX_ATTEMPTS 必须在 1-8 之间")
	}
	if c.WxMaxQps < 1 || c.WxMaxQps > 50 {
		return fmt.Errorf("WX_MAX_QPS 必须在 1-50 之间")
	}
	if c.ComponentAESKey != "" && len(c.ComponentAESKey) != 43 {
		return fmt.Errorf("WX_COMPONENT_ENCODING_AES_KEY 必须是 43 个字符")
	}
	if c.ComponentAESKeyPrev != "" && len(c.ComponentAESKeyPrev) != 43 {
		return fmt.Errorf("WX_COMPONENT_ENCODING_AES_KEY_PREV 必须是 43 个字符")
	}
	return nil
}

// WeChatConfigured 判断是否具备调用微信接口的最小凭据。
func (c *Config) WeChatConfigured() bool {
	if c.EnableMockWX {
		return true
	}
	return c.ComponentAppID != "" && c.ComponentAppSecret != "" &&
		c.ComponentVerifyToken != "" && c.ComponentAESKey != ""
}

// CallbackReady 判断回调加解密是否可用（mock 模式下用内置凭据）。
func (c *Config) CallbackReady() bool {
	return c.ComponentVerifyToken != "" && len(c.ComponentAESKey) == 43
}

// DSN 返回 MySQL 连接串。
func (c *Config) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName)
}

// CallbackSchemeSafe 报告回调地址是否为 https（微信回调须公网可达，https 对应 443）。
func (c *Config) CallbackSchemeSafe() bool {
	return strings.HasPrefix(c.PublicBaseURL, "https://")
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

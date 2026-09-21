// 微信第三方平台管理平台服务端入口。
package main

import (
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"wx-platform/server/internal/auth"
	"wx-platform/server/internal/batch"
	"wx-platform/server/internal/callback"
	"wx-platform/server/internal/config"
	"wx-platform/server/internal/core"
	"wx-platform/server/internal/database"
	"wx-platform/server/internal/handler"
	"wx-platform/server/internal/mockwx"
	"wx-platform/server/internal/router"
	"wx-platform/server/internal/secretbox"
	"wx-platform/server/internal/service"
	"wx-platform/server/internal/wxapi"
	"wx-platform/server/internal/wxcrypt"
	"wx-platform/server/internal/wxjob"
	"wx-platform/server/internal/wxtoken"
)

func main() {
	// .env 可选：存在则加载，不存在时使用进程环境变量。
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("配置错误: %v", err)
	}
	if cfg.SecretEncKeyDerived {
		log.Printf("提示：未设置 SECRET_ENC_KEY，已由 JWT_SECRET 派生加密密钥（生产建议显式配置 32 字节 base64 密钥）")
	}
	if !cfg.WeChatConfigured() {
		log.Printf("警告：第三方平台凭据未配置完整，服务可启动但无法调用微信接口（请见 README 的接入指引）")
	}

	db, err := database.Connect(cfg.DSN())
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	if err := database.EnsurePlatformState(db, cfg.ComponentAppID); err != nil {
		log.Fatalf("平台状态初始化失败: %v", err)
	}
	if err := database.SeedRuntimeSettings(db, cfg); err != nil {
		log.Fatalf("运行参数初始化失败: %v", err)
	}
	if err := database.SeedDefaultAuditProfile(db); err != nil {
		log.Fatalf("提审配置初始化失败: %v", err)
	}

	box, err := secretbox.New(cfg.SecretEncKey)
	if err != nil {
		log.Fatalf("加密组件初始化失败: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	am := auth.NewManager(cfg.JWTSecret, cfg.JWTTokenTTL, cfg.AdminPassword)
	limiter := auth.NewLoginLimiter(cfg.LoginRateMax, cfg.LoginRateWin)
	container := service.NewContainer(db, cfg, box, am, limiter)
	env := container.Env

	// ---- 微信能力（含可选的本地模拟器）----
	baseURL := cfg.WxAPIBase
	mockServer := startMockIfNeeded(cfg, &baseURL)

	recorder := &service.WxCallRecorder{Repos: env.Repos}
	wxClient := wxapi.New(wxapi.Options{
		BaseURL: baseURL,
		Timeout: cfg.WxRequestTimeout,
		MaxQPS:  cfg.WxMaxQps,
		Logger:  recorder,
	})
	tokenStore := core.NewTokenStore(env.Repos)
	tokens := wxtoken.New(wxClient, tokenStore, tokenStore, box, cfg.ComponentAppID, cfg.ComponentAppSecret)
	if err := tokens.Load(ctx); err != nil {
		log.Printf("警告：加载已持久化的票据/令牌失败（不影响启动）: %v", err)
	}
	container.AttachWeChat(wxClient, tokens)

	// ---- 微信回调（公开路由，不进 JSON 契约）----
	vars := callback.Options{
		Tickets:        tokens,
		Events:         env.Repos.Callbacks,
		Business:       container.NewCallbackBusiness(),
		ComponentAppid: cfg.ComponentAppID,
		Async:          true,
	}
	if cfg.ComponentVerifyToken != "" && len(cfg.ComponentAESKey) == 43 {
		crypto, err := wxcrypt.New(cfg.ComponentVerifyToken, cfg.ComponentAESKey, cfg.ComponentAppID, cfg.ComponentAESKeyPrev)
		if err != nil {
			log.Fatalf("初始化消息加解密失败: %v", err)
		}
		vars.Crypto = crypto
	} else {
		log.Printf("警告：未配置消息校验 Token / 消息加解密 Key，回调接口会返回 400（微信无法完成接入校验）")
	}
	callbacks := callback.New(vars)

	// ---- 批量作业引擎 ----
	engine := batch.New(wxjob.NewEngineStore(env), batch.Options{
		Concurrency: func() int {
			settings, err := env.Settings.Get()
			if err != nil || settings.JobConcurrency <= 0 {
				return cfg.JobConcurrency
			}
			return settings.JobConcurrency
		},
		MaxAttempts: func() int {
			settings, err := env.Settings.Get()
			if err != nil || settings.JobMaxAttempts <= 0 {
				return cfg.JobMaxAttempts
			}
			return settings.JobMaxAttempts
		},
		PollInterval: 2 * time.Second,
		LockName:     "wx_platform_job_engine:" + cfg.DBName,
		Logger:       log.Printf,
	})
	container.AttachEngine(engine)
	if err := engine.Start(ctx); err != nil {
		log.Printf("警告：作业引擎未启动（%v）：平台仍可浏览与创建作业，但不会执行", err)
	} else {
		log.Printf("批量作业引擎已启动（并发上限由设置页控制）")
	}

	srv := handler.NewServer(container)
	engine2 := router.NewRouter(srv, am, callbacks)

	// ---- 定时对账与清理（审核状态兜底、授权方资料刷新、票据告警、日志清理）----
	service.NewScheduler(container).Start(ctx)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           engine2,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		log.Printf("收到退出信号，正在关闭服务…")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			log.Printf("关闭 HTTP 服务出错: %v", err)
		}
		if mockServer != nil {
			mockServer.Close()
		}
	}()

	authURL, msgURL := core.CallbackURLs(cfg.PublicBaseURL)
	log.Printf("微信第三方平台管理平台启动，监听 :%s", cfg.ServerPort)
	log.Printf("请在开放平台后台填写：授权事件接收 URL = %s，消息与事件接收 URL = %s", authURL, msgURL)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("服务启动失败: %v", err)
	}
	log.Printf("服务已退出")
}

// startMockIfNeeded 在 MOCK_WX=1 时启动内置模拟微信服务端，并把微信 API 基址指向它。
//
// 这样可以在没有真实凭据的情况下端到端验证授权、模板、批量上传/提审/发布全链路。
func startMockIfNeeded(cfg *config.Config, baseURL *string) *httptest.Server {
	if !cfg.EnableMockWX {
		return nil
	}
	mock := mockwx.New(mockwx.Options{
		ComponentAppID: firstNonEmpty(cfg.ComponentAppID, "wx_mock_component"),
		VerifyToken:    firstNonEmpty(cfg.ComponentVerifyToken, "mock_verify_token"),
		EncodingAESKey: firstNonEmpty(cfg.ComponentAESKey, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq"),
	})
	srv := httptest.NewServer(mock.Handler())
	*baseURL = srv.URL
	log.Printf("⚠️  MOCK_WX=1：已启用内置模拟微信服务端 %s（不会调用真实微信，仅供本地联调）", srv.URL)
	return srv
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

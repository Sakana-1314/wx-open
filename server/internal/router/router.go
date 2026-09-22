// Package router 组装 Gin 引擎：中间件、鉴权分组与路由注册。
package router

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"

	"wx-platform/server/internal/auth"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/handler"
)

// HealthResponse 健康检查响应。
type HealthResponse struct {
	Status string `json:"status"`
}

// CallbackRegistrar 微信回调路由的注册者。
//
// 回调收发 XML、成功时需返回纯文本 success，与 JSON 契约不兼容，
// 因此不进 openapi.yaml，而是在这里以公开路由单独注册。
type CallbackRegistrar interface {
	RegisterCallbacks(r gin.IRouter)
}

// NewRouter 构建 Gin 引擎。
//
// 鉴权策略：POST /api/v1/auth/login 与 /healthz 公开，/callback/* 由微信侧以自己的
// msg_signature 校验（不适用 JWT），其余 /api/v1/* 均需有效 JWT 且 user_type=admin。
func NewRouter(srv *handler.Server, am *auth.Manager, callbacks CallbackRegistrar) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), recoveryMiddleware(), corsMiddleware())

	// 框架兜底：404 / 405 统一输出 JSON。
	r.HandleMethodNotAllowed = true
	r.NoRoute(noRouteHandler)
	r.NoMethod(noMethodHandler)

	// 健康检查同时支持 GET 与 HEAD：Docker/K8s/负载均衡的探针常用 HEAD
	// （wget --spider、curl -I），只注册 GET 会返回 405，容器永远无法变为 healthy。
	healthz := func(c *gin.Context) {
		c.JSON(http.StatusOK, HealthResponse{Status: "ok"})
	}
	r.GET("/healthz", healthz)
	r.HEAD("/healthz", healthz)

	if callbacks != nil {
		callbacks.RegisterCallbacks(r)
	}

	api := r.Group("/api/v1")
	api.Use(apiAuthMiddleware(am))
	gen.RegisterHandlersWithOptions(api, srv, gen.GinServerOptions{
		BaseURL:      "",
		ErrorHandler: oapiErrorHandler,
	})

	return r
}

// apiAuthMiddleware 校验 JWT：登录端点放行，其余要求 admin。
func apiAuthMiddleware(am *auth.Manager) gin.HandlerFunc {
	jwtMW := auth.JWTMiddleware(am)
	adminMW := auth.RequireAdmin()
	return func(c *gin.Context) {
		if c.FullPath() == "/api/v1/auth/login" {
			c.Next()
			return
		}
		jwtMW(c)
		if c.IsAborted() {
			return
		}
		adminMW(c)
	}
}

// corsMiddleware 允许跨域：来源按 Origin 回显，缺失时按 Referer 解析，均无则 *。
// 鉴权走 Authorization: Bearer（非 Cookie），无需精确来源白名单。
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if origin := resolveAllowOrigin(c.GetHeader("Origin"), c.GetHeader("Referer")); origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func resolveAllowOrigin(origin, referer string) string {
	if o := originFrom(origin); o != "" {
		return o
	}
	if o := originFrom(referer); o != "" {
		return o
	}
	return "*"
}

func originFrom(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

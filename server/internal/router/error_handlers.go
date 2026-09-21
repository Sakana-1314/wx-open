// 框架级异常统一 JSON 输出。
//
// 业务层 / handler / auth 中间件均已按契约输出 gen.Error（code + message）；
// 本文件补齐框架兜底路径，保证 404 / 405 / 500（panic）/ 参数绑定错误
// 也返回同构 JSON，而不是 gin 默认的纯文本或空响应体。
package router

import (
	"log"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"wx-platform/server/internal/gen"
)

func errorResponse(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gen.Error{Code: code, Message: message})
}

// recoveryMiddleware 捕获 panic：服务端记录堆栈，客户端收到统一 JSON 500。
func recoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[panic] %s %s: %v\n%s", c.Request.Method, c.Request.URL.Path, rec, debug.Stack())
				errorResponse(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务器内部错误")
			}
		}()
		c.Next()
	}
}

// noRouteHandler 未知路由返回 JSON 404。
func noRouteHandler(c *gin.Context) {
	errorResponse(c, http.StatusNotFound, "NOT_FOUND", "接口不存在")
}

// noMethodHandler 方法不允许返回 JSON 405。
func noMethodHandler(c *gin.Context) {
	errorResponse(c, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "请求方法不被允许")
}

// oapiErrorHandler 把 oapi-codegen 的路径/查询参数解析错误转成统一 JSON。
func oapiErrorHandler(c *gin.Context, err error, statusCode int) {
	code := "BAD_REQUEST"
	if statusCode == http.StatusNotFound {
		code = "NOT_FOUND"
	}
	errorResponse(c, statusCode, code, "请求参数错误: "+err.Error())
}

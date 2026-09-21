// Package handler 实现 oapi-codegen 生成的 ServerInterface（HTTP 编解码层）。
//
// 分层约定：handler 只做参数绑定、调用 service、写响应；业务规则一律在 service，
// 微信协议细节一律在 wxcrypt / wxapi / wxtoken。
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
)

// writeError 输出契约 Error（code + message）。
func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gen.Error{Code: code, Message: message})
}

// writeBadRequest 请求体或参数格式错误。
func writeBadRequest(c *gin.Context, message string) {
	writeError(c, http.StatusBadRequest, "BAD_REQUEST", message)
}

// writeValidation 业务校验失败。
func writeValidation(c *gin.Context, message string) {
	writeError(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", message)
}

// writeNotFound 资源不存在。
func writeNotFound(c *gin.Context, message string) {
	writeError(c, http.StatusNotFound, "NOT_FOUND", message)
}

// writeConflict 状态冲突。
func writeConflict(c *gin.Context, message string) {
	writeError(c, http.StatusConflict, "CONFLICT", message)
}

// writeServiceError 把 service 层错误映射为 HTTP 响应。
//
// 映射规则集中在 service 包定义的哨兵错误上，避免每个 handler 重复判断。
func writeServiceError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	var apiErr *core.WeChatError
	if errors.As(err, &apiErr) {
		// 微信侧失败：502 + 具体 errcode，便于前端直接展示官方 errmsg 与处置建议。
		e := gen.Error{Code: "WECHAT_API_ERROR", Message: apiErr.Error()}
		code := apiErr.Errcode
		e.Errcode = &code
		c.JSON(http.StatusBadGateway, e)
		return
	}
	switch {
	case errors.Is(err, core.ErrNotFound):
		writeNotFound(c, err.Error())
	case errors.Is(err, core.ErrConflict):
		writeConflict(c, err.Error())
	case errors.Is(err, core.ErrValidation):
		writeValidation(c, err.Error())
	case errors.Is(err, core.ErrUnauthorized):
		writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", err.Error())
	case errors.Is(err, core.ErrNotConfigured):
		writeError(c, http.StatusConflict, "WECHAT_NOT_CONFIGURED", err.Error())
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
	}
}

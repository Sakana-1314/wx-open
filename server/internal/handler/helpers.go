package handler

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"wx-platform/server/internal/auth"
	"wx-platform/server/internal/gen"
)

// bindOptionalJSON 绑定可选的 JSON 请求体：空 body（io.EOF）视为缺省，不报错。
//
// 契约里若干 POST 的 body 是可选或全可选字段（如同步作业、版本回退），
// 前端可能发送空对象或不带 body，两种都要接受。
func bindOptionalJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil && !errors.Is(err, io.EOF) {
		writeBadRequest(c, "请求体格式错误")
		return false
	}
	return true
}

// actorOf 取当前操作者（用于操作日志）。
func actorOf(c *gin.Context) string {
	if claims := auth.ClaimsFrom(c); claims != nil {
		return claims.Subject
	}
	return auth.AdminUsername
}

// noContent 写出 204。
func noContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// bindJSON 绑定必填 JSON 请求体，失败时已写出 400。
func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		writeBadRequest(c, "请求体格式错误")
		return false
	}
	return true
}

// callAndRespond 统一「调用服务 → 错误映射 → 写出响应」的收尾动作。
func callAndRespond[T any](c *gin.Context, res T, err error, status int) {
	if err != nil {
		writeServiceError(c, err)
		return
	}
	c.JSON(status, res)
}

// 编译期确保生成的 DTO 不会因为契约重命名而静默失配。
var (
	_ = gen.Error{}
	_ = gen.AppidSelection{}
)

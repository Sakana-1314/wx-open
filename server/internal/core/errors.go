// Package core 提供各业务服务共用的地基：哨兵错误与微信错误包装、目标小程序选择、
// ext_json 渲染、授权链接拼接、运行参数与平台状态，以及数据访问聚合（Repos）。
//
// handler 只做 HTTP 编解码，所有规则集中在这里；对微信的调用一律经 wxapi/wxtoken。
package core

import (
	"errors"
	"fmt"

	"wx-platform/server/internal/model"
)

// 哨兵错误：handler 依据它们映射 HTTP 状态码。
var (
	// ErrNotFound 资源不存在。
	ErrNotFound = errors.New("资源不存在")
	// ErrConflict 状态冲突（重复在途作业、额度耗尽、前置条件不满足等）。
	ErrConflict = errors.New("状态冲突")
	// ErrValidation 参数或业务校验失败。
	ErrValidation = errors.New("参数校验失败")
	// ErrUnauthorized 认证失败。
	ErrUnauthorized = errors.New("认证失败")
	// ErrNotConfigured 第三方平台凭据未配置。
	ErrNotConfigured = errors.New("第三方平台凭据未配置（WX_COMPONENT_APPID / APPSECRET / VERIFY_TOKEN / ENCODING_AES_KEY）")
)

// WeChatError 微信侧返回的错误（含返回码、分类与处置建议）。
type WeChatError struct {
	Endpoint string
	Method   string
	Appid    string
	Errcode  int
	Errmsg   string
	Class    model.ErrorClass
	Text     string
	Hint     string
}

// Error 实现 error。
func (e *WeChatError) Error() string {
	if e.Errcode == 0 {
		return fmt.Sprintf("调用微信接口 %s 失败: %s", e.Endpoint, e.Errmsg)
	}
	msg := fmt.Sprintf("微信接口 %s 返回 %d（%s）", e.Endpoint, e.Errcode, e.Errmsg)
	if e.Text != "" {
		msg += "：" + e.Text
	}
	if e.Hint != "" {
		msg += "；建议：" + e.Hint
	}
	return msg
}

// NotFound 构造 ErrNotFound（带上下文）。
func NotFound(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrNotFound, fmt.Sprintf(format, args...))
}

// Conflict 构造 ErrConflict（带上下文）。
func Conflict(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrConflict, fmt.Sprintf(format, args...))
}

// Validation 构造 ErrValidation（带上下文）。
func Validation(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

// Internal 包装内部错误（数据库等）。
func Internal(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("内部错误: %w", err)
}

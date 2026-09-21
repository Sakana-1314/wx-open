package wxapi

import (
	"errors"
	"fmt"
	"strings"

	"wx-platform/server/internal/model"
)

// APIError 微信侧调用失败。
//
// 覆盖三种情形：
//   - HTTP 200 但 errcode != 0；
//   - HTTP 非 2xx；
//   - 网络 / 超时 / JSON 解析失败（此时 Errcode == -1）。
type APIError struct {
	Endpoint   string
	Method     string
	Appid      string
	Errcode    int
	Errmsg     string
	HTTPStatus int
	Raw        string // 原始响应体（截断）

	// cause 底层错误（网络 / 解析 / 限流取消），不参与对外签名，供 errors.Is / errors.As 穿透。
	cause error
}

// newNetworkError 构造网络 / 本地失败（Errcode == -1，Raw 保留可读原因）。
func newNetworkError(spec callSpec, reason string, cause error) *APIError {
	raw := reason
	if cause != nil {
		raw = reason + "：" + cause.Error()
	}
	return &APIError{
		Endpoint: spec.path,
		Method:   spec.method,
		Appid:    spec.appid,
		Errcode:  -1,
		Errmsg:   raw,
		Raw:      truncateRaw([]byte(raw)),
		cause:    cause,
	}
}

// errOrNil 把 *APIError 转为 error，避免「接口非空但底层指针为 nil」的经典陷阱。
//
// 直接 return 一个 (*APIError)(nil) 会给调用方一个 != nil 的 error，
// 所有返回 error 的公开方法都必须经过本函数。
func errOrNil(apiErr *APIError) error {
	if apiErr == nil {
		return nil
	}
	return apiErr
}

// Error 返回中文错误描述：含 errcode、官方 errmsg、本地中文说明与处置建议。
func (e *APIError) Error() string {
	if e == nil {
		return "<nil>"
	}
	var b strings.Builder
	b.WriteString("微信接口调用失败：")
	b.WriteString(e.Method)
	b.WriteString(" ")
	b.WriteString(e.Endpoint)
	if e.Appid != "" {
		fmt.Fprintf(&b, "（appid=%s）", e.Appid)
	}
	if e.HTTPStatus != 0 && (e.HTTPStatus < 200 || e.HTTPStatus > 299) {
		fmt.Fprintf(&b, "，HTTP 状态码 %d", e.HTTPStatus)
	}
	fmt.Fprintf(&b, "，errcode=%d", e.Errcode)
	if e.Errmsg != "" {
		fmt.Fprintf(&b, "，微信 errmsg=%q", e.Errmsg)
	}
	fmt.Fprintf(&b, "，本地说明=%s", model.ErrcodeText(e.Errcode))
	if _, _, hint, ok := model.ErrcodeRule(e.Errcode); ok && hint != "" {
		fmt.Fprintf(&b, "（处置建议：%s）", hint)
	}
	fmt.Fprintf(&b, "，分类=%s", e.Class())
	if e.Raw != "" {
		fmt.Fprintf(&b, "，原始响应=%s", e.Raw)
	}
	return b.String()
}

// Class 返回返回码的处置分类（基于 model.ClassifyErrcode）。
func (e *APIError) Class() model.ErrorClass {
	if e == nil {
		return model.ClassUnknown
	}
	return model.ClassifyErrcode(e.Errcode)
}

// Unwrap 暴露底层错误，便于调用方用 errors.Is(err, context.DeadlineExceeded) 判断超时。
func (e *APIError) Unwrap() error { return e.cause }

// IsAPIError 判断 err 是否为 *APIError（支持包装链）。
func IsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

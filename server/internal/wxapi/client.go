// Package wxapi 是微信开放平台「第三方平台」服务端 HTTP 客户端。
//
// 设计要点（逐条对应 docs/wx-docs 单页文档与 docs/reference/wx-open-platform-api.md）：
//   - 令牌一律只作为 URL query 参数 access_token 传递，业务字段才进 body；
//   - 每个接口的 HTTP 方法都按文档逐个核对（详见各方法的中文注释，GET 与「必须发 {}」都单独标注）；
//   - 全局 QPS 限流使用标准库实现的令牌桶，Options.MaxQPS <= 0 时不限流；
//   - 所有调用（含失败）都会在 defer 中把 CallRecord 交给 Options.Logger，
//     Request / Response 都会脱敏（access_token / secret / refresh_token / ticket 一律替换为 "***"）；
//   - errcode == 0 视为成功；HTTP 非 2xx、网络失败、JSON 解析失败一律返回 *APIError（网络失败时 Errcode == -1）。
//
// 与 docs/reference/wx-open-platform-api.md 冲突时以 docs/wx-docs 单页文档为准，冲突点写在对应方法注释里。
package wxapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"wx-platform/server/internal/model"
)

// 默认值（Options 未显式给出时使用）。
const (
	// DefaultBaseURL 微信开放平台接口默认域名。
	DefaultBaseURL = "https://api.weixin.qq.com"
	// DefaultTimeout 单次调用默认超时。
	DefaultTimeout = 15 * time.Second
	// DefaultMaxQPS 默认每秒最大调用次数。
	DefaultMaxQPS = 8
)

// 日志与错误信息截断限制（避免把小程序代码包、ext_json、二维码二进制写进日志）。
const (
	// maxLogStringLen 日志中单个字符串字段的最大长度。
	maxLogStringLen = 256
	// maxLogArrayLen 日志中数组保留的最大元素个数。
	maxLogArrayLen = 20
	// maxLogMapLen 日志中 map 保留的最大键个数。
	maxLogMapLen = 40
	// maxLogBodyBytes 日志中整条请求 / 响应体序列化后的最大字节数。
	maxLogBodyBytes = 4096
	// maxErrorRawLen APIError.Raw 保留的最大字节数。
	maxErrorRawLen = 512
	// maxResponseBytes 单次响应读取上限（提审素材返回体很小，二维码图片也只有百 KB 级）。
	maxResponseBytes = 8 << 20
	// maskedValue 脱敏后的占位值。
	maskedValue = "***"
)

// sensitiveKeyParts 命中即脱敏的字段名片段（小写比较）。
//
// 覆盖 access_token / authorizer_access_token / component_access_token / refresh_token /
// authorizer_refresh_token / appsecret / component_secret / component_appsecret / component_verify_ticket。
var sensitiveKeyParts = []string{"token", "secret", "ticket", "password", "passwd", "apikey", "api_key"}

// Options 客户端构造参数。
type Options struct {
	BaseURL string        // 默认 https://api.weixin.qq.com
	Timeout time.Duration // 默认 15s
	MaxQPS  int           // 默认 8；<=0 表示不限
	Logger  CallLogger    // 可选
}

// CallRecord 单次微信接口调用的记录。
type CallRecord struct {
	Endpoint   string           // 如 "/wxa/commit"
	Method     string           // GET / POST
	Scope      model.TokenScope // 本次调用使用的令牌归属
	Appid      string           // 本次调用针对的 appid（授权方 appid，或第三方平台 appid）
	HTTPStatus int              // HTTP 状态码，0 表示请求未发出（网络 / 限流失败）
	OK         bool             // HTTP 2xx 且 errcode == 0
	Errcode    int              // 微信返回码；网络失败为 -1
	Errmsg     string           // 微信 errmsg 或本地原因
	DurationMs int64            // 耗时（毫秒）
	Request    map[string]any   // 已脱敏的请求参数（query + body 平铺）
	Response   map[string]any   // 已脱敏并截断的响应体
}

// CallLogger 调用记录接收者（可选）。
type CallLogger interface{ LogCall(rec CallRecord) }

// Client 微信开放平台第三方平台客户端。可并发使用。
type Client struct {
	baseURL string
	hc      *http.Client
	logger  CallLogger
	limiter *tokenBucket // nil 表示不限流
	maxQPS  int
}

// New 构造客户端。
//
// MaxQPS 语义说明：任务书同时写了「默认 8」与「<= 0 表示不限」，二者对 0 的含义互斥；
// 因为 Go 的 int 零值无法与「未设置」区分，这里取：0 == 用默认值 8，负数 == 不限流。
func New(opts Options) *Client {
	base := strings.TrimSpace(opts.BaseURL)
	if base == "" {
		base = DefaultBaseURL
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	qps := opts.MaxQPS
	if qps == 0 {
		qps = DefaultMaxQPS
	}
	c := &Client{
		baseURL: strings.TrimRight(base, "/"),
		hc:      &http.Client{Timeout: timeout},
		logger:  opts.Logger,
		maxQPS:  qps,
	}
	if qps > 0 {
		c.limiter = newTokenBucket(qps)
	}
	return c
}

// BaseURL 返回生效的接口域名（不含结尾斜杠）。
func (c *Client) BaseURL() string { return c.baseURL }

// ---------------------------------------------------------------------------
// 请求 / 响应基础设施
// ---------------------------------------------------------------------------

// callSpec 一次调用的内部描述。
type callSpec struct {
	method    string // http.MethodGet / http.MethodPost
	path      string // 如 "/wxa/commit"
	scope     model.TokenScope
	appid     string
	token     string         // 非空时作为 access_token query 参数
	query     url.Values     // 额外 query 参数（如 revertcoderelease 的 action / app_version）
	jsonBody  any            // 非 nil 时序列化为 JSON body
	emptyJSON bool           // 文档要求「必须发空 JSON {}」的 POST
	multipart *multipartBody // 非 nil 时以 multipart/form-data 上传
	binary    bool           // 期望二进制响应（get_qrcode）
}

// multipartBody multipart/form-data 上传内容。
type multipartBody struct {
	fieldName   string
	fileName    string
	contentType string
	content     []byte
}

// rawResponse 读回的原始响应。
type rawResponse struct {
	status      int
	contentType string
	body        []byte
}

// requestRecord 生成脱敏后的请求参数记录（query 与 body 字段平铺）。
func (s callSpec) requestRecord() map[string]any {
	rec := map[string]any{}
	for k, vs := range s.query {
		if len(vs) > 0 {
			rec[k] = vs[0]
		}
	}
	if s.token != "" {
		// 只记录「用了令牌」这一事实，绝不落原文。
		rec["access_token"] = maskedValue
	}
	if s.multipart != nil {
		rec[s.multipart.fieldName+"_filename"] = s.multipart.fileName
		rec[s.multipart.fieldName+"_size"] = len(s.multipart.content)
		rec[s.multipart.fieldName+"_content_type"] = s.multipart.contentType
	}
	if s.jsonBody != nil {
		if b, err := json.Marshal(s.jsonBody); err == nil {
			var m map[string]any
			if json.Unmarshal(b, &m) == nil {
				for k, v := range m {
					rec[k] = v
				}
			}
		}
	}
	return sanitizeMap(rec)
}

// do 执行一次 HTTP 调用，负责限流、脱敏、日志与错误归类。
//
// 返回的 rawResponse 仅在 HTTP 2xx 且 errcode == 0（或二进制成功）时非 nil。
func (c *Client) do(ctx context.Context, spec callSpec) (*rawResponse, *APIError) {
	start := time.Now()
	rec := &CallRecord{
		Endpoint: spec.path,
		Method:   spec.method,
		Scope:    spec.scope,
		Appid:    spec.appid,
		Request:  spec.requestRecord(),
	}
	// 每次调用（无论成败）都在 defer 中把记录交给 Logger。
	defer func() {
		rec.DurationMs = time.Since(start).Milliseconds()
		if c.logger != nil {
			c.logger.LogCall(*rec)
		}
	}()

	fail := func(status int, apiErr *APIError, resp map[string]any) (*rawResponse, *APIError) {
		rec.HTTPStatus = status
		rec.OK = false
		rec.Errcode = apiErr.Errcode
		rec.Errmsg = apiErr.Errmsg
		rec.Response = resp
		return nil, apiErr
	}

	// 1. 全局 QPS 限流（令牌桶，标准库实现）。
	if c.limiter != nil {
		if err := c.limiter.Wait(ctx); err != nil {
			return fail(0, newNetworkError(spec, "等待限流令牌失败/被取消", err), nil)
		}
	}

	// 2. 组装 URL：令牌只进 query，不进 body。
	q := url.Values{}
	for k, vs := range spec.query {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	if spec.token != "" {
		q.Set("access_token", spec.token)
	}
	target := c.baseURL + spec.path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}

	// 3. 组装 body。
	var (
		bodyBytes   []byte
		contentType string
		err         error
	)
	switch {
	case spec.multipart != nil:
		bodyBytes, contentType, err = spec.multipart.build()
		if err != nil {
			return fail(0, newNetworkError(spec, "构造 multipart 请求体失败", err), nil)
		}
	case spec.jsonBody != nil:
		bodyBytes, err = json.Marshal(spec.jsonBody)
		if err != nil {
			return fail(0, newNetworkError(spec, "序列化请求体失败", err), nil)
		}
		contentType = "application/json"
	case spec.emptyJSON:
		// 文档要求「必须发空 JSON {}」：不传 data 会返回 44002 empty post data。
		bodyBytes = []byte("{}")
		contentType = "application/json"
	}

	req, err := http.NewRequestWithContext(ctx, spec.method, target, bytes.NewReader(bodyBytes))
	if err != nil {
		return fail(0, newNetworkError(spec, "构造 HTTP 请求失败", err), nil)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json, image/*")

	resp, err := c.hc.Do(req)
	if err != nil {
		return fail(0, newNetworkError(spec, "请求微信接口失败（网络/超时）", err), nil)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fail(resp.StatusCode, newNetworkError(spec, "读取微信响应失败", err), nil)
	}
	respContentType := resp.Header.Get("Content-Type")
	respRecord := responseRecord(respContentType, data)

	// 4. HTTP 非 2xx：一律算失败。
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		apiErr := &APIError{
			Endpoint:   spec.path,
			Method:     spec.method,
			Appid:      spec.appid,
			HTTPStatus: resp.StatusCode,
			Raw:        truncateRaw(data),
		}
		if code, msg, ok := envelopeOf(data); ok {
			apiErr.Errcode, apiErr.Errmsg = code, msg
		} else {
			apiErr.Errcode = -1
			apiErr.Errmsg = fmt.Sprintf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(data)))
		}
		return fail(resp.StatusCode, apiErr, respRecord)
	}

	// 5. 二进制响应（体验版二维码）：非 JSON 即成功，直接返回原始字节。
	//    失败时微信返回的是 JSON errcode，因此 JSON 响应继续走下面的信封解析路径。
	if spec.binary && !looksLikeJSON(respContentType, data) {
		rec.HTTPStatus = resp.StatusCode
		rec.Errcode = 0
		rec.OK = true
		rec.Response = respRecord
		return &rawResponse{status: resp.StatusCode, contentType: respContentType, body: data}, nil
	}

	// 6. 空响应体容错：按「无字段」处理（微信正常都会返回 JSON）。
	if len(bytes.TrimSpace(data)) == 0 {
		rec.HTTPStatus = resp.StatusCode
		rec.Errcode = 0
		rec.OK = true
		rec.Response = respRecord
		return &rawResponse{status: resp.StatusCode, contentType: respContentType, body: data}, nil
	}

	// 7. 解析 errcode / errmsg 信封。
	code, msg, ok := envelopeOf(data)
	if !ok {
		apiErr := &APIError{
			Endpoint:   spec.path,
			Method:     spec.method,
			Appid:      spec.appid,
			Errcode:    -1,
			Errmsg:     "解析微信响应 JSON 失败",
			HTTPStatus: resp.StatusCode,
			Raw:        truncateRaw(data),
		}
		return fail(resp.StatusCode, apiErr, respRecord)
	}
	if code != 0 {
		apiErr := &APIError{
			Endpoint:   spec.path,
			Method:     spec.method,
			Appid:      spec.appid,
			Errcode:    code,
			Errmsg:     msg,
			HTTPStatus: resp.StatusCode,
			Raw:        truncateRaw(data),
		}
		return fail(resp.StatusCode, apiErr, respRecord)
	}

	rec.HTTPStatus = resp.StatusCode
	rec.Errcode = 0
	rec.OK = true
	rec.Response = respRecord
	return &rawResponse{status: resp.StatusCode, contentType: respContentType, body: data}, nil
}

// jsonCall 执行调用并把响应体反序列化到 out（out 可为 nil）。
func (c *Client) jsonCall(ctx context.Context, spec callSpec, out any) *APIError {
	res, apiErr := c.do(ctx, spec)
	if apiErr != nil {
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(res.body, out); err != nil {
		return &APIError{
			Endpoint:   spec.path,
			Method:     spec.method,
			Appid:      spec.appid,
			Errcode:    -1,
			Errmsg:     "解析微信响应失败：" + err.Error(),
			HTTPStatus: res.status,
			Raw:        truncateRaw(res.body),
		}
	}
	return nil
}

// getJSON 发 GET 请求（文档要求 GET 的接口一律走这里；GET 不带 body）。
func (c *Client) getJSON(ctx context.Context, path string, scope model.TokenScope, appid, token string, query url.Values, out any) *APIError {
	return c.jsonCall(ctx, callSpec{
		method: http.MethodGet, path: path, scope: scope, appid: appid, token: token, query: query,
	}, out)
}

// postJSON 发 POST 请求，body 为 JSON 序列化后的业务字段。
func (c *Client) postJSON(ctx context.Context, path string, scope model.TokenScope, appid, token string, body, out any) *APIError {
	return c.jsonCall(ctx, callSpec{
		method: http.MethodPost, path: path, scope: scope, appid: appid, token: token, jsonBody: body,
	}, out)
}

// postEmpty 发「必须带空 JSON {}」的 POST 请求。
//
// 文档明确提示：post 的 data 为空不等于不需要传 data，否则返回 44002 empty post data。
func (c *Client) postEmpty(ctx context.Context, path string, scope model.TokenScope, appid, token string, out any) *APIError {
	return c.jsonCall(ctx, callSpec{
		method: http.MethodPost, path: path, scope: scope, appid: appid, token: token, emptyJSON: true,
	}, out)
}

// build 生成 multipart/form-data 请求体与 Content-Type。
func (m *multipartBody) build() ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="%s"; filename="%s"`, m.fieldName, m.fileName)}
	if m.contentType != "" {
		h["Content-Type"] = []string{m.contentType}
	}
	part, err := w.CreatePart(h)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(m.content); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// looksLikeJSON 判断响应是否为 JSON（用于二进制接口的错误分流）。
func looksLikeJSON(contentType string, data []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "json") {
		return true
	}
	trimmed := bytes.TrimSpace(data)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// envelopeOf 从响应体中取出 errcode / errmsg。
//
// 单个 JSON 对象且无 errcode 字段（如 get_authorizer_option 只返回 option_name/option_value）视为成功。
// errcode 兼容 number 与 string 两种写法。
func envelopeOf(body []byte) (int, string, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return 0, "", false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return 0, "", false
	}
	code := 0
	if v, ok := raw["errcode"]; ok {
		code = flexInt(v)
	}
	msg := ""
	if v, ok := raw["errmsg"]; ok {
		msg = flexString(v)
	}
	return code, msg, true
}

// ---------------------------------------------------------------------------
// 日志脱敏与截断
// ---------------------------------------------------------------------------

// isSensitiveKey 判断字段名是否需要脱敏。
func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, p := range sensitiveKeyParts {
		if strings.Contains(k, p) {
			return true
		}
	}
	return false
}

// sanitizeMap 递归脱敏并截断 map，最后按整体大小做一次兜底截断。
func sanitizeMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	n := 0
	for k, v := range m {
		if n >= maxLogMapLen {
			out["_truncated_keys"] = len(m) - maxLogMapLen
			break
		}
		n++
		if isSensitiveKey(k) {
			out[k] = maskedValue
			continue
		}
		out[k] = sanitizeValue(v, 0)
	}
	return capBySize(out)
}

// sanitizeValue 递归脱敏任意 JSON 值。
func sanitizeValue(v any, depth int) any {
	if depth > 6 {
		return "…(层级截断)"
	}
	switch t := v.(type) {
	case string:
		return truncateString(t)
	case map[string]any:
		out := make(map[string]any, len(t))
		n := 0
		for k, vv := range t {
			if n >= maxLogMapLen {
				out["_truncated_keys"] = len(t) - maxLogMapLen
				break
			}
			n++
			if isSensitiveKey(k) {
				out[k] = maskedValue
				continue
			}
			out[k] = sanitizeValue(vv, depth+1)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for i, vv := range t {
			if i >= maxLogArrayLen {
				out = append(out, fmt.Sprintf("…(共 %d 项，已截断)", len(t)))
				break
			}
			out = append(out, sanitizeValue(vv, depth+1))
		}
		return out
	default:
		return v
	}
}

// truncateString 截断过长字符串。
func truncateString(s string) string {
	if len(s) <= maxLogStringLen {
		return s
	}
	return s[:maxLogStringLen] + fmt.Sprintf("…(共 %d 字节，已截断)", len(s))
}

// capBySize 对序列化后过大的记录做兜底截断。
func capBySize(m map[string]any) map[string]any {
	b, err := json.Marshal(m)
	if err != nil || len(b) <= maxLogBodyBytes {
		return m
	}
	preview := string(b)
	if len(preview) > maxLogBodyBytes {
		preview = preview[:maxLogBodyBytes]
	}
	return map[string]any{
		"_truncated": true,
		"_bytes":     len(b),
		"_preview":   preview,
	}
}

// responseRecord 把响应体转换为可记录的 map（JSON 解析失败或二进制时给出摘要）。
func responseRecord(contentType string, data []byte) map[string]any {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		var m map[string]any
		if err := json.Unmarshal(trimmed, &m); err == nil {
			return sanitizeMap(m)
		}
	}
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []any
		if err := json.Unmarshal(trimmed, &arr); err == nil {
			return sanitizeMap(map[string]any{"_array": arr})
		}
	}
	return map[string]any{
		"_content_type": contentType,
		"_bytes":        len(data),
		"_binary":       true,
	}
}

// truncateRaw 截断原始响应体（保留在 APIError.Raw 中便于定位）。
func truncateRaw(data []byte) string {
	if len(data) <= maxErrorRawLen {
		return string(data)
	}
	return string(data[:maxErrorRawLen]) + fmt.Sprintf("…(共 %d 字节，已截断)", len(data))
}

// ---------------------------------------------------------------------------
// 固定类型解析辅助
// ---------------------------------------------------------------------------

// flexInt 把 JSON 原始值解析为 int（兼容 number 与 string，兼容小数）。
func flexInt(raw json.RawMessage) int {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return 0
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
		if f, err := n.Float64(); err == nil {
			return int(f)
		}
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			str = strings.TrimSpace(str)
			if i, err := strconv.ParseInt(str, 10, 64); err == nil {
				return int(i)
			}
			if f, err := strconv.ParseFloat(str, 64); err == nil {
				return int(f)
			}
		}
	}
	return 0
}

// flexInt64 把 JSON 原始值解析为 int64（兼容 number 与 string）。
func flexInt64(raw json.RawMessage) int64 {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return 0
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		if i, err := n.Int64(); err == nil {
			return i
		}
		if f, err := n.Float64(); err == nil {
			return int64(f)
		}
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			str = strings.TrimSpace(str)
			if i, err := strconv.ParseInt(str, 10, 64); err == nil {
				return i
			}
			if f, err := strconv.ParseFloat(str, 64); err == nil {
				return int64(f)
			}
		}
	}
	return 0
}

// flexString 把 JSON 原始值解析为 string（兼容 number）。
func flexString(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if s == "" || s == "null" {
		return ""
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(raw, &str); err == nil {
			return str
		}
		return ""
	}
	return s
}

// ---------------------------------------------------------------------------
// 令牌桶限流（标准库实现，不引第三方依赖）
// ---------------------------------------------------------------------------

// tokenBucket 令牌桶：容量与补充速率均为 qps（即允许 1 秒的量突发）。
type tokenBucket struct {
	mu       sync.Mutex
	rate     float64 // 每秒补充的令牌数
	capacity float64
	tokens   float64
	last     time.Time
}

// newTokenBucket 构造令牌桶。
func newTokenBucket(qps int) *tokenBucket {
	r := float64(qps)
	if r < 1 {
		r = 1
	}
	return &tokenBucket{rate: r, capacity: r, tokens: r, last: time.Now()}
}

// Wait 阻塞直到取得一个令牌或 ctx 结束。
func (b *tokenBucket) Wait(ctx context.Context) error {
	for {
		b.mu.Lock()
		now := time.Now()
		if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
			b.tokens += elapsed * b.rate
			if b.tokens > b.capacity {
				b.tokens = b.capacity
			}
			b.last = now
		}
		if b.tokens >= 1 {
			b.tokens--
			b.mu.Unlock()
			return nil
		}
		need := (1 - b.tokens) / b.rate
		b.mu.Unlock()

		if need <= 0 {
			continue
		}
		timer := time.NewTimer(time.Duration(need * float64(time.Second)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

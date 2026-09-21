// Package mockwx 实现一个「模拟微信开放平台服务端」的进程内 HTTP 服务，用于在没有真实微信凭据的情况下
// 端到端验证整个第三方平台：授权 → 模板库 → 批量上传代码 → 隐私检测 → 提审 → 审核结果推送 → 发布。
//
// 设计要点：
//   - 只依赖标准库与同项目内的 internal/wxcrypt，不引任何第三方依赖；
//   - 路径与字段名按官方文档实现（docs/wx-docs/*.md、docs/reference/wx-open-platform-api.md），响应体做了
//     必要裁剪，但字段名与大小写保持与官方一致（例如 get_latest_auditstatus 返回体里的 ScreenShot）；
//   - 令牌体系模拟真实行为：component_access_token 必须先用 component_verify_ticket 换取，授权方令牌由
//     api_query_auth / api_authorizer_token 签发；缺失 / 无效 / 过期分别返回 41001 / 40001 / 42001，
//     用于验证上层（internal/wxtoken）的令牌缓存与刷新逻辑；
//   - 推送（ticket / 授权事件 / 审核结果）用 wxcrypt 按官方安全模式加密，URL 上同时给出 3 字段 signature
//     与 4 字段 msg_signature，供上层按官方口径验签（官方明确「不要用 signature 验证」）；
//   - 内部状态全部由 Server.mu 保护，可并发调用（go test -race 通过）。
//
// 与真实微信的有意简化：不校验出口 IP、不校验 component_appid / component_appsecret、不实现频率限制、
// 不要求 /wxa/commit 的 template_id 真实存在。这些都不影响链路验证。
package mockwx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"wx-platform/server/internal/wxcrypt"
)

const (
	// defaultComponentAppID Options.ComponentAppID 留空时的默认第三方平台 appid。
	defaultComponentAppID = "wx_mock_component"
	// defaultVerifyToken Options.VerifyToken 留空时的默认消息校验 Token。
	defaultVerifyToken = "mock_verify_token"
	// defaultEncodingAESKey Options.EncodingAESKey 留空时的默认 43 字符密钥
	// （32 字节 "0123456789abcdef0123456789abcdef" 的 base64 去掉补位 '='）。
	defaultEncodingAESKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"

	// tokenTTL component_access_token / authorizer_access_token 有效期（官方 7200 秒）。
	tokenTTL = 7200 * time.Second
	// preAuthCodeTTL 预授权码有效期（官方正文 1800 秒）。
	preAuthCodeTTL = 1800 * time.Second
	// authCodeTTL 授权码有效期（官方「回调携带 expires_in=600」）。
	authCodeTTL = 600 * time.Second

	// templateLimit 模板库数量上限（官方：200 个，超限 85065）。
	templateLimit = 200
	// codePermID 小程序开发与数据分析权限集 id（/wxa/* 代码管理接口需要）。
	codePermID = 18
	// qrPermID 体验版二维码额外允许的权限集 id（官方 get_qrcode 标注 18、86）。
	qrPermID = 86

	// concurrencyWindow SetConcurrentLimit 开启后，重复调用的判定窗口。
	// 说明：真实微信的 9402202 是「上一次请求尚未结束」，本模拟器无法长期挂住请求，
	// 因此把「未结束」定义为该窗口内（默认 30ms），窗口内同一 appid 的 commit/submit_audit 会返回 9402202。
	concurrencyWindow = 30 * time.Millisecond

	// defaultQuotaRest / defaultQuotaLimit 提审额度默认值（服务商级、所有小程序共用）。
	defaultQuotaRest    = 50
	defaultQuotaLimit   = 50
	defaultSpeedupRest  = 5
	defaultSpeedupLimit = 5

	// defaultSupportVersion 最低基础库默认版本。
	defaultSupportVersion = "2.20.1"
)

// Options 模拟器配置。
type Options struct {
	// ComponentAppID 第三方平台 appid，例如 wx_mock_component。留空取 wx_mock_component。
	// 注意：它同时是推送报文加密时的 receiveid（授权事件接收 URL 口径），
	// 因此上层应当传入与自身配置一致的 component appid。
	ComponentAppID string
	// VerifyToken 消息校验 Token，留空取 mock_verify_token。
	VerifyToken string
	// EncodingAESKey 43 字符的消息加解密 Key，留空取内置默认值。
	EncodingAESKey string
}

// Call 一次模拟接口调用记录（测试断言用）。
type Call struct {
	// Endpoint 请求路径，例如 "/wxa/commit"。
	Endpoint string
	// Method HTTP 方法。
	Method string
	// Token query 中的 access_token（兼容 component_access_token 写法）。
	Token string
	// Body 解析后的 JSON body；非 JSON 请求体为 nil。
	Body map[string]any
	// At 调用发生时间。
	At time.Time
}

// tokenKind 令牌归属。
type tokenKind int

const (
	// kindComponent 第三方平台自身令牌。
	kindComponent tokenKind = iota
	// kindAuthorizer 授权方令牌。
	kindAuthorizer
)

// tokenRec 模拟微信签发的 access_token 记录。
type tokenRec struct {
	kind     tokenKind
	appid    string // kindAuthorizer 时为授权方 appid
	expireAt time.Time
}

// authCodeRec 授权码（商家扫码回调 / 授权事件推送中携带）。
type authCodeRec struct {
	appid    string
	expireAt time.Time
}

// auditRec 一次代码审核单。
type auditRec struct {
	id          int64
	status      int // 0 成功 / 1 驳回 / 2 审核中 / 3 已撤回 / 4 审核延后
	reason      string
	screenshot  string
	userVersion string
	userDesc    string
	submitTime  int64
	speedup     bool
	released    bool
}

// historyVersion 可回退的历史线上版本。
type historyVersion struct {
	appVersion  int64
	userVersion string
	userDesc    string
	commitTime  int64
}

// grayPlan 分阶段（灰度）发布计划。任务书未要求灰度能力，这里补齐是为了让上层若实现了
// 灰度发布也不必因 404 中断链路。
type grayPlan struct {
	status                  int
	createTimestamp         int64
	grayPercentage          int
	supportDebugerFirst     bool
	supportExperiencerFirst bool
}

// draftRec 草稿箱记录（由开发者工具上传产生，模拟器预置若干条）。
type draftRec struct {
	id          int64
	createTime  int64
	userVersion string
	userDesc    string
	sourceAppID string
	sourceName  string
	developer   string
}

// templateRec 模板库记录。
type templateRec struct {
	id           int64
	draftID      int64
	createTime   int64
	userVersion  string
	userDesc     string
	templateType int
	sourceAppID  string
	sourceName   string
}

// privacySetting 小程序用户隐私保护指引。
type privacySetting struct {
	codeExist    int
	privacyList  []string
	settingList  []any
	ownerSetting map[string]any
	updateTime   int64
}

// quotaRec 服务商级提审与加急额度（所有小程序共用）。
type quotaRec struct {
	Rest         int
	Limit        int
	SpeedupRest  int
	SpeedupLimit int
}

// authorizer 模拟的授权方（小程序）全部状态。
type authorizer struct {
	appid         string
	nickName      string
	userName      string // gh_ 开头的原始 ID
	alias         string
	principalName string
	headImg       string
	qrcodeURL     string
	funcInfoIDs   []int
	authorized    bool
	authTime      int64
	refreshToken  string
	tokenSeq      int

	// 代码与版本
	expVersion    string
	expDesc       string
	expTime       int64
	extJSON       string
	privacyDoneAt time.Time

	audits        map[int64]*auditRec
	auditSeq      int64
	latestAuditID int64

	releaseVersion    string
	releaseDesc       string
	releaseTime       int64
	releaseAppVersion int64
	versionSeq        int64
	history           []historyVersion
	gray              *grayPlan

	// 配置类状态
	visitStatus     int
	requestDomain   []string
	wsRequestDomain []string
	uploadDomain    []string
	downloadDomain  []string
	webviewDomains  []string
	supportVersion  string
	options         map[string]string

	// 撤回审核额度（每账号每天 5 次、每月 10 次）
	undoDay        string
	undoDayCount   int
	undoMonth      string
	undoMonthCount int
}

// Server 模拟微信开放平台。
type Server struct {
	opts Options
	// componentRawID 第三方平台的 gh_ 原始 ID（由 component appid 派生），用作推送包体的 ToUserName。
	componentRawID string
	crypt          *wxcrypt.WXBizMsgCrypt
	// cryptErr New 时构造加解密器失败的原因（例如 EncodingAESKey 不是 43 字符）。
	// 保留下来而不是 panic，是为了让配置错误以「推送方法返回 error」的形式被测试发现。
	cryptErr error

	routes map[string]endpoint

	mu sync.Mutex

	// 票据：ticketSet 保存所有推送过的 ticket（便于上层用较早的 ticket 兜底，官方明确允许）。
	ticketSet  map[string]time.Time
	lastTicket string
	ticketSeq  int

	// 令牌注册表：token -> 记录。
	tokens map[string]*tokenRec

	// 预授权码 / 授权码
	preAuthCodes map[string]time.Time
	authCodes    map[string]authCodeRec
	authCodeSeq  int

	// 授权方
	auths     map[string]*authorizer
	authOrder []string
	appSeq    int
	probeSeq  int

	// 提审素材编号（/wxa/uploadmedia）
	mediaSeq int

	// 模板库与草稿箱
	drafts         map[int64]*draftRec
	templates      map[int64]*templateRec
	nextTemplateID int64

	// 隐私设置（按 appid）
	privacySettings map[string]*privacySetting

	// 额度
	quota quotaRec

	// 调用记录
	calls []Call

	// 故障注入与行为开关
	forced          map[string]int
	privacyDelay    time.Duration
	concurrentLimit bool
	inflight        map[string]time.Time
	// concurrentWindow 见 concurrencyWindow 常量；测试可直接改小/改大以获得确定性。
	concurrentWindow time.Duration
}

// New 构造模拟器。opts 中的空字段会退化为内置默认值。
func New(opts Options) *Server {
	if opts.ComponentAppID == "" {
		opts.ComponentAppID = defaultComponentAppID
	}
	if opts.VerifyToken == "" {
		opts.VerifyToken = defaultVerifyToken
	}
	if opts.EncodingAESKey == "" {
		opts.EncodingAESKey = defaultEncodingAESKey
	}
	s := &Server{
		opts:             opts,
		ticketSet:        make(map[string]time.Time),
		tokens:           make(map[string]*tokenRec),
		preAuthCodes:     make(map[string]time.Time),
		authCodes:        make(map[string]authCodeRec),
		auths:            make(map[string]*authorizer),
		drafts:           make(map[int64]*draftRec),
		templates:        make(map[int64]*templateRec),
		privacySettings:  make(map[string]*privacySetting),
		quota:            quotaRec{Rest: defaultQuotaRest, Limit: defaultQuotaLimit, SpeedupRest: defaultSpeedupRest, SpeedupLimit: defaultSpeedupLimit},
		forced:           make(map[string]int),
		inflight:         make(map[string]time.Time),
		concurrentWindow: concurrencyWindow,
	}
	s.componentRawID = rawIDFor(opts.ComponentAppID)
	s.crypt, s.cryptErr = wxcrypt.New(opts.VerifyToken, opts.EncodingAESKey, opts.ComponentAppID, "")
	s.seedDrafts()
	s.routes = s.buildRoutes()
	return s
}

// Handler 返回可挂到 httptest.NewServer 的处理器；所有经过它的调用都会记入 Calls()。
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serveHTTP)
}

// endpointKind 端点要求的令牌类型。
type endpointKind int

const (
	// tokenNone 无需 access_token（换令牌类接口）。
	tokenNone endpointKind = iota
	// tokenComponent 必须是 component_access_token。
	tokenComponent
	// tokenAuthorizer 必须是 authorizer_access_token。
	tokenAuthorizer
	// tokenAny 两种令牌都接受：官方文档对少数接口的 token 口径自相矛盾
	// （例如 get/set_authorizer_option、类目、域名、服务状态、基础库等），
	// 为了不误伤上层实现，这类端点同时接受两类令牌，且不校验权限集。
	tokenAny
)

// endpoint 一条路由定义。
type endpoint struct {
	// method 要求的 HTTP 方法，方法不符返回 43001/43002（官方「require GET/POST method」）。
	method string
	kind   endpointKind
	// perms 允许的权限集 id 列表；仅当使用授权方令牌时校验，任一命中即通过，全不命中返回 48001。
	perms []int
	// handle 业务处理。
	handle func(rc *request)
}

// request 一次请求的上下文。
type request struct {
	srv   *Server
	w     http.ResponseWriter
	r     *http.Request
	path  string
	token string
	raw   []byte
	body  map[string]any
	kind  tokenKind
	auth  *authorizer
}

// serveHTTP 统一入口：记录调用 → 故障注入 → 路由 → 令牌校验 → 业务处理。
func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	body := parseJSONObject(raw)
	query := r.URL.Query()
	token := query.Get("access_token")
	if token == "" {
		// 旧版文档把查询参数写成 component_access_token，微信两个名字都接受。
		token = query.Get("component_access_token")
	}
	s.recordCall(Call{Endpoint: r.URL.Path, Method: r.Method, Token: token, Body: body, At: time.Now()})

	// 故障注入优先级最高：命中即立刻返回，并且已经记入 Calls()。
	s.mu.Lock()
	forced := s.forced[r.URL.Path]
	s.mu.Unlock()
	if forced != 0 {
		s.reply(w, forced, "mock forced error")
		return
	}

	ep, ok := s.routes[r.URL.Path]
	if !ok {
		s.reply(w, 404, "mock: unknown endpoint "+r.URL.Path)
		return
	}
	if ep.method != "" && ep.method != r.Method {
		if ep.method == http.MethodGet {
			s.reply(w, 43001, "require GET method")
		} else {
			s.reply(w, 43002, "require POST method")
		}
		return
	}

	rc := &request{srv: s, w: w, r: r, path: r.URL.Path, token: token, raw: raw, body: body}
	if s.authenticate(rc, ep) {
		ep.handle(rc)
	}
}

// authenticate 校验 access_token（存在性 / 有效性 / 过期 / 归属 / 权限集）。
//
// 返回 false 时已经写出错误响应。错误码口径：
//   - 41001 access_token missing：query 里没带；
//   - 40001 invalid credential：带了但模拟器不认识的令牌（含已被「取消授权」作废的令牌）；
//   - 42001 access_token expired：签发过但已过期；
//   - 61014 must use component token for component api：component 类接口用了授权方令牌；
//   - 48001 api unauthorized：授权方没有对应权限集（例如代码管理需要 18）。
func (s *Server) authenticate(rc *request, ep endpoint) bool {
	if ep.kind == tokenNone {
		return true
	}
	if rc.token == "" {
		rc.fail(41001, "access_token missing")
		return false
	}

	now := time.Now()
	s.mu.Lock()
	rec := s.tokens[rc.token]
	var auth *authorizer
	if rec != nil && rec.kind == kindAuthorizer {
		auth = s.auths[rec.appid]
	}
	s.mu.Unlock()

	if rec == nil {
		rc.fail(40001, "invalid credential, access_token is invalid")
		return false
	}
	if now.After(rec.expireAt) {
		rc.fail(42001, "access_token expired")
		return false
	}
	rc.kind = rec.kind
	rc.auth = auth

	switch ep.kind {
	case tokenComponent:
		if rec.kind != kindComponent {
			rc.fail(61014, "must use component token for component api")
			return false
		}
	case tokenAuthorizer:
		if rec.kind != kindAuthorizer {
			rc.fail(40001, "invalid credential, access_token is invalid: /wxa/* 需要 authorizer_access_token")
			return false
		}
	case tokenAny:
		// 见 tokenAny 注释：两类令牌都接受。
	}

	if rec.kind == kindAuthorizer {
		if auth == nil || !auth.authorized {
			rc.fail(40001, "invalid credential, access_token is invalid: 该小程序已取消授权")
			return false
		}
		if len(ep.perms) > 0 && !hasAnyPerm(auth, ep.perms) {
			rc.fail(48001, fmt.Sprintf("api unauthorized: 授权方未授权权限集 %v", ep.perms))
			return false
		}
	}
	return true
}

// hasAnyPerm 判断授权方是否命中任一权限集。
func hasAnyPerm(a *authorizer, perms []int) bool {
	for _, want := range perms {
		for _, got := range a.funcInfoIDs {
			if got == want {
				return true
			}
		}
	}
	return false
}

// kv JSON 响应字段（按传入顺序输出，便于人工比对官方示例）。
type kv struct {
	key string
	val any
}

// field 构造一个响应字段。
func field(key string, val any) kv { return kv{key: key, val: val} }

// reply 输出统一 JSON 响应：errcode / errmsg 固定在前，其余字段按调用方顺序追加。
func (s *Server) reply(w http.ResponseWriter, errcode int, errmsg string, fields ...kv) {
	var buf bytes.Buffer
	buf.WriteString(`{"errcode":`)
	writeJSONValue(&buf, errcode)
	buf.WriteString(`,"errmsg":`)
	writeJSONValue(&buf, errmsg)
	for _, f := range fields {
		buf.WriteString(`,"`)
		buf.WriteString(f.key)
		buf.WriteString(`":`)
		writeJSONValue(&buf, f.val)
	}
	buf.WriteString("}")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// writeJSONValue 序列化单个 JSON 值，失败时退化为 null。
func writeJSONValue(buf *bytes.Buffer, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		buf.WriteString("null")
		return
	}
	buf.Write(b)
}

// fail 输出错误响应。
func (rc *request) fail(errcode int, errmsg string) {
	rc.srv.reply(rc.w, errcode, errmsg)
}

// ok 输出成功响应。
func (rc *request) ok(fields ...kv) {
	rc.srv.reply(rc.w, 0, "ok", fields...)
}

// str 读取字符串字段，缺失或类型不符返回 ""。
func (rc *request) str(key string) string {
	if rc.body == nil {
		return ""
	}
	if v, ok := rc.body[key].(string); ok {
		return v
	}
	return ""
}

// num 读取数字字段。
func (rc *request) num(key string) (int64, bool) {
	if rc.body == nil {
		return 0, false
	}
	switch v := rc.body[key].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case json.Number:
		n, err := v.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

// strSlice 读取字符串数组字段，不存在返回 nil。
func (rc *request) strSlice(key string) []string {
	if rc.body == nil {
		return nil
	}
	raw, ok := rc.body[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// requireObject 校验请求体是一个 JSON 对象。
//
// 官方对 /wxa/release、/wxa/getversioninfo、/wxa/getvisitstatus、/cgi-bin/wxopen/getweappsupportversion
// 明确要求「post 的 data 为空，不等于不需要传 data」，缺失即 44002 empty post data；
// wantEmpty 为 true 时（/wxa/get_effective_domain、/wxa/get_effective_webviewdomain）还要求对象必须为空 {}。
func (rc *request) requireObject(wantEmpty bool) bool {
	if len(bytes.TrimSpace(rc.raw)) == 0 {
		rc.fail(44002, "empty post data")
		return false
	}
	if rc.body == nil {
		rc.fail(47001, "data format error: body must be a JSON object")
		return false
	}
	if wantEmpty && len(rc.body) != 0 {
		rc.fail(44002, "invalid post data: expect {}")
		return false
	}
	return true
}

// resolveAppid 解析本次调用指向哪个小程序。
//
// 优先用 authorizer_access_token 归属；否则（tokenAny 端点被 component 令牌调用时）依次尝试
// body 里的 appid / authorizer_appid，最后在「只登记了一个小程序」时兜底，便于上层按
// 「第三方平台自身接口」的口径调用类目 / 域名 / 服务状态等接口。
func (rc *request) resolveAppid() (*authorizer, bool) {
	if rc.auth != nil {
		return rc.auth, true
	}
	s := rc.srv
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range []string{"appid", "authorizer_appid"} {
		if id := rc.str(key); id != "" {
			if a, ok := s.auths[id]; ok {
				return a, true
			}
		}
	}
	if len(s.authOrder) == 1 {
		return s.auths[s.authOrder[0]], true
	}
	rc.fail(40013, "invalid appid: 请使用 authorizer_access_token 或在 body 中提供 appid")
	return nil, false
}

// parseJSONObject 尽力把请求体解析为 JSON 对象；非 JSON 或非对象返回 nil。
func parseJSONObject(raw []byte) map[string]any {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// recordCall 追加一条调用记录。
func (s *Server) recordCall(c Call) {
	s.mu.Lock()
	s.calls = append(s.calls, c)
	s.mu.Unlock()
}

// Calls 返回全部调用记录的快照（按发生顺序）。Body 为浅拷贝，仅供断言读取。
func (s *Server) Calls() []Call {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Call, len(s.calls))
	copy(out, s.calls)
	return out
}

// ResetCalls 清空调用记录。
func (s *Server) ResetCalls() {
	s.mu.Lock()
	s.calls = nil
	s.mu.Unlock()
}

// ForceErrcode 让某个接口固定返回指定微信返回码（endpoint 形如 "/wxa/commit"），0 表示取消。
func (s *Server) ForceErrcode(endpoint string, errcode int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if errcode == 0 {
		delete(s.forced, endpoint)
		return
	}
	s.forced[endpoint] = errcode
}

// SetPrivacyCheckDelay 设置 commit 之后隐私检测任务需要多久才算完成（默认 0，即立即可查）。
// 在延迟期内 GetCodePrivacyInfo 返回 61039（官方语义：检查任务未完成，请稍等一分钟再重试）。
func (s *Server) SetPrivacyCheckDelay(d time.Duration) {
	s.mu.Lock()
	s.privacyDelay = d
	s.mu.Unlock()
}

// SetConcurrentLimit 开启后，同一 appid 的 commit/submit_audit 若在上一次未结束前重复调用则返回 9402202。
// 「未结束」以内部并发窗口（默认 30ms）衡量，窗口内的重复调用即视为并发。
func (s *Server) SetConcurrentLimit(on bool) {
	s.mu.Lock()
	s.concurrentLimit = on
	s.mu.Unlock()
}

// SetQuota 设置提审与加急额度（服务商级、所有小程序共用）。
func (s *Server) SetQuota(rest, limit, speedupRest, speedupLimit int) {
	s.mu.Lock()
	s.quota = quotaRec{Rest: rest, Limit: limit, SpeedupRest: speedupRest, SpeedupLimit: speedupLimit}
	s.mu.Unlock()
}

// AddAuthorizer 登记一个「已授权」小程序，返回其 appid（形如 wx_mock_app_1）。
// funcInfoIDs 是授权给第三方平台的权限集 id 列表（代码管理需要 18，留空则代码类接口会返回 48001）。
func (s *Server) AddAuthorizer(nickName string, funcInfoIDs []int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appSeq++
	appid := fmt.Sprintf("wx_mock_app_%d", s.appSeq)
	a := s.newAuthorizerLocked(appid, nickName, funcInfoIDs)
	a.authorized = true
	a.authTime = time.Now().Unix()
	a.refreshToken = fmt.Sprintf("mock_refresh_%s", appid)
	s.auths[appid] = a
	s.authOrder = append(s.authOrder, appid)
	return appid
}

// CreateAuthCode 为一个已登记/待授权的小程序生成授权码。
//
// 模拟商家扫码授权后回调携带的 auth_code，以及授权事件推送里的 AuthorizationCode；
// 该授权码可用于 api_query_auth 换取 authorizer_access_token + authorizer_refresh_token。
// appid 尚未登记时会自动登记为「待授权」（默认授予权限集 18）。
func (s *Server) CreateAuthCode(appid string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureAuthorizerLocked(appid)
	s.authCodeSeq++
	code := fmt.Sprintf("mock_auth_code_%d_%s", s.authCodeSeq, appid)
	s.authCodes[code] = authCodeRec{appid: appid, expireAt: time.Now().Add(authCodeTTL)}
	return code
}

// ExtJSONOf 返回某小程序最近一次 commit 的 ext_json 原文。
func (s *Server) ExtJSONOf(appid string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.auths[appid]; ok {
		return a.extJSON
	}
	return ""
}

// AuditStatusOf 返回某小程序最近一次审核单的 auditid 与 status；ok 为 false 表示还没有提审过。
func (s *Server) AuditStatusOf(appid string) (auditID int64, status int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, found := s.auths[appid]
	if !found || a.latestAuditID == 0 {
		return 0, 0, false
	}
	rec, found := a.audits[a.latestAuditID]
	if !found {
		return 0, 0, false
	}
	return rec.id, rec.status, true
}

// ReleasedVersionOf 返回某小程序当前线上版本号；ok 为 false 表示还没发布过。
func (s *Server) ReleasedVersionOf(appid string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.auths[appid]
	if !ok || a.releaseVersion == "" {
		return "", false
	}
	return a.releaseVersion, true
}

// newAuthorizerLocked 构造授权方记录（调用方必须已持有 s.mu）。
func (s *Server) newAuthorizerLocked(appid, nickName string, funcInfoIDs []int) *authorizer {
	s.probeSeq++
	if nickName == "" {
		nickName = fmt.Sprintf("模拟小程序%d", s.probeSeq)
	}
	funcs := make([]int, 0, len(funcInfoIDs))
	funcs = append(funcs, funcInfoIDs...)
	return &authorizer{
		appid:           appid,
		nickName:        nickName,
		userName:        fmt.Sprintf("gh_%012x", uint64(0xfb9688c2a4b0)+uint64(s.probeSeq)),
		alias:           strings.ReplaceAll(appid, "_", "-"),
		principalName:   nickName + "（模拟主体）",
		headImg:         "https://mmbiz.qpic.cn/mock/head_" + appid + ".png",
		qrcodeURL:       "https://mp.weixin.qq.com/mock/qrcode/" + appid + ".png",
		funcInfoIDs:     funcs,
		audits:          make(map[int64]*auditRec),
		visitStatus:     1,
		supportVersion:  defaultSupportVersion,
		options:         map[string]string{"location_report": "0", "voice_recognize": "0", "customer_service": "0"},
		webviewDomains:  []string{},
		requestDomain:   []string{},
		wsRequestDomain: []string{},
		uploadDomain:    []string{},
		downloadDomain:  []string{},
	}
}

// ensureAuthorizerLocked 返回 appid 对应的授权方记录，不存在时登记为「待授权」（默认权限集 18）。
// 调用方必须已持有 s.mu。
func (s *Server) ensureAuthorizerLocked(appid string) *authorizer {
	if a, ok := s.auths[appid]; ok {
		return a
	}
	a := s.newAuthorizerLocked(appid, "", []int{codePermID})
	a.authorized = false
	s.auths[appid] = a
	s.authOrder = append(s.authOrder, appid)
	return a
}

// seedDrafts 预置草稿箱数据，让「草稿 → 模板」链路无需真实开发者工具即可走通。
func (s *Server) seedDrafts() {
	now := time.Now().Unix()
	s.drafts[1] = &draftRec{id: 1, createTime: now - 3600, userVersion: "1.0.0", userDesc: "模拟草稿：首发版本",
		sourceAppID: "wx_mock_dev_1", sourceName: "模拟开发小程序", developer: "mock"}
	s.drafts[2] = &draftRec{id: 2, createTime: now - 1800, userVersion: "1.0.1", userDesc: "模拟草稿：修复版本",
		sourceAppID: "wx_mock_dev_1", sourceName: "模拟开发小程序", developer: "mock"}
	s.nextTemplateID = 0
}

// issueTokenLocked 签发一个 access_token（调用方必须已持有 s.mu）。
func (s *Server) issueTokenLocked(kind tokenKind, appid string) (string, time.Time) {
	var token string
	if kind == kindComponent {
		token = fmt.Sprintf("mock_component_token_%d_%d", time.Now().UnixNano(), len(s.tokens))
	} else {
		a := s.auths[appid]
		if a != nil {
			a.tokenSeq++
			token = fmt.Sprintf("mock_authorizer_token_%d_%s", a.tokenSeq, appid)
		} else {
			token = fmt.Sprintf("mock_authorizer_token_%d_%s", time.Now().UnixNano(), appid)
		}
	}
	expireAt := time.Now().Add(tokenTTL)
	s.tokens[token] = &tokenRec{kind: kind, appid: appid, expireAt: expireAt}
	return token, expireAt
}

// revokeTokensLocked 作废某授权方的全部令牌（取消授权时调用，调用方必须已持有 s.mu）。
func (s *Server) revokeTokensLocked(appid string) {
	for token, rec := range s.tokens {
		if rec.kind == kindAuthorizer && rec.appid == appid {
			delete(s.tokens, token)
		}
	}
}

// inConcurrentWindow 判断「同一 appid 的同一操作」是否落在并发窗口内（命中即应返回 9402202）。
//
// 未开启并发限制时永远返回 false。inflight 里存的是窗口截止时间，窗口自然过期后自动放行，
// 因此不需要 defer 释放，也不会阻塞请求处理。
func (rc *request) inConcurrentWindow(op string) bool {
	s := rc.srv
	if rc.auth == nil {
		return false
	}
	key := rc.auth.appid + ":" + op
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.concurrentLimit {
		return false
	}
	now := time.Now()
	if until, busy := s.inflight[key]; busy && now.Before(until) {
		return true
	}
	s.inflight[key] = now.Add(s.concurrentWindow)
	return false
}

// orEmpty 把 nil 切片转成空切片，避免响应里出现 null。
func orEmpty(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

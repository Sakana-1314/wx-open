package wxauth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"wx-platform/server/internal/config"
	"wx-platform/server/internal/core"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/secretbox"
	"wx-platform/server/internal/wxapi"
	"wx-platform/server/internal/wxtoken"
)

// 本包是集成测试：用 httptest 模拟微信开放平台接口 + 真实 core.Repos（MySQL）。
//
//	TEST_DB_DSN='root:@tcp(127.0.0.1:3306)/wx_platform_test?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test ./internal/wxauth/ -v
//
// 未设置 TEST_DB_DSN 时全部用例跳过（与 internal/repo 的集成测试约定一致）。

const (
	testComponentAppid  = "wxcomponent0001"
	testAuthorizerAppid = "wxauthorizer0001"
	testRefreshToken    = "REFRESH-TOKEN-PLAINTEXT-0001"
)

// ---- 数据准备 ----

// testDB 连接测试库并建表 + 清空（每个用例一份干净数据）。
//
// 连接候选顺序：TEST_DB_DSN → 同主机的项目默认账号（wxplatform）→ 本机 unix socket 的 root。
// 这样既能直接跑在按文档给出 TEST_DB_DSN 的机器上，也能跑在本容器这种「MariaDB 只允许
// socket 登录 root、root@tcp 一律 1698」的环境里（候选可用性由实际连接结果决定）。
// 连上之后还会尽量切到独立库（<原库>_wxauth），避免与 internal/repo 的集成测试并行执行时互相清表。
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DB_DSN"))
	if dsn == "" {
		t.Skip("未设置 TEST_DB_DSN，跳过 wxauth 集成测试（示例：TEST_DB_DSN='root:@tcp(127.0.0.1:3306)/wx_platform_test?charset=utf8mb4&parseTime=True&loc=Local'）")
	}
	db, used := openWorkingDB(t, dsn)
	if strings.Contains(used, "_wxauth") {
		t.Logf("wxauth 集成测试使用数据库：%s", maskDSN(used))
	} else {
		t.Logf("wxauth 集成测试使用数据库：%s（本账号无建库权限，未使用独立库：请与其它包的集成测试串行执行，避免互相清表）", maskDSN(used))
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	models := []any{
		&model.PlatformState{},
		&model.Authorizer{},
		&model.TokenCache{},
		&model.CodeDraft{},
		&model.CodeTemplate{},
		&model.AuditProfile{},
		&model.OperationLog{},
		&model.ApiCallLog{},
		&model.CallbackEvent{},
		&model.RuntimeSetting{},
	}
	if err := db.AutoMigrate(models...); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	for _, m := range models {
		if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(m).Error; err != nil {
			t.Fatalf("清空测试表失败: %v", err)
		}
	}
	return db
}

// dsnPattern 拆分 DSN：前缀（含 "@tcp(host:port)/" 或 "@unix(sock)/"）、库名、参数。
var dsnPattern = regexp.MustCompile(`^([^?]*\)/)([^?]*)(\?.*)?$`)

// dsnCredPattern 拆分 DSN 的账号密码部分。
var dsnCredPattern = regexp.MustCompile(`^([^:@/]*)(?::([^@]*))?@(.+)$`)

// openWorkingDB 依次尝试候选 DSN：
// 优先选「能创建独立测试库」的连接（这样与其它包的集成测试并行时不会互相清表），
// 都不行时退回第一个能连上的连接。
func openWorkingDB(t *testing.T, dsn string) (*gorm.DB, string) {
	t.Helper()
	candidates := []string{dsn}
	for _, cred := range [][2]string{{"wxplatform", "wxplatform_dev_password"}, {"root", ""}} {
		if alt, ok := dsnWithCredential(dsn, cred[0], cred[1]); ok {
			candidates = append(candidates, alt)
		}
	}
	if name, ok := dsnDBName(dsn); ok {
		candidates = append(candidates, "root:@unix(/run/mysqld/mysqld.sock)/"+name+"?charset=utf8mb4&parseTime=True&loc=Local")
	}

	failures := make([]string, 0, len(candidates))
	var fallback *gorm.DB
	var fallbackDSN string
	for _, candidate := range candidates {
		db, err := openTestConn(candidate)
		if err != nil {
			failures = append(failures, maskDSN(candidate)+": "+err.Error())
			continue
		}
		if isolated, ok := isolatedDSN(t, db, candidate); ok {
			closeConn(db)
			next, err := openTestConn(isolated)
			if err != nil {
				failures = append(failures, maskDSN(isolated)+": "+err.Error())
				continue
			}
			if fallback != nil {
				closeConn(fallback)
			}
			return next, isolated
		}
		if fallback == nil {
			fallback, fallbackDSN = db, candidate
			continue
		}
		closeConn(db)
	}
	if fallback != nil {
		return fallback, fallbackDSN
	}
	t.Fatalf("连接测试库失败（已尝试 %d 个候选）：%s", len(candidates), strings.Join(failures, "；"))
	return nil, ""
}

// openTestConn 建立一个可用的测试库连接。
func openTestConn(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// closeConn 关闭连接池。
func closeConn(db *gorm.DB) {
	if db == nil {
		return
	}
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// isolatedDSN 在测试实例上创建独立库并返回其 DSN；无权限或无法解析时返回 false（沿用原库）。
func isolatedDSN(t *testing.T, db *gorm.DB, dsn string) (string, bool) {
	t.Helper()
	parts := dsnPattern.FindStringSubmatch(dsn)
	if len(parts) != 4 || parts[2] == "" {
		return "", false
	}
	isolated := parts[2] + "_wxauth"
	if err := db.Exec("CREATE DATABASE IF NOT EXISTS `" + isolated + "` CHARACTER SET utf8mb4").Error; err != nil {
		return "", false
	}
	return parts[1] + isolated + parts[3], true
}

// dsnWithCredential 替换 DSN 里的账号密码；已是该账号时返回 false。
func dsnWithCredential(dsn, user, password string) (string, bool) {
	m := dsnCredPattern.FindStringSubmatch(dsn)
	if m == nil {
		return "", false
	}
	currentUser, currentPassword := m[1], m[2]
	if currentUser == user && currentPassword == password {
		return "", false
	}
	credential := user
	if password != "" {
		credential += ":" + password
	}
	return credential + "@" + m[3], true
}

// dsnDBName 取出 DSN 里的库名。
func dsnDBName(dsn string) (string, bool) {
	parts := dsnPattern.FindStringSubmatch(dsn)
	if len(parts) != 4 || parts[2] == "" {
		return "", false
	}
	return parts[2], true
}

// maskDSN 打印 DSN 时隐藏口令。
func maskDSN(dsn string) string {
	m := dsnCredPattern.FindStringSubmatch(dsn)
	if m == nil || m[2] == "" {
		return dsn
	}
	return m[1] + ":***@" + m[3]
}

// testEnv 一套真实依赖：模拟微信服务端 + MySQL + 真实令牌管理器。
type testEnv struct {
	t   *testing.T
	db  *gorm.DB
	env *core.Env
	box *secretbox.Box
	wx  *mockWX
	cfg *config.Config
}

// newTestEnv 构造默认环境（第三方平台凭据齐全，令牌接口已就绪）。
func newTestEnv(t *testing.T) *testEnv { return newTestEnvWithConfig(t, nil) }

// newTestEnvWithConfig 构造环境，允许用例改写配置（例如清空 ComponentAppID 验证凭据门禁）。
func newTestEnvWithConfig(t *testing.T, mutate func(*config.Config)) *testEnv {
	t.Helper()
	wx := newMockWX(t)
	db := testDB(t)
	box, err := secretbox.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("构造加密组件失败: %v", err)
	}
	cfg := &config.Config{
		ComponentAppID:     testComponentAppid,
		ComponentAppSecret: "component-secret",
		PublicBaseURL:      "https://platform.example.com",
		WxAPIBase:          wx.server.URL,
		WxRequestTimeout:   5 * time.Second,
		WxMaxQps:           -1,
	}
	if mutate != nil {
		mutate(cfg)
	}
	env := core.NewEnv(db, cfg, box)
	client := wxapi.New(wxapi.Options{BaseURL: wx.server.URL, Timeout: 5 * time.Second, MaxQPS: -1})
	store := core.NewTokenStore(env.Repos)
	tokens := wxtoken.New(client, store, store, box, cfg.ComponentAppID, cfg.ComponentAppSecret)
	if err := tokens.SaveTicket(context.Background(), "ticket-for-test", time.Now()); err != nil {
		t.Fatalf("保存 component_verify_ticket 失败: %v", err)
	}
	env.Wx = client
	env.Tokens = tokens
	return &testEnv{t: t, db: db, env: env, box: box, wx: wx, cfg: cfg}
}

// seedAuthorizer 造一条「已授权」记录（refresh_token 用真实 secretbox 加密入库）。
func (te *testEnv) seedAuthorizer(appid, refreshToken string, funcIDs []int) *model.Authorizer {
	te.t.Helper()
	if refreshToken == "" {
		refreshToken = testRefreshToken
	}
	cipher, err := te.box.Seal(refreshToken)
	if err != nil {
		te.t.Fatalf("加密 refresh_token 失败: %v", err)
	}
	if funcIDs == nil {
		funcIDs = []int{model.PermissionSetDev}
	}
	now := time.Now()
	row := &model.Authorizer{
		Appid:                 appid,
		NickName:              "测试小程序-" + appid,
		AuthorizationStatus:   model.AuthStatusAuthorized,
		AuthorizedAt:          &now,
		RefreshTokenCipher:    string(cipher),
		RefreshTokenUpdatedAt: &now,
		CodeSource:            model.CodeSourceTemplate,
		Enabled:               true,
		FuncInfo:              model.IntSlice(funcIDs),
	}
	if err := te.env.Repos.Authorizers.Upsert(context.Background(), row); err != nil {
		te.t.Fatalf("写入授权方记录失败: %v", err)
	}
	return row
}

// authorizerRow 读库里的授权方记录（读不到直接失败）。
func (te *testEnv) authorizerRow(appid string) *model.Authorizer {
	te.t.Helper()
	row, err := te.env.Repos.Authorizers.Get(context.Background(), appid)
	if err != nil {
		te.t.Fatalf("读取授权方记录失败: %v", err)
	}
	return row
}

// authorizerCount 统计某 appid 的记录数（含软删，用于断言幂等不重复建记录）。
func (te *testEnv) authorizerCount(appid string) int64 {
	te.t.Helper()
	var n int64
	if err := te.db.Model(&model.Authorizer{}).Where("appid = ?", appid).Count(&n).Error; err != nil {
		te.t.Fatalf("统计授权方记录失败: %v", err)
	}
	return n
}

// operationActions 返回审计日志里的动作名（倒序无关，仅用于断言包含关系）。
func (te *testEnv) operationActions() []string {
	te.t.Helper()
	rows := make([]model.OperationLog, 0)
	if err := te.db.Order("id ASC").Find(&rows).Error; err != nil {
		te.t.Fatalf("查询操作日志失败: %v", err)
	}
	out := make([]string, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].Action)
	}
	return out
}

// requireAction 断言审计日志里出现过某个动作。
func (te *testEnv) requireAction(action string) {
	te.t.Helper()
	for _, a := range te.operationActions() {
		if a == action {
			return
		}
	}
	te.t.Fatalf("审计日志里没有动作 %s（实际：%v）", action, te.operationActions())
}

// ---- 模拟微信服务端 ----

// recordedCall 一次被模拟服务端收到的调用。
type recordedCall struct {
	Method string
	Path   string
	Query  url.Values
	Body   map[string]any
}

// wxHandler 模拟接口的处理函数（收到解析好的请求体）。
type wxHandler func(w http.ResponseWriter, call recordedCall)

// mockWX httptest 版微信开放平台。
type mockWX struct {
	server *httptest.Server
	mu     sync.Mutex
	routes map[string]wxHandler
	calls  []recordedCall
}

// newMockWX 构造模拟服务端并预置令牌相关接口（几乎所有用例都依赖它们）。
func newMockWX(t *testing.T) *mockWX {
	t.Helper()
	m := &mockWX{routes: map[string]wxHandler{}}
	m.server = httptest.NewServer(http.HandlerFunc(m.serve))
	t.Cleanup(m.server.Close)
	m.jsonRoute("/cgi-bin/component/api_component_token", map[string]any{
		"component_access_token": "COMPONENT-ACCESS-TOKEN",
		"expires_in":             7200,
	})
	m.jsonRoute("/cgi-bin/component/api_authorizer_token", map[string]any{
		"authorizer_access_token": "AUTHORIZER-ACCESS-TOKEN",
		"expires_in":              7200,
	})
	return m
}

// serve 记录调用并分发到注册的处理器；未注册的接口按「成功且无字段」返回。
func (m *mockWX) serve(w http.ResponseWriter, r *http.Request) {
	call := recordedCall{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Body: map[string]any{}}
	if raw, err := io.ReadAll(r.Body); err == nil && len(bytes.TrimSpace(raw)) > 0 {
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err == nil {
			call.Body = body
		}
	}
	m.mu.Lock()
	m.calls = append(m.calls, call)
	handler := m.routes[r.URL.Path]
	m.mu.Unlock()
	if handler == nil {
		writeMockJSON(w, map[string]any{"errcode": 0, "errmsg": "ok"})
		return
	}
	handler(w, call)
}

// route 注册接口处理器。
func (m *mockWX) route(path string, handler wxHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes[path] = handler
}

// jsonRoute 注册返回固定 JSON 的接口。
func (m *mockWX) jsonRoute(path string, payload any) {
	m.route(path, func(w http.ResponseWriter, _ recordedCall) { writeMockJSON(w, payload) })
}

// failRoute 注册固定失败的接口（errcode != 0）。
func (m *mockWX) failRoute(path string, errcode int, errmsg string) {
	m.route(path, func(w http.ResponseWriter, _ recordedCall) {
		writeMockJSON(w, map[string]any{"errcode": errcode, "errmsg": errmsg})
	})
}

// callsTo 返回某个接口被调用的情况（按调用先后）。
func (m *mockWX) callsTo(path string) []recordedCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]recordedCall, 0, len(m.calls))
	for _, c := range m.calls {
		if c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

// countTo 某个接口被调用的次数。
func (m *mockWX) countTo(path string) int { return len(m.callsTo(path)) }

// lastCallTo 某个接口的最后一次调用（没有则为零值）。
func (m *mockWX) lastCallTo(path string) recordedCall {
	calls := m.callsTo(path)
	if len(calls) == 0 {
		return recordedCall{Body: map[string]any{}}
	}
	return calls[len(calls)-1]
}

// pathSequence 返回被调用接口的顺序（断言「先登记再配置」这类顺序用）。
func (m *mockWX) pathSequence() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.calls))
	for _, c := range m.calls {
		out = append(out, c.Path)
	}
	return out
}

// indexOfPath 返回某个接口第一次出现的下标（未出现返回 -1）。
func indexOfPath(seq []string, path string) int {
	for i, p := range seq {
		if p == path {
			return i
		}
	}
	return -1
}

// writeMockJSON 输出 JSON 响应。
func writeMockJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(payload)
}

// int64InBody 读取请求体里的数字字段（JSON 数字统一是 float64）。
func int64InBody(call recordedCall, key string) int64 {
	if v, ok := call.Body[key].(float64); ok {
		return int64(v)
	}
	return 0
}

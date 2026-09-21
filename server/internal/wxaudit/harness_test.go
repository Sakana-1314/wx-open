// Package wxaudit_test 是 internal/wxaudit 的集成测试。
//
// 约定（与 internal/repo 的集成测试一致）：
//   - 数据访问用**真实 MySQL**（TEST_DB_DSN，未设置时整包跳过）；
//   - 微信侧一律用 httptest.NewServer + wxapi.New(Options{BaseURL})，**不碰真实微信**；
//   - 用例自己注册需要的接口，未注册的接口返回 404：避免「忘了注册」被当成微信业务失败。
//
// 运行：
//
//	TEST_DB_DSN='root:@tcp(127.0.0.1:3306)/wx_platform_test?charset=utf8mb4&parseTime=True&loc=Local' \
//	    go test ./internal/wxaudit/ -v
//
// ⚠️ 共享测试库的并发风险：本仓库各测试包（repo / wxauth / wxjob / wxaudit …）都用同一个
// TEST_DB_DSN，且每个用例都会清表。`go test ./...` 默认**并行跑不同包**，会出现「A 包清表、
// B 包读到空数据」的互相破坏。本包已尽量降低依赖（令牌层走内存、用例只清自己用到的表、
// 关键步骤前重新写回前置数据），但仍建议集成测试用串行方式跑：
//
//	TEST_DB_DSN=... go test -p 1 ./...
package wxaudit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"wx-platform/server/internal/config"
	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
	"wx-platform/server/internal/secretbox"
	"wx-platform/server/internal/wxapi"
	"wx-platform/server/internal/wxaudit"
	"wx-platform/server/internal/wxtoken"
)

// ---------------------------------------------------------------------------
// 真实数据库
// ---------------------------------------------------------------------------

// testDB 连接测试库并迁移/清空本包用到的表；未设置 TEST_DB_DSN 时跳过。
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DB_DSN"))
	if dsn == "" {
		t.Skip("未设置 TEST_DB_DSN，跳过 wxaudit 集成测试（示例：TEST_DB_DSN='root:@tcp(127.0.0.1:3306)/wx_platform_test?charset=utf8mb4&parseTime=True&loc=Local'）")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	models := testModels()
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

// testModels 本包用到的全部模型（只动这些表，避免影响同库的其它测试包）。
func testModels() []any {
	return []any{
		&model.PlatformState{},
		&model.Authorizer{},
		&model.TokenCache{},
		&model.CodeTemplate{},
		&model.AuditProfile{},
		&model.AuditRecord{},
		&model.ReleaseRecord{},
		&model.UndoQuotaUsage{},
		&model.OperationLog{},
		&model.RuntimeSetting{},
	}
}

// ---------------------------------------------------------------------------
// 模拟微信服务端
// ---------------------------------------------------------------------------

// mockCall 一次模拟调用的记录。
type mockCall struct {
	Method string
	Path   string
	Query  url.Values
	Body   string
}

// mockWx 按需应答的微信模拟服务端。
type mockWx struct {
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	calls  []mockCall
}

// newMockWx 构造空的模拟服务端（未注册的接口返回 404）。
func newMockWx() *mockWx { return &mockWx{routes: map[string]http.HandlerFunc{}} }

// ServeHTTP 记录调用并按注册的路由应答。
func (m *mockWx) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "读取请求体失败", http.StatusInternalServerError)
		return
	}
	m.mu.Lock()
	m.calls = append(m.calls, mockCall{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.Query(),
		Body:   string(body),
	})
	handler := m.routes[r.URL.Path]
	m.mu.Unlock()

	if handler == nil {
		http.Error(w, "mockwx 未注册的接口 "+r.Method+" "+r.URL.Path, http.StatusNotFound)
		return
	}
	handler(w, r)
}

// route 注册一个接口应答。
func (m *mockWx) route(path string, fn http.HandlerFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes[path] = fn
}

// jsonReply 注册固定 JSON 应答（原样写回，便于精确控制字段大小写与 errcode）。
func (m *mockWx) jsonReply(path, body string) {
	m.route(path, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	})
}

// errcode 注册只返回 errcode/errmsg 的应答（模拟微信侧业务失败）。
func (m *mockWx) errcode(path string, code int, msg string) {
	m.jsonReply(path, fmt.Sprintf(`{"errcode":%d,"errmsg":%q}`, code, msg))
}

// callsTo 返回某个接口的调用记录。
func (m *mockWx) callsTo(path string) []mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockCall, 0, len(m.calls))
	for _, c := range m.calls {
		if c.Path == path {
			out = append(out, c)
		}
	}
	return out
}

// callCount 返回某个接口的调用次数。
func (m *mockWx) callCount(path string) int { return len(m.callsTo(path)) }

// ---------------------------------------------------------------------------
// 内存令牌层
// ---------------------------------------------------------------------------

// memTokenStore 内存版令牌 / 授权方存储，直接满足 wxtoken.Store + wxtoken.AuthorizerStore。
//
// 为什么不复用 core.NewTokenStore(env.Repos)：token_cache 与 platform_state 属于**共享测试库**，
// 其它测试包会 TRUNCATE 它们。令牌层是本包用例的「前置条件」而不是被测对象，
// 放进内存后本包只依赖自己写入的表数据，避免与并发测试包互相破坏。
type memTokenStore struct {
	mu     sync.Mutex
	tokens map[string]model.TokenCache
}

// newMemTokenStore 构造内存令牌存储。
func newMemTokenStore() *memTokenStore {
	return &memTokenStore{tokens: map[string]model.TokenCache{}}
}

// tokenKey 令牌的唯一键（scope + appid）。
func tokenKey(scope model.TokenScope, appid string) string { return string(scope) + "|" + appid }

// GetState 平台状态（本包不需要真实票据，返回空状态即可）。
func (m *memTokenStore) GetState(context.Context) (*model.PlatformState, error) {
	return &model.PlatformState{ID: 1}, nil
}

// SaveTicket 记录票据（内存实现不需要持久化）。
func (m *memTokenStore) SaveTicket(context.Context, string, time.Time) error { return nil }

// SaveComponentToken 记录第三方平台令牌（内存实现不需要持久化）。
func (m *memTokenStore) SaveComponentToken(context.Context, string, time.Time) error { return nil }

// MarkPushOK 记录推送时间（内存实现不需要持久化）。
func (m *memTokenStore) MarkPushOK(context.Context, time.Time) error { return nil }

// GetToken 读取令牌；未命中返回 repo.ErrNotFound（与真实仓储语义一致）。
func (m *memTokenStore) GetToken(_ context.Context, scope model.TokenScope, appid string) (*model.TokenCache, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cached, ok := m.tokens[tokenKey(scope, appid)]
	if !ok {
		return nil, repo.ErrNotFound
	}
	return &cached, nil
}

// PutToken 写入令牌。
func (m *memTokenStore) PutToken(_ context.Context, scope model.TokenScope, appid, token string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[tokenKey(scope, appid)] = model.TokenCache{Scope: scope, Appid: appid, Token: token, ExpiresAt: expiresAt}
	return nil
}

// DeleteToken 删除令牌。
func (m *memTokenStore) DeleteToken(_ context.Context, scope model.TokenScope, appid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tokens, tokenKey(scope, appid))
	return nil
}

// Get 授权方（本包不需要刷新令牌：令牌都由用例直接预置，取不到即视为未登记）。
func (m *memTokenStore) Get(context.Context, string) (*model.Authorizer, error) {
	return nil, repo.ErrNotFound
}

// UpdateRefreshToken 更新 refresh_token 密文（内存实现不需要持久化）。
func (m *memTokenStore) UpdateRefreshToken(context.Context, string, []byte, time.Time) error {
	return nil
}

// ListAll 列出全部授权方（内存实现为空）。
func (m *memTokenStore) ListAll(context.Context) ([]model.Authorizer, error) { return nil, nil }

// Upsert 写入授权方（内存实现不需要持久化）。
func (m *memTokenStore) Upsert(context.Context, *model.Authorizer) error { return nil }

// ---------------------------------------------------------------------------
// 用例环境
// ---------------------------------------------------------------------------

// plantedApp 用例登记过的小程序（并发跑测试包时用于重新写回前置数据）。
type plantedApp struct {
	authorizer *model.Authorizer
	withToken  bool
}

// fixture 一个用例的完整运行环境：真实仓储 + 内存令牌层 + 模拟微信 + 三个服务。
type fixture struct {
	t         *testing.T
	ctx       context.Context
	db        *gorm.DB
	env       *core.Env
	mock      *mockWx
	audit     *wxaudit.AuditService
	release   *wxaudit.ReleaseService
	preflight *wxaudit.PreflightService
	planted   []plantedApp
}

// newFixture 构造用例环境。
func newFixture(t *testing.T) *fixture {
	t.Helper()
	db := testDB(t)
	mock := newMockWx()
	server := httptest.NewServer(mock)
	t.Cleanup(server.Close)

	box, err := secretbox.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		t.Fatalf("构造加密组件失败: %v", err)
	}
	env := core.NewEnv(db, &config.Config{}, box)
	client := wxapi.New(wxapi.Options{BaseURL: server.URL})
	tokens := wxtoken.New(client, newMemTokenStore(), newMemTokenStore(), box, "wx_component_test", "component_secret")
	env.Wx = client
	env.Tokens = tokens

	f := &fixture{t: t, ctx: context.Background(), db: db, env: env, mock: mock}
	f.audit = wxaudit.NewAuditService(env)
	f.release = wxaudit.NewReleaseService(env)
	f.preflight = wxaudit.NewPreflightService(env)
	return f
}

// seed 幂等地重新写回本用例登记过的小程序。
//
// 用途：`go test ./...` 并行跑不同包时，其它包的清表可能落在用例中途；
// 在关键调用前调用 seed() 可以把前置数据补回来（窗口缩小到毫秒级）。
func (f *fixture) seed() {
	f.t.Helper()
	for _, item := range f.planted {
		f.plant(item.authorizer, item.withToken)
	}
}

// plant 写入一个小程序（可选预置 authorizer_access_token）。
func (f *fixture) plant(a *model.Authorizer, withToken bool) {
	f.t.Helper()
	if err := f.env.Repos.Authorizers.Upsert(f.ctx, a); err != nil {
		f.t.Fatalf("登记小程序失败: %v", err)
	}
	if !withToken {
		return
	}
	if err := f.env.Tokens.StoreAuthorizerToken(f.ctx, a.Appid, "token_"+a.Appid, 7200); err != nil {
		f.t.Fatalf("预置 authorizer_access_token 失败: %v", err)
	}
}

// app 登记一个「已授权」的小程序，并预置可用的 authorizer_access_token。
//
// 令牌直接写内存令牌层：本包测的是审核/发布/体检逻辑，不需要再模拟一遍
// wxtoken 的换取链路（那条链路由 wxtoken 与 mockwx 负责验证）。
func (f *fixture) app(appid, nick string, funcInfo ...int) *model.Authorizer {
	f.t.Helper()
	authorizedAt := time.Now()
	a := &model.Authorizer{
		Appid:               appid,
		NickName:            nick,
		HeadImg:             "https://example.com/" + appid + ".png",
		Signature:           "示例简介",
		AuthorizationStatus: model.AuthStatusAuthorized,
		AuthorizedAt:        &authorizedAt,
		Enabled:             true,
		CodeSource:          model.CodeSourceTemplate,
		FuncInfo:            model.IntSlice(funcInfo),
	}
	f.plant(a, true)
	f.planted = append(f.planted, plantedApp{authorizer: a, withToken: true})
	return a
}

// appCustom 登记一个自定义字段的小程序（默认已授权 + 预置令牌），并纳入 seed 范围。
//
// 用例需要「资料不全」「代码来源是直传」这类特殊账号时用它，而不是先 app() 再改库：
// seed() 会按登记时的内容重新写回，改库的字段会被抹掉。
func (f *fixture) appCustom(a *model.Authorizer) *model.Authorizer {
	f.t.Helper()
	if a.AuthorizationStatus == "" {
		a.AuthorizationStatus = model.AuthStatusAuthorized
	}
	a.Enabled = true
	f.plant(a, true)
	f.planted = append(f.planted, plantedApp{authorizer: a, withToken: true})
	return a
}

// appUnauthorized 登记一个「已取消授权」的小程序。
func (f *fixture) appUnauthorized(appid, nick string, funcInfo ...int) *model.Authorizer {
	f.t.Helper()
	a := &model.Authorizer{
		Appid:               appid,
		NickName:            nick,
		AuthorizationStatus: model.AuthStatusUnauthorized,
		Enabled:             true,
		CodeSource:          model.CodeSourceTemplate,
		FuncInfo:            model.IntSlice(funcInfo),
	}
	f.plant(a, false)
	f.planted = append(f.planted, plantedApp{authorizer: a, withToken: false})
	return a
}

// clearTable 清空指定表（用例需要「空表」这个确定前提时显式调用，避免依赖 fixture 初始化时机）。
func (f *fixture) clearTable(dest any) {
	f.t.Helper()
	if err := f.db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(dest).Error; err != nil {
		f.t.Fatalf("清空 %T 失败: %v", dest, err)
	}
}

// seedAudit 落一条本地审核台账。
func (f *fixture) seedAudit(appid string, auditID int64, status int, userVersion string) {
	f.t.Helper()
	now := time.Now()
	rec := &model.AuditRecord{
		Appid:       appid,
		AuditID:     auditID,
		UserVersion: userVersion,
		UserDesc:    "示例提审说明",
		Status:      status,
		Source:      model.AuditSourceAPI,
		StatusTime:  &now,
	}
	if err := f.env.Repos.Audits.Upsert(f.ctx, rec); err != nil {
		f.t.Fatalf("落审核台账失败: %v", err)
	}
}

// auditRecords 读取某小程序的审核历史。
func (f *fixture) auditRecords(appid string) []model.AuditRecord {
	f.t.Helper()
	rows, err := f.env.Repos.Audits.HistoryByApp(f.ctx, appid, 50)
	if err != nil {
		f.t.Fatalf("查询审核台账失败: %v", err)
	}
	return rows
}

// releaseRecords 读取某小程序的发布台账。
func (f *fixture) releaseRecords(appid string) []model.ReleaseRecord {
	f.t.Helper()
	rows, total, err := f.env.Repos.Releases.List(f.ctx, appid, 1, 200)
	if err != nil {
		f.t.Fatalf("查询发布台账失败: %v", err)
	}
	if int(total) == 0 {
		return nil
	}
	return rows
}

// operationLogs 读取某类操作审计。
func (f *fixture) operationLogs(action string) []model.OperationLog {
	f.t.Helper()
	rows, _, err := f.env.Repos.Operations.List(f.ctx, action, 1, 200)
	if err != nil {
		f.t.Fatalf("查询操作日志失败: %v", err)
	}
	return rows
}

// auditQuota 读取服务商额度缓存。
func (f *fixture) cachedQuota() *gen.AuditQuota {
	f.t.Helper()
	return f.env.Platform.CachedQuota(f.ctx)
}

// cachedQuotaStable 读取服务商额度缓存；为空时重新查询一次再读。
//
// 共享测试库的 runtime_settings 可能被并发跑的其它测试包清空，
// 因此「查询落库 → 读缓存」这类断言容易偶发为空，这里做一次自愈。
func (f *fixture) cachedQuotaStable(appid string) *gen.AuditQuota {
	f.t.Helper()
	f.seed()
	if quota := f.cachedQuota(); quota != nil {
		return quota
	}
	f.seed()
	if _, err := f.audit.Quota(f.ctx, appid); err != nil {
		return f.cachedQuota()
	}
	return f.cachedQuota()
}

// revertRoute 注册回退接口：action=get_history_version 返回历史版本，其余（真正的回退）返回 revertBody。
func (f *fixture) revertRoute(historyJSON, revertBody string) {
	f.mock.route("/wxa/revertcoderelease", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("action") == "get_history_version" {
			_, _ = io.WriteString(w, historyJSON)
			return
		}
		_, _ = io.WriteString(w, revertBody)
	})
}

// appids 构造「显式指定小程序」的选择参数。
func appids(list ...string) gen.AppidSelection {
	items := append([]string(nil), list...)
	return gen.AppidSelection{Appids: &items}
}

// mustJSON 把任意值序列化成 JSON（用例里拼装期望值时用）。
func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	return string(b)
}

// checkOf 从体检项里取某项检查。
func checkOf(t *testing.T, item gen.PreflightItem, key string) gen.PreflightCheck {
	t.Helper()
	for _, c := range item.Checks {
		if c.Key == key {
			return c
		}
	}
	t.Fatalf("体检项里没有 %s：实际 %s", key, checkKeys(item))
	return gen.PreflightCheck{}
}

// checkKeys 列出体检项里的 check key（失败时报错用）。
func checkKeys(item gen.PreflightItem) string {
	keys := make([]string, 0, len(item.Checks))
	for _, c := range item.Checks {
		keys = append(keys, string(c.Status)+":"+c.Key)
	}
	return strings.Join(keys, ", ")
}

// hintOf 取检查项的 hint（为空返回空串）。
func hintOf(c gen.PreflightCheck) string {
	if c.Hint == nil {
		return ""
	}
	return *c.Hint
}

// ts 构造固定时间（用于断言 StatusTime 等）。
func ts(sec int64) time.Time { return time.Unix(sec, 0) }

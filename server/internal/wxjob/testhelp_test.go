package wxjob

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
	"wx-platform/server/internal/wxapi"
	"wx-platform/server/internal/wxtoken"
)

// 本包是集成测试：需要可写的 MySQL / MariaDB 库与假的微信服务端。
//
//	TEST_DB_DSN='root:@tcp(127.0.0.1:3306)/wx_platform_test?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test ./internal/wxjob/ -v
//
// 未设置 TEST_DB_DSN 时全部用例跳过；微信侧一律用 httptest 假服务（不依赖真实微信）。

// testDB 建立测试库连接并迁移 / 清空本包用到的表；未设置 TEST_DB_DSN 时跳过。
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DB_DSN"))
	if dsn == "" {
		t.Skip("未设置 TEST_DB_DSN，跳过 wxjob 集成测试（示例：TEST_DB_DSN='root:@tcp(127.0.0.1:3306)/wx_platform_test?charset=utf8mb4&parseTime=True&loc=Local'）")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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

	if err := db.AutoMigrate(testModels()...); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	for _, m := range testModels() {
		if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(m).Error; err != nil {
			t.Fatalf("清空测试表失败: %v", err)
		}
	}
	return db
}

// testModels 本包用到的模型。
func testModels() []any {
	return []any{
		&model.PlatformState{},
		&model.Authorizer{},
		&model.TokenCache{},
		&model.CodeTemplate{},
		&model.AuditProfile{},
		&model.BatchJob{},
		&model.BatchJobItem{},
		&model.AuditRecord{},
		&model.ReleaseRecord{},
		&model.UndoQuotaUsage{},
		&model.RuntimeSetting{},
	}
}

// testApp 一套干净的测试依赖（env + db，可选假微信服务端）。
type testApp struct {
	t    *testing.T
	env  *core.Env
	db   *gorm.DB
	stub *stubWX
	svc  *JobService
}

// newTestApp 构造测试依赖（微信客户端按需用 withStub 注入）。
func newTestApp(t *testing.T) *testApp {
	t.Helper()
	db := testDB(t)
	cfg := &config.Config{
		JobConcurrency:      3,
		JobMaxAttempts:      3,
		PrivacyCheckMaxWait: 10 * time.Minute,
		AuditResultMaxWait:  7 * 24 * time.Hour,
	}
	env := core.NewEnv(db, cfg, nil)
	return &testApp{t: t, env: env, db: db, svc: NewJobService(env)}
}

// withStub 启动假微信服务端并注入 env（幂等）。
func (a *testApp) withStub() *stubWX {
	a.t.Helper()
	if a.stub == nil {
		a.stub = newStubWX(a.t)
		a.stub.attach(a.env)
	}
	return a.stub
}

// ctx 测试用上下文。
func (a *testApp) ctx() context.Context { return context.Background() }

// ---- 假微信服务端 ----

// stubWX 假微信服务端：按路径注册响应，并记录收到的请求（用于断言「预览不调用微信」）。
type stubWX struct {
	srv    *httptest.Server
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	calls  []string
	bodies map[string][]map[string]any
}

func newStubWX(t *testing.T) *stubWX {
	t.Helper()
	s := &stubWX{
		routes: map[string]http.HandlerFunc{},
		bodies: map[string][]map[string]any{},
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubWX) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.calls = append(s.calls, r.URL.Path)
	if len(body) > 0 {
		var m map[string]any
		if json.Unmarshal(body, &m) == nil {
			s.bodies[r.URL.Path] = append(s.bodies[r.URL.Path], m)
		}
	}
	handler := s.routes[r.URL.Path]
	s.mu.Unlock()

	if handler == nil {
		writeJSON(w, `{"errcode":0,"errmsg":"ok"}`)
		return
	}
	handler(w, r)
}

// on 注册某个路径的处理函数。
func (s *stubWX) on(path string, fn http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[path] = fn
}

// json 注册固定 JSON 响应（微信成功 / 失败都是 HTTP 200 + errcode）。
func (s *stubWX) json(path, body string) {
	s.on(path, func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, body) })
}

// attach 把假微信服务与令牌管理器注入 env（令牌直接读 token_cache，不请求微信换令牌）。
func (s *stubWX) attach(env *core.Env) {
	env.Wx = wxapi.New(wxapi.Options{BaseURL: s.srv.URL, MaxQPS: -1})
	store := core.NewTokenStore(env.Repos)
	env.Tokens = wxtoken.New(env.Wx, store, store, nil, "wxcomponent", "component-secret")
}

// callCount 收到的请求总数。
func (s *stubWX) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// callsTo 某个路径收到的请求数。
func (s *stubWX) callsTo(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if c == path {
			n++
		}
	}
	return n
}

// firstCallIndex 某个路径首次被调用的序号（未调用返回 -1），用于断言步骤顺序。
func (s *stubWX) firstCallIndex(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.calls {
		if c == path {
			return i
		}
	}
	return -1
}

// bodyTo 某个路径最后一次收到的 JSON body。
func (s *stubWX) bodyTo(path string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := s.bodies[path]
	if len(list) == 0 {
		return nil
	}
	return list[len(list)-1]
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write([]byte(body))
}

// ---- 造数据 ----

// seedAuthorizer 落库一个已授权小程序（默认启用 + 具备开发权限集 18）。
func (a *testApp) seedAuthorizer(appid string, mutate ...func(*model.Authorizer)) *model.Authorizer {
	a.t.Helper()
	au := &model.Authorizer{
		Appid:               appid,
		NickName:            "测试小程序",
		AuthorizationStatus: model.AuthStatusAuthorized,
		FuncInfo:            model.IntSlice{model.PermissionSetDev},
		Enabled:             true,
		CodeSource:          model.CodeSourceTemplate,
	}
	for _, fn := range mutate {
		fn(au)
	}
	if err := a.env.Repos.Authorizers.Upsert(a.ctx(), au); err != nil {
		a.t.Fatalf("写入授权小程序失败: %v", err)
	}
	return au
}

// seedTemplate 落库一个模板（templateType 0=普通模板，1=标准模板）。
func (a *testApp) seedTemplate(templateID int64, templateType int) *model.CodeTemplate {
	a.t.Helper()
	tpl := &model.CodeTemplate{
		TemplateID:   templateID,
		TemplateType: templateType,
		UserVersion:  "1.0.0",
		UserDesc:     "模板描述",
	}
	if err := a.db.Create(tpl).Error; err != nil {
		a.t.Fatalf("写入模板失败: %v", err)
	}
	return tpl
}

// seedProfile 落库一个提审配置。
func (a *testApp) seedProfile(itemList model.JSONMap) *model.AuditProfile {
	a.t.Helper()
	p := &model.AuditProfile{
		Name:     "配置-" + strings.ReplaceAll(a.t.Name(), "/", "-") + "-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		ItemList: itemList,
	}
	if err := a.db.Create(p).Error; err != nil {
		a.t.Fatalf("写入提审配置失败: %v", err)
	}
	return p
}

// seedToken 预置授权方令牌（避开换取令牌的网络调用）。
func (a *testApp) seedToken(appid string) {
	a.t.Helper()
	err := a.env.Repos.Tokens.Put(a.ctx(), model.TokenScopeAuthorizer, appid, "mock-authorizer-token", time.Now().Add(2*time.Hour))
	if err != nil {
		a.t.Fatalf("预置授权方令牌失败: %v", err)
	}
}

// setSetting 写入运行参数。
func (a *testApp) setSetting(key, value string) {
	a.t.Helper()
	if err := a.env.Repos.Settings.Put(a.ctx(), key, value); err != nil {
		a.t.Fatalf("写入运行参数 %s 失败: %v", key, err)
	}
}

// seedJob 直接落库一个作业与它的全部步骤子项（便于单独验证执行器）。
func (a *testApp) seedJob(jobType model.JobType, appid string, payload model.JSONMap) (*model.BatchJob, map[model.JobStep]*model.BatchJobItem) {
	a.t.Helper()
	if payload == nil {
		payload = model.JSONMap{}
	}
	if _, ok := payload[payloadKeyType]; !ok {
		payload[payloadKeyType] = string(jobType)
	}
	if _, ok := payload[payloadKeyAppids]; !ok {
		payload[payloadKeyAppids] = []string{appid}
	}
	steps := jobType.Steps()
	jobID, err := newJobID()
	if err != nil {
		a.t.Fatalf("生成作业 ID 失败: %v", err)
	}
	job := &model.BatchJob{ID: jobID, Type: jobType, Status: model.JobStatusRunning, Total: len(steps), Payload: payload}
	items := make([]model.BatchJobItem, 0, len(steps))
	for _, st := range steps {
		items = append(items, model.BatchJobItem{JobID: jobID, Step: st, Appid: appid, Status: model.ItemStatusPending})
	}
	if err := a.env.Repos.Jobs.Create(a.ctx(), job, items); err != nil {
		a.t.Fatalf("写入作业失败: %v", err)
	}
	return a.loadJob(jobID)
}

// loadJob 重新读取作业与子项（拿到主键与数据库默认值）。
func (a *testApp) loadJob(jobID string) (*model.BatchJob, map[model.JobStep]*model.BatchJobItem) {
	a.t.Helper()
	job, err := a.env.Repos.Jobs.Get(a.ctx(), jobID)
	if err != nil {
		a.t.Fatalf("读取作业失败: %v", err)
	}
	stored, err := a.env.Repos.JobItems.AllByJob(a.ctx(), jobID)
	if err != nil {
		a.t.Fatalf("读取作业子项失败: %v", err)
	}
	out := make(map[model.JobStep]*model.BatchJobItem, len(stored))
	for i := range stored {
		copied := stored[i]
		out[copied.Step] = &copied
	}
	return job, out
}

// reloadItem 重新读取子项（校验落库结果）。
func (a *testApp) reloadItem(id uint) *model.BatchJobItem {
	a.t.Helper()
	item, err := a.env.Repos.JobItems.Get(a.ctx(), id)
	if err != nil {
		a.t.Fatalf("读取子项失败: %v", err)
	}
	return item
}

// ---- 小工具 ----

// validAuditItem 一个合法的提审项（类目与 categoryStubBody 一致）。
func validAuditItem() map[string]any {
	return map[string]any{
		"title":        "首页",
		"tag":          "教育",
		"first_class":  "教育",
		"second_class": "在线教育",
		"first_id":     100,
		"second_id":    200,
	}
}

// categoryStubBody /wxa/get_category 的假响应。
const categoryStubBody = `{"errcode":0,"errmsg":"ok","category_list":[{"first_class":"教育","second_class":"在线教育","third_class":"","first_id":100,"second_id":200,"third_id":0}]}`

// appidOf 生成测试用 appid（微信 appid 以 wx 开头；用哈希保证不同用例互不重名）。
func appidOf(name string) string {
	sum := sha1.Sum([]byte(name))
	return "wx" + hex.EncodeToString(sum[:])[:16]
}

func ptr(s string) *string { return &s }

func intPtr(v int) *int { return &v }

func int64Ptr(v int64) *int64 { return &v }

// appidSelection 构造显式指定 appid 的选择条件。
func appidSelection(appids ...string) gen.AppidSelection {
	return gen.AppidSelection{Appids: &appids}
}

// containsAll 断言文本包含全部片段，失败时打印完整文本便于排查。
func containsAll(t *testing.T, text string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("期望文本包含 %q，实际为：%s", want, text)
		}
	}
}

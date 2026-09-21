// Package e2e 用内置模拟微信服务端驱动整个平台，端到端验证
// 「回调票据 → 授权 → 模板库 → 批量上传代码 → 批量提审 → 审核结果推送 → 批量发布」全链路。
//
// 运行方式：
//
//	make e2e-mock
//	# 或（注意用独立的 e2e 库：本测试会清空全部业务表）
//	TEST_DB_DSN='wxplatform:wxplatform_dev_password@tcp(127.0.0.1:3306)/wx_platform_e2e?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test ./internal/e2e/ -v
//
// 未设置 TEST_DB_DSN 时整体跳过（与 repo 层的集成测试约定一致）。
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"wx-platform/server/internal/auth"
	"wx-platform/server/internal/batch"
	"wx-platform/server/internal/callback"
	"wx-platform/server/internal/config"
	"wx-platform/server/internal/core"
	"wx-platform/server/internal/database"
	"wx-platform/server/internal/handler"
	"wx-platform/server/internal/mockwx"
	"wx-platform/server/internal/router"
	"wx-platform/server/internal/secretbox"
	"wx-platform/server/internal/service"
	"wx-platform/server/internal/wxapi"
	"wx-platform/server/internal/wxcrypt"
	"wx-platform/server/internal/wxjob"
	"wx-platform/server/internal/wxtoken"

	"gorm.io/gorm"
)

const (
	testComponentAppID = "wx_mock_component"
	testVerifyToken    = "mock_verify_token"
	testAESKey         = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFG" // 43 字符
	testAdminPassword  = "e2e-admin-password"
	testJWTSecret      = "e2e-jwt-secret-that-is-long-enough-0123456789"
)

// env 集成测试环境。
type env struct {
	t            *testing.T
	cfg          *config.Config
	db           *gorm.DB
	mock         *mockwx.Server
	wxSrv        *httptest.Server
	platform     *httptest.Server
	container    *service.Container
	engine       *batch.Engine
	callbackBase string
	token        string
}

func TestFullBatchFlow(t *testing.T) {
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		t.Skip("未设置 TEST_DB_DSN，跳过端到端测试（请先执行 mysql -u root < db/init.sql）")
	}
	e := newEnv(t, dsn)
	ctx := context.Background()

	// ---- 1. 登录 ----
	var login struct {
		Token string `json:"token"`
	}
	e.post("/api/v1/auth/login", map[string]any{"username": "admin", "password": testAdminPassword}, &login, http.StatusOK)
	if login.Token == "" {
		t.Fatalf("登录未返回令牌")
	}
	e.token = login.Token

	// ---- 2. 微信推送 component_verify_ticket → 平台状态应显示票据新鲜 ----
	if _, status, err := e.mock.PushTicket(ctx, e.callbackBase+"/callback/component"); err != nil || status != http.StatusOK {
		t.Fatalf("推送票据失败: status=%d err=%v", status, err)
	}
	status := e.platformStatus()
	if !status.TicketFresh {
		t.Fatalf("票据未被识别为新鲜: %+v", status)
	}

	// ---- 3. 生成授权链接（预授权码来自 mock）----
	var authURL struct {
		PreAuthCode string `json:"preAuthCode"`
		PcUrl       string `json:"pcUrl"`
		MobileUrl   string `json:"mobileUrl"`
	}
	e.get("/api/v1/authorizers/authorization-url?authType=2", &authURL, http.StatusOK)
	if authURL.PreAuthCode == "" || !strings.Contains(authURL.PcUrl, "componentloginpage") {
		t.Fatalf("授权链接异常: %+v", authURL)
	}

	// ---- 4. 商家完成授权（模拟扫码后带 auth_code 跳回平台）----
	appid := e.mock.AddAuthorizer("端到端测试小程序", []int{18, 30})
	authCode := e.mock.CreateAuthCode(appid)
	var authResp struct {
		Appid            string `json:"appid"`
		HasDevPermission *bool  `json:"hasDevPermission"`
	}
	e.post("/api/v1/authorizers/authorize", map[string]any{"authCode": authCode}, &authResp, http.StatusOK)
	if authResp.Appid != appid {
		t.Fatalf("授权登记返回 appid=%s，期望 %s", authResp.Appid, appid)
	}
	if authResp.HasDevPermission == nil || !*authResp.HasDevPermission {
		t.Fatalf("应识别出已授权开发权限集（18）")
	}

	// 授权事件推送路径也必须幂等可用。
	if _, status, err := e.mock.PushAuthorizeEvent(ctx, e.callbackBase+"/callback/component", "authorized", appid); err != nil || status != http.StatusOK {
		t.Fatalf("推送授权事件失败: status=%d err=%v", status, err)
	}

	// ---- 5. 草稿箱 → 模板库 ----
	e.post("/api/v1/drafts/sync", nil, nil, http.StatusOK)
	var drafts struct {
		Items []struct {
			DraftId int64 `json:"draftId"`
		} `json:"items"`
	}
	e.get("/api/v1/drafts", &drafts, http.StatusOK)
	if len(drafts.Items) == 0 {
		t.Fatalf("草稿箱为空（mock 应预置 2 条）")
	}
	draftID := drafts.Items[0].DraftId

	var templates struct {
		Items []struct {
			TemplateId  int64  `json:"templateId"`
			UserVersion string `json:"userVersion"`
		} `json:"items"`
		Limit int `json:"limit"`
	}
	e.post(fmt.Sprintf("/api/v1/drafts/%d/add-to-template", draftID), map[string]any{"templateType": 0}, &templates, http.StatusOK)
	if len(templates.Items) == 0 {
		t.Fatalf("添加到模板库后模板列表仍为空")
	}
	templateID := templates.Items[0].TemplateId
	e.patch(fmt.Sprintf("/api/v1/templates/%d", templateID), map[string]any{"isDefault": true}, nil, http.StatusOK)

	// ---- 6. 提审配置（类目字段与 mock 的 get_category 对齐）----
	var profile struct {
		Id int64 `json:"id"`
	}
	e.post("/api/v1/audit-profiles", map[string]any{
		"name":      "端到端提审配置",
		"isDefault": true,
		"itemList": map[string]any{
			"address":      "index",
			"tag":          "工具",
			"title":        "首页",
			"first_class":  "工具",
			"second_class": "信息查询",
			"third_class":  "天气",
			"first_id":     1,
			"second_id":    101,
			"third_id":     1001,
		},
		"versionDesc": "端到端测试版本",
	}, &profile, http.StatusCreated)

	selection := map[string]any{"appids": []string{appid}}

	// ---- 7. 前置体检 ----
	var pre struct {
		ReadyCount   int `json:"readyCount"`
		BlockedCount int `json:"blockedCount"`
		Items        []struct {
			Ready  bool `json:"ready"`
			Checks []struct {
				Key     string `json:"key"`
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"checks"`
		} `json:"items"`
	}
	e.post("/api/v1/preflight", map[string]any{"selection": selection, "purpose": "commit"}, &pre, http.StatusOK)
	if pre.ReadyCount != 1 {
		t.Fatalf("体检应通过 1 个小程序，实际 ready=%d blocked=%d 详情=%+v", pre.ReadyCount, pre.BlockedCount, pre.Items)
	}

	// ---- 8. 批量上传代码：先预览再执行 ----
	jobReq := map[string]any{
		"type":      "commit",
		"selection": selection,
		"commit": map[string]any{
			"templateId":         templateID,
			"userVersionPattern": "v{{date}}-{{seq}}",
			"userDescPattern":    "端到端批量下发 {{nick_name}}",
			"extOverrides":       map[string]string{"apiBaseUrl": "https://e2e.example.com", "storeId": "S-E2E"},
		},
	}

	var preview struct {
		ValidCount   int `json:"validCount"`
		InvalidCount int `json:"invalidCount"`
		Items        []struct {
			Appid    string         `json:"appid"`
			Valid    bool           `json:"valid"`
			Endpoint string         `json:"endpoint"`
			Payload  map[string]any `json:"payload"`
			Problems []string       `json:"problems"`
		} `json:"items"`
	}
	e.post("/api/v1/jobs/preview", jobReq, &preview, http.StatusOK)
	if preview.ValidCount != 1 || preview.InvalidCount != 0 {
		t.Fatalf("预览结果异常: valid=%d invalid=%d items=%+v", preview.ValidCount, preview.InvalidCount, preview.Items)
	}
	if preview.Items[0].Endpoint != "/wxa/commit" {
		t.Fatalf("预览端点异常: %s", preview.Items[0].Endpoint)
	}
	extJSON, _ := preview.Items[0].Payload["ext_json"].(string)
	if !strings.Contains(extJSON, appid) || !strings.Contains(extJSON, "https://e2e.example.com") {
		t.Fatalf("预览的 ext_json 未包含 appid 或变量：%s", extJSON)
	}
	if version, _ := preview.Items[0].Payload["user_version"].(string); version == "" || strings.Contains(version, "{{") {
		t.Fatalf("版本号未正确渲染：%q", version)
	}

	commitJobID := e.createAndRun(jobReq)
	// mock 侧应记录到这次 commit，且体验版版本号与预览一致。
	gotExt := e.mock.ExtJSONOf(appid)
	if !strings.Contains(gotExt, "S-E2E") {
		t.Fatalf("mock 侧未收到预期 ext_json：%s", gotExt)
	}

	// ---- 9. 批量提审 ----
	auditReq := map[string]any{
		"type":      "submit_audit",
		"selection": selection,
		"audit":     map[string]any{"auditProfileId": profile.Id, "versionDescPattern": "端到端提审"},
	}
	auditJobID := e.createAndRun(auditReq)
	if auditJobID == commitJobID {
		t.Fatalf("作业 ID 不应重复")
	}
	auditID, auditStatus, ok := e.mock.AuditStatusOf(appid)
	if !ok || auditStatus != 2 {
		t.Fatalf("提审后 mock 侧审核状态应为 2（审核中），实际 ok=%v status=%d", ok, auditStatus)
	}

	// ---- 10. 审核结果推送 → 台账应更新为「审核成功」----
	if _, status, err := e.mock.PushAuditResult(ctx, e.callbackBase+"/callback/message/"+appid, appid, "weapp_audit_success", ""); err != nil || status != http.StatusOK {
		t.Fatalf("推送审核结果失败: status=%d err=%v", status, err)
	}
	e.waitAuditStatus(appid, 0)

	// ---- 11. 批量发布 ----
	releaseReq := map[string]any{"type": "release", "selection": selection}
	e.createAndRun(releaseReq)
	if version, ok := e.mock.ReleasedVersionOf(appid); !ok || version == "" {
		t.Fatalf("发布后 mock 侧应有线上版本")
	}

	// ---- 12. 台账与日志 ----
	var audits struct {
		Total int `json:"total"`
		Items []struct {
			AuditId int64 `json:"auditId"`
			Status  int   `json:"status"`
		} `json:"items"`
	}
	e.get("/api/v1/audits?appid="+appid, &audits, http.StatusOK)
	if audits.Total == 0 || audits.Items[0].AuditId != auditID {
		t.Fatalf("审核台账异常: %+v (期望 auditId=%d)", audits, auditID)
	}

	var releases struct {
		Total int `json:"total"`
	}
	e.get("/api/v1/releases?appid="+appid, &releases, http.StatusOK)
	if releases.Total == 0 {
		t.Fatalf("发布台账为空")
	}

	var callbacks struct {
		Total int `json:"total"`
	}
	e.get("/api/v1/logs/callbacks", &callbacks, http.StatusOK)
	if callbacks.Total < 3 {
		t.Fatalf("回调事件日志过少：%d（应至少包含票据、授权、审核结果三条）", callbacks.Total)
	}

	var calls struct {
		Total int `json:"total"`
		Items []struct {
			Endpoint string         `json:"endpoint"`
			Request  map[string]any `json:"request"`
		} `json:"items"`
	}
	e.get("/api/v1/logs/api-calls?pageSize=200", &calls, http.StatusOK)
	if calls.Total == 0 {
		t.Fatalf("微信调用日志为空")
	}
	raw, _ := json.Marshal(calls.Items)
	if strings.Contains(string(raw), "mock_component_token") || strings.Contains(string(raw), "authorizer_access_token\":\"mock") {
		t.Fatalf("调用日志泄露了令牌原文：%s", string(raw[:min(len(raw), 300)]))
	}

	t.Logf("✅ 端到端链路通过：commit 作业=%s、提审作业=%s（auditId=%d）、发布作业完成=%v",
		commitJobID, auditJobID, auditID, true)
}

// ---- 环境搭建 ----

func newEnv(t *testing.T, dsn string) *env {
	t.Helper()
	db, err := database.Connect(dsn)
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}
	resetDB(t, db)

	cfg := &config.Config{
		ServerPort:           "0",
		PublicBaseURL:        "http://127.0.0.1",
		DBHost:               "127.0.0.1",
		DBPort:               "3306",
		DBName:               "wx_platform_test",
		JWTSecret:            testJWTSecret,
		JWTTokenTTL:          time.Hour,
		AdminPassword:        testAdminPassword,
		LoginRateMax:         1000,
		LoginRateWin:         time.Minute,
		ComponentAppID:       testComponentAppID,
		ComponentAppSecret:   "mock_component_secret",
		ComponentVerifyToken: testVerifyToken,
		ComponentAESKey:      testAESKey,
		SecretEncKey:         bytes.Repeat([]byte{7}, 32),
		WxRequestTimeout:     10 * time.Second,
		WxMaxQps:             20,
		JobConcurrency:       2,
		JobMaxAttempts:       3,
		LogRetentionDays:     30,
		PrivacyCheckMaxWait:  60 * time.Second,
		AuditResultMaxWait:   2 * time.Minute,
	}

	if err := database.EnsurePlatformState(db, cfg.ComponentAppID); err != nil {
		t.Fatalf("初始化平台状态失败: %v", err)
	}
	if err := database.SeedRuntimeSettings(db, cfg); err != nil {
		t.Fatalf("初始化运行参数失败: %v", err)
	}

	box, err := secretbox.New(cfg.SecretEncKey)
	if err != nil {
		t.Fatalf("初始化加密组件失败: %v", err)
	}
	am := auth.NewManager(cfg.JWTSecret, cfg.JWTTokenTTL, cfg.AdminPassword)
	limiter := auth.NewLoginLimiter(cfg.LoginRateMax, cfg.LoginRateWin)
	container := service.NewContainer(db, cfg, box, am, limiter)

	mock := mockwx.New(mockwx.Options{
		ComponentAppID: testComponentAppID,
		VerifyToken:    testVerifyToken,
		EncodingAESKey: testAESKey,
	})
	wxSrv := httptest.NewServer(mock.Handler())
	t.Cleanup(wxSrv.Close)

	// 与生产一致地挂上调用日志记录器：这样 api_call_logs 里才有内容，
	// 也才能真正验证「令牌/密钥不会出现在日志里」这条安全红线。
	recorder := &service.WxCallRecorder{Repos: container.Env.Repos}
	wxClient := wxapi.New(wxapi.Options{
		BaseURL: wxSrv.URL,
		Timeout: cfg.WxRequestTimeout,
		MaxQPS:  cfg.WxMaxQps,
		Logger:  recorder,
	})
	tokenStore := core.NewTokenStore(container.Env.Repos)
	tokens := wxtoken.New(wxClient, tokenStore, tokenStore, box, cfg.ComponentAppID, cfg.ComponentAppSecret)
	container.AttachWeChat(wxClient, tokens)

	crypto, err := wxcrypt.New(testVerifyToken, testAESKey, testComponentAppID, "")
	if err != nil {
		t.Fatalf("初始化消息加解密失败: %v", err)
	}
	cb := callback.New(callback.Options{
		Crypto:         crypto,
		Tickets:        tokens,
		Events:         container.Env.Repos.Callbacks,
		Business:       container.NewCallbackBusiness(),
		ComponentAppid: testComponentAppID,
		Async:          false, // 同步处理，便于断言
	})

	engine := batch.New(wxjob.NewEngineStore(container.Env), batch.Options{
		Concurrency:  func() int { return 2 },
		MaxAttempts:  func() int { return 3 },
		PollInterval: 50 * time.Millisecond,
		LockName:     "wx_platform_e2e_engine",
		Logger:       t.Logf,
	})
	container.AttachEngine(engine)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("启动作业引擎失败: %v", err)
	}

	platformSrv := httptest.NewServer(router.NewRouter(handler.NewServer(container), am, cb))
	t.Cleanup(platformSrv.Close)

	return &env{
		t:            t,
		cfg:          cfg,
		db:           db,
		mock:         mock,
		wxSrv:        wxSrv,
		platform:     platformSrv,
		container:    container,
		engine:       engine,
		callbackBase: platformSrv.URL,
	}
}

// resetDB 清空全部业务表，保证多次运行结果稳定。
func resetDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	tables := []string{
		"batch_job_items", "batch_jobs", "audit_records", "release_records", "undo_quota_usage",
		"api_call_logs", "callback_events", "operation_logs", "code_templates", "code_drafts",
		"audit_profiles", "authorizers", "token_cache", "platform_state", "runtime_settings",
	}
	for _, table := range tables {
		if err := db.Exec("DELETE FROM " + table).Error; err != nil {
			t.Fatalf("清空表 %s 失败: %v", table, err)
		}
	}
}

// ---- HTTP 辅助 ----

type platformStatusResp struct {
	ComponentAppid string   `json:"componentAppid"`
	TicketFresh    bool     `json:"ticketFresh"`
	AuthAuthorized int      `json:"authAuthorized"`
	Warnings       []string `json:"warnings"`
}

func (e *env) platformStatus() platformStatusResp {
	var out platformStatusResp
	e.get("/api/v1/platform/status", &out, http.StatusOK)
	return out
}

func (e *env) get(path string, out any, wantStatus int) {
	e.t.Helper()
	e.do(http.MethodGet, path, nil, out, wantStatus)
}

func (e *env) post(path string, body any, out any, wantStatus int) {
	e.t.Helper()
	e.do(http.MethodPost, path, body, out, wantStatus)
}

func (e *env) patch(path string, body any, out any, wantStatus int) {
	e.t.Helper()
	e.do(http.MethodPatch, path, body, out, wantStatus)
}

func (e *env) do(method, path string, body any, out any, wantStatus int) {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			e.t.Fatalf("序列化请求体失败: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.platform.URL+path, reader)
	if err != nil {
		e.t.Fatalf("构造请求失败: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if e.token != "" {
		req.Header.Set("Authorization", "Bearer "+e.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s 失败: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		e.t.Fatalf("%s %s 期望 HTTP %d，实际 %d，响应：%s", method, path, wantStatus, resp.StatusCode, string(raw))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("%s %s 解析响应失败: %v（原文：%s）", method, path, err, string(raw))
		}
	}
}

// createAndRun 创建作业并等待其结束，返回作业 ID。
func (e *env) createAndRun(jobReq map[string]any) string {
	e.t.Helper()
	var job struct {
		Id     string `json:"id"`
		Status string `json:"status"`
	}
	e.post("/api/v1/jobs", jobReq, &job, http.StatusCreated)
	if job.Id == "" {
		e.t.Fatalf("创建作业未返回 ID")
	}
	e.post("/api/v1/jobs/"+job.Id+"/start", nil, nil, http.StatusOK)
	e.waitJobDone(job.Id)
	return job.Id
}

// waitJobDone 轮询作业直到进入终态或超时。
func (e *env) waitJobDone(jobID string) {
	e.t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		var job struct {
			Status    string  `json:"status"`
			Succeeded int     `json:"succeeded"`
			Failed    int     `json:"failed"`
			Skipped   int     `json:"skipped"`
			ErrorNote *string `json:"errorNote"`
		}
		e.get("/api/v1/jobs/"+jobID, &job, http.StatusOK)
		last = fmt.Sprintf("status=%s succeeded=%d failed=%d skipped=%d", job.Status, job.Succeeded, job.Failed, job.Skipped)
		switch job.Status {
		case "succeeded":
			return
		case "partial_failed", "failed", "canceled", "interrupted":
			// 打印子项明细便于定位失败原因。
			e.t.Fatalf("作业 %s 未成功结束：%s（errorNote=%v）\n%s", jobID, last, job.ErrorNote, e.jobItemSummary(jobID))
		}
		time.Sleep(100 * time.Millisecond)
	}
	e.t.Fatalf("作业 %s 超时未结束：%s\n%s", jobID, last, e.jobItemSummary(jobID))
}

// jobItemSummary 汇总子项的错误信息，失败时输出，方便一眼看出是哪一步、什么 errcode。
func (e *env) jobItemSummary(jobID string) string {
	var items struct {
		Items []struct {
			Step    string  `json:"step"`
			Status  string  `json:"status"`
			Errcode *int    `json:"errcode"`
			Errmsg  *string `json:"errmsg"`
		} `json:"items"`
	}
	e.get("/api/v1/jobs/"+jobID+"/items?pageSize=100", &items, http.StatusOK)
	var b strings.Builder
	for _, it := range items.Items {
		code := 0
		if it.Errcode != nil {
			code = *it.Errcode
		}
		msg := ""
		if it.Errmsg != nil {
			msg = *it.Errmsg
		}
		fmt.Fprintf(&b, "  [%s] %s errcode=%d errmsg=%s\n", it.Step, it.Status, code, msg)
	}
	// 附上 mock 侧关键调用（endpoint + 令牌尾部 + 请求体摘要），便于判断失败到底发生在哪一步。
	b.WriteString("mock 调用记录（关键接口）：\n")
	for _, call := range e.mock.Calls() {
		switch call.Endpoint {
		case "/wxa/commit", "/wxa/submit_audit", "/wxa/security/get_code_privacy_info", "/cgi-bin/component/api_authorizer_token":
			tokenTail := call.Token
			if len(tokenTail) > 12 {
				tokenTail = "…" + tokenTail[len(tokenTail)-12:]
			}
			fmt.Fprintf(&b, "  %s %s token=%s body=%v\n", call.Method, call.Endpoint, tokenTail, call.Body)
		}
	}
	return b.String()
}

// waitAuditStatus 轮询审核台账直到状态符合预期。
func (e *env) waitAuditStatus(appid string, want int) {
	e.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		var audits struct {
			Items []struct {
				Status int `json:"status"`
			} `json:"items"`
		}
		e.get("/api/v1/audits?appid="+appid, &audits, http.StatusOK)
		if len(audits.Items) > 0 && audits.Items[0].Status == want {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	var audits struct {
		Items []struct {
			AuditId int64 `json:"auditId"`
			Status  int   `json:"status"`
		} `json:"items"`
	}
	e.get("/api/v1/audits?appid="+appid, &audits, http.StatusOK)
	e.t.Fatalf("审核台账状态未变为 %d，当前：%+v", want, audits.Items)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

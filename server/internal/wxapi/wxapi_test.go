package wxapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"wx-platform/server/internal/model"
)

// ---------------------------------------------------------------------------
// 测试基础设施
// ---------------------------------------------------------------------------

// capturedReq 测试服务器收到的请求。
type capturedReq struct {
	Method      string
	Path        string
	Query       url.Values
	RawQuery    string
	Body        []byte
	ContentType string
}

// capture 请求记录器。
type capture struct {
	mu   sync.Mutex
	reqs []capturedReq
}

func (c *capture) add(r capturedReq) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reqs = append(c.reqs, r)
}

func (c *capture) last(t *testing.T) capturedReq {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotEmpty(t, c.reqs, "测试服务器未收到任何请求")
	return c.reqs[len(c.reqs)-1]
}

func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reqs)
}

// newServer 启动 httptest 服务器并返回指向它的客户端（不限流，避免拖慢测试）。
func newServer(t *testing.T, qps int, reply func(w http.ResponseWriter, r *http.Request, body []byte)) (*Client, *capture) {
	t.Helper()
	cap := &capture{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		cap.add(capturedReq{
			Method:      r.Method,
			Path:        r.URL.Path,
			Query:       r.URL.Query(),
			RawQuery:    r.URL.RawQuery,
			Body:        b,
			ContentType: r.Header.Get("Content-Type"),
		})
		reply(w, r, b)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(Options{BaseURL: srv.URL, MaxQPS: qps}), cap
}

// replyOK 统一返回成功信封。
func replyOK(w http.ResponseWriter, r *http.Request, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
}

// replyJSON 返回固定 JSON。
func replyJSON(payload string) func(w http.ResponseWriter, r *http.Request, body []byte) {
	return func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}
}

// requireSubset 断言 got 至少包含 want 中的所有字段（map 递归子集，其它类型 DeepEqual）。
func requireSubset(t *testing.T, want, got map[string]any, path string) {
	t.Helper()
	for k, wv := range want {
		gv, ok := got[k]
		require.True(t, ok, "%s.%s 缺失：实际 %v", path, k, got)
		switch w := wv.(type) {
		case map[string]any:
			gm, ok := gv.(map[string]any)
			require.True(t, ok, "%s.%s 应为对象，实际 %T", path, k, gv)
			requireSubset(t, w, gm, path+"."+k)
		case []any:
			ga, ok := gv.([]any)
			require.True(t, ok, "%s.%s 应为数组，实际 %T", path, k, gv)
			require.Len(t, ga, len(w), "%s.%s 数组长度", path, k)
			for i := range w {
				wm, isMap := w[i].(map[string]any)
				gm, gotMap := ga[i].(map[string]any)
				if isMap && gotMap {
					requireSubset(t, wm, gm, fmt.Sprintf("%s.%s[%d]", path, k, i))
					continue
				}
				require.Equal(t, w[i], ga[i], "%s.%s[%d]", path, k, i)
			}
		default:
			require.Equal(t, wv, gv, "%s.%s", path, k)
		}
	}
}

// ---------------------------------------------------------------------------
// 1. 接口路由：URL 路径、HTTP 方法、token 在 query、请求体 JSON 字段
// ---------------------------------------------------------------------------

// TestEndpointRoutes 覆盖全部接口的路径 / 方法 / token / body 字段断言。
func TestEndpointRoutes(t *testing.T) {
	const (
		ct = "COMPONENT_ACCESS_TOKEN_VALUE"
		at = "AUTHORIZER_ACCESS_TOKEN_VALUE"
	)
	ctx := context.Background()
	tmpType := 1

	cases := []struct {
		name    string
		call    func(c *Client) error
		method  string
		path    string
		token   string // 期望的 access_token query 值；空 = 不应带 token
		query   map[string]string
		body    map[string]any // 期望的 body 字段子集；nil = 不检查
		rawBody string         // 精确 body（用于断言「必须发 {}」）
	}{
		{
			name: "component_token",
			call: func(c *Client) error {
				_, err := c.ComponentToken(ctx, ComponentTokenRequest{ComponentAppid: "wxc1", ComponentAppsecret: "sec", ComponentVerifyTicket: "tk"})
				return err
			},
			method: http.MethodPost,
			path:   "/cgi-bin/component/api_component_token",
			body: map[string]any{
				"component_appid":         "wxc1",
				"component_appsecret":     "sec",
				"component_verify_ticket": "tk",
			},
		},
		{
			name: "start_push_ticket",
			call: func(c *Client) error {
				return c.StartPushTicket(ctx, StartPushTicketRequest{ComponentAppid: "wxc1", ComponentSecret: "sec"})
			},
			method: http.MethodPost,
			path:   "/cgi-bin/component/api_start_push_ticket",
			body:   map[string]any{"component_appid": "wxc1", "component_secret": "sec"},
		},
		{
			name:   "create_preauthcode",
			call:   func(c *Client) error { _, err := c.CreatePreAuthCode(ctx, ct, "wxc1"); return err },
			method: http.MethodPost,
			path:   "/cgi-bin/component/api_create_preauthcode",
			token:  ct,
			body:   map[string]any{"component_appid": "wxc1"},
		},
		{
			name:   "query_auth",
			call:   func(c *Client) error { _, err := c.QueryAuth(ctx, ct, "wxc1", "AUTHCODE"); return err },
			method: http.MethodPost,
			path:   "/cgi-bin/component/api_query_auth",
			token:  ct,
			body:   map[string]any{"component_appid": "wxc1", "authorization_code": "AUTHCODE"},
		},
		{
			name:   "authorizer_token",
			call:   func(c *Client) error { _, err := c.AuthorizerToken(ctx, ct, "wxc1", "wxa1", "REFRESH"); return err },
			method: http.MethodPost,
			path:   "/cgi-bin/component/api_authorizer_token",
			token:  ct,
			body:   map[string]any{"component_appid": "wxc1", "authorizer_appid": "wxa1", "authorizer_refresh_token": "REFRESH"},
		},
		{
			name:   "authorizer_list",
			call:   func(c *Client) error { _, err := c.GetAuthorizerList(ctx, ct, "wxc1", 0, 100); return err },
			method: http.MethodPost,
			path:   "/cgi-bin/component/api_get_authorizer_list",
			token:  ct,
			body:   map[string]any{"component_appid": "wxc1", "offset": float64(0), "count": float64(100)},
		},
		{
			name:   "authorizer_info",
			call:   func(c *Client) error { _, err := c.GetAuthorizerInfo(ctx, ct, "wxc1", "wxa1"); return err },
			method: http.MethodPost,
			path:   "/cgi-bin/component/api_get_authorizer_info",
			token:  ct,
			body:   map[string]any{"component_appid": "wxc1", "authorizer_appid": "wxa1"},
		},
		{
			name: "get_authorizer_option",
			call: func(c *Client) error {
				_, err := c.GetAuthorizerOption(ctx, ct, "wxc1", "wxa1", "voice_recognize")
				return err
			},
			method: http.MethodPost,
			path:   "/cgi-bin/component/get_authorizer_option",
			token:  ct,
			body:   map[string]any{"component_appid": "wxc1", "authorizer_appid": "wxa1", "option_name": "voice_recognize"},
		},
		{
			name:   "set_authorizer_option",
			call:   func(c *Client) error { return c.SetAuthorizerOption(ctx, ct, "wxc1", "wxa1", "voice_recognize", "1") },
			method: http.MethodPost,
			path:   "/cgi-bin/component/set_authorizer_option",
			token:  ct,
			body:   map[string]any{"option_name": "voice_recognize", "option_value": "1"},
		},
		{
			// tpl_gettemplatedraftlist.md：文档要求 GET
			name:   "template_draft_list_get",
			call:   func(c *Client) error { _, err := c.GetTemplateDraftList(ctx, ct); return err },
			method: http.MethodGet,
			path:   "/wxa/gettemplatedraftlist",
			token:  ct,
		},
		{
			// tpl_gettemplatelist.md：文档要求 GET；template_type 作为 URL 参数
			name:   "template_list_get",
			call:   func(c *Client) error { _, err := c.GetTemplateList(ctx, ct, &tmpType); return err },
			method: http.MethodGet,
			path:   "/wxa/gettemplatelist",
			token:  ct,
			query:  map[string]string{"template_type": "1"},
		},
		{
			name:   "template_list_get_without_type",
			call:   func(c *Client) error { _, err := c.GetTemplateList(ctx, ct, nil); return err },
			method: http.MethodGet,
			path:   "/wxa/gettemplatelist",
			token:  ct,
			query:  map[string]string{"template_type": ""},
		},
		{
			name:   "add_to_template",
			call:   func(c *Client) error { return c.AddToTemplate(ctx, ct, 77, 0) },
			method: http.MethodPost,
			path:   "/wxa/addtotemplate",
			token:  ct,
			body:   map[string]any{"draft_id": float64(77), "template_type": float64(0)},
		},
		{
			name:   "delete_template",
			call:   func(c *Client) error { return c.DeleteTemplate(ctx, ct, 88) },
			method: http.MethodPost,
			path:   "/wxa/deletetemplate",
			token:  ct,
			body:   map[string]any{"template_id": float64(88)},
		},
		{
			name: "commit",
			call: func(c *Client) error {
				return c.Commit(ctx, at, "wxa1", CommitRequest{TemplateID: 95, ExtJSON: `{"extAppid":"wxa1"}`, UserVersion: "1.0.0", UserDesc: "描述"})
			},
			method: http.MethodPost,
			path:   "/wxa/commit",
			token:  at,
			body: map[string]any{
				"template_id":  float64(95),
				"ext_json":     `{"extAppid":"wxa1"}`,
				"user_version": "1.0.0",
				"user_desc":    "描述",
			},
		},
		{
			// code_getcodepage.md：文档要求 GET
			name:   "get_page_get",
			call:   func(c *Client) error { _, err := c.GetPage(ctx, at, "wxa1"); return err },
			method: http.MethodGet,
			path:   "/wxa/get_page",
			token:  at,
		},
		{
			name: "submit_audit",
			call: func(c *Client) error {
				_, err := c.SubmitAudit(ctx, at, "wxa1", SubmitAuditRequest{
					ItemList:    []SubmitAuditItem{{Address: "index", FirstClass: "工具", SecondClass: "备忘录", FirstID: 1, SecondID: 2}},
					VersionDesc: "首个版本",
				})
				return err
			},
			method: http.MethodPost,
			path:   "/wxa/submit_audit",
			token:  at,
			body: map[string]any{
				"version_desc": "首个版本",
				"item_list": []any{
					map[string]any{"address": "index", "first_class": "工具", "second_class": "备忘录", "first_id": float64(1), "second_id": float64(2)},
				},
			},
		},
		{
			name:   "get_auditstatus",
			call:   func(c *Client) error { _, err := c.GetAuditStatus(ctx, at, "wxa1", 123); return err },
			method: http.MethodPost,
			path:   "/wxa/get_auditstatus",
			token:  at,
			body:   map[string]any{"auditid": float64(123)},
		},
		{
			// code_getlatestauditstatus.md：文档要求 GET
			name:   "get_latest_auditstatus_get",
			call:   func(c *Client) error { _, err := c.GetLatestAuditStatus(ctx, at, "wxa1"); return err },
			method: http.MethodGet,
			path:   "/wxa/get_latest_auditstatus",
			token:  at,
		},
		{
			// code_undoaudit.md：文档要求 GET
			name:   "undo_code_audit_get",
			call:   func(c *Client) error { return c.UndoCodeAudit(ctx, at, "wxa1") },
			method: http.MethodGet,
			path:   "/wxa/undocodeaudit",
			token:  at,
		},
		{
			// code_release.md：POST 且必须发 {}
			name:    "release_empty_json",
			call:    func(c *Client) error { return c.Release(ctx, at, "wxa1") },
			method:  http.MethodPost,
			path:    "/wxa/release",
			token:   at,
			rawBody: "{}",
		},
		{
			// code_getversioninfo.md：POST 且必须发 {}
			name:    "get_version_info_empty_json",
			call:    func(c *Client) error { _, err := c.GetVersionInfo(ctx, at, "wxa1"); return err },
			method:  http.MethodPost,
			path:    "/wxa/getversioninfo",
			token:   at,
			rawBody: "{}",
		},
		{
			// code_getvisitstatus.md：POST 且必须发 {}
			name:    "get_visit_status_empty_json",
			call:    func(c *Client) error { _, err := c.GetVisitStatus(ctx, at, "wxa1"); return err },
			method:  http.MethodPost,
			path:    "/wxa/getvisitstatus",
			token:   at,
			rawBody: "{}",
		},
		{
			// code_getsupportversion.md：POST 且必须发 {}
			name:    "get_support_version_empty_json",
			call:    func(c *Client) error { _, err := c.GetSupportVersion(ctx, at, "wxa1"); return err },
			method:  http.MethodPost,
			path:    "/cgi-bin/wxopen/getweappsupportversion",
			token:   at,
			rawBody: "{}",
		},
		{
			// code_setvisitstatus.md：POST /wxa/change_visitstatus
			name:   "set_visit_status",
			call:   func(c *Client) error { return c.SetVisitStatus(ctx, at, "wxa1", "close") },
			method: http.MethodPost,
			path:   "/wxa/change_visitstatus",
			token:  at,
			body:   map[string]any{"action": "close"},
		},
		{
			name:   "set_support_version",
			call:   func(c *Client) error { return c.SetSupportVersion(ctx, at, "wxa1", "2.20.1") },
			method: http.MethodPost,
			path:   "/cgi-bin/wxopen/setweappsupportversion",
			token:  at,
			body:   map[string]any{"version": "2.20.1"},
		},
		{
			// code_revertcoderelease.md：文档要求 GET；app_version 是 URL 参数
			name:   "revert_code_release_get_no_version",
			call:   func(c *Client) error { return c.RevertCodeRelease(ctx, at, "wxa1", nil) },
			method: http.MethodGet,
			path:   "/wxa/revertcoderelease",
			token:  at,
			query:  map[string]string{"app_version": ""},
		},
		{
			name:   "revert_code_release_get_with_version",
			call:   func(c *Client) error { v := int64(123); return c.RevertCodeRelease(ctx, at, "wxa1", &v) },
			method: http.MethodGet,
			path:   "/wxa/revertcoderelease",
			token:  at,
			query:  map[string]string{"app_version": "123"},
		},
		{
			name:   "list_history_versions_get",
			call:   func(c *Client) error { _, err := c.ListHistoryVersions(ctx, at, "wxa1"); return err },
			method: http.MethodGet,
			path:   "/wxa/revertcoderelease",
			token:  at,
			query:  map[string]string{"action": "get_history_version"},
		},
		{
			name:   "speedup_audit",
			call:   func(c *Client) error { return c.SpeedUpAudit(ctx, at, "wxa1", 456) },
			method: http.MethodPost,
			path:   "/wxa/speedupaudit",
			token:  at,
			body:   map[string]any{"auditid": float64(456)},
		},
		{
			// code_setcodeauditquota.md：文档要求 GET
			name:   "query_quota_get",
			call:   func(c *Client) error { _, err := c.QueryQuota(ctx, at, "wxa1"); return err },
			method: http.MethodGet,
			path:   "/wxa/queryquota",
			token:  at,
		},
		{
			name: "gray_release",
			call: func(c *Client) error {
				return c.GrayRelease(ctx, at, "wxa1", GrayReleaseRequest{GrayPercentage: 30, SupportDebugerFirst: true})
			},
			method: http.MethodPost,
			path:   "/wxa/grayrelease",
			token:  at,
			body:   map[string]any{"gray_percentage": float64(30), "support_debuger_first": true},
		},
		{
			// code_getgrayreleaseplan.md：文档要求 GET
			name:   "get_gray_release_plan_get",
			call:   func(c *Client) error { _, err := c.GetGrayReleasePlan(ctx, at, "wxa1"); return err },
			method: http.MethodGet,
			path:   "/wxa/getgrayreleaseplan",
			token:  at,
		},
		{
			// code_revertgrayrelease.md：文档要求 GET
			name:   "revert_gray_release_get",
			call:   func(c *Client) error { return c.RevertGrayRelease(ctx, at, "wxa1") },
			method: http.MethodGet,
			path:   "/wxa/revertgrayrelease",
			token:  at,
		},
		{
			// code_getcodeprivacyinfo.md：文档要求 GET
			name:   "code_privacy_info_get",
			call:   func(c *Client) error { _, err := c.GetCodePrivacyInfo(ctx, at, "wxa1"); return err },
			method: http.MethodGet,
			path:   "/wxa/security/get_code_privacy_info",
			token:  at,
		},
		{
			// api_getallcategoryname：GET /wxa/get_category
			name:   "all_category_name_get",
			call:   func(c *Client) error { _, err := c.GetAllCategoryName(ctx, at, "wxa1"); return err },
			method: http.MethodGet,
			path:   "/wxa/get_category",
			token:  at,
		},
		{
			// api_getsettingcategories：GET /cgi-bin/wxopen/getcategory（权限集 30）
			name:   "setting_categories_get",
			call:   func(c *Client) error { _, err := c.GetSettingCategories(ctx, at, "wxa1"); return err },
			method: http.MethodGet,
			path:   "/cgi-bin/wxopen/getcategory",
			token:  at,
		},
		{
			// privacyVer == nil 时也要发 {}
			name:    "privacy_setting_empty_json",
			call:    func(c *Client) error { _, err := c.GetPrivacySetting(ctx, at, "wxa1", nil); return err },
			method:  http.MethodPost,
			path:    "/cgi-bin/component/getprivacysetting",
			token:   at,
			rawBody: "{}",
		},
		{
			name:   "privacy_setting_with_ver",
			call:   func(c *Client) error { v := 2; _, err := c.GetPrivacySetting(ctx, at, "wxa1", &v); return err },
			method: http.MethodPost,
			path:   "/cgi-bin/component/getprivacysetting",
			token:  at,
			body:   map[string]any{"privacy_ver": float64(2)},
		},
		{
			name: "modify_server_domain_set",
			call: func(c *Client) error {
				_, err := c.ModifyServerDomain(ctx, at, "wxa1", "set", &DomainSet{
					RequestDomain:   []string{"https://api.example.com"},
					WSRequestDomain: []string{"wss://ws.example.com"},
					UploadDomain:    []string{"https://up.example.com"},
					DownloadDomain:  []string{"https://down.example.com"},
					UDPDomain:       []string{"udp://udp.example.com"},
					TCPDomain:       []string{"tcp://tcp.example.com"},
				})
				return err
			},
			method: http.MethodPost,
			path:   "/wxa/modify_domain",
			token:  at,
			body: map[string]any{
				"action":          "set",
				"requestdomain":   []any{"https://api.example.com"},
				"wsrequestdomain": []any{"wss://ws.example.com"},
				"uploaddomain":    []any{"https://up.example.com"},
				"downloaddomain":  []any{"https://down.example.com"},
				"udpdomain":       []any{"udp://udp.example.com"},
				"tcpdomain":       []any{"tcp://tcp.example.com"},
			},
		},
		{
			// get 时不传域名数组
			name:   "modify_server_domain_get",
			call:   func(c *Client) error { _, err := c.ModifyServerDomain(ctx, at, "wxa1", "get", nil); return err },
			method: http.MethodPost,
			path:   "/wxa/modify_domain",
			token:  at,
			body:   map[string]any{"action": "get"},
		},
		{
			name:   "modify_jump_domain_add",
			call:   func(c *Client) error { return c.ModifyJumpDomain(ctx, at, "wxa1", "add", []string{"https://m.qq.com"}) },
			method: http.MethodPost,
			path:   "/wxa/setwebviewdomain",
			token:  at,
			body:   map[string]any{"action": "add", "webviewdomain": []any{"https://m.qq.com"}},
		},
		{
			name: "modify_server_domain_directly_set",
			call: func(c *Client) error {
				return c.ModifyServerDomainDirectly(ctx, at, "wxa1", "set", &DomainSet{RequestDomain: []string{"https://direct.example.com"}})
			},
			method: http.MethodPost,
			path:   "/wxa/modify_domain_directly",
			token:  at,
			body:   map[string]any{"action": "set", "requestdomain": []any{"https://direct.example.com"}},
		},
		{
			name: "modify_jump_domain_directly_add",
			call: func(c *Client) error {
				_, err := c.ModifyJumpDomainDirectly(ctx, at, "wxa1", "add", []string{"https://direct.qq.com"})
				return err
			},
			method: http.MethodPost,
			path:   "/wxa/setwebviewdomain_directly",
			token:  at,
			body:   map[string]any{"action": "add", "webviewdomain": []any{"https://direct.qq.com"}},
		},
		{
			// 官方单页文档为 POST，且需要空 JSON（44002）
			name:    "effective_server_domain_post_empty_json",
			call:    func(c *Client) error { _, err := c.GetEffectiveServerDomain(ctx, at, "wxa1"); return err },
			method:  http.MethodPost,
			path:    "/wxa/get_effective_domain",
			token:   at,
			rawBody: "{}",
		},
		{
			name:    "effective_jump_domain_post_empty_json",
			call:    func(c *Client) error { _, err := c.GetEffectiveJumpDomain(ctx, at, "wxa1"); return err },
			method:  http.MethodPost,
			path:    "/wxa/get_effective_webviewdomain",
			token:   at,
			rawBody: "{}",
		},
		{
			name:    "jump_domain_confirmfile_post_empty_json",
			call:    func(c *Client) error { _, _, err := c.GetJumpDomainConfirmFile(ctx, at, "wxa1"); return err },
			method:  http.MethodPost,
			path:    "/wxa/get_webviewdomain_confirmfile",
			token:   at,
			rawBody: "{}",
		},
		{
			name: "third_party_server_domain",
			call: func(c *Client) error {
				_, err := c.ModifyThirdPartyServerDomain(ctx, ct, "add", "www.qq.com;wx.qq.com", true)
				return err
			},
			method: http.MethodPost,
			path:   "/cgi-bin/component/modify_wxa_server_domain",
			token:  ct,
			body: map[string]any{
				"action":                       "add",
				"wxa_server_domain":            "www.qq.com;wx.qq.com",
				"is_modify_published_together": true,
			},
		},
		{
			// get 时只发 action
			name:   "third_party_server_domain_get",
			call:   func(c *Client) error { _, err := c.ModifyThirdPartyServerDomain(ctx, ct, "get", "", false); return err },
			method: http.MethodPost,
			path:   "/cgi-bin/component/modify_wxa_server_domain",
			token:  ct,
			body:   map[string]any{"action": "get"},
		},
		{
			name: "third_party_jump_domain",
			call: func(c *Client) error {
				_, err := c.ModifyThirdPartyJumpDomain(ctx, ct, "set", "www.qq.com", false)
				return err
			},
			method: http.MethodPost,
			path:   "/cgi-bin/component/modify_wxa_jump_domain",
			token:  ct,
			body: map[string]any{
				"action":                       "set",
				"wxa_jump_h5_domain":           "www.qq.com",
				"is_modify_published_together": false,
			},
		},
		{
			name:    "third_party_jump_confirmfile_post_empty_json",
			call:    func(c *Client) error { _, _, err := c.GetThirdPartyJumpDomainConfirmFile(ctx, ct); return err },
			method:  http.MethodPost,
			path:    "/cgi-bin/component/get_domain_confirmfile",
			token:   ct,
			rawBody: "{}",
		},
	}

	require.GreaterOrEqual(t, len(cases), 12, "至少覆盖 12 个接口")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client, cap := newServer(t, -1, replyOK)
			require.NoError(t, tc.call(client))

			got := cap.last(t)
			require.Equal(t, tc.method, got.Method, "HTTP 方法不符")
			require.Equal(t, tc.path, got.Path, "URL 路径不符")
			require.Equal(t, tc.token, got.Query.Get("access_token"), "access_token 必须只在 query 中")

			for k, v := range tc.query {
				require.Equal(t, v, got.Query.Get(k), "query 参数 %s", k)
			}

			if tc.rawBody != "" {
				require.Equal(t, tc.rawBody, strings.TrimSpace(string(got.Body)), "必须发送 %s 而不是空 body", tc.rawBody)
				return
			}
			if tc.body == nil {
				if tc.method == http.MethodGet {
					require.Empty(t, got.Body, "GET 请求不应带 body")
				}
				return
			}
			var gotBody map[string]any
			require.NoError(t, json.Unmarshal(got.Body, &gotBody), "请求体应为 JSON")
			requireSubset(t, tc.body, gotBody, tc.path)
			// token 不应出现在 body 中
			require.NotContains(t, string(got.Body), "access_token", "access_token 只能作为 query 参数")
		})
	}
}

// TestUploadMediaMultipart 校验 multipart/form-data 上传（字段名固定 media）。
func TestUploadMediaMultipart(t *testing.T) {
	client, cap := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","type":"image","mediaid":"MEDIA-ID-1"}`))
	mediaID, err := client.UploadMedia(context.Background(), "AT", "wxa1", "shot.png", []byte("PNGDATA"), "image/png")
	require.NoError(t, err)
	require.Equal(t, "MEDIA-ID-1", mediaID)

	got := cap.last(t)
	require.Equal(t, http.MethodPost, got.Method)
	require.Equal(t, "/wxa/uploadmedia", got.Path)
	require.Equal(t, "AT", got.Query.Get("access_token"))
	require.True(t, strings.HasPrefix(got.ContentType, "multipart/form-data; boundary="), "必须是 multipart/form-data，实际 %q", got.ContentType)
	body := string(got.Body)
	require.Contains(t, body, `name="media"`)
	require.Contains(t, body, `filename="shot.png"`)
	require.Contains(t, body, "image/png")
	require.Contains(t, body, "PNGDATA")
}

// ---------------------------------------------------------------------------
// 2. 错误路径：errcode != 0 / HTTP 非 2xx / 非法 JSON
// ---------------------------------------------------------------------------

// TestErrorClasses errcode != 0 时返回 *APIError 且分类正确。
func TestErrorClasses(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		code  int
		class model.ErrorClass
		msg   string
	}{
		{85085, model.ClassRateLimited, "submit audit reach limit"},
		{61004, model.ClassEnvironment, "access clientip is not registered"},
		{9402203, model.ClassPermanent, "标准模板extjson错误"},
		{40001, model.ClassTokenExpired, "invalid credential access_token isinvalid or not latest"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("errcode_%d", tc.code), func(t *testing.T) {
			payload := fmt.Sprintf(`{"errcode":%d,"errmsg":%q}`, tc.code, tc.msg)
			client, _ := newServer(t, -1, replyJSON(payload))

			_, err := client.SubmitAudit(ctx, "AT", "wxa1", SubmitAuditRequest{
				ItemList: []SubmitAuditItem{{FirstClass: "工具", SecondClass: "备忘录", FirstID: 1, SecondID: 2}},
			})
			require.Error(t, err)

			apiErr, ok := IsAPIError(err)
			require.True(t, ok, "应返回 *APIError，实际 %T", err)
			require.Equal(t, tc.code, apiErr.Errcode)
			require.Equal(t, tc.msg, apiErr.Errmsg)
			require.Equal(t, tc.class, apiErr.Class())
			require.Equal(t, tc.class, model.ClassifyErrcode(tc.code))
			require.Equal(t, "/wxa/submit_audit", apiErr.Endpoint)
			require.Equal(t, http.MethodPost, apiErr.Method)
			require.Equal(t, 200, apiErr.HTTPStatus)
			require.Contains(t, apiErr.Raw, fmt.Sprintf(`"errcode":%d`, tc.code))

			// 错误串必须含 errcode、官方 errmsg、本地中文说明
			text := apiErr.Error()
			require.Contains(t, text, fmt.Sprintf("errcode=%d", tc.code))
			require.Contains(t, text, tc.msg)
			require.Contains(t, text, model.ErrcodeText(tc.code))
			require.Contains(t, text, "分类=")

			// 直接作为 error 使用同样可识别
			_, ok = IsAPIError(fmt.Errorf("包装: %w", err))
			require.True(t, ok, "IsAPIError 需支持包装链")
			if _, ok := IsAPIError(nil); ok {
				t.Fatal("IsAPIError(nil) 应为 false")
			}
		})
	}
}

// TestHTTP500 与非法 JSON 的错误路径。
func TestHTTPErrorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("http_500", func(t *testing.T) {
		client, _ := newServer(t, -1, func(w http.ResponseWriter, r *http.Request, body []byte) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal server error"))
		})
		err := client.Release(ctx, "AT", "wxa1")
		require.Error(t, err)
		apiErr, ok := IsAPIError(err)
		require.True(t, ok)
		require.Equal(t, 500, apiErr.HTTPStatus)
		require.Equal(t, -1, apiErr.Errcode, "无法解析 errcode 时用 -1")
		require.Contains(t, apiErr.Raw, "internal server error")
		require.Equal(t, model.ClassRetryable, apiErr.Class())
		require.Contains(t, apiErr.Error(), "HTTP 状态码 500")
	})

	t.Run("http_500_with_json_body", func(t *testing.T) {
		client, _ := newServer(t, -1, func(w http.ResponseWriter, r *http.Request, body []byte) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"errcode":42001,"errmsg":"access_token expired"}`))
		})
		_, err := client.GetPage(ctx, "AT", "wxa1")
		apiErr, ok := IsAPIError(err)
		require.True(t, ok)
		require.Equal(t, 502, apiErr.HTTPStatus)
		require.Equal(t, 42001, apiErr.Errcode, "HTTP 非 2xx 时仍应尽量取到微信 errcode")
		require.Equal(t, model.ClassTokenExpired, apiErr.Class())
	})

	t.Run("invalid_json", func(t *testing.T) {
		client, _ := newServer(t, -1, func(w http.ResponseWriter, r *http.Request, body []byte) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("<html>not json</html>"))
		})
		_, err := client.GetPage(ctx, "AT", "wxa1")
		require.Error(t, err)
		apiErr, ok := IsAPIError(err)
		require.True(t, ok)
		require.Equal(t, -1, apiErr.Errcode)
		require.Contains(t, apiErr.Errmsg, "解析")
		require.Contains(t, apiErr.Raw, "not json")
	})

	t.Run("json_body_type_mismatch", func(t *testing.T) {
		// errcode == 0 但字段类型不符（page_list 期望数组）
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","page_list":"not-an-array"}`))
		_, err := client.GetPage(ctx, "AT", "wxa1")
		require.Error(t, err)
		apiErr, ok := IsAPIError(err)
		require.True(t, ok)
		require.Equal(t, -1, apiErr.Errcode)
		require.Contains(t, apiErr.Errmsg, "解析微信响应失败")
	})

	t.Run("context_timeout", func(t *testing.T) {
		client, _ := newServer(t, -1, func(w http.ResponseWriter, r *http.Request, body []byte) {
			time.Sleep(200 * time.Millisecond)
			replyOK(w, r, body)
		})
		ctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
		defer cancel()
		_, err := client.GetPage(ctx, "AT", "wxa1")
		apiErr, ok := IsAPIError(err)
		require.True(t, ok)
		require.Equal(t, -1, apiErr.Errcode)
		require.True(t, errors.Is(err, context.DeadlineExceeded), "应能用 errors.Is 判断超时")
	})
}

// ---------------------------------------------------------------------------
// 3. CallLogger：被调用、且不落令牌原文
// ---------------------------------------------------------------------------

type memLogger struct {
	mu   sync.Mutex
	recs []CallRecord
}

func (l *memLogger) LogCall(rec CallRecord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recs = append(l.recs, rec)
}

func (l *memLogger) all() []CallRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]CallRecord, len(l.recs))
	copy(out, l.recs)
	return out
}

func TestCallLoggerRedaction(t *testing.T) {
	const (
		componentToken = "COMPONENT-TOKEN-PLAINTEXT"
		componentSec   = "COMPONENT-SECRET-PLAINTEXT"
		refreshTok     = "REFRESH-TOKEN-PLAINTEXT"
	)
	ctx := context.Background()

	logger := &memLogger{}
	payload := fmt.Sprintf(`{"errcode":0,"errmsg":"ok","authorization_info":{
		"authorizer_appid":"wxa1",
		"authorizer_access_token":"AUTHORIZER-TOKEN-PLAINTEXT",
		"expires_in":7200,
		"authorizer_refresh_token":%q,
		"func_info":[{"funcscope_category":{"id":18,"type":0,"name":"小程序开发","desc":"开发管理与数据分析"}}]}}`, refreshTok)
	client, cap := newServer(t, -1, replyJSON(payload))
	client.logger = logger

	resp, err := client.QueryAuth(ctx, componentToken, "wxc1", "AUTHCODE")
	require.NoError(t, err)
	require.Equal(t, "AUTHORIZER-TOKEN-PLAINTEXT", resp.AuthorizationInfo.AuthorizerAccessToken)
	require.Equal(t, refreshTok, resp.AuthorizationInfo.AuthorizerRefreshToken)

	recs := logger.all()
	require.Len(t, recs, 1, "每次调用都要交给 Logger")
	rec := recs[0]
	require.Equal(t, "/cgi-bin/component/api_query_auth", rec.Endpoint)
	require.Equal(t, http.MethodPost, rec.Method)
	require.Equal(t, model.TokenScopeComponent, rec.Scope)
	require.Equal(t, "wxc1", rec.Appid)
	require.Equal(t, 200, rec.HTTPStatus)
	require.True(t, rec.OK)
	require.Equal(t, 0, rec.Errcode)
	require.Equal(t, "", rec.Errmsg)
	require.GreaterOrEqual(t, rec.DurationMs, int64(0))

	// token 确实被送到了 query（功能正确）
	require.Equal(t, componentToken, cap.last(t).Query.Get("access_token"))

	// 记录里绝对不能有令牌原文
	dump, err := json.Marshal(rec.Request)
	require.NoError(t, err)
	require.NotContains(t, string(dump), componentToken)
	require.Equal(t, maskedValue, rec.Request["access_token"])
	require.Equal(t, "wxc1", rec.Request["component_appid"])
	require.Equal(t, "AUTHCODE", rec.Request["authorization_code"])

	respDump, err := json.Marshal(rec.Response)
	require.NoError(t, err)
	require.NotContains(t, string(respDump), "AUTHORIZER-TOKEN-PLAINTEXT")
	require.NotContains(t, string(respDump), refreshTok)
	require.Contains(t, string(respDump), maskedValue)

	// secret / refresh_token 字段同样脱敏
	logger2 := &memLogger{}
	client2, _ := newServer(t, -1, replyOK)
	client2.logger = logger2
	require.NoError(t, client2.StartPushTicket(ctx, StartPushTicketRequest{ComponentAppid: "wxc1", ComponentSecret: componentSec}))
	_, err = client2.AuthorizerToken(ctx, componentToken, "wxc1", "wxa1", refreshTok)
	require.NoError(t, err)
	recs = logger2.all()
	require.Len(t, recs, 2)
	dump2, err := json.Marshal(recs)
	require.NoError(t, err)
	require.NotContains(t, string(dump2), componentSec)
	require.NotContains(t, string(dump2), refreshTok)
	require.NotContains(t, string(dump2), componentToken)

	// 失败调用同样要有记录
	logger3 := &memLogger{}
	client3, _ := newServer(t, -1, replyJSON(`{"errcode":85085,"errmsg":"submit audit reach limit"}`))
	client3.logger = logger3
	_, err = client3.SubmitAudit(ctx, "AT", "wxa1", SubmitAuditRequest{ItemList: []SubmitAuditItem{{FirstClass: "工具", SecondClass: "备忘录", FirstID: 1, SecondID: 2}}})
	require.Error(t, err)
	recs = logger3.all()
	require.Len(t, recs, 1)
	require.False(t, recs[0].OK)
	require.Equal(t, 85085, recs[0].Errcode)
	require.Contains(t, recs[0].Errmsg, "submit audit reach limit")
}

// ---------------------------------------------------------------------------
// 4. get_qrcode：二进制分支与 JSON 错误分支
// ---------------------------------------------------------------------------

func TestGetTrialQRCodeBinary(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 1, 2, 3}
	client, cap := newServer(t, -1, func(w http.ResponseWriter, r *http.Request, body []byte) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-disposition", `attachment; filename="QRCode.jpg"`)
		_, _ = w.Write(png)
	})
	contentType, data, err := client.GetTrialQRCode(context.Background(), "AT", "wxa1", "page/index?action=1")
	require.NoError(t, err)
	require.Equal(t, "image/jpeg", contentType)
	require.Equal(t, png, data)

	got := cap.last(t)
	require.Equal(t, http.MethodGet, got.Method)
	require.Equal(t, "/wxa/get_qrcode", got.Path)
	require.Equal(t, "AT", got.Query.Get("access_token"))
	// path 必须经过 urlencode：? 与 / 都要转义
	require.Contains(t, got.RawQuery, "path=page%2Findex%3Faction%3D1")
}

func TestGetTrialQRCodeJSONError(t *testing.T) {
	client, _ := newServer(t, -1, replyJSON(`{"errcode":40001,"errmsg":"invalid credential access_token isinvalid or not latest"}`))
	contentType, data, err := client.GetTrialQRCode(context.Background(), "AT", "wxa1", "")
	require.Error(t, err)
	require.Empty(t, contentType)
	require.Nil(t, data)

	apiErr, ok := IsAPIError(err)
	require.True(t, ok, "JSON 错误分支也必须返回 *APIError，实际 %T", err)
	require.Equal(t, 40001, apiErr.Errcode)
	require.Equal(t, model.ClassTokenExpired, apiErr.Class())
	require.Equal(t, "/wxa/get_qrcode", apiErr.Endpoint)
}

// ---------------------------------------------------------------------------
// 5. 兼容性：auditid 两种类型、screenshot 两种大小写、func_info 两种嵌套、version_list
// ---------------------------------------------------------------------------

func TestSubmitAuditAuditIDTypes(t *testing.T) {
	ctx := context.Background()

	t.Run("number", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","auditid":1234567890123}`))
		resp, err := client.SubmitAudit(ctx, "AT", "wxa1", SubmitAuditRequest{ItemList: []SubmitAuditItem{{FirstClass: "工具", SecondClass: "备忘录", FirstID: 1, SecondID: 2}}})
		require.NoError(t, err)
		require.Equal(t, int64(1234567890123), resp.AuditID)
	})

	t.Run("string", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","auditid":"1234567890123"}`))
		resp, err := client.SubmitAudit(ctx, "AT", "wxa1", SubmitAuditRequest{ItemList: []SubmitAuditItem{{FirstClass: "工具", SecondClass: "备忘录", FirstID: 1, SecondID: 2}}})
		require.NoError(t, err)
		require.Equal(t, int64(1234567890123), resp.AuditID)
	})

	t.Run("missing", func(t *testing.T) {
		client, _ := newServer(t, -1, replyOK)
		resp, err := client.SubmitAudit(ctx, "AT", "wxa1", SubmitAuditRequest{ItemList: []SubmitAuditItem{{FirstClass: "工具", SecondClass: "备忘录", FirstID: 1, SecondID: 2}}})
		require.NoError(t, err)
		require.Equal(t, int64(0), resp.AuditID)
	})
}

func TestAuditStatusScreenshotCasing(t *testing.T) {
	ctx := context.Background()

	t.Run("lowercase_screenshot", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","auditid":"1001","status":1,"reason":"类目不符","screenshot":"media_a|media_b","user_version":"1.0.0","user_desc":"desc","submit_audit_time":1700000000}`))
		resp, err := client.GetAuditStatus(ctx, "AT", "wxa1", 1001)
		require.NoError(t, err)
		require.Equal(t, int64(1001), resp.AuditID)
		require.Equal(t, 1, resp.Status)
		require.Equal(t, "media_a|media_b", resp.Screenshot)
		require.Empty(t, resp.ScreenShotAlt)
		require.Equal(t, int64(1700000000), resp.SubmitAuditTime)
		require.Equal(t, "1.0.0", resp.UserVersion)
	})

	t.Run("uppercase_screenshot", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","auditid":1002,"status":1,"reason":"类目不符","ScreenShot":"media_c|media_d"}`))
		resp, err := client.GetLatestAuditStatus(ctx, "AT", "wxa1")
		require.NoError(t, err)
		require.Equal(t, int64(1002), resp.AuditID)
		require.Empty(t, resp.Screenshot)
		require.Equal(t, "media_c|media_d", resp.ScreenShotAlt)
	})
}

func TestFuncInfoNestedAndFlat(t *testing.T) {
	ctx := context.Background()

	t.Run("funcscope_category", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","authorization_info":{"authorizer_appid":"wxa1","authorizer_access_token":"AT","expires_in":7200,"authorizer_refresh_token":"RT","func_info":[{"funcscope_category":{"id":18,"type":0,"name":"小程序开发","desc":"开发管理与数据分析"}},{"funcscope_category":{"id":30,"type":0,"name":"类目管理","desc":""}}]}}`))
		resp, err := client.QueryAuth(ctx, "CT", "wxc1", "CODE")
		require.NoError(t, err)
		require.Len(t, resp.AuthorizationInfo.FuncInfo, 2)
		require.Equal(t, 18, resp.AuthorizationInfo.FuncInfo[0].ID)
		require.Equal(t, "小程序开发", resp.AuthorizationInfo.FuncInfo[0].Name)
		require.Equal(t, 30, resp.AuthorizationInfo.FuncInfo[1].ID)
	})

	t.Run("flat", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","authorization_info":{"func_info":[{"id":18,"type":0,"name":"小程序开发","desc":"开发管理与数据分析"}]}}`))
		resp, err := client.QueryAuth(ctx, "CT", "wxc1", "CODE")
		require.NoError(t, err)
		require.Len(t, resp.AuthorizationInfo.FuncInfo, 1)
		require.Equal(t, 18, resp.AuthorizationInfo.FuncInfo[0].ID)
	})
}

func TestVersionListFlexible(t *testing.T) {
	ctx := context.Background()

	t.Run("array", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","version_list":[{"app_version":1,"user_version":"1.0.0","user_desc":"a","commit_time":1700000000},{"app_version":2,"user_version":"1.0.1","user_desc":"b","commit_time":1700000100}]}`))
		list, err := client.ListHistoryVersions(ctx, "AT", "wxa1")
		require.NoError(t, err)
		require.Len(t, list, 2)
		require.Equal(t, int64(2), list[1].AppVersion)
		require.Equal(t, "1.0.1", list[1].UserVersion)
		require.Equal(t, int64(1700000100), list[1].CommitTime)
	})

	t.Run("single_object", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","version_list":{"app_version":3,"user_version":"1.0.2","user_desc":"c","commit_time":1700000200}}`))
		list, err := client.ListHistoryVersions(ctx, "AT", "wxa1")
		require.NoError(t, err)
		require.Len(t, list, 1)
		require.Equal(t, int64(3), list[0].AppVersion)
	})

	t.Run("missing", func(t *testing.T) {
		client, _ := newServer(t, -1, replyOK)
		list, err := client.ListHistoryVersions(ctx, "AT", "wxa1")
		require.NoError(t, err)
		require.Nil(t, list)
	})
}

func TestDomainResponseParsing(t *testing.T) {
	ctx := context.Background()

	t.Run("responsible_server_domain_flatten", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok",
			"mp_domain":{"requestdomain":["https://mp.example.com"],"wsrequestdomain":[],"uploaddomain":[],"downloaddomain":[],"udpdomain":[],"tcpdomain":[]},
			"third_domain":{"requestdomain":["https://third.example.com"]},
			"effective_domain":{"requestdomain":["https://eff.example.com"]}}`))
		got, err := client.GetEffectiveServerDomain(ctx, "AT", "wxa1")
		require.NoError(t, err)
		require.Equal(t, []string{"https://mp.example.com"}, got["mp_domain.requestdomain"])
		require.Equal(t, []string{"https://third.example.com"}, got["third_domain.requestdomain"])
		require.Equal(t, []string{"https://eff.example.com"}, got["effective_domain.requestdomain"])
	})

	t.Run("effective_jump_domain", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","mp_webviewdomain":["https://a.example.com"],"effective_webviewdomain":["https://b.example.com"]}`))
		got, err := client.GetEffectiveJumpDomain(ctx, "AT", "wxa1")
		require.NoError(t, err)
		require.Equal(t, []string{"https://a.example.com"}, got["mp_webviewdomain"])
		require.Equal(t, []string{"https://b.example.com"}, got["effective_webviewdomain"])
	})

	t.Run("modify_jump_domain_directly_webviewdomain", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","webviewdomain":["https://direct.example.com"]}`))
		resp, err := client.ModifyJumpDomainDirectly(ctx, "AT", "wxa1", "get", nil)
		require.NoError(t, err)
		require.Equal(t, []string{"https://direct.example.com"}, resp.WebviewDomain)
	})

	t.Run("modify_server_domain_invalid", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","requestdomain":["https://ok.example.com"],"invalid_requestdomain":["https://bad.example.com"],"no_icp_domain":["bad.example.com"]}`))
		resp, err := client.ModifyServerDomain(ctx, "AT", "wxa1", "set", &DomainSet{RequestDomain: []string{"https://ok.example.com"}})
		require.NoError(t, err)
		require.Equal(t, []string{"https://bad.example.com"}, resp.InvalidRequestDomain)
		require.Equal(t, []string{"bad.example.com"}, resp.NoICPDomain)
	})

	t.Run("confirm_file", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","file_name":"abc.txt","file_content":"hello"}`))
		name, content, err := client.GetJumpDomainConfirmFile(ctx, "AT", "wxa1")
		require.NoError(t, err)
		require.Equal(t, "abc.txt", name)
		require.Equal(t, "hello", content)
	})

	t.Run("third_party_domain_map", func(t *testing.T) {
		client, _ := newServer(t, -1, replyJSON(`{"errcode":0,"errmsg":"ok","published_wxa_server_domain":"www.qq.com","testing_wxa_server_domain":"test.qq.com","invalid_wxa_server_domain":"bad.qq.com"}`))
		got, err := client.ModifyThirdPartyServerDomain(ctx, "CT", "get", "", false)
		require.NoError(t, err)
		require.Equal(t, "0", got["errcode"])
		require.Equal(t, "ok", got["errmsg"])
		require.Equal(t, "www.qq.com", got["published_wxa_server_domain"])
		require.Equal(t, "bad.qq.com", got["invalid_wxa_server_domain"])
	})
}

// ---------------------------------------------------------------------------
// 6. 客户端本身：默认值、QPS 限流
// ---------------------------------------------------------------------------

func TestNewDefaults(t *testing.T) {
	c := New(Options{})
	require.Equal(t, "https://api.weixin.qq.com", c.BaseURL(), "默认域名")
	require.NotNil(t, c.limiter, "MaxQPS == 0 时使用默认值 8（见 New 的注释说明）")
	require.Equal(t, DefaultMaxQPS, c.maxQPS)
	require.Equal(t, DefaultTimeout, c.hc.Timeout)

	c2 := New(Options{BaseURL: "https://example.com/", Timeout: time.Second, MaxQPS: -1})
	require.Equal(t, "https://example.com", c2.BaseURL(), "BaseURL 去掉结尾斜杠")
	require.Nil(t, c2.limiter, "MaxQPS < 0 表示不限流")
	require.Equal(t, time.Second, c2.hc.Timeout)
}

func TestQPSLimit(t *testing.T) {
	client, cap := newServer(t, 2, replyOK)
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 3; i++ {
		require.NoError(t, client.Release(ctx, "AT", "wxa1"))
	}
	elapsed := time.Since(start)
	require.Equal(t, 3, cap.count())
	require.GreaterOrEqual(t, elapsed, 400*time.Millisecond, "QPS=2 时第 3 次调用必须等待补桶（实际 %v）", elapsed)

	// 不限流时不应有明显等待
	unlimited, _ := newServer(t, -1, replyOK)
	start = time.Now()
	for i := 0; i < 3; i++ {
		require.NoError(t, unlimited.Release(ctx, "AT", "wxa1"))
	}
	require.Less(t, time.Since(start), 200*time.Millisecond)
}

func TestLoggerOptional(t *testing.T) {
	// 未设置 Logger 时不应 panic
	client, _ := newServer(t, -1, replyOK)
	require.Nil(t, client.logger)
	require.NoError(t, client.Release(context.Background(), "AT", "wxa1"))
}

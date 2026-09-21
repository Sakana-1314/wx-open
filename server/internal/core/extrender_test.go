package core

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func testContext() RenderContext {
	return RenderContext{
		Appid:        "wx_mock_app_1",
		NickName:     "华星一号店",
		TemplateID:   42,
		TemplateType: 0,
		Seq:          3,
		Now:          time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
		GlobalVars:   map[string]string{"env": "prod", "apiBaseUrl": "https://global.example.com"},
		AppVars:      map[string]string{"storeId": "S001", "env": "staging"},
		JobOverrides: map[string]string{"apiBaseUrl": "https://job.example.com"},
	}
}

// TestRenderExtJSONSimpleMode 简单模式：自动生成 extAppid + ext，且变量优先级为 全局 < 小程序 < 作业。
func TestRenderExtJSONSimpleMode(t *testing.T) {
	res, err := RenderExtJSON("", testContext())
	if err != nil {
		t.Fatalf("简单模式渲染失败: %v", err)
	}
	if res.Ext["extAppid"] != "wx_mock_app_1" {
		t.Fatalf("extAppid 未写入: %v", res.Ext["extAppid"])
	}
	ext, ok := res.Ext["ext"].(map[string]any)
	if !ok {
		t.Fatalf("ext 字段类型异常: %T", res.Ext["ext"])
	}
	if ext["env"] != "staging" {
		t.Fatalf("小程序级变量应覆盖全局变量，实际 %v", ext["env"])
	}
	if ext["apiBaseUrl"] != "https://job.example.com" {
		t.Fatalf("作业级变量应覆盖全部，实际 %v", ext["apiBaseUrl"])
	}
	if ext["storeId"] != "S001" {
		t.Fatalf("缺少小程序级变量 storeId: %v", ext)
	}
	if ext["nick_name"] != "华星一号店" {
		t.Fatalf("缺少内置变量 nick_name: %v", ext)
	}
	// 平台相关内置变量不应进入小程序可见的 ext。
	for _, key := range []string{"appid", "template_id", "seq", "timestamp"} {
		if _, exists := ext[key]; exists {
			t.Fatalf("内置变量 %s 不应写入 ext", key)
		}
	}
	if res.VarSource["env"] != "小程序" || res.VarSource["apiBaseUrl"] != "作业覆盖" {
		t.Fatalf("变量来源标注不正确: %v", res.VarSource)
	}
}

// TestRenderExtJSONAdvancedMode 高级模式：占位符替换 + 保留完整字段。
func TestRenderExtJSONAdvancedMode(t *testing.T) {
	tpl := `{
	  "ext": {"storeId": "{{storeId}}", "apiBaseUrl": "{{apiBaseUrl}}", "appName": "{{nick_name}}"},
	  "pages": ["index", "pages/list/index"],
	  "window": {"navigationBarTitleText": "{{nick_name}}"},
	  "tabBar": {"list": [{"pagePath": "index", "text": "首页"}]},
	  "requiredPrivateInfos": ["getLocation"]
	}`
	res, err := RenderExtJSON(tpl, testContext())
	if err != nil {
		t.Fatalf("高级模式渲染失败: %v", err)
	}
	if res.Ext["extAppid"] != "wx_mock_app_1" {
		t.Fatalf("应自动补 extAppid")
	}
	window := res.Ext["window"].(map[string]any)
	if window["navigationBarTitleText"] != "华星一号店" {
		t.Fatalf("window 占位符未替换: %v", window)
	}
	if !strings.Contains(res.ExtJSON, "requiredPrivateInfos") {
		t.Fatalf("完整字段应被保留: %s", res.ExtJSON)
	}
	// 必须是合法 JSON（能被二次解析），因为要作为字符串提交给 /wxa/commit。
	var round map[string]any
	if err := json.Unmarshal([]byte(res.ExtJSON), &round); err != nil {
		t.Fatalf("ext_json 不是合法 JSON 字符串: %v", err)
	}
}

// TestRenderExtJSONUnknownPlaceholder 未定义变量必须提前报错，避免把 {{xxx}} 发到微信侧。
func TestRenderExtJSONUnknownPlaceholder(t *testing.T) {
	_, err := RenderExtJSON(`{"ext":{"a":"{{not_defined}}"}}`, testContext())
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("期望校验错误，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "not_defined") {
		t.Fatalf("错误信息应指出缺失的变量名: %v", err)
	}
}

// TestRenderExtJSONInvalidJSON 非法 JSON 必须被拦住。
func TestRenderExtJSONInvalidJSON(t *testing.T) {
	if _, err := RenderExtJSON(`{"ext": {`, testContext()); !errors.Is(err, ErrValidation) {
		t.Fatalf("期望校验错误，实际 %v", err)
	}
}

// TestRenderExtJSONStandardTemplateGuard 标准模板只允许三个键（官方 9402203）。
func TestRenderExtJSONStandardTemplateGuard(t *testing.T) {
	ctx := testContext()
	ctx.TemplateType = 1
	_, err := RenderExtJSON(`{"ext":{},"pages":["index"]}`, ctx)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("期望校验错误，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "9402203") {
		t.Fatalf("错误信息应提示 9402203: %v", err)
	}
	// 三个键的组合必须通过。
	if _, err := RenderExtJSON(`{"ext":{"a":"1"},"window":{}}`, ctx); err != nil {
		t.Fatalf("标准模板合法组合不应报错: %v", err)
	}
}

// TestRenderExtJSONEmptyPages 空 pages 会触发官方 85047。
func TestRenderExtJSONEmptyPages(t *testing.T) {
	_, err := RenderExtJSON(`{"ext":{},"pages":[]}`, testContext())
	if !errors.Is(err, ErrValidation) || !strings.Contains(err.Error(), "85047") {
		t.Fatalf("期望 pages 为空被拦下并提示 85047，实际 %v", err)
	}
}

// TestRenderExtJSONBadExtAppid extAppid 不是小程序 appid 时提示。
func TestRenderExtJSONBadExtAppid(t *testing.T) {
	_, err := RenderExtJSON(`{"extAppid":"not-an-appid","ext":{}}`, testContext())
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("期望校验错误，实际 %v", err)
	}
}

// TestRenderPattern 版本号模板渲染。
func TestRenderPattern(t *testing.T) {
	got, err := RenderPattern("v{{date}}-{{seq}}", testContext())
	if err != nil {
		t.Fatalf("渲染失败: %v", err)
	}
	if got != "v2026-03-04-3" {
		t.Fatalf("渲染结果异常: %s", got)
	}
	if out, err := RenderPattern("", testContext()); err != nil || out != "" {
		t.Fatalf("空模式应返回空串: %q %v", out, err)
	}
}

// TestValidateUserVersion 官方限制 user_version ≤64 字符。
func TestValidateUserVersion(t *testing.T) {
	if err := ValidateUserVersion(""); !errors.Is(err, ErrValidation) {
		t.Fatalf("空版本号应报错")
	}
	if err := ValidateUserVersion(strings.Repeat("a", 64)); err != nil {
		t.Fatalf("64 字符应当合法: %v", err)
	}
	if err := ValidateUserVersion(strings.Repeat("a", 65)); !errors.Is(err, ErrValidation) {
		t.Fatalf("65 字符应报错")
	}
	// 中文按字符计数：64 个汉字合法。
	if err := ValidateUserVersion(strings.Repeat("版", 64)); err != nil {
		t.Fatalf("64 个汉字应当合法: %v", err)
	}
}

// TestNormalizeTags 标签规范化：去空、去重、保序。
func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{" 华东 ", "华东", "", "直营"})
	if len(got) != 2 || got[0] != "华东" || got[1] != "直营" {
		t.Fatalf("标签规范化结果异常: %v", got)
	}
}

// TestValidateAuditItemCommon 提审项长度校验（title ≤32，tag 至多 10 个且每个 ≤20）。
func TestValidateAuditItemCommon(t *testing.T) {
	if err := ValidateAuditItemCommon("首页", "门店 点单"); err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if err := ValidateAuditItemCommon(strings.Repeat("标", 33), ""); !errors.Is(err, ErrValidation) {
		t.Fatalf("超长标题应报错")
	}
	if err := ValidateAuditItemCommon("首页", strings.Repeat("t ", 11)); !errors.Is(err, ErrValidation) {
		t.Fatalf("标签超过 10 个应报错")
	}
	if err := ValidateAuditItemCommon("首页", strings.Repeat("标", 21)); !errors.Is(err, ErrValidation) {
		t.Fatalf("单个标签超过 20 字应报错")
	}
}

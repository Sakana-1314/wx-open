package core

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"wx-platform/server/internal/model"
)

// 本文件实现批量下发的个性化核心：把「一份模板代码」适配到「数十个小程序」。
//
// 官方依据（docs/wx-docs/code_commit.md）：
//   - /wxa/commit 的 ext_json 必须是**字符串化的 JSON**（Go 侧需二次编码）；
//   - ext 字段是唯一允许放自定义数据的位置，小程序侧用 wx.getExtConfigSync() 读取；
//   - 普通模板支持全量字段；标准模板（template_type=1）仅支持 {extAppid, ext, window}，
//     传其他参数会报 9402203（标准模板官方已下架，本平台默认只支持普通模板）；
//   - user_version 长度不超过 64 个字符。

// placeholderPattern 匹配 {{变量名}} 占位符。
var placeholderPattern = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.]+)\s*\}\}`)

// maxUserVersionLen 官方限制：user_version 不超过 64 个字符。
const maxUserVersionLen = 64

// RenderContext 渲染上下文：变量合并优先级为 全局 < 小程序级 < 作业级，内置变量优先于同名自定义变量。
type RenderContext struct {
	Appid        string
	NickName     string
	TemplateID   int64
	TemplateType int
	Seq          int
	Now          time.Time
	GlobalVars   map[string]string
	AppVars      map[string]string
	JobOverrides map[string]string
}

// builtinVars 内置变量。
func (c RenderContext) builtinVars() map[string]string {
	now := c.Now
	if now.IsZero() {
		now = time.Now()
	}
	return map[string]string{
		"appid":       c.Appid,
		"nick_name":   c.NickName,
		"template_id": fmt.Sprintf("%d", c.TemplateID),
		"date":        now.Format("2006-01-02"),
		"time":        now.Format("15:04:05"),
		"datetime":    now.Format("2006-01-02 15:04:05"),
		"timestamp":   fmt.Sprintf("%d", now.Unix()),
		"seq":         fmt.Sprintf("%d", c.Seq),
	}
}

// mergedVars 合并变量，返回结果与变量来源说明（便于预览页展示每个值来自哪一层）。
func (c RenderContext) mergedVars() (map[string]string, map[string]string) {
	merged := map[string]string{}
	sources := map[string]string{}
	apply := func(values map[string]string, source string) {
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			merged[k] = values[k]
			sources[k] = source
		}
	}
	apply(c.GlobalVars, "平台默认")
	apply(c.AppVars, "小程序")
	apply(c.JobOverrides, "作业覆盖")
	apply(c.builtinVars(), "内置")
	return merged, sources
}

// substitute 替换占位符；未定义的变量会返回错误（提前拦住「发出去的 ext_json 里还带着 {{xxx}}」）。
func substitute(input string, vars map[string]string) (string, error) {
	var missing []string
	out := placeholderPattern.ReplaceAllStringFunc(input, func(match string) string {
		key := placeholderPattern.FindStringSubmatch(match)[1]
		if value, ok := vars[key]; ok {
			return value
		}
		missing = append(missing, key)
		return match
	})
	if len(missing) > 0 {
		sort.Strings(missing)
		return "", Validation("使用了未定义的变量：%s（可用变量见创建任务页的说明）", strings.Join(uniqueStrings(missing), "、"))
	}
	return out, nil
}

// ExtRenderResult 渲染结果。
type ExtRenderResult struct {
	ExtJSON   string            `json:"extJson"`
	Ext       map[string]any    `json:"ext"`
	Vars      map[string]string `json:"vars"`
	VarSource map[string]string `json:"varSource"`
	Warnings  []string          `json:"warnings"`
}

// RenderExtJSON 生成某个小程序的最终 ext_json。
//
// template 为空时走「简单模式」：自动生成 {extAppid, ext:{合并后的变量}}，
// 覆盖「同一套代码、不同后端地址 / 门店号」这类最常见的多小程序场景。
// template 非空时走「高级模式」：用户提供的 JSON 模板，支持 {{变量}} 占位符与
// pages/extPages/window/tabBar/subPackages/plugins/requiredPrivateInfos 等完整字段。
func RenderExtJSON(template string, ctx RenderContext) (*ExtRenderResult, error) {
	vars, sources := ctx.mergedVars()

	result := &ExtRenderResult{Vars: vars, VarSource: sources}
	var raw string
	if strings.TrimSpace(template) == "" {
		built, err := buildSimpleExtJSON(ctx, vars)
		if err != nil {
			return nil, err
		}
		raw = built
	} else {
		substituted, err := substitute(template, vars)
		if err != nil {
			return nil, err
		}
		raw = substituted
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, Validation("ext_json 不是合法的 JSON 对象：%v（渲染后的内容：%s）", err, truncateForMessage(raw, 300))
	}
	// 二次确认没有残留占位符（例如 JSON 里被转义成 \u007b 之类的情况）。
	if leftover := placeholderPattern.FindAllString(raw, -1); len(leftover) > 0 {
		return nil, Validation("ext_json 中仍残留未替换的占位符：%s", strings.Join(leftover, "、"))
	}

	// 显式写入 extAppid：官方语义是把授权方 appid 合并进 app.json，避免用户漏填。
	if _, ok := parsed["extAppid"]; !ok {
		parsed["extAppid"] = ctx.Appid
	}
	if err := validateExtJSON(parsed, ctx.TemplateType); err != nil {
		return nil, err
	}

	normalized, err := json.Marshal(parsed)
	if err != nil {
		return nil, Internal(fmt.Errorf("序列化 ext_json 失败: %w", err))
	}
	result.Ext = parsed
	result.ExtJSON = string(normalized)
	return result, nil
}

// buildSimpleExtJSON 简单模式：只放 extAppid 与自定义数据。
func buildSimpleExtJSON(ctx RenderContext, vars map[string]string) (string, error) {
	// 内置变量中与平台相关的几项不进 ext，避免小程序侧误用。
	ext := map[string]string{}
	for key, value := range vars {
		switch key {
		case "appid", "template_id", "seq", "timestamp":
			continue
		}
		ext[key] = value
	}
	payload := map[string]any{
		"extAppid": ctx.Appid,
		"ext":      ext,
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return "", Internal(fmt.Errorf("构造 ext_json 失败: %w", err))
	}
	return string(out), nil
}

// allowedStandardTemplateKeys 标准模板（template_type=1）允许的 ext_json 键。
// 官方：标准模板仅支持 {"extAppid":"", "ext": {}, "window": {}}，否则报 9402203。
var allowedStandardTemplateKeys = map[string]bool{
	"extAppid": true,
	"ext":      true,
	"window":   true,
}

// validateExtJSON 校验 ext_json 字段。
func validateExtJSON(parsed map[string]any, templateType int) error {
	if templateType == 1 {
		var illegal []string
		for key := range parsed {
			if !allowedStandardTemplateKeys[key] {
				illegal = append(illegal, key)
			}
		}
		if len(illegal) > 0 {
			sort.Strings(illegal)
			return Validation("标准模板的 ext_json 仅支持 extAppid/ext/window 三个键，"+
				"当前多出：%s（继续提交会被微信拒绝，errcode 9402203）；建议改用普通模板", strings.Join(illegal, "、"))
		}
	}
	if extAppid, ok := parsed["extAppid"]; ok {
		if s, ok := extAppid.(string); ok && s != "" && !strings.HasPrefix(s, "wx") {
			return Validation("extAppid 看起来不是小程序 appid：%s", s)
		}
	}
	if pages, ok := parsed["pages"]; ok {
		list, isList := pages.([]any)
		if !isList || len(list) == 0 {
			// 官方：pages 字段为空会报 85047；有限支持（只能是模板页面的子集）。
			return Validation("ext_json 的 pages 必须是非空数组（为空会报 85047），且只能是模板已有页面的子集")
		}
	}
	return nil
}

// RenderPattern 渲染版本号 / 描述等文本模板（支持 {{date}}、{{nick_name}}、{{seq}} 等）。
func RenderPattern(pattern string, ctx RenderContext) (string, error) {
	if strings.TrimSpace(pattern) == "" {
		return "", nil
	}
	vars, _ := ctx.mergedVars()
	return substitute(pattern, vars)
}

// ValidateUserVersion 校验 user_version：官方限制不超过 64 个字符。
func ValidateUserVersion(version string) error {
	if strings.TrimSpace(version) == "" {
		return Validation("版本号不能为空")
	}
	if len([]rune(version)) > maxUserVersionLen {
		return Validation("版本号长度不能超过 %d 个字符（当前 %d）", maxUserVersionLen, len([]rune(version)))
	}
	return nil
}

// NormalizeTags 规范化标签：去空、去重、保持输入顺序。
func NormalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

// ValidateAuditItemCommon 提审项的公共校验（长度限制来自官方 submit_audit 文档）。
func ValidateAuditItemCommon(title, tag string) error {
	if len([]rune(title)) > 32 {
		return Validation("提审标题不能超过 32 个字符")
	}
	if strings.TrimSpace(tag) != "" {
		tags := strings.Fields(tag)
		if len(tags) > 10 {
			return Validation("提审标签最多 10 个（以空格分隔）")
		}
		for _, t := range tags {
			if len([]rune(t)) > 20 {
				return Validation("单个提审标签不能超过 20 个字符：%s", t)
			}
		}
	}
	return nil
}

// MapExtJSONToModel 把渲染结果转成可落库的 JSONMap（预览与作业载荷用）。
func MapExtJSONToModel(parsed map[string]any) model.JSONMap {
	return model.JSONMap(parsed)
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func truncateForMessage(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

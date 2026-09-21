package wxjob

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// 作业载荷（batch_jobs.payload）的键名。载荷是 JSON 列，任务详情页展示与重试都依赖它。
const (
	payloadKeyType      = "type"
	payloadKeyAppids    = "appids"
	payloadKeySelection = "selection"
	payloadKeyCommit    = "commit"
	payloadKeyAudit     = "audit"
	payloadKeyRelease   = "release"
	payloadKeySingle    = "single"

	// payloadKeyPauseService toggle_visit 作业专用：true=暂停服务（action=close），false=恢复服务（action=open）。
	payloadKeyPauseService = "pause_service"
)

// settingGlobalExtVars 全局 ext 变量的运行参数键（JSON 对象，可选）。
//
// 变量合并优先级：全局（本键） < 小程序 ExtVars < 作业 ExtOverrides，内置变量优先级最高。
const settingGlobalExtVars = "ext_global_vars"

// SingleOptions 单步作业（加急 / 回退 / 服务状态）的附加参数。
//
// 说明：服务状态的首选来源是契约字段 gen.JobCreateRequest.PauseService（服务端会写入
// 载荷的 pause_service）。本结构体是给「契约未覆盖」的场景留的旁路，由 CreateSingle 传入，
// 写入载荷的 single 段；两个来源同时存在时以 pause_service 为准。
type SingleOptions struct {
	// AuditID 指定审核单号（加急用）；为 0 时按 speedup_audit 的取参规则取最近一次审核单。
	AuditID int64
	// AppVersion 指定回退版本号（revert 用）；为 nil 表示回退到上一个版本（官方默认行为）。
	AppVersion *int64
	// VisitAction 服务状态动作：open（可见）/ close（暂停服务）。
	// 优先级低于 gen.JobCreateRequest.PauseService。
	VisitAction string
}

// toMap 转成载荷里的 single 段。
func (o SingleOptions) toMap() map[string]any {
	out := map[string]any{
		"audit_id":     o.AuditID,
		"visit_action": o.VisitAction,
	}
	if o.AppVersion != nil {
		out["app_version"] = *o.AppVersion
	} else {
		out["app_version"] = nil
	}
	return out
}

// commitOptions 上传代码参数（载荷 commit 段）。
type commitOptions struct {
	TemplateID         int64
	ExtTemplate        string
	ExtOverrides       map[string]string
	UserVersionPattern string
	UserDescPattern    string
}

// auditOptions 提审参数（载荷 audit 段）。
type auditOptions struct {
	ProfileID          *uint
	VersionDescPattern string
	PrivacyAPINotUse   *bool
	OrderPath          string
}

// releaseOptions 发布参数（载荷 release 段）。
type releaseOptions struct {
	GrayPercentage          *int
	SupportDebugerFirst     bool
	SupportExperiencerFirst bool
}

// planOptions 一次作业的全部参数（载荷的业务部分）。
type planOptions struct {
	commit  commitOptions
	audit   auditOptions
	release releaseOptions
	single  SingleOptions
	// pauseService toggle_visit 专用：true=暂停服务，false=恢复服务（来自契约的 pauseService）。
	pauseService *bool
}

// optionsFromRequest 从创建请求解析作业参数。
func optionsFromRequest(req gen.JobCreateRequest) planOptions {
	var out planOptions
	if req.Commit != nil {
		out.commit = commitOptions{
			TemplateID:         req.Commit.TemplateId,
			ExtTemplate:        derefString(req.Commit.ExtTemplate),
			ExtOverrides:       derefStringMap(req.Commit.ExtOverrides),
			UserVersionPattern: derefString(req.Commit.UserVersionPattern),
			UserDescPattern:    derefString(req.Commit.UserDescPattern),
		}
	}
	if req.Audit != nil {
		out.audit = auditOptions{
			ProfileID:          uintPtrFromInt64(req.Audit.AuditProfileId),
			VersionDescPattern: derefString(req.Audit.VersionDescPattern),
			PrivacyAPINotUse:   req.Audit.PrivacyApiNotUse,
			OrderPath:          derefString(req.Audit.OrderPath),
		}
	}
	if req.Release != nil {
		out.release = releaseOptions{
			GrayPercentage:          req.Release.GrayPercentage,
			SupportDebugerFirst:     derefBool(req.Release.SupportDebugerFirst),
			SupportExperiencerFirst: derefBool(req.Release.SupportExperiencerFirst),
		}
	}
	out.pauseService = req.PauseService
	return out
}

// optionsFromPayload 从作业载荷还原参数（执行器路径：进程重启后依然可复现请求体）。
func optionsFromPayload(payload model.JSONMap) (planOptions, error) {
	var out planOptions
	if payload == nil {
		return out, errors.New("作业载荷为空：无法复现请求参数，请重新创建作业")
	}
	if cm := jsonMapOf(payload[payloadKeyCommit]); cm != nil {
		out.commit = commitOptions{
			TemplateID:         jsonInt64Of(cm["template_id"]),
			ExtTemplate:        jsonStringOf(cm["ext_template"]),
			ExtOverrides:       jsonStringMapOf(cm["ext_overrides"]),
			UserVersionPattern: jsonStringOf(cm["user_version_pattern"]),
			UserDescPattern:    jsonStringOf(cm["user_desc_pattern"]),
		}
	}
	if am := jsonMapOf(payload[payloadKeyAudit]); am != nil {
		out.audit = auditOptions{
			ProfileID:          uintPtrFromJSON(am["profile_id"]),
			VersionDescPattern: jsonStringOf(am["version_desc_pattern"]),
			PrivacyAPINotUse:   jsonBoolPtrOf(am["privacy_api_not_use"]),
			OrderPath:          jsonStringOf(am["order_path"]),
		}
	}
	if rm := jsonMapOf(payload[payloadKeyRelease]); rm != nil {
		out.release = releaseOptions{
			GrayPercentage:          jsonIntPtrOf(rm["gray_percentage"]),
			SupportDebugerFirst:     jsonBoolOf(rm["support_debuger_first"]),
			SupportExperiencerFirst: jsonBoolOf(rm["support_experiencer_first"]),
		}
	}
	out.pauseService = jsonBoolPtrOf(payload[payloadKeyPauseService])
	if sm := jsonMapOf(payload[payloadKeySingle]); sm != nil {
		out.single = SingleOptions{
			AuditID:     jsonInt64Of(sm["audit_id"]),
			AppVersion:  jsonInt64PtrOf(sm["app_version"]),
			VisitAction: jsonStringOf(sm["visit_action"]),
		}
	}
	return out, nil
}

// mergeSingle 把单步参数并入载荷参数。
func (o planOptions) mergeSingle(single SingleOptions) planOptions {
	if single.AuditID != 0 || single.AppVersion != nil || strings.TrimSpace(single.VisitAction) != "" {
		o.single = single
	}
	return o
}

// toPayload 组装作业载荷（含类型、目标 appid、选择条件与全部业务参数）。
func (o planOptions) toPayload(jobType model.JobType, appids []string, selection gen.AppidSelection) model.JSONMap {
	out := model.JSONMap{
		payloadKeyType:   string(jobType),
		payloadKeyAppids: append([]string{}, appids...),
	}
	if sel := jsonToMap(selection); sel != nil {
		out[payloadKeySelection] = sel
	}
	out[payloadKeyCommit] = map[string]any{
		"template_id":          o.commit.TemplateID,
		"ext_template":         o.commit.ExtTemplate,
		"ext_overrides":        stringMapToAny(o.commit.ExtOverrides),
		"user_version_pattern": o.commit.UserVersionPattern,
		"user_desc_pattern":    o.commit.UserDescPattern,
	}
	audit := map[string]any{
		"version_desc_pattern": o.audit.VersionDescPattern,
		"privacy_api_not_use":  boolPtrToAny(o.audit.PrivacyAPINotUse),
		"order_path":           o.audit.OrderPath,
	}
	if o.audit.ProfileID != nil {
		audit["profile_id"] = *o.audit.ProfileID
	} else {
		audit["profile_id"] = nil
	}
	out[payloadKeyAudit] = audit
	out[payloadKeyRelease] = map[string]any{
		"gray_percentage":           intPtrToAny(o.release.GrayPercentage),
		"support_debuger_first":     o.release.SupportDebugerFirst,
		"support_experiencer_first": o.release.SupportExperiencerFirst,
	}
	out[payloadKeySingle] = o.single.toMap()
	out[payloadKeyPauseService] = boolPtrToAny(o.pauseService)
	return out
}

// payloadAppids 读取载荷里的目标 appid 列表（用于稳定 seq，让变量渲染可复现）。
func payloadAppids(payload model.JSONMap) []string {
	raw, ok := payload[payloadKeyAppids]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		if ss, ok := raw.([]string); ok {
			return ss
		}
		return nil
	}
	out := make([]string, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// seqOf 返回 appid 在载荷列表中的序号（从 1 开始；未登记时返回 1）。
func seqOf(payload model.JSONMap, appid string) int {
	for i, v := range payloadAppids(payload) {
		if v == appid {
			return i + 1
		}
	}
	return 1
}

// ---- 小工具（JSON 与指针转换，载荷往返后数字会变成 float64，读取必须容错） ----

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func derefBool(v *bool) bool { return v != nil && *v }

func derefStringMap(v *map[string]string) map[string]string {
	if v == nil {
		return nil
	}
	return *v
}

func uintPtrFromInt64(v *int64) *uint {
	if v == nil || *v <= 0 {
		return nil
	}
	n := uint(*v)
	return &n
}

func uintPtrFromJSON(v any) *uint {
	n := jsonInt64Of(v)
	if n <= 0 {
		return nil
	}
	u := uint(n)
	return &u
}

func jsonStringOf(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func jsonInt64Of(v any) int64 {
	switch t := v.(type) {
	case nil:
		return 0
	case int:
		return int64(t)
	case int32:
		return int64(t)
	case int64:
		return t
	case uint:
		return int64(t)
	case uint64:
		return int64(t)
	case float64:
		return int64(t)
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			return 0
		}
		return n
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

func jsonIntOf(v any) int { return int(jsonInt64Of(v)) }

func jsonIntPtrOf(v any) *int {
	if v == nil {
		return nil
	}
	n := jsonIntOf(v)
	return &n
}

func jsonInt64PtrOf(v any) *int64 {
	if v == nil {
		return nil
	}
	n := jsonInt64Of(v)
	return &n
}

func jsonBoolOf(v any) bool {
	b, _ := jsonBoolValue(v)
	return b
}

func jsonBoolPtrOf(v any) *bool {
	b, ok := jsonBoolValue(v)
	if !ok {
		return nil
	}
	return &b
}

func jsonBoolValue(v any) (bool, bool) {
	switch t := v.(type) {
	case nil:
		return false, false
	case bool:
		return t, true
	case string:
		s := strings.TrimSpace(strings.ToLower(t))
		if s == "" {
			return false, false
		}
		return s == "true" || s == "1" || s == "yes", true
	case float64:
		return t != 0, true
	case int:
		return t != 0, true
	}
	return false, false
}

func jsonMapOf(v any) map[string]any {
	switch t := v.(type) {
	case nil:
		return nil
	case map[string]any:
		return t
	case model.JSONMap:
		return map[string]any(t)
	}
	return nil
}

func jsonStringMapOf(v any) map[string]string {
	switch t := v.(type) {
	case nil:
		return nil
	case map[string]string:
		return t
	case model.JSONStringMap:
		return map[string]string(t)
	case map[string]any:
		out := make(map[string]string, len(t))
		for k, val := range t {
			out[k] = fmt.Sprint(val)
		}
		return out
	}
	return nil
}

func stringMapToAny(in map[string]string) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func boolPtrToAny(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

func intPtrToAny(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

// jsonToMap 把任意值转成 map[string]any（用于把契约的 selection 原样存进载荷）。
func jsonToMap(v any) map[string]any {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

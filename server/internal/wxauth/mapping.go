package wxauth

import (
	"encoding/json"
	"time"

	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// 本文件是 model ↔ gen(DTO) 的映射，以及 domain_snapshot JSON 列的读写口径。
//
// domain_snapshot 列的结构（本包写入、本包与前端读取）：
//
//	{
//	  "effective":  {requestDomain:[], wsRequestDomain:[], uploadDomain:[], downloadDomain:[],
//	                 udpDomain:[], tcpDomain:[], businessDomain:[]},   // 发布后生效的域名
//	  "registered": {requestDomain:[], businessDomain:[]},              // 第三方平台侧已登记的域名
//	  "missing":    ["serverDomain", "businessDomain"],                 // 本次没取到的项（快照缺失）
//	  "fetchedAt":  "2024-05-01T10:00:00+08:00"
//	}
//
// 键名与 gen.DomainSnapshot 的 JSON 名一致，因此详情接口可以直接反序列化透出。

const (
	snapshotKeyEffective  = "effective"
	snapshotKeyRegistered = "registered"
	snapshotKeyMissing    = "missing"
	snapshotKeyFetchedAt  = "fetchedAt"
	snapshotKeyCategories = "categories"
)

// toAuthorizer 把库里的授权方记录转成列表 DTO（空字符串不转指针，避免前端到处判空）。
func toAuthorizer(a *model.Authorizer) gen.Authorizer {
	out := gen.Authorizer{
		Appid:               a.Appid,
		AuthorizationStatus: gen.AuthorizationStatus(a.AuthorizationStatus),
		Enabled:             a.Enabled,
		HasDevPermission:    a.HasDevPermission(),
		CreatedAt:           a.CreatedAt,
		AuthorizedAt:        a.AuthorizedAt,
		LastSyncAt:          a.LastSyncAt,
		AccountStatus:       intPtr(a.AccountStatus),
		AuditProfileId:      auditProfileIDPtr(a.AuditProfileID),
	}
	out.UpdatedAt = timePtr(a.UpdatedAt)
	out.RefreshTokenUpdatedAt = a.RefreshTokenUpdatedAt
	if a.CodeSource != "" {
		cs := gen.AuthorizerCodeSource(a.CodeSource)
		out.CodeSource = &cs
	}
	out.NickName = optional(a.NickName)
	out.HeadImg = optional(a.HeadImg)
	out.QrcodeUrl = optional(a.QrcodeURL)
	out.UserName = optional(a.UserName)
	if len(a.FuncInfo) > 0 {
		ids := append([]int(nil), a.FuncInfo...)
		out.FuncInfoIds = &ids
	}
	out.GroupName = optional(a.GroupName)
	out.Remark = optional(a.Remark)
	if len(a.Tags) > 0 {
		tags := append([]string(nil), a.Tags...)
		out.Tags = &tags
	}
	if len(a.ExtVars) > 0 {
		vars := make(map[string]string, len(a.ExtVars))
		for k, v := range a.ExtVars {
			vars[k] = v
		}
		out.ExtVars = &vars
	}
	out.PrincipalName = optional(a.PrincipalName)
	out.Signature = optional(a.Signature)
	return out
}

// toAuthorizerDetail 把库里的授权方记录转成详情 DTO（含权限集、类目、域名快照与体检信息）。
func toAuthorizerDetail(a *model.Authorizer) *gen.AuthorizerDetail {
	out := &gen.AuthorizerDetail{
		Appid:                    a.Appid,
		AuthorizationStatus:      gen.AuthorizationStatus(a.AuthorizationStatus),
		Enabled:                  a.Enabled,
		HasDevPermission:         a.HasDevPermission(),
		CreatedAt:                a.CreatedAt,
		AuthorizedAt:             a.AuthorizedAt,
		LastSyncAt:               a.LastSyncAt,
		LastPreflightAt:          a.LastPreflightAt,
		RefreshTokenUpdatedAt:    a.RefreshTokenUpdatedAt,
		AccountStatus:            intPtr(a.AccountStatus),
		ServiceTypeId:            intPtr(a.ServiceTypeID),
		VerifyTypeId:             intPtr(a.VerifyTypeID),
		RegisterType:             intPtr(a.RegisterType),
		AuditProfileId:           auditProfileIDPtr(a.AuditProfileID),
		PrivacySettingConfigured: a.PrivacyConfigured,
	}
	out.UpdatedAt = timePtr(a.UpdatedAt)
	if a.CodeSource != "" {
		cs := gen.AuthorizerDetailCodeSource(a.CodeSource)
		out.CodeSource = &cs
	}
	out.NickName = optional(a.NickName)
	out.HeadImg = optional(a.HeadImg)
	out.QrcodeUrl = optional(a.QrcodeURL)
	out.UserName = optional(a.UserName)
	out.Alias = optional(a.Alias)
	out.PrincipalName = optional(a.PrincipalName)
	out.Signature = optional(a.Signature)
	out.GroupName = optional(a.GroupName)
	out.Remark = optional(a.Remark)
	if len(a.FuncInfo) > 0 {
		ids := append([]int(nil), a.FuncInfo...)
		out.FuncInfoIds = &ids
	}
	if len(a.Tags) > 0 {
		tags := append([]string(nil), a.Tags...)
		out.Tags = &tags
	}
	if len(a.ExtVars) > 0 {
		vars := make(map[string]string, len(a.ExtVars))
		for k, v := range a.ExtVars {
			vars[k] = v
		}
		out.ExtVars = &vars
	}
	if len(a.BusinessInfo) > 0 {
		info := map[string]any(a.BusinessInfo)
		out.BusinessInfo = &info
	}
	if len(a.AuditOverride) > 0 {
		override := map[string]any(a.AuditOverride)
		out.AuditOverride = &override
	}
	out.MiniProgramCategories = categoryPairs(a.MiniProgramCats)
	out.EffectiveDomains = snapshotPart(a.DomainSnapshot, snapshotKeyEffective)
	out.RegisteredDomains = snapshotPart(a.DomainSnapshot, snapshotKeyRegistered)
	return out
}

// toCodeDraft 草稿箱条目 → DTO。
func toCodeDraft(d *model.CodeDraft) gen.CodeDraft {
	out := gen.CodeDraft{
		DraftId:                d.DraftID,
		SourceMiniProgram:      optional(d.SourceMiniProgram),
		SourceMiniProgramAppid: optional(d.SourceMiniProgramAppid),
		UserVersion:            optional(d.UserVersion),
		UserDesc:               optional(d.UserDesc),
		Developer:              optional(d.Developer),
		SyncedAt:               d.SyncedAt,
	}
	if d.CreateTime != 0 {
		out.CreateTime = &d.CreateTime
	}
	return out
}

// toCodeTemplate 模板库条目 → DTO。
func toCodeTemplate(t *model.CodeTemplate) gen.CodeTemplate {
	out := gen.CodeTemplate{
		TemplateId:             t.TemplateID,
		TemplateType:           gen.CodeTemplateTemplateType(t.TemplateType),
		SourceMiniProgram:      optional(t.SourceMiniProgram),
		SourceMiniProgramAppid: optional(t.SourceMiniProgramAppid),
		UserVersion:            optional(t.UserVersion),
		UserDesc:               optional(t.UserDesc),
		Developer:              optional(t.Developer),
		IsDefault:              t.IsDefault,
		SyncedAt:               t.SyncedAt,
	}
	if t.DraftID != 0 {
		out.DraftId = &t.DraftID
	}
	if t.CreateTime != 0 {
		out.CreateTime = &t.CreateTime
	}
	out.Note = optional(t.Note)
	return out
}

// optional 非空才转指针。
func optional(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// auditProfileIDPtr 把模型里的 *uint 提审配置 id 转成契约的 *int64（0 视为未设置）。
func auditProfileIDPtr(id *uint) *int64 {
	if id == nil || *id == 0 {
		return nil
	}
	v := int64(*id)
	return &v
}

// snapshotPart 读取 domain_snapshot 里的某一组域名。
func snapshotPart(m model.JSONMap, key string) *gen.DomainSnapshot {
	if len(m) == 0 {
		return nil
	}
	raw, ok := m[key]
	if !ok || raw == nil {
		return nil
	}
	return decodeDomainSnapshot(raw)
}

// decodeDomainSnapshot 把任意 JSON 值解码成域名快照（写库前是 map[string]any，读库后是 []any）。
func decodeDomainSnapshot(raw any) *gen.DomainSnapshot {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var snap gen.DomainSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil
	}
	if snap.RequestDomain == nil && snap.WsRequestDomain == nil && snap.UploadDomain == nil &&
		snap.DownloadDomain == nil && snap.UdpDomain == nil && snap.TcpDomain == nil &&
		snap.BusinessDomain == nil {
		return nil
	}
	return &snap
}

// categoryPairs 读取小程序类目快照（结构见 syncOne：{"categories":[{"first","second"}]}）。
func categoryPairs(m model.JSONMap) *[]gen.CategoryPair {
	if len(m) == 0 {
		return nil
	}
	raw, ok := m[snapshotKeyCategories]
	if !ok || raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var list []gen.CategoryPair
	if err := json.Unmarshal(b, &list); err != nil || len(list) == 0 {
		return nil
	}
	return &list
}

// ---- 域名快照构造 ----

// serverDomainSnapshot 从 get_effective_domain 的扁平结果（"组.类型" → 域名列表）里挑出「发布后生效」的那一组。
func serverDomainSnapshot(flat map[string][]string) map[string]any {
	for _, group := range []string{"effective_domain", "mp_domain", "third_domain", "direct_domain"} {
		if out := domainGroupSnapshot(flat, group); out != nil {
			return out
		}
	}
	return nil
}

// domainGroupSnapshot 取指定组（mp_domain / third_domain / direct_domain / effective_domain）的域名。
func domainGroupSnapshot(flat map[string][]string, group string) map[string]any {
	fields := map[string]string{
		"requestdomain":   "requestDomain",
		"wsrequestdomain": "wsRequestDomain",
		"uploaddomain":    "uploadDomain",
		"downloaddomain":  "downloadDomain",
		"udpdomain":       "udpDomain",
		"tcpdomain":       "tcpDomain",
	}
	out := map[string]any{}
	for key, field := range fields {
		if v := flat[group+"."+key]; len(v) > 0 {
			out[field] = append([]string(nil), v...)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// businessDomainOf 从 get_effective_webviewdomain 的结果里挑出「发布后生效」的业务域名。
func businessDomainOf(flat map[string][]string) []string {
	for _, key := range []string{"effective_webviewdomain", "mp_webviewdomain", "third_webviewdomain", "direct_webviewdomain"} {
		if v := flat[key]; len(v) > 0 {
			return append([]string(nil), v...)
		}
	}
	return nil
}

// mergeDomainSnapshot 把本次取到的域名并入上一次的快照：取不到的项保留旧值并记入 missing，
// 这样「拉取失败」不会把上一次已知的域名信息抹掉（同步失败不致命，只标记快照缺失）。
func mergeDomainSnapshot(existing model.JSONMap, effective, registered map[string]any, missing []string) model.JSONMap {
	out := model.JSONMap{}
	for k, v := range existing {
		out[k] = v
	}
	if len(effective) > 0 {
		out[snapshotKeyEffective] = mergeDomainFields(out[snapshotKeyEffective], effective)
	}
	if len(registered) > 0 {
		out[snapshotKeyRegistered] = mergeDomainFields(out[snapshotKeyRegistered], registered)
	}
	out[snapshotKeyFetchedAt] = time.Now().Format(time.RFC3339)
	if len(missing) > 0 {
		out[snapshotKeyMissing] = missing
	} else {
		delete(out, snapshotKeyMissing)
	}
	return out
}

// mergeDomainFields 逐字段合并：新取到的字段覆盖旧值，未取到的字段保留旧值。
func mergeDomainFields(old any, fresh map[string]any) map[string]any {
	merged := map[string]any{}
	if old != nil {
		if snap := decodeDomainSnapshot(old); snap != nil {
			b, err := json.Marshal(snap)
			if err == nil {
				_ = json.Unmarshal(b, &merged)
			}
		}
	}
	for k, v := range fresh {
		merged[k] = v
	}
	return merged
}

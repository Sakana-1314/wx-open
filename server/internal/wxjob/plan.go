package wxjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
	"wx-platform/server/internal/wxapi"
)

// 各步骤将调用的微信接口路径（预览页展示用）。
const (
	endpointCommit        = "/wxa/commit"
	endpointPrivacyCheck  = "/wxa/security/get_code_privacy_info"
	endpointSubmitAudit   = "/wxa/submit_audit"
	endpointRelease       = "/wxa/release"
	endpointGrayRelease   = "/wxa/grayrelease"
	endpointAuditStatus   = "/wxa/get_latest_auditstatus"
	endpointUndoAudit     = "/wxa/undocodeaudit"
	endpointSpeedUpAudit  = "/wxa/speedupaudit"
	endpointRevertRelease = "/wxa/revertcoderelease"
	endpointVisitStatus   = "/wxa/change_visitstatus"
)

// maxAuditItemCount 官方限制：item_list 1-5 项（超出报 85023）。
const maxAuditItemCount = 5

// plannedStep 某个小程序某一步骤的最终请求体。
type plannedStep struct {
	Step     model.JobStep
	Endpoint string
	Payload  model.JSONMap
}

// plannedItem 某个小程序的完整计划（含该校验不通过的原因）。
type plannedItem struct {
	Appid    string
	NickName string
	Steps    []plannedStep
	Problems []string
}

// valid 报告该项是否通过全部校验。
func (p *plannedItem) valid() bool { return len(p.Problems) == 0 }

// endpoint 首个步骤的接口路径（预览页「将被调用的微信接口」）。
func (p *plannedItem) endpoint() string {
	if len(p.Steps) == 0 {
		return ""
	}
	return p.Steps[0].Endpoint
}

// payload 预览用请求体：单步作业即为该步请求体；多步作业额外带 steps 明细，
// 让详情页一次看全「上传代码→隐私检测→提审→发布」的每个请求体。
func (p *plannedItem) payload() map[string]any {
	if len(p.Steps) == 0 {
		return map[string]any{}
	}
	out := map[string]any{}
	for k, v := range p.Steps[0].Payload {
		out[k] = v
	}
	if len(p.Steps) > 1 {
		steps := make([]map[string]any, 0, len(p.Steps))
		for _, st := range p.Steps {
			body := map[string]any{}
			for k, v := range st.Payload {
				body[k] = v
			}
			steps = append(steps, map[string]any{
				"step":     string(st.Step),
				"endpoint": st.Endpoint,
				"payload":  body,
			})
		}
		out["steps"] = steps
	}
	return out
}

// planContext 一次作业的参数与缓存（模板 / 提审配置）。
//
// 生命周期约定：Preview/Create 在单个请求内顺序使用；执行器每次 Execute 新建一个，
// 不要跨协程共享（引擎会并发执行同一作业的多个子项）。
type planContext struct {
	repos      *core.Repos
	jobType    model.JobType
	opts       planOptions
	globalVars map[string]string

	tplCache map[int64]*model.CodeTemplate
	tplErr   map[int64]error

	profile    *model.AuditProfile
	profileSet bool
	profileErr error
}

// buildPlan 依据创建请求解析目标小程序，并逐项算出最终请求体（**不调用微信**）。
func (s *JobService) buildPlan(ctx context.Context, req gen.JobCreateRequest, single SingleOptions) (*planContext, []plannedItem, error) {
	jobType := model.JobType(req.Type)
	if string(req.Type) == "" || !jobType.Valid() {
		return nil, nil, core.Validation("不支持的作业类型：%q（可选：commit / submit_audit / release / pipeline / "+
			"sync_info / sync_audit_status / undo_audit / speedup_audit / set_domain / revert / toggle_visit）", string(req.Type))
	}
	opts := optionsFromRequest(req).mergeSingle(single)
	if err := checkRequestOptions(jobType, opts, req); err != nil {
		return nil, nil, err
	}

	pc := &planContext{
		repos:      s.repos(),
		jobType:    jobType,
		opts:       opts,
		globalVars: s.globalExtVars(ctx),
		tplCache:   map[int64]*model.CodeTemplate{},
		tplErr:     map[int64]error{},
	}

	authorizers, err := core.ResolveSelection(ctx, s.repos(), req.Selection)
	if err != nil {
		return nil, nil, err
	}

	planned := make([]plannedItem, 0, len(authorizers))
	// 显式指定但本地没有授权记录的小程序也要出现在预览里，
	// 否则用户只会看到「少了几个」而不知道为什么。
	for _, appid := range missingSelectionAppids(req.Selection, authorizers) {
		planned = append(planned, plannedItem{
			Appid:    appid,
			Problems: []string{"未找到该小程序的授权记录：请让商家重新扫码授权，或核对 appid 是否正确"},
		})
	}
	for i := range authorizers {
		planned = append(planned, pc.buildItem(ctx, &authorizers[i], i+1))
	}
	return pc, planned, nil
}

// checkRequestOptions 校验「整体性」参数（缺失时整个请求都不成立）。
func checkRequestOptions(jobType model.JobType, opts planOptions, req gen.JobCreateRequest) error {
	switch jobType {
	case model.JobTypeCommit, model.JobTypePipeline:
		if opts.commit.TemplateID <= 0 {
			return core.Validation("上传代码作业必须指定代码模板（commit.templateId）：请在「代码模板库」页同步模板后选择")
		}
		if strings.TrimSpace(opts.commit.UserVersionPattern) == "" {
			return core.Validation("上传代码作业必须提供版本号模板（commit.userVersionPattern），" +
				"例如 {{date}}-{{seq}}；渲染结果不能超过 64 个字符")
		}
	case model.JobTypeSubmitAudit:
		// 提审配置允许缺省：缺省时取默认提审配置，取不到则在逐项校验里报错。
	case model.JobTypeRelease:
		if opts.release.GrayPercentage != nil && (*opts.release.GrayPercentage < 0 || *opts.release.GrayPercentage > 100) {
			return core.Validation("灰度比例必须是 0-100 的整数（官方返回 85081）")
		}
	case model.JobTypeSetDomain:
		if req.Domain == nil {
			return core.Validation("域名配置作业必须提供 domain 参数")
		}
	}
	return nil
}

// buildItem 计算单个小程序的计划（授权前置 + 各步骤请求体 + 校验原因）。
func (pc *planContext) buildItem(ctx context.Context, a *model.Authorizer, seq int) plannedItem {
	it := plannedItem{Appid: a.Appid, NickName: a.NickName}
	it.Problems = append(it.Problems, authorizerProblems(a)...)

	for _, step := range pc.jobType.Steps() {
		switch step {
		case model.StepCommit:
			planned, _, problems := pc.commitStep(ctx, a, seq)
			it.Steps = append(it.Steps, planned)
			it.Problems = append(it.Problems, problems...)
		case model.StepPrivacyCheck:
			it.Steps = append(it.Steps, plannedStep{
				Step:     model.StepPrivacyCheck,
				Endpoint: endpointPrivacyCheck,
				Payload:  model.JSONMap{},
			})
		case model.StepSubmitAudit:
			planned, _, problems := pc.submitAuditStep(ctx, a, seq)
			it.Steps = append(it.Steps, planned)
			it.Problems = append(it.Problems, problems...)
		case model.StepRelease:
			planned, problems := pc.releaseStep()
			it.Steps = append(it.Steps, planned)
			it.Problems = append(it.Problems, problems...)
		case model.StepSingle:
			singlePayload := model.JSONMap(pc.opts.single.toMap())
			singlePayload[payloadKeyPauseService] = boolPtrToAny(pc.opts.pauseService)
			it.Steps = append(it.Steps, plannedStep{
				Step:     model.StepSingle,
				Endpoint: endpointForJobType(pc.jobType),
				Payload:  singlePayload,
			})
		}
	}
	return it
}

// authorizerProblems 授权前置校验（已授权 / 启用 / 开发权限集 18）。
func authorizerProblems(a *model.Authorizer) []string {
	if a == nil {
		return []string{"授权记录缺失"}
	}
	var problems []string
	if a.AuthorizationStatus != model.AuthStatusAuthorized {
		problems = append(problems, "该小程序已取消授权：请让商家重新扫码授权后再创建作业")
	}
	if !a.Enabled {
		problems = append(problems, "该小程序已在本平台停用：如确需操作请先在「小程序」页启用")
	}
	if !a.HasDevPermission() {
		problems = append(problems, fmt.Sprintf("该小程序未授权「%s」（权限集 %d）：上传代码 / 提审 / 发布都依赖它，"+
			"请让商家重新扫码授权并勾选该权限集", model.PermissionSetNames[model.PermissionSetDev], model.PermissionSetDev))
	}
	return problems
}

// commitRequest /wxa/commit 的请求参数（预览与执行共用同一份，保证「预览即所发」）。
//
// ExtJSON 是**字符串化的 JSON**（官方要求二次编码），预览里按原样展示发给微信的内容。
type commitRequest struct {
	TemplateID  int64
	ExtJSON     string
	UserVersion string
	UserDesc    string
}

// toAPI 转成微信客户端请求结构。
func (r commitRequest) toAPI() wxapi.CommitRequest {
	return wxapi.CommitRequest{
		TemplateID:  r.TemplateID,
		ExtJSON:     r.ExtJSON,
		UserVersion: r.UserVersion,
		UserDesc:    r.UserDesc,
	}
}

// toMap 转成可落库 / 可展示的键值（字段名与微信一致）。
func (r commitRequest) toMap() map[string]any {
	return map[string]any{
		"template_id":  r.TemplateID,
		"ext_json":     r.ExtJSON,
		"user_version": r.UserVersion,
		"user_desc":    r.UserDesc,
	}
}

// commitStep 拼装 /wxa/commit 请求体：template_id / ext_json / user_version / user_desc。
func (pc *planContext) commitStep(ctx context.Context, a *model.Authorizer, seq int) (plannedStep, commitRequest, []string) {
	st := plannedStep{
		Step:     model.StepCommit,
		Endpoint: endpointCommit,
		Payload: model.JSONMap{
			"template_id": pc.opts.commit.TemplateID,
		},
	}
	var problems []string

	tpl, err := pc.template(ctx, pc.opts.commit.TemplateID)
	if err != nil {
		problems = append(problems, fmt.Sprintf("所选模板不存在（template_id=%d）：请先在「代码模板库」页同步模板列表后重试"+
			"（微信会返回 85014）", pc.opts.commit.TemplateID))
	} else {
		st.Payload["template_type"] = tpl.TemplateType
		st.Payload["template_user_version"] = tpl.UserVersion
		if tpl.TemplateType != 0 {
			problems = append(problems, "所选模板是标准模板（template_type=1）：官方已下架该模板类型，"+
				"批量下发会返回 9402203，请改用普通模板（template_type=0）")
		}
	}

	rc := pc.renderContext(ctx, a, seq)

	version, verr := core.RenderPattern(pc.opts.commit.UserVersionPattern, rc)
	if verr != nil {
		problems = append(problems, verr.Error())
	} else {
		st.Payload["user_version"] = version
		if err := core.ValidateUserVersion(version); err != nil {
			problems = append(problems, err.Error())
		}
	}

	desc, derr := core.RenderPattern(pc.opts.commit.UserDescPattern, rc)
	if derr != nil {
		problems = append(problems, derr.Error())
	}
	st.Payload["user_desc"] = desc

	req := commitRequest{TemplateID: pc.opts.commit.TemplateID}
	if version != "" {
		req.UserVersion = version
	}
	req.UserDesc = desc

	ext, eerr := core.RenderExtJSON(pc.opts.commit.ExtTemplate, rc)
	if eerr != nil {
		problems = append(problems, eerr.Error())
	} else {
		st.Payload["ext_json"] = ext.ExtJSON
		req.ExtJSON = ext.ExtJSON
	}
	return st, req, problems
}

// auditRequest /wxa/submit_audit 的请求参数（预览与执行共用同一份）。
type auditRequest struct {
	Items            []auditItem
	VersionDesc      string
	PrivacyAPINotUse *bool
	OrderPath        string
}

// toAPI 转成微信客户端请求结构。
func (r auditRequest) toAPI() wxapi.SubmitAuditRequest {
	items := make([]wxapi.SubmitAuditItem, 0, len(r.Items))
	for _, it := range r.Items {
		items = append(items, it.toAPI())
	}
	return wxapi.SubmitAuditRequest{
		ItemList:         items,
		VersionDesc:      r.VersionDesc,
		PrivacyAPINotUse: r.PrivacyAPINotUse,
		OrderPath:        r.OrderPath,
	}
}

// toMap 转成可落库 / 可展示的键值。
func (r auditRequest) toMap() map[string]any {
	out := map[string]any{
		"item_list":    auditItemsToMaps(r.Items),
		"version_desc": r.VersionDesc,
		"order_path":   r.OrderPath,
	}
	if r.PrivacyAPINotUse != nil {
		out["privacy_api_not_use"] = *r.PrivacyAPINotUse
	}
	return out
}

// submitAuditStep 拼装 /wxa/submit_audit 请求体：item_list / version_desc / privacy_api_not_use / order_path。
func (pc *planContext) submitAuditStep(ctx context.Context, a *model.Authorizer, seq int) (plannedStep, auditRequest, []string) {
	st := plannedStep{Step: model.StepSubmitAudit, Endpoint: endpointSubmitAudit, Payload: model.JSONMap{}}
	var req auditRequest
	var problems []string

	profile, err := pc.auditProfile(ctx)
	if err != nil {
		problems = append(problems, "提审配置不可用："+err.Error()+
			"（请在「提审配置」页新建配置并设为默认，或在创建作业时指定 audit.auditProfileId）")
		return st, req, problems
	}

	items, ierr := mergeAuditItems(profile, a)
	if ierr != nil {
		problems = append(problems, ierr.Error())
	}
	req.Items = items
	st.Payload["item_list"] = auditItemsToMaps(items)
	problems = append(problems, validateAuditItems(items)...)

	rc := pc.renderContext(ctx, a, seq)
	desc, derr := core.RenderPattern(pc.opts.audit.VersionDescPattern, rc)
	if derr != nil {
		problems = append(problems, derr.Error())
	}
	req.VersionDesc = desc
	req.PrivacyAPINotUse = pc.privacyAPINotUse(profile)
	req.OrderPath = pc.orderPath(profile)

	st.Payload["version_desc"] = desc
	st.Payload["privacy_api_not_use"] = boolPtrToAny(req.PrivacyAPINotUse)
	st.Payload["order_path"] = req.OrderPath
	return st, req, problems
}

// releaseStep 拼装 /wxa/release 或 /wxa/grayrelease 请求体。
func (pc *planContext) releaseStep() (plannedStep, []string) {
	st := plannedStep{Step: model.StepRelease, Endpoint: endpointRelease, Payload: model.JSONMap{}}
	gray := pc.opts.release.GrayPercentage
	if gray == nil {
		return st, nil
	}
	st.Endpoint = endpointGrayRelease
	st.Payload["gray_percentage"] = *gray
	st.Payload["support_debuger_first"] = pc.opts.release.SupportDebugerFirst
	st.Payload["support_experiencer_first"] = pc.opts.release.SupportExperiencerFirst

	var problems []string
	if *gray < 0 || *gray > 100 {
		problems = append(problems, "灰度比例必须是 0-100 的整数（官方返回 85081）")
	}
	if *gray == 0 && !pc.opts.release.SupportExperiencerFirst && !pc.opts.release.SupportDebugerFirst {
		problems = append(problems, "灰度比例为 0 时必须指定「先支持体验者」或「先支持开发者」（官方 grayrelease 说明）")
	}
	return st, problems
}

// renderContext 组装变量渲染上下文（全局 < 小程序 ExtVars < 作业 ExtOverrides）。
func (pc *planContext) renderContext(ctx context.Context, a *model.Authorizer, seq int) core.RenderContext {
	rc := core.RenderContext{
		Appid:        a.Appid,
		NickName:     a.NickName,
		TemplateID:   pc.opts.commit.TemplateID,
		Seq:          seq,
		Now:          time.Now(),
		GlobalVars:   pc.globalVars,
		AppVars:      map[string]string(a.ExtVars),
		JobOverrides: pc.opts.commit.ExtOverrides,
	}
	if tpl, err := pc.template(ctx, pc.opts.commit.TemplateID); err == nil {
		rc.TemplateType = tpl.TemplateType
	}
	return rc
}

// template 带缓存的模板查询（一次作业内同一模板只查一次库）。
func (pc *planContext) template(ctx context.Context, id int64) (*model.CodeTemplate, error) {
	if tpl, ok := pc.tplCache[id]; ok {
		return tpl, nil
	}
	if err, ok := pc.tplErr[id]; ok {
		return nil, err
	}
	if pc.repos == nil {
		return nil, errors.New("数据访问未初始化")
	}
	tpl, err := pc.repos.Templates.Get(ctx, id)
	if err != nil {
		pc.tplErr[id] = err
		return nil, err
	}
	pc.tplCache[id] = tpl
	return tpl, nil
}

// auditProfile 解析提审配置：作业指定的 profile_id 优先，其次默认提审配置。
func (pc *planContext) auditProfile(ctx context.Context) (*model.AuditProfile, error) {
	if pc.profileSet {
		return pc.profile, pc.profileErr
	}
	pc.profileSet = true
	if pc.repos == nil {
		pc.profileErr = errors.New("数据访问未初始化")
		return nil, pc.profileErr
	}

	var id *uint
	if pc.opts.audit.ProfileID != nil {
		id = pc.opts.audit.ProfileID
	} else if defaultID := pc.defaultProfileID(ctx); defaultID != nil {
		id = defaultID
	}
	if id == nil {
		pc.profileErr = errors.New("没有指定提审配置，且平台没有默认提审配置")
		return nil, pc.profileErr
	}

	profile, err := pc.repos.Profiles.Get(ctx, *id)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			pc.profileErr = fmt.Errorf("提审配置不存在（id=%d）", *id)
		} else {
			pc.profileErr = err
		}
		return nil, pc.profileErr
	}
	pc.profile = profile
	return profile, nil
}

// defaultProfileID 读取运行参数里的默认提审配置 id。
func (pc *planContext) defaultProfileID(ctx context.Context) *uint {
	if pc.repos == nil {
		return nil
	}
	value, err := pc.repos.Settings.Get(ctx, model.SettingDefaultAuditProfileID)
	if err != nil {
		return nil
	}
	n := jsonInt64Of(value)
	if n <= 0 {
		return nil
	}
	id := uint(n)
	return &id
}

// privacyAPINotUse 决策 privacy_api_not_use：作业显式指定优先，其次提审配置。
func (pc *planContext) privacyAPINotUse(profile *model.AuditProfile) *bool {
	if pc.opts.audit.PrivacyAPINotUse != nil {
		return pc.opts.audit.PrivacyAPINotUse
	}
	if profile != nil {
		return profile.PrivacyAPINotUse
	}
	return nil
}

// orderPath 决策 order_path：作业显式指定优先，其次提审配置。
func (pc *planContext) orderPath(profile *model.AuditProfile) string {
	if strings.TrimSpace(pc.opts.audit.OrderPath) != "" {
		return pc.opts.audit.OrderPath
	}
	if profile != nil {
		return profile.OrderPath
	}
	return ""
}

// globalExtVars 读取全局 ext 变量（运行参数 ext_global_vars，JSON 对象；缺省为空）。
func (s *JobService) globalExtVars(ctx context.Context) map[string]string {
	if s.repos() == nil {
		return nil
	}
	value, err := s.repos().Settings.Get(ctx, settingGlobalExtVars)
	if err != nil || strings.TrimSpace(value) == "" {
		return nil
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

// missingSelectionAppids 返回显式指定但本地没有授权记录的 appid（保持用户给定顺序）。
func missingSelectionAppids(sel gen.AppidSelection, found []model.Authorizer) []string {
	if sel.Appids == nil || len(*sel.Appids) == 0 {
		return nil
	}
	have := make(map[string]bool, len(found))
	for i := range found {
		have[found[i].Appid] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, appid := range *sel.Appids {
		id := strings.TrimSpace(appid)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if !have[id] {
			out = append(out, id)
		}
	}
	return out
}

// endpointForJobType 单步作业对应的接口路径。
func endpointForJobType(t model.JobType) string {
	switch t {
	case model.JobTypeSyncAuditStatus:
		return endpointAuditStatus
	case model.JobTypeUndoAudit:
		return endpointUndoAudit
	case model.JobTypeSpeedUpAudit:
		return endpointSpeedUpAudit
	case model.JobTypeRevert:
		return endpointRevertRelease
	case model.JobTypeToggleVisit:
		return endpointVisitStatus
	}
	return ""
}

// ---- 提审项（item_list）组装与校验 ----

// auditItem 规范化后的提审项。
type auditItem struct {
	Address     string
	Tag         string
	Title       string
	FirstClass  string
	SecondClass string
	ThirdClass  string
	FirstID     int
	SecondID    int
	ThirdID     int
}

// toMap 转成载荷 / 预览里的可读键值（字段名与微信一致）。
func (it auditItem) toMap() map[string]any {
	out := map[string]any{
		"first_class":  it.FirstClass,
		"second_class": it.SecondClass,
		"third_class":  it.ThirdClass,
		"first_id":     it.FirstID,
		"second_id":    it.SecondID,
		"third_id":     it.ThirdID,
	}
	if it.Address != "" {
		out["address"] = it.Address
	}
	if it.Tag != "" {
		out["tag"] = it.Tag
	}
	if it.Title != "" {
		out["title"] = it.Title
	}
	return out
}

// toAPI 转成微信客户端请求结构。
func (it auditItem) toAPI() wxapi.SubmitAuditItem {
	return wxapi.SubmitAuditItem{
		Address:     it.Address,
		Tag:         it.Tag,
		FirstClass:  it.FirstClass,
		SecondClass: it.SecondClass,
		ThirdClass:  it.ThirdClass,
		Title:       it.Title,
		FirstID:     it.FirstID,
		SecondID:    it.SecondID,
		ThirdID:     it.ThirdID,
	}
}

func auditItemsToMaps(items []auditItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, it.toMap())
	}
	return out
}

// mergeAuditItems 由提审配置的 item_list + 小程序的 AuditOverride 合并出最终提审项。
//
// 兼容两种配置形态：整段是一个 item_list 数组（{"item_list":[...]}），或本身就是单个提审项。
// AuditOverride 为单个提审项，逐键覆盖（空值不覆盖）。
func mergeAuditItems(profile *model.AuditProfile, a *model.Authorizer) ([]auditItem, error) {
	if profile == nil {
		return nil, errors.New("提审配置为空")
	}
	raw, err := extractAuditItemMaps(profile.ItemList)
	if err != nil {
		return nil, fmt.Errorf("提审配置「%s」的 item_list 不可用：%w", profile.Name, err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("提审配置「%s」没有配置 item_list（提审至少需要 1 项）", profile.Name)
	}
	override := map[string]any(nil)
	if a != nil && a.AuditOverride != nil {
		override = map[string]any(a.AuditOverride)
	}

	items := make([]auditItem, 0, len(raw))
	for _, one := range raw {
		merged := make(map[string]any, len(one)+len(override))
		for k, v := range one {
			merged[k] = v
		}
		for k, v := range override {
			if isBlankValue(v) {
				continue
			}
			merged[k] = v
		}
		items = append(items, auditItemFromMap(merged))
	}
	return items, nil
}

// extractAuditItemMaps 从配置 JSON 里取出提审项列表。
func extractAuditItemMaps(in model.JSONMap) ([]map[string]any, error) {
	if len(in) == 0 {
		return nil, nil
	}
	for _, key := range []string{"item_list", "items", "list"} {
		raw, ok := in[key]
		if !ok {
			continue
		}
		arr, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("字段 %s 必须是数组", key)
		}
		out := make([]map[string]any, 0, len(arr))
		for _, one := range arr {
			if m := jsonMapOf(one); m != nil {
				out = append(out, m)
			}
		}
		return out, nil
	}
	if hasAuditItemKey(in) {
		return []map[string]any{map[string]any(in)}, nil
	}
	return nil, errors.New("缺少 item_list（应包含 first_class/second_class/first_id/second_id 等字段）")
}

// hasAuditItemKey 判断 JSON 对象是否直接就是一个提审项的字段集合。
func hasAuditItemKey(in map[string]any) bool {
	for _, key := range []string{"first_class", "first_id", "second_class", "second_id", "title", "address"} {
		if _, ok := in[key]; ok {
			return true
		}
	}
	return false
}

// auditItemFromMap 把 JSON 对象读成规范化提审项（兼容数字被解析成 float64）。
func auditItemFromMap(in map[string]any) auditItem {
	return auditItem{
		Address:     jsonStringOf(in["address"]),
		Tag:         jsonStringOf(in["tag"]),
		Title:       jsonStringOf(in["title"]),
		FirstClass:  jsonStringOf(in["first_class"]),
		SecondClass: jsonStringOf(in["second_class"]),
		ThirdClass:  jsonStringOf(in["third_class"]),
		FirstID:     jsonIntOf(in["first_id"]),
		SecondID:    jsonIntOf(in["second_id"]),
		ThirdID:     jsonIntOf(in["third_id"]),
	}
}

// validateAuditItems 提审项校验：数量 1-5、类目六字段、标题与标签长度。
func validateAuditItems(items []auditItem) []string {
	var problems []string
	if len(items) == 0 {
		return []string{"提审项 item_list 不能为空（至少 1 项）"}
	}
	if len(items) > maxAuditItemCount {
		problems = append(problems, fmt.Sprintf("提审项 item_list 最多 %d 项（当前 %d 项，官方返回 85023）",
			maxAuditItemCount, len(items)))
	}
	for i, it := range items {
		label := fmt.Sprintf("第 %d 个提审项", i+1)
		var missing []string
		if strings.TrimSpace(it.FirstClass) == "" {
			missing = append(missing, "first_class")
		}
		if strings.TrimSpace(it.SecondClass) == "" {
			missing = append(missing, "second_class")
		}
		if it.FirstID <= 0 {
			missing = append(missing, "first_id")
		}
		if it.SecondID <= 0 {
			missing = append(missing, "second_id")
		}
		if len(missing) > 0 {
			problems = append(problems, fmt.Sprintf("%s 缺少类目字段（%s 必填，取值必须来自微信 getAllCategoryName）："+
				"请在「提审配置」页补齐类目", label, strings.Join(missing, "、")))
		}
		if it.ThirdID != 0 && strings.TrimSpace(it.ThirdClass) == "" {
			problems = append(problems, fmt.Sprintf("%s 填了 third_id 却没有 third_class：请补齐三级类目名称", label))
		}
		if err := core.ValidateAuditItemCommon(it.Title, it.Tag); err != nil {
			problems = append(problems, fmt.Sprintf("%s：%s", label, err.Error()))
		}
	}
	return problems
}

// isBlankValue 判断覆盖值是否为空（空值不覆盖配置里的原值）。
func isBlankValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(t) == ""
	case float64:
		return t == 0
	case int:
		return t == 0
	case int64:
		return t == 0
	}
	return false
}

package wxaudit

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// PreflightService 下发代码 / 提审 / 发布前的逐项体检。
//
// 体检不是「调用微信的一次操作」，而是把官方文档里最容易踩的硬性前置条件
// （权限集 18/30、昵称头像简介、类目审核、隐私指引、域名登记、服务商额度、模板库）
// 一次性摊开给运维看：pass 可直接执行，warn 可执行但有风险，fail 必须先修，
// unknown 表示本次无法判定（绝不臆断为 fail）。
type PreflightService struct{ base }

// NewPreflightService 构造体检服务。
func NewPreflightService(env *core.Env) *PreflightService {
	return &PreflightService{base: base{env: env}}
}

// 体检项 key（与 api/openapi.yaml 的 PreflightCheck.key 约定一致）与中文名。
const (
	checkAuthorization = "authorization"
	checkDevPermission = "dev_permission"
	checkProfile       = "profile"
	checkCategory      = "category"
	checkPrivacy       = "privacy"
	checkDomains       = "domains"
	checkQuota         = "quota"
	checkTemplate      = "template"

	labelCategory = "类目"
	labelPrivacy  = "用户隐私保护指引"
	labelDomains  = "服务器 / 业务域名"
	labelQuota    = "提审额度"
)

// templateTypeNormal 普通模板（官方唯一可用类型；标准模板已下架）。
const templateTypeNormal = 0

// selectionTarget 体检的操作审计对象类型（作用于一批小程序，而不是单个）。
const selectionTarget = "selection"

// Run 逐个小程序体检，返回逐项结果、汇总计数与服务商额度。
//
// purpose 决定检查项集合与错误码映射：commit 检查模板库，submit_audit 检查服务商提审额度，
// release 只做能直接阻断发布的检查（线上版本情况由发布接口的 85019/85020 提示兜底）。
func (s *PreflightService) Run(ctx context.Context, req gen.PreflightRequest) (*gen.PreflightResponse, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	if !req.Purpose.Valid() {
		return nil, core.Validation("purpose 必须是 commit / submit_audit / release 之一，当前为 %q", string(req.Purpose))
	}
	list, err := core.ResolveSelection(ctx, s.env.Repos, req.Selection)
	if err != nil {
		return nil, mapError(err)
	}

	// 提审额度是服务商级的：一次体检只探测一次，所有小程序复用同一份结果。
	var quota *quotaProbe
	if req.Purpose == gen.PreflightRequestPurposeSubmitAudit {
		quota = s.probeQuota(ctx, list)
	}

	items := make([]gen.PreflightItem, 0, len(list))
	readyCount := 0
	for i := range list {
		item := s.runItem(ctx, &list[i], req, quota)
		if item.Ready {
			readyCount++
		}
		items = append(items, item)
	}

	out := &gen.PreflightResponse{
		Items:        items,
		ReadyCount:   readyCount,
		BlockedCount: len(items) - readyCount,
	}
	if quota != nil && quota.quota != nil {
		out.AuditQuota = quota.quota
	} else {
		out.AuditQuota = s.cachedQuota(ctx)
	}
	s.logOperation(ctx, actionPreflight, selectionTarget, "", model.JSONMap{
		"purpose": string(req.Purpose),
		"total":   len(items),
		"ready":   readyCount,
		"blocked": len(items) - readyCount,
	})
	return out, nil
}

// runItem 对单个小程序执行全部体检项，并回写提审前置快照（隐私配置 + 域名快照）。
func (s *PreflightService) runItem(ctx context.Context, a *model.Authorizer, req gen.PreflightRequest, quota *quotaProbe) gen.PreflightItem {
	item := gen.PreflightItem{Appid: a.Appid, Checks: make([]gen.PreflightCheck, 0, 8)}
	if strings.TrimSpace(a.NickName) != "" {
		item.NickName = strPtr(a.NickName)
	}
	item.Checks = append(item.Checks,
		checkAuthorizationItem(a),
		checkDevPermissionItem(a),
		checkProfileItem(a),
	)

	// 未授权的小程序不逐个接口去撞令牌：既慢，也会把「已取消授权」淹没在一堆令牌错误里。
	var (
		categoryCheck = authSkipItem(checkCategory, labelCategory)
		privacyCheck  = authSkipItem(checkPrivacy, labelPrivacy)
		domainCheck   = authSkipItem(checkDomains, labelDomains)
		snapshot      model.JSONMap
		gotDomains    bool
	)
	if a.AuthorizationStatus == model.AuthStatusAuthorized {
		categoryCheck = s.checkCategoryItem(ctx, a)
		privacyCheck = s.checkPrivacyItem(ctx, a)
		domainCheck, snapshot, gotDomains = s.checkDomainsItem(ctx, a, req.RequiredDomains)
	}
	item.Checks = append(item.Checks, categoryCheck, privacyCheck, domainCheck)

	if quota != nil {
		item.Checks = append(item.Checks, quota.check())
	}
	if req.Purpose == gen.PreflightRequestPurposeCommit {
		item.Checks = append(item.Checks, s.checkTemplateItem(ctx, a))
	}

	// 回写快照：只有真正取到隐私指引结果时才写，否则会把「未取到」误记成「未配置」；
	// repo 层 domains 传 nil 表示本次没取到域名信息（会覆盖旧快照为 NULL，由调用方决定）。
	if privacyCheck.Status != gen.Unknown {
		if err := s.env.Repos.Authorizers.UpdatePreflightSnapshot(ctx, a.Appid, privacyCheck.Status == gen.Pass, snapshot, time.Now()); err != nil {
			log.Printf("[wxaudit] 保存提审前置快照失败(appid=%s): %v", a.Appid, err)
		}
	} else if gotDomains {
		log.Printf("[wxaudit] 未取到隐私指引结果(appid=%s)，跳过提审前置快照写入", a.Appid)
	}

	item.Ready = true
	for _, c := range item.Checks {
		if c.Status == gen.Fail {
			item.Ready = false
			break
		}
	}
	return item
}

// newCheck 构造一个体检项（hint 为空时不输出）。
func newCheck(key, label string, status gen.PreflightCheckStatus, message, hint string) gen.PreflightCheck {
	out := gen.PreflightCheck{Key: key, Label: label, Status: status, Message: message}
	if strings.TrimSpace(hint) != "" {
		h := hint
		out.Hint = &h
	}
	return out
}

// authSkipItem 未授权小程序的占位体检项：不调用微信接口，避免无效调用与误导性报错。
func authSkipItem(key, label string) gen.PreflightCheck {
	return newCheck(key, label, gen.Unknown,
		"未检查：该小程序已取消授权，平台不再代调用其接口（请先让商家重新扫码授权）", "")
}

// checkAuthorizationItem 授权状态：只有 authorized 才能代调用。
func checkAuthorizationItem(a *model.Authorizer) gen.PreflightCheck {
	const key, label = checkAuthorization, "授权状态"
	if a.AuthorizationStatus == model.AuthStatusAuthorized {
		msg := "已授权"
		if a.AuthorizedAt != nil {
			msg = fmt.Sprintf("已授权（授权时间 %s）", a.AuthorizedAt.Format("2006-01-02 15:04:05"))
		}
		return newCheck(key, label, gen.Pass, msg, "")
	}
	return newCheck(key, label, gen.Fail,
		fmt.Sprintf("授权状态为 %s：本平台无法代调用该小程序的接口（上传代码/提审/发布都会失败）", string(a.AuthorizationStatus)),
		"请让商家重新扫码授权（「授权管理」→ 生成授权链接）；取消授权后本地仍保留历史数据，但不会再代调用。")
}

// checkDevPermissionItem 开发权限集（18）：代码上传 / 提审 / 发布的前提，且是互斥权限集。
func checkDevPermissionItem(a *model.Authorizer) gen.PreflightCheck {
	const key, label = checkDevPermission, "开发权限集"
	if a.HasDevPermission() {
		return newCheck(key, label, gen.Pass,
			fmt.Sprintf("已授权「小程序开发与数据分析」（权限集 %d）", model.PermissionSetDev), "")
	}
	return newCheck(key, label, gen.Fail,
		fmt.Sprintf("未授权「小程序开发与数据分析」（权限集 %d）：无法为该小程序上传代码、提审或发布", model.PermissionSetDev),
		"需商家重新扫码授权并勾选该权限集（它是互斥权限集：与其它互斥权限集不能同时勾选）；授权后执行「同步授权信息」刷新本地权限集快照。")
}

// checkProfileItem 昵称 / 头像 / 简介完整性：缺失会导致提审报 86002。
func checkProfileItem(a *model.Authorizer) gen.PreflightCheck {
	const key, label = checkProfile, "小程序资料"
	missing := make([]string, 0, 3)
	if strings.TrimSpace(a.NickName) == "" {
		missing = append(missing, "昵称")
	}
	if strings.TrimSpace(a.HeadImg) == "" {
		missing = append(missing, "头像")
	}
	if strings.TrimSpace(a.Signature) == "" {
		missing = append(missing, "简介")
	}
	if len(missing) == 0 {
		return newCheck(key, label, gen.Pass, "昵称 / 头像 / 简介均已设置", "")
	}
	return newCheck(key, label, gen.Warn,
		fmt.Sprintf("本地快照缺少 %s（来源：最近一次同步的授权信息）", strings.Join(missing, "、")),
		"提审会报 86002（miniprogram have not completed init procedure）：先在小程序后台补全昵称/头像/简介，再执行「同步授权信息」刷新本地快照。")
}

// checkCategoryItem 类目：必须有 audit_status=3（审核通过）的类目，否则提审报 85008。
//
// 该类目接口（GET /cgi-bin/wxopen/getcategory）需要权限集 30「小程序基本信息管理」，
// 与代码管理的 18 不是同一个权限集；调用失败只报 unknown，不臆断为 fail。
func (s *PreflightService) checkCategoryItem(ctx context.Context, a *model.Authorizer) gen.PreflightCheck {
	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return newCheck(checkCategory, labelCategory, gen.Unknown,
			"未检查：无法获取该小程序的调用令牌（"+err.Error()+"）", "")
	}
	resp, err := s.env.Wx.GetSettingCategories(ctx, token, a.Appid)
	if err != nil {
		return newCheck(checkCategory, labelCategory, gen.Unknown,
			"未检查：调用 /cgi-bin/wxopen/getcategory 失败（"+err.Error()+"）",
			fmt.Sprintf("该类目接口需要权限集 %d「小程序基本信息管理」（与代码管理的权限集 %d 不同）：若为权限集缺失，请让商家重新扫码授权并勾选。",
				model.PermissionSetBasicInfo, model.PermissionSetDev))
	}
	if resp == nil {
		return newCheck(checkCategory, labelCategory, gen.Unknown, "未检查：微信未返回类目数据", "")
	}

	approved, pending, rejected := 0, 0, 0
	for _, c := range resp.Categories {
		switch c.AuditStatus {
		case 3:
			approved++
		case 1:
			pending++
		case 2:
			rejected++
		}
	}
	if approved > 0 {
		return newCheck(checkCategory, labelCategory, gen.Pass,
			fmt.Sprintf("已有 %d 个审核通过的类目（共 %d 个：审核中 %d 个、审核不通过 %d 个）",
				approved, len(resp.Categories), pending, rejected), "")
	}
	msg := "该小程序还没有配置任何类目"
	if len(resp.Categories) > 0 {
		msg = fmt.Sprintf("没有审核通过的类目（共 %d 个：审核中 %d 个、审核不通过 %d 个）",
			len(resp.Categories), pending, rejected)
	}
	return newCheck(checkCategory, labelCategory, gen.Fail, msg,
		"先在「类目管理」里添加类目并通过审核：提审 item_list 的类目必须来自已审核通过的类目，否则提审报 85008（当前小程序没有已经审核通过的类目）。")
}

// checkPrivacyItem 用户隐私保护指引：code_exist=1 视为已配置。
//
// 官方只有文字要求、没有专门错误码（属人工审核规则），因此缺失时只报 warn；
// privacy_ver 不传时微信默认取开发版（提审看的就是开发版）。
func (s *PreflightService) checkPrivacyItem(ctx context.Context, a *model.Authorizer) gen.PreflightCheck {
	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return newCheck(checkPrivacy, labelPrivacy, gen.Unknown,
			"未检查：无法获取该小程序的调用令牌（"+err.Error()+"）", "")
	}
	resp, err := s.env.Wx.GetPrivacySetting(ctx, token, a.Appid, nil)
	if err != nil {
		return newCheck(checkPrivacy, labelPrivacy, gen.Unknown,
			"未检查：调用 /cgi-bin/component/getprivacysetting 失败（"+err.Error()+"）", "")
	}
	if resp == nil {
		return newCheck(checkPrivacy, labelPrivacy, gen.Unknown, "未检查：微信未返回隐私指引数据", "")
	}
	if resp.CodeExist == 1 {
		return newCheck(checkPrivacy, labelPrivacy, gen.Pass,
			fmt.Sprintf("已配置用户隐私保护指引（code_exist=1，声明隐私接口 %d 项）", len(resp.PrivacyList)), "")
	}
	return newCheck(checkPrivacy, labelPrivacy, gen.Warn,
		"未配置用户隐私保护指引（code_exist=0）",
		"提审前需配置用户隐私保护指引，否则审核会被驳回（官方没有专门错误码，属于人工审核规则）：在小程序后台「设置 → 服务内容声明 → 用户隐私保护指引」里配置。")
}

// checkDomainsItem 域名：校验要求域名是否已在「发布后生效」的域名集合里。
//
// 授权托管后小程序只能使用第三方平台登记的域名（modify_wxa_server_domain /
// modify_wxa_jump_domain），且域名要在发布上线后才生效；未提供要求域名时只做信息展示（warn）。
// 返回的 snapshot 用于回写 authorizers.domain_snapshot。
func (s *PreflightService) checkDomainsItem(ctx context.Context, a *model.Authorizer, required *gen.DomainRequirements) (gen.PreflightCheck, model.JSONMap, bool) {
	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return newCheck(checkDomains, labelDomains, gen.Unknown,
			"未检查：无法获取该小程序的调用令牌（"+err.Error()+"）", ""), nil, false
	}

	// 官方：两个接口都要求 POST 且请求体为 {}（否则 44002 empty post data）。
	serverRaw, serverErr := s.env.Wx.GetEffectiveServerDomain(ctx, token, a.Appid)
	jumpRaw, jumpErr := s.env.Wx.GetEffectiveJumpDomain(ctx, token, a.Appid)
	serverFields := effectiveDomains(serverRaw)
	jumpFields := effectiveDomains(jumpRaw)
	snapshot := domainSnapshot(serverFields, jumpFields)
	gotServer, gotJump := serverErr == nil, jumpErr == nil

	var requireRequest, requireBusiness []string
	if required != nil {
		if required.RequestDomain != nil {
			requireRequest = *required.RequestDomain
		}
		if required.BusinessDomain != nil {
			requireBusiness = *required.BusinessDomain
		}
	}

	if len(requireRequest) == 0 && len(requireBusiness) == 0 {
		if !gotServer && !gotJump {
			return newCheck(checkDomains, labelDomains, gen.Unknown,
				"未检查：get_effective_domain 与 get_effective_webviewdomain 均调用失败（"+
					joinErrs(serverErr, jumpErr)+"）", ""), snapshot, false
		}
		msg := fmt.Sprintf("已生效：服务器域名 %d 个、业务域名 %d 个%s",
			countDomains(serverFields), countDomains(jumpFields), domainSample(serverFields, jumpFields))
		return newCheck(checkDomains, labelDomains, gen.Warn, msg,
			"授权托管后小程序只能使用第三方平台登记的域名（modify_wxa_server_domain / modify_wxa_jump_domain），且域名在发布上线后才生效。本次未提供要求域名，仅做信息展示。"), snapshot, true
	}
	if len(requireRequest) > 0 && !gotServer {
		return newCheck(checkDomains, labelDomains, gen.Unknown,
			"未检查：要求的服务器域名无法核对，get_effective_domain 调用失败（"+errText(serverErr)+"）", ""), snapshot, false
	}
	if len(requireBusiness) > 0 && !gotJump {
		return newCheck(checkDomains, labelDomains, gen.Unknown,
			"未检查：要求的业务域名无法核对，get_effective_webviewdomain 调用失败（"+errText(jumpErr)+"）", ""), snapshot, false
	}

	serverSet := domainSet(serverFields)
	jumpSet := domainSet(jumpFields)
	missing := make([]string, 0, 4)
	for _, d := range requireRequest {
		if !serverSet[normalizeDomain(d)] {
			missing = append(missing, d)
		}
	}
	for _, d := range requireBusiness {
		if !jumpSet[normalizeDomain(d)] {
			missing = append(missing, d)
		}
	}
	if len(missing) == 0 {
		return newCheck(checkDomains, labelDomains, gen.Pass,
			fmt.Sprintf("要求域名均已生效（服务器域名 %d 个、业务域名 %d 个）", len(serverSet), len(jumpSet)), ""), snapshot, true
	}
	return newCheck(checkDomains, labelDomains, gen.Fail,
		"以下要求域名未生效："+strings.Join(missing, "、"),
		"授权托管后只能使用第三方平台登记的域名：先在第三方平台登记（服务器域名 modify_wxa_server_domain / 业务域名 modify_wxa_jump_domain），再为小程序配置，并且发布上线后才生效。"), snapshot, true
}

// checkTemplateItem 代码模板库（purpose=commit 时检查）：
// 只有「代码来源=template 且模板库有可用普通模板」才能批量下发代码。
func (s *PreflightService) checkTemplateItem(ctx context.Context, a *model.Authorizer) gen.PreflightCheck {
	const key, label = checkTemplate, "代码模板库"
	if a.CodeSource != model.CodeSourceTemplate {
		return newCheck(key, label, gen.Fail,
			fmt.Sprintf("代码来源为 %q，不是模板库（template）：平台无法代该小程序上传代码", string(a.CodeSource)),
			"先绑定开发小程序上传代码并添加到模板库（模板库上限 200 个），或在「授权管理」里把代码来源改为 template；direct_commit（开发者工具直传）方式下平台不代上传代码。")
	}
	templates, err := s.env.Repos.Templates.List(ctx, intPtr(templateTypeNormal))
	if err != nil {
		return newCheck(key, label, gen.Unknown, "未检查：读取本地模板库失败（"+err.Error()+"）", "")
	}
	if len(templates) == 0 {
		return newCheck(key, label, gen.Fail, "模板库还没有可用的普通模板",
			"先用开发小程序上传代码（开发者工具 / miniprogram-ci 直传草稿箱），再在「模板库」里把草稿添加到代码模板库（tpl_addtotemplate，上限 200 个）；标准模板官方已下架，只能用普通模板。")
	}
	return newCheck(key, label, gen.Pass,
		fmt.Sprintf("模板库有 %d 个可用普通模板（上限 %d 个）", len(templates), model.TemplateLibraryLimit), "")
}

// quotaProbe 一次体检内的服务商额度探测结果。
//
// 额度是服务商级、旗下小程序共用的，所以任意一个已授权小程序的令牌都能查；
// 体检里每个小程序都展示同一个结果，但只调用微信一次。
type quotaProbe struct {
	quota  *gen.AuditQuota // 本次查询成功的结果
	cached *gen.AuditQuota // 最近一次缓存值（查询失败时回退）
	err    error           // 本次查询的失败原因
}

// probeQuota 用第一个能取到令牌的小程序探测服务商额度。
//
// 某个小程序取令牌失败（例如刚取消授权）不影响探测：换下一个继续，
// 全部拿不到时才把最后一次失败原因返回给调用方。
func (s *PreflightService) probeQuota(ctx context.Context, list []model.Authorizer) *quotaProbe {
	p := &quotaProbe{cached: s.cachedQuota(ctx)}
	for i := range list {
		appid := list[i].Appid
		token, err := s.token(ctx, appid)
		if err != nil {
			p.err = err
			continue
		}
		resp, err := s.env.Wx.QueryQuota(ctx, token, appid)
		if err != nil {
			p.err = mapError(err)
			continue
		}
		p.quota = quotaFromResponse(resp, time.Now())
		p.err = nil
		s.saveQuota(ctx, resp)
		return p
	}
	return p
}

// check 生成提审额度体检项（所有小程序复用同一份结果）。
func (p *quotaProbe) check() gen.PreflightCheck {
	current, stale := p.quota, false
	if current == nil {
		current, stale = p.cached, true
	}
	if current == nil || current.Rest == nil || current.Limit == nil {
		msg := "未检查：无法查询服务商提审额度"
		if p.err != nil {
			msg += "（" + p.err.Error() + "）"
		}
		return newCheck(checkQuota, labelQuota, gen.Unknown, msg,
			"额度为服务商级、旗下小程序共用；可在审核管理页重试，或先在「平台状态」页确认凭据与令牌是否就绪。")
	}

	prefix := ""
	if stale {
		prefix = "本次查询失败，展示最近一次缓存值；"
	}
	if *current.Rest <= 0 {
		return newCheck(checkQuota, labelQuota, gen.Fail,
			fmt.Sprintf("%s服务商提审额度已用尽（rest=%d / limit=%d）", prefix, *current.Rest, *current.Limit),
			"继续提审会返回 85085：请在「小程序服务商助手」申请临时额度；额度为服务商级、旗下小程序共用。")
	}
	status := gen.Pass
	if stale {
		// 拿不到实时额度就不该说「没问题」，但也不该拦住批量提审。
		status = gen.Warn
	}
	return newCheck(checkQuota, labelQuota, status,
		fmt.Sprintf("%s剩余提审额度 %d/%d（加急剩余 %s）", prefix, *current.Rest, *current.Limit, speedupText(current)), "")
}

// effectiveDomains 把 get_effective_domain / get_effective_webviewdomain 的扁平结果
// 归一成「域名维度 → 域名列表」。
//
// 真实接口按 mp_domain / third_domain / direct_domain / effective_domain 四组返回，
// wxapi 已打平成 "effective_domain.requestdomain" 形式的 key；本项目的内置模拟器只返回
// 不带前缀的 key。因此这里优先取 key 中含 effective 的分组（发布后真正生效的那组），
// 没有该分组时退回全部取值，两种返回都能正确比对。
func effectiveDomains(raw map[string][]string) map[string][]string {
	effective := map[string][]string{}
	all := map[string][]string{}
	for key, values := range raw {
		field := domainFieldOf(key)
		all[field] = append(all[field], values...)
		if strings.Contains(strings.ToLower(key), "effective") {
			effective[field] = append(effective[field], values...)
		}
	}
	if len(effective) > 0 {
		return effective
	}
	return all
}

// domainFieldOf 取域名 key 的最后一段（requestdomain / webviewdomain ...）。
func domainFieldOf(key string) string {
	if idx := strings.LastIndex(key, "."); idx >= 0 {
		return key[idx+1:]
	}
	return key
}

// domainSet 把所有维度的域名规整成集合（小写、去协议头与结尾斜杠）。
func domainSet(byField map[string][]string) map[string]bool {
	out := map[string]bool{}
	for _, values := range byField {
		for _, v := range values {
			if n := normalizeDomain(v); n != "" {
				out[n] = true
			}
		}
	}
	return out
}

// normalizeDomain 归一化域名：去协议头、去结尾斜杠、转小写，便于与要求域名比对。
func normalizeDomain(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	return strings.TrimSuffix(v, "/")
}

// countDomains 统计去重后的域名数量。
func countDomains(byField map[string][]string) int { return len(domainSet(byField)) }

// domainSample 生成域名样例（最多 3 个），便于运维一眼看到当前生效的域名。
func domainSample(serverFields, jumpFields map[string][]string) string {
	sample := make([]string, 0, 3)
	for _, source := range []map[string][]string{serverFields, jumpFields} {
		for _, d := range sortedKeys(domainSet(source)) {
			sample = append(sample, d)
			if len(sample) >= 3 {
				break
			}
		}
		if len(sample) > 0 {
			break
		}
	}
	if len(sample) == 0 {
		return ""
	}
	return "（如 " + strings.Join(sample, "、") + "）"
}

// domainSnapshot 生成回写 authorizers.domain_snapshot 的快照（键名与契约的 DomainSnapshot 一致）。
func domainSnapshot(serverFields, jumpFields map[string][]string) model.JSONMap {
	return model.JSONMap{
		"requestDomain":   copyList(serverFields["requestdomain"]),
		"wsRequestDomain": copyList(serverFields["wsrequestdomain"]),
		"uploadDomain":    copyList(serverFields["uploaddomain"]),
		"downloadDomain":  copyList(serverFields["downloaddomain"]),
		"udpDomain":       copyList(serverFields["udpdomain"]),
		"tcpDomain":       copyList(serverFields["tcpdomain"]),
		"businessDomain":  copyList(jumpFields["webviewdomain"]),
	}
}

// copyList 复制一份字符串数组（避免把 wxapi 返回的切片别名写进数据库 JSON 列）。
func copyList(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// sortedKeys 返回集合的稳定顺序（便于测试断言与展示）。
func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// speedupText 加急额度展示文本。
func speedupText(quota *gen.AuditQuota) string {
	if quota.SpeedupRest == nil || quota.SpeedupLimit == nil {
		return "未知"
	}
	return fmt.Sprintf("%d/%d", *quota.SpeedupRest, *quota.SpeedupLimit)
}

// errText 安全取错误文本（nil 返回空串）。
func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// joinErrs 拼接多个错误的文本。
func joinErrs(errs ...error) string {
	parts := make([]string, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			parts = append(parts, err.Error())
		}
	}
	if len(parts) == 0 {
		return "未知原因"
	}
	return strings.Join(parts, "；")
}

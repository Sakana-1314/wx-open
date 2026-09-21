package wxauth

import (
	"context"
	"strings"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/wxapi"
)

// 官方域名数量限制（写进错误文案，便于运维知道为什么被拒）。
const (
	// maxThirdPartyServerDomains 第三方平台服务器域名池上限（modify_wxa_server_domain，超限 65316）。
	maxThirdPartyServerDomains = 1000
	// maxThirdPartyJumpDomains 第三方平台业务域名上限（modify_wxa_jump_domain）。
	maxThirdPartyJumpDomains = 300
)

// directModeTip 快速配置（direct）的提示语：域名要等发布上线后才生效。
const directModeTip = "已提交快速配置：域名需发布上线后才生效（未发布时会在下一次发布成功后生效）；" +
	"同一小程序每月域名修改次数有限（超限返回 86102，第三方平台侧为 45104）"

// domainPlan 一次域名配置的计划（入参清洗后的形态）。
type domainPlan struct {
	action          gen.DomainApplyRequestAction
	mode            gen.DomainApplyRequestMode
	serverDomains   []string
	businessDomains []string
}

// ApplyDomains 批量配置小程序的服务器域名 / 业务域名。
//
// 两种模式（官方流程差异）：
//   - registered：先把域名登记到第三方平台（modify_wxa_server_domain / modify_wxa_jump_domain），
//     再逐个小程序调 modify_domain / setwebviewdomain —— 必须先登记，否则返回 85017/85018；
//   - direct：直接调 modify_domain_directly / setwebviewdomain_directly（规则对齐普通小程序，需发布上线后生效）。
//
// 逐项返回结果：单个小程序失败不中断其它小程序（批量运维要一次拿到全部失败清单）。
func (s *AppService) ApplyDomains(ctx context.Context, req gen.DomainApplyRequest) (*gen.DomainApplyResponse, error) {
	if err := s.ensureWeChat(); err != nil {
		return nil, err
	}
	if err := s.ensureRepos(); err != nil {
		return nil, err
	}
	if !req.Mode.Valid() {
		return nil, core.Validation("mode 只能是 registered（先登记到第三方平台再配置）或 direct（快速配置，需发布上线后生效）")
	}
	if !req.Action.Valid() {
		return nil, core.Validation("action 只能是 add（新增）/ delete（删除）/ set（覆盖）/ get（查询）")
	}

	plan := domainPlan{action: req.Action, mode: req.Mode}
	if req.ServerDomain != nil {
		plan.serverDomains = dedupeStrings(*req.ServerDomain.RequestDomain)
	}
	if req.BusinessDomain != nil {
		plan.businessDomains = append(plan.businessDomains, dedupeStrings(*req.BusinessDomain)...)
	}
	// 业务域名在契约里有两处（请求顶层的 businessDomain 与 serverDomain.businessDomain），合并去重。
	if req.ServerDomain != nil && req.ServerDomain.BusinessDomain != nil {
		plan.businessDomains = append(plan.businessDomains, dedupeStrings(*req.ServerDomain.BusinessDomain)...)
	}
	plan.businessDomains = dedupeStrings(plan.businessDomains)

	if req.Action != gen.Get && len(plan.serverDomains) == 0 && len(plan.businessDomains) == 0 {
		return nil, core.Validation("请至少提供一个服务器域名（serverDomain.requestDomain）或业务域名（businessDomain）")
	}

	list, err := core.ResolveSelection(ctx, s.repos(), req.Selection)
	if err != nil {
		return nil, err
	}

	items := make([]gen.DomainApplyItem, 0, len(list))
	operationDetail := model.JSONMap{
		"mode":            string(plan.mode),
		"action":          string(plan.action),
		"serverDomains":   len(plan.serverDomains),
		"businessDomains": len(plan.businessDomains),
		"targetCount":     len(list),
	}
	if plan.mode == gen.Registered && plan.action != gen.Get &&
		(len(plan.serverDomains) > 0 || len(plan.businessDomains) > 0) {
		if err := s.registerPlatformDomains(ctx, plan); err != nil {
			// 域名没进第三方平台域名池时，后面每个小程序的代调用都会报 85017/85018，
			// 因此逐项直接给出同一个失败原因（仍返回逐项结构，便于前端展示）。
			code, message := errcodeOf(err), err.Error()
			for i := range list {
				items = append(items, gen.DomainApplyItem{
					Appid:   list[i].Appid,
					Ok:      false,
					Errcode: code,
					Errmsg:  &message,
				})
			}
			operationDetail["succeeded"], operationDetail["failed"] = 0, len(items)
			operationDetail["error"] = message
			s.writeOperation(ctx, actionApplyDomains, targetAuthorizer, "", operationDetail)
			return &gen.DomainApplyResponse{Items: items}, nil
		}
	}

	succeeded := 0
	for i := range list {
		item := s.applyDomainsToApp(ctx, &list[i], plan)
		if item.Ok {
			succeeded++
		}
		items = append(items, item)
	}
	operationDetail["succeeded"], operationDetail["failed"] = succeeded, len(items)-succeeded
	s.writeOperation(ctx, actionApplyDomains, targetAuthorizer, "", operationDetail)
	return &gen.DomainApplyResponse{Items: items}, nil
}

// registerPlatformDomains 把域名登记到第三方平台（registered 模式的第一步）。
func (s *AppService) registerPlatformDomains(ctx context.Context, plan domainPlan) error {
	token, err := s.componentToken(ctx)
	if err != nil {
		return err
	}
	if len(plan.serverDomains) > maxThirdPartyServerDomains {
		return core.Validation("第三方平台服务器域名最多 %d 个（微信限制），当前 %d 个：请分批配置（超限返回 65316）",
			maxThirdPartyServerDomains, len(plan.serverDomains))
	}
	if len(plan.businessDomains) > maxThirdPartyJumpDomains {
		return core.Validation("第三方平台业务域名最多 %d 个（微信限制），当前 %d 个：请分批配置",
			maxThirdPartyJumpDomains, len(plan.businessDomains))
	}
	if len(plan.serverDomains) > 0 {
		// 域名以 ; 分隔；modifyPublishedTogether=true 表示同时修改「全网发布版」的域名值。
		if _, err := s.env.Wx.ModifyThirdPartyServerDomain(
			ctx, token, string(plan.action), strings.Join(plan.serverDomains, ";"), true); err != nil {
			return wxErr("", err)
		}
	}
	if len(plan.businessDomains) > 0 {
		// 业务域名同样必须先在第三方平台登记（否则代配置时报 85017/89020）。
		if _, err := s.env.Wx.ModifyThirdPartyJumpDomain(
			ctx, token, string(plan.action), strings.Join(plan.businessDomains, ";"), true); err != nil {
			return wxErr("", err)
		}
	}
	return nil
}

// applyDomainsToApp 对单个小程序执行域名配置，失败原因与无效域名明细都带回。
func (s *AppService) applyDomainsToApp(ctx context.Context, a *model.Authorizer, plan domainPlan) gen.DomainApplyItem {
	item := gen.DomainApplyItem{Appid: a.Appid}
	if a.AuthorizationStatus != model.AuthStatusAuthorized {
		item.Errmsg = strPtr("该小程序不是「已授权」状态，已跳过域名配置：需商家重新授权后再操作")
		return item
	}
	token, err := s.authorizerToken(ctx, a.Appid)
	if err != nil {
		item.Errcode, item.Errmsg = errcodeOf(err), strPtr(err.Error())
		return item
	}

	var (
		problems   []string
		invalid    []string
		missingICP []string
	)
	noteErr := func(step string, err error) {
		wrapped := wxErr(a.Appid, err)
		problems = append(problems, step+"："+wrapped.Error())
		if item.Errcode == nil {
			item.Errcode = errcodeOf(wrapped)
		}
	}

	if len(plan.serverDomains) > 0 {
		if plan.mode == gen.Registered {
			resp, err := s.env.Wx.ModifyServerDomain(ctx, token, a.Appid, string(plan.action),
				&wxapi.DomainSet{RequestDomain: plan.serverDomains})
			if err != nil {
				noteErr("服务器域名配置失败", err)
			} else {
				invalid = append(invalid, dedupeStrings(resp.InvalidRequestDomain)...)
				invalid = append(invalid, dedupeStrings(resp.InvalidUploadDomain)...)
				invalid = dedupeStrings(invalid)
				missingICP = dedupeStrings(resp.NoICPDomain)
			}
		} else {
			if err := s.env.Wx.ModifyServerDomainDirectly(ctx, token, a.Appid, string(plan.action),
				&wxapi.DomainSet{RequestDomain: plan.serverDomains}); err != nil {
				noteErr("服务器域名配置失败（快速配置）", err)
			}
		}
	}

	if len(plan.businessDomains) > 0 {
		if plan.mode == gen.Registered {
			if err := s.env.Wx.ModifyJumpDomain(ctx, token, a.Appid, string(plan.action), plan.businessDomains); err != nil {
				noteErr("业务域名配置失败", err)
			}
		} else {
			if _, err := s.env.Wx.ModifyJumpDomainDirectly(ctx, token, a.Appid, string(plan.action), plan.businessDomains); err != nil {
				noteErr("业务域名配置失败（快速配置）", err)
			}
		}
	}

	if len(invalid) > 0 {
		item.InvalidDomains = &invalid
		problems = append(problems, "以下域名不符合微信域名规则（必须 ICP 备案、不能是 IP 或 api.weixin.qq.com，且不能带协议头）："+
			strings.Join(invalid, "、"))
	}
	if len(missingICP) > 0 {
		item.MissingIcpDomains = &missingICP
		problems = append(problems, "以下域名缺少 ICP 备案（先完成备案再配置）："+strings.Join(missingICP, "、"))
	}
	if len(problems) > 0 {
		message := strings.Join(problems, "；")
		item.Ok = false
		item.Errmsg = &message
		return item
	}

	item.Ok = true
	if plan.mode == gen.Direct {
		// 快速配置的最大坑：域名要到发布上线后才生效，必须显式提示。
		item.Errmsg = strPtr(directModeTip)
	}
	return item
}

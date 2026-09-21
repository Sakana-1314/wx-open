package wxauth

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
	"wx-platform/server/internal/wxapi"
)

// AuthorizerService 已授权小程序的管理：列表 / 详情 / 本地维护字段 / 删除 / 信息同步 /
// 授权链路（授权链接与授权码换令牌）/ 授权方选项 / 授权变更回调。
type AuthorizerService struct {
	base
}

// NewAuthorizerService 构造授权方服务（handler 与回调业务分发直接使用）。
func NewAuthorizerService(env *core.Env) *AuthorizerService {
	return &AuthorizerService{base: newBase(env)}
}

// extVarKeyPattern ext 变量名的合法形态：ext_json 模板里以 {{变量名}} 引用，只允许字母数字下划线。
var extVarKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// missingRefreshTokenAdvice 全量重拉令牌时，微信未返回 refresh_token 的官方处置建议。
const missingRefreshTokenAdvice = "微信未返回 refresh_token：该账号可能已取消授权或授权已过期，" +
	"需商家重新扫码授权（重新授权后本平台会自动补齐 refresh_token）"

// List 分页查询已授权小程序。
func (s *AuthorizerService) List(ctx context.Context, params gen.ListAuthorizersParams) (*gen.AuthorizerListResponse, error) {
	if err := s.ensureRepos(); err != nil {
		return nil, err
	}
	page, pageSize := pageOf(params.Page), pageSizeOf(params.PageSize)
	// IncludeDisabled 固定为 true：管理列表必须能看到「已停用」的小程序，否则运维把开关关掉后
	// 记录就从列表里消失、再也无法重新启用或删除；「停用」只影响批量作业的目标选择
	// （core.ResolveSelection 会过滤掉 enabled=false 的小程序）。
	filter := repo.AuthorizerFilter{
		Keyword:         derefString(params.Keyword),
		GroupName:       derefString(params.GroupName),
		Tag:             derefString(params.Tag),
		IncludeDisabled: true,
		Page:            page,
		PageSize:        pageSize,
	}
	if params.Status != nil {
		if !params.Status.Valid() {
			return nil, core.Validation("status 只能是 authorized（已授权）或 unauthorized（已取消授权）")
		}
		filter.Status = string(*params.Status)
	}
	if params.HasDevPermission != nil {
		want := *params.HasDevPermission
		filter.HasDevPermission = &want
	}

	items, total, err := s.repos().Authorizers.List(ctx, filter)
	if err != nil {
		return nil, core.Internal(err)
	}
	out := &gen.AuthorizerListResponse{
		Items:    make([]gen.Authorizer, 0, len(items)),
		Total:    int(total),
		Page:     page,
		PageSize: pageSize,
	}
	for i := range items {
		out.Items = append(out.Items, toAuthorizer(&items[i]))
	}
	return out, nil
}

// Get 小程序详情（含权限集、类目与域名快照）。
func (s *AuthorizerService) Get(ctx context.Context, appid string) (*gen.AuthorizerDetail, error) {
	a, err := s.getAuthorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	return toAuthorizerDetail(a), nil
}

// Update 编辑本地维护字段：备注 / 分组 / 标签 / 启停 / ext 变量 / 提审配置 / 代码来源。
//
// 只写请求里出现的字段（DTO 的可选字段用指针表达「未提供」），因此不会误清空其它字段。
func (s *AuthorizerService) Update(ctx context.Context, appid string, req gen.AuthorizerUpdateRequest) (*gen.AuthorizerDetail, error) {
	a, err := s.getAuthorizer(ctx, appid)
	if err != nil {
		return nil, err
	}

	fields := map[string]any{}
	changed := make([]string, 0, 8)
	if req.Remark != nil {
		fields["remark"] = strings.TrimSpace(*req.Remark)
		changed = append(changed, "remark")
	}
	if req.GroupName != nil {
		fields["group_name"] = strings.TrimSpace(*req.GroupName)
		changed = append(changed, "groupName")
	}
	if req.Tags != nil {
		// 标签去空白去重（core.NormalizeTags），保持前端输入顺序。
		fields["tags"] = model.StringSlice(core.NormalizeTags(*req.Tags))
		changed = append(changed, "tags")
	}
	if req.ExtVars != nil {
		vars := make(model.JSONStringMap, len(*req.ExtVars))
		for key, value := range *req.ExtVars {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				return nil, core.Validation("ext 变量名不能为空")
			}
			if !extVarKeyPattern.MatchString(trimmed) {
				return nil, core.Validation("ext 变量名 %q 不合法：只允许字母、数字与下划线（ext_json 模板里用 {{%s}} 引用）", trimmed, trimmed)
			}
			vars[trimmed] = value
		}
		fields["ext_vars"] = vars
		changed = append(changed, "extVars")
	}
	if req.AuditProfileId != nil {
		if *req.AuditProfileId <= 0 {
			return nil, core.Validation("auditProfileId 必须为正整数")
		}
		if _, err := s.repos().Profiles.Get(ctx, uint(*req.AuditProfileId)); err != nil {
			if errors.Is(err, repo.ErrNotFound) {
				return nil, core.Validation("提审配置 %d 不存在：请先在「提审配置」页创建", *req.AuditProfileId)
			}
			return nil, core.Internal(err)
		}
		fields["audit_profile_id"] = uint(*req.AuditProfileId)
		changed = append(changed, "auditProfileId")
	}
	if req.Enabled != nil {
		fields["enabled"] = *req.Enabled
		changed = append(changed, "enabled")
	}
	if req.CodeSource != nil {
		if !req.CodeSource.Valid() {
			return nil, core.Validation("codeSource 只能是 template（平台用模板库批量下发）或 direct_commit（开发者工具直传）")
		}
		fields["code_source"] = model.CodeSource(*req.CodeSource)
		changed = append(changed, "codeSource")
	}
	if req.AuditOverride != nil {
		fields["audit_override"] = model.JSONMap(*req.AuditOverride)
		changed = append(changed, "auditOverride")
	}
	if len(fields) == 0 {
		return toAuthorizerDetail(a), nil
	}

	if err := s.repos().Authorizers.UpdateFields(ctx, appid, fields); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, core.NotFound("小程序 %s 不存在", appid)
		}
		return nil, core.Internal(err)
	}
	updated, err := s.getAuthorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	s.writeOperation(ctx, actionUpdateAuthorizer, targetAuthorizer, appid, model.JSONMap{"fields": changed})
	return toAuthorizerDetail(updated), nil
}

// Delete 删除已取消授权的小程序记录（软删除，物理数据保留供审计）。
//
// 已授权的小程序不允许删除：否则会出现「微信侧仍授权、平台无记录」的状态，
// 后续作业与对账都会失去依据（官方要求先解除授权）。
func (s *AuthorizerService) Delete(ctx context.Context, appid string) error {
	a, err := s.getAuthorizer(ctx, appid)
	if err != nil {
		return err
	}
	if a.AuthorizationStatus == model.AuthStatusAuthorized {
		return core.Conflict("小程序 %s 仍处于「已授权」状态，不能删除：请先在微信侧解除授权（平台收到取消授权通知后）再删除记录", appid)
	}
	if err := s.repos().Authorizers.SoftDelete(ctx, appid); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return core.NotFound("小程序 %s 不存在", appid)
		}
		return core.Internal(err)
	}
	s.writeOperation(ctx, actionDeleteAuthorizer, targetAuthorizer, appid, nil)
	return nil
}

// Sync 同步单个小程序的资料、权限集与提审前置快照（域名 + 隐私指引）。
//
// 域名与隐私指引属于「尽力而为」：拉取失败不致命，只把缺失项记进快照，
// 避免个别接口故障让整次同步失败（资料本身已经更新成功）。
func (s *AuthorizerService) Sync(ctx context.Context, appid string) (*gen.AuthorizerDetail, error) {
	if err := s.ensureWeChat(); err != nil {
		return nil, err
	}
	if _, err := s.getAuthorizer(ctx, appid); err != nil {
		return nil, err
	}
	if err := s.syncOne(ctx, appid); err != nil {
		return nil, err
	}
	updated, err := s.getAuthorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	s.writeOperation(ctx, actionSync, targetAuthorizer, appid, nil)
	return toAuthorizerDetail(updated), nil
}

// SyncMany 按目标选择逐项同步，返回逐项结果。
//
// 单项失败不中断其它项（作业与运维都希望一次拿到全部失败清单），
// 因此这里永远返回 SyncSummary + nil error：真正的「整体失败」只有目标解析失败与凭据缺失。
func (s *AuthorizerService) SyncMany(ctx context.Context, sel gen.AppidSelection) (*gen.SyncSummary, error) {
	if err := s.ensureWeChat(); err != nil {
		return nil, err
	}
	if err := s.ensureRepos(); err != nil {
		return nil, err
	}
	list, err := core.ResolveSelection(ctx, s.repos(), sel)
	if err != nil {
		return nil, err
	}

	details := make([]gen.SyncDetail, 0, len(list))
	summary := &gen.SyncSummary{Total: len(list)}
	for i := range list {
		appid := list[i].Appid
		if err := s.syncOne(ctx, appid); err != nil {
			summary.Failed++
			details = append(details, gen.SyncDetail{
				Key:     appid,
				Ok:      false,
				Errcode: errcodeOf(err),
				Errmsg:  strPtr(err.Error()),
			})
			continue
		}
		summary.Succeeded++
		details = append(details, gen.SyncDetail{Key: appid, Ok: true})
	}
	summary.Details = &details
	s.writeOperation(ctx, actionSyncMany, targetAuthorizer, "", model.JSONMap{
		"total":     summary.Total,
		"succeeded": summary.Succeeded,
		"failed":    summary.Failed,
	})
	return summary, nil
}

// ResyncTokens 用 api_get_authorizer_list 全量重拉 authorizer_refresh_token
// （refresh_token 丢失或失效时的官方恢复路径）。
func (s *AuthorizerService) ResyncTokens(ctx context.Context) (*gen.SyncSummary, error) {
	if err := s.ensureWeChat(); err != nil {
		return nil, err
	}
	result, err := s.env.Tokens.ResyncRefreshTokens(ctx)
	if err != nil {
		return nil, wxErr("", err)
	}
	details := make([]gen.SyncDetail, 0, len(result.Missing))
	for _, appid := range result.Missing {
		key := appid
		if key == "" {
			key = "(微信未返回 appid)"
		}
		details = append(details, gen.SyncDetail{
			Key:    key,
			Ok:     false,
			Errmsg: strPtr(missingRefreshTokenAdvice),
		})
	}
	summary := &gen.SyncSummary{
		Total:     result.Total,
		Succeeded: result.Updated,
		Failed:    len(result.Missing),
		Details:   &details,
	}
	s.writeOperation(ctx, actionResyncTokens, targetAuthorizer, "", model.JSONMap{
		"total":   result.Total,
		"updated": result.Updated,
		"missing": len(result.Missing),
	})
	return summary, nil
}

// syncOne 单个小程序的同步实现（资料 + 权限集 + 域名/隐私快照）。
func (s *AuthorizerService) syncOne(ctx context.Context, appid string) error {
	current, err := s.getAuthorizer(ctx, appid)
	if err != nil {
		return err
	}
	now := time.Now()
	token, err := s.componentToken(ctx)
	if err != nil {
		return err
	}
	resp, err := s.env.Wx.GetAuthorizerInfo(ctx, token, s.componentAppid(), appid)
	if err != nil {
		return wxErr(appid, err)
	}
	info := resp.AuthorizerInfo

	fields := map[string]any{
		"nick_name":       info.NickName,
		"head_img":        info.HeadImg,
		"qrcode_url":      info.QRCodeURL,
		"user_name":       info.UserName,
		"alias":           info.Alias,
		"principal_name":  info.PrincipalName,
		"signature":       info.Signature,
		"service_type_id": info.ServiceTypeInfo.ID,
		"verify_type_id":  info.VerifyTypeInfo.ID,
		"register_type":   info.RegisterType,
		"account_status":  info.AccountStatus,
		"last_sync_at":    now,
	}
	// business_info 为空表示微信没返回，不能把已有的覆盖成空。
	if len(info.BusinessInfo) > 0 {
		fields["business_info"] = model.JSONMap(info.BusinessInfo)
	}
	// func_info 为空同样按「未返回」处理，避免把已授权权限集清空（权限集由商家授权决定）。
	if ids := funcIDsOf(resp.AuthorizationInfo.FuncInfo); len(ids) > 0 {
		fields["func_info"] = model.IntSlice(ids)
	}
	if cats := categorySnapshot(info); len(cats) > 0 {
		fields["mini_program_cats"] = model.JSONMap(cats)
	}
	// Sync 不改变授权状态：取消授权由回调（unauthorized）负责，避免自动把已取消的账号「复活」。
	if current.AuthorizedAt == nil {
		fields["authorized_at"] = now
	}
	if err := s.repos().Authorizers.UpdateFields(ctx, appid, fields); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return core.NotFound("小程序 %s 不存在", appid)
		}
		return core.Internal(err)
	}

	// 提审前置快照：域名（服务器/业务）与用户隐私保护指引是否已配置。
	missing := make([]string, 0, 3)
	var effective, registered map[string]any
	privacyConfigured := false
	privacyKnown := false
	if current.PrivacyConfigured != nil {
		privacyConfigured = *current.PrivacyConfigured
	}
	if authorizerToken, err := s.env.Tokens.AuthorizerToken(ctx, appid); err != nil {
		// 没有可用令牌（例如已取消授权、refresh_token 失效）：三个快照项都算缺失。
		missing = append(missing, "serverDomain", "businessDomain", "privacySetting")
	} else {
		if sd, err := s.env.Wx.GetEffectiveServerDomain(ctx, authorizerToken, appid); err == nil {
			effective = serverDomainSnapshot(sd)
			registered = domainGroupSnapshot(sd, "third_domain")
		} else {
			missing = append(missing, "serverDomain")
		}
		if jd, err := s.env.Wx.GetEffectiveJumpDomain(ctx, authorizerToken, appid); err == nil {
			if bd := businessDomainOf(jd); len(bd) > 0 {
				if effective == nil {
					effective = map[string]any{}
				}
				effective["businessDomain"] = bd
			}
			if rb := dedupeStrings(jd["third_webviewdomain"]); len(rb) > 0 {
				if registered == nil {
					registered = map[string]any{}
				}
				registered["businessDomain"] = rb
			}
		} else {
			missing = append(missing, "businessDomain")
		}
		// 官方：code_exist == 1 表示已经配置过用户隐私保护指引。
		if privacy, err := s.env.Wx.GetPrivacySetting(ctx, authorizerToken, appid, nil); err == nil {
			privacyKnown = true
			privacyConfigured = privacy.CodeExist == 1
		} else {
			missing = append(missing, "privacySetting")
		}
	}
	snapshot := mergeDomainSnapshot(current.DomainSnapshot, effective, registered, missing)
	var saveErr error
	if privacyKnown {
		saveErr = s.repos().Authorizers.UpdatePreflightSnapshot(ctx, appid, privacyConfigured, snapshot, now)
	} else {
		// 隐私指引没探测到（接口失败）时不能写成 false：false 会被体检当成「未配置隐私指引」，
		// 属于误报。这里用局部更新只写域名快照与体检时间，隐私结论保持 NULL（未探测）或原值。
		saveErr = s.repos().Authorizers.UpdateFields(ctx, appid, map[string]any{
			"domain_snapshot":   snapshot,
			"last_preflight_at": now,
		})
	}
	if saveErr != nil {
		if errors.Is(saveErr, repo.ErrNotFound) {
			return core.NotFound("小程序 %s 不存在", appid)
		}
		return core.Internal(saveErr)
	}
	return nil
}

// funcIDsOf 取权限集 id 列表（去重并保持微信返回顺序）。
func funcIDsOf(scopes []wxapi.FuncScope) []int {
	out := make([]int, 0, len(scopes))
	seen := map[int]bool{}
	for _, sc := range scopes {
		if sc.ID == 0 || seen[sc.ID] {
			continue
		}
		seen[sc.ID] = true
		out = append(out, sc.ID)
	}
	return out
}

// categorySnapshot 把微信返回的小程序类目转成落库结构（{"categories":[{"first","second"}]}）。
func categorySnapshot(info wxapi.AuthorizerInfo) map[string]any {
	list := make([]map[string]string, 0, len(info.MiniProgramInfo.Categories))
	for _, c := range info.MiniProgramInfo.Categories {
		list = append(list, map[string]string{"first": c.First, "second": c.Second})
	}
	if len(list) == 0 {
		return nil
	}
	return map[string]any{snapshotKeyCategories: list}
}

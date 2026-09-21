package wxauth

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
	"wx-platform/server/internal/wxapi"
)

// 预授权码有效期兜底值（官方正文写 1800 秒；微信返回 0 时兜底）。
const defaultPreAuthCodeTTL = 1800

// missingDevPermissionWarning 缺少开发权限集（18）的提示语。
// 权限集 18 是互斥权限集，代码上传 / 提审 / 发布全部依赖它，且必须由商家重新扫码勾选。
const missingDevPermissionWarning = "该小程序未授权「小程序开发与数据分析」（权限集 18）：需商家重新扫码授权并勾选" +
	"『小程序开发与数据分析』，否则无法为它上传代码 / 提审 / 发布。"

// maxMessageLogBytes 代收消息写审计日志时的原文上限。
const maxMessageLogBytes = 4000

// AuthorizationURL 生成预授权码与授权链接（PC / 移动端 / 二维码内容）。
//
// 官方流程：component_access_token → api_create_preauthcode → 拼接授权页链接；
// 商家授权完成后，微信既会跳转 redirect_uri?auth_code=xxx，也会推送 authorized 事件。
func (s *AuthorizerService) AuthorizationURL(ctx context.Context, params gen.GetAuthorizationUrlParams) (*gen.AuthorizationUrlResponse, error) {
	// 凭据门禁：没有平台 appid 连链接都拼不出来，直接返回明确的未配置错误。
	if s.componentAppid() == "" {
		return nil, core.ErrNotConfigured
	}
	// 先做本地参数校验（不消耗微信额度），再取令牌与预授权码。
	authType := core.AuthTypeMiniProgram
	if params.AuthType != nil {
		if !params.AuthType.Valid() {
			return nil, core.Validation("authType 取值不合法：%d（1 公众号 / 2 小程序 / 3 公众号+小程序 / 4 小程序推客 / 5 视频号 / 6 全部 / 8 带货助手）", int(*params.AuthType))
		}
		authType = core.AuthType(*params.AuthType)
	}
	categoryIDs, err := parseCategoryIDList(derefString(params.CategoryIdList))
	if err != nil {
		return nil, err
	}
	redirect := derefString(params.RedirectUri)
	if redirect == "" {
		redirect = core.BuildRedirectURI(s.publicBaseURL())
	}
	bizAppid := derefString(params.BizAppid)

	token, err := s.componentToken(ctx)
	if err != nil {
		return nil, err
	}
	pre, err := s.env.Wx.CreatePreAuthCode(ctx, token, s.componentAppid())
	if err != nil {
		return nil, wxErr(bizAppid, err)
	}
	preAuthCode := strings.TrimSpace(pre.PreAuthCode)
	if preAuthCode == "" {
		return nil, core.Internal(errors.New("微信未返回 pre_auth_code，请稍后重试"))
	}

	pcURL, mobileURL, err := core.AuthURLRequest{
		ComponentAppid: s.componentAppid(),
		PreAuthCode:    preAuthCode,
		RedirectURI:    redirect,
		AuthType:       authType,
		BizAppid:       bizAppid,
		CategoryIDs:    categoryIDs,
	}.AuthURLs()
	if err != nil {
		return nil, err
	}

	expiresIn := pre.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = defaultPreAuthCodeTTL
	}
	out := &gen.AuthorizationUrlResponse{
		PcUrl:            pcURL,
		MobileUrl:        mobileURL,
		QrCodeContent:    pcURL,
		PreAuthCode:      preAuthCode,
		ExpiresInSeconds: expiresIn,
	}
	at := gen.AuthType(int(authType))
	out.AuthType = &at
	out.RedirectUri = strPtr(redirect)
	if bizAppid != "" {
		out.BizAppid = strPtr(bizAppid)
	}
	s.writeOperation(ctx, actionAuthorizationURL, targetAuthorizer, bizAppid, model.JSONMap{
		"authType":     int(authType),
		"bizAppid":     bizAppid,
		"redirectUri":  redirect,
		"categoryIds":  categoryIDs,
		"expiresInSec": expiresIn,
	})
	return out, nil
}

// CompleteAuthorize 用授权回调携带的 auth_code 换取令牌并登记小程序。
func (s *AuthorizerService) CompleteAuthorize(ctx context.Context, req gen.AuthorizeRequest) (*gen.AuthorizeResponse, error) {
	code := strings.TrimSpace(req.AuthCode)
	if code == "" {
		return nil, core.Validation("授权码（authCode）不能为空")
	}
	if err := s.ensureWeChat(); err != nil {
		return nil, err
	}
	token, err := s.componentToken(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := s.env.Wx.QueryAuth(ctx, token, s.componentAppid(), code)
	if err != nil {
		return nil, wxErr("", err)
	}
	info := resp.AuthorizationInfo
	if strings.TrimSpace(info.AuthorizerAppid) == "" {
		return nil, core.Validation("微信未返回 authorizer_appid：授权码可能已过期（42003），请让商家重新扫描授权链接")
	}
	return s.registerAuthorized(ctx, info, actionAuthorize, "authCode")
}

// registerAuthorized 把授权码换取的授权信息落库（CompleteAuthorize 与 OnAuthorized 共用）。
//
// 幂等性保证：
//   - 以 appid 为准 upsert，重复推送不会产生重复记录；
//   - 从「库中已有记录」出发只覆盖授权相关字段，因此商家重复授权不会清掉平台侧维护的
//     备注 / 分组 / 标签 / ext 变量 / 提审配置（auditProfileId）/ 代码来源等字段。
func (s *AuthorizerService) registerAuthorized(ctx context.Context, info wxapi.AuthorizationInfo, action, source string) (*gen.AuthorizeResponse, error) {
	if err := s.ensureRepos(); err != nil {
		return nil, err
	}
	if s.env.Box == nil {
		return nil, core.Internal(errors.New("加密组件未初始化，无法保存 refresh_token"))
	}
	appid := strings.TrimSpace(info.AuthorizerAppid)
	now := time.Now()
	funcIDs := funcIDsOf(info.FuncInfo)

	existing, err := s.repos().Authorizers.Get(ctx, appid)
	switch {
	case err == nil:
	case errors.Is(err, repo.ErrNotFound):
		existing = nil
	default:
		return nil, core.Internal(err)
	}

	record := &model.Authorizer{
		Appid:               appid,
		AuthorizationStatus: model.AuthStatusAuthorized,
		CodeSource:          model.CodeSourceTemplate,
		Enabled:             true,
	}
	if existing != nil {
		record = existing
	}
	previousStatus := record.AuthorizationStatus
	previousFuncs := append([]int(nil), record.FuncInfo...)

	record.Appid = appid
	record.AuthorizationStatus = model.AuthStatusAuthorized
	if existing == nil {
		// 新记录的默认值；已存在时保留运维在平台上设置的「启用 / 停用」开关。
		record.Enabled = true
		if record.CodeSource == "" {
			record.CodeSource = model.CodeSourceTemplate
		}
	}
	if record.AuthorizedAt == nil || previousStatus != model.AuthStatusAuthorized {
		at := now
		record.AuthorizedAt = &at
	}
	if len(funcIDs) > 0 {
		record.FuncInfo = model.IntSlice(funcIDs)
	}
	if refresh := strings.TrimSpace(info.AuthorizerRefreshToken); refresh != "" {
		cipher, cerr := s.env.Box.Seal(refresh)
		if cerr != nil {
			return nil, core.Internal(fmt.Errorf("加密 refresh_token 失败: %w", cerr))
		}
		at := now
		record.RefreshTokenCipher = string(cipher)
		record.RefreshTokenUpdatedAt = &at
	}

	if uerr := s.repos().Authorizers.Upsert(ctx, record); uerr != nil {
		return nil, core.Internal(uerr)
	}
	if existing == nil {
		// 可能是「之前被软删、现在重新授权」的 appid：Upsert 会保留 deleted_at，这里显式恢复可见。
		if uerr := s.repos().Authorizers.UpdateFields(ctx, appid, map[string]any{"deleted_at": nil}); uerr != nil {
			return nil, core.Internal(uerr)
		}
	}

	warnings := make([]string, 0, 3)
	// 补齐昵称 / 头像 / 原始 ID / 主体名称等资料；失败不致命（授权本身已经登记成功）。
	if token, terr := s.componentToken(ctx); terr != nil {
		warnings = append(warnings, "已登记授权，但未取得第三方平台令牌，账号资料未补齐："+terr.Error()+"；可在「授权管理」里点「同步」重试。")
	} else if profileIDs, perr := s.applyAuthorizerProfile(ctx, token, appid, record, now); perr != nil {
		warnings = append(warnings, "已登记授权，但拉取账号资料失败："+perr.Error()+"；可在「授权管理」里点「同步」重试。")
	} else if len(profileIDs) > 0 {
		// 资料接口同时返回「当前」权限集，且它已经写进库：返回值与落库内容保持一致。
		funcIDs = profileIDs
	}

	// 商家重复扫码时权限集可能变化（新增或漏勾），逐项提示便于运维核对。
	if len(previousFuncs) > 0 && !sameIntSet(previousFuncs, funcIDs) {
		warnings = append(warnings, "本次授权的权限集与上次不一致：当前为 "+describeFuncIDs(funcIDs)+"；原为 "+describeFuncIDs(previousFuncs)+"。")
	}
	hasDev := containsInt(funcIDs, model.PermissionSetDev)
	if !hasDev {
		warnings = append(warnings, missingDevPermissionWarning)
	}

	// 授权码已经换回了令牌，直接写缓存可省掉一次 api_authorizer_token 调用。
	if err := s.env.Tokens.StoreAuthorizerToken(ctx, appid, info.AuthorizerAccessToken, info.ExpiresIn); err != nil {
		return nil, core.Internal(fmt.Errorf("缓存 authorizer_access_token 失败: %w", err))
	}

	out := &gen.AuthorizeResponse{
		Appid:               appid,
		AuthorizationStatus: gen.AuthorizationStatus(model.AuthStatusAuthorized),
		FuncInfoIds:         funcIDs,
		HasDevPermission:    boolPtr(hasDev),
	}
	if len(warnings) > 0 {
		out.Warnings = &warnings
	}
	s.writeOperation(ctx, action, targetAuthorizer, appid, model.JSONMap{
		"source":           source,
		"funcInfoIds":      funcIDs,
		"hasDevPermission": hasDev,
		"warnings":         warnings,
	})
	return out, nil
}

// applyAuthorizerProfile 用 api_get_authorizer_info 更新资料与权限集，返回微信返回的权限集 id。
func (s *AuthorizerService) applyAuthorizerProfile(ctx context.Context, token, appid string, current *model.Authorizer, now time.Time) ([]int, error) {
	resp, err := s.env.Wx.GetAuthorizerInfo(ctx, token, s.componentAppid(), appid)
	if err != nil {
		return nil, wxErr(appid, err)
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
	// business_info / func_info 为空表示微信没返回，按「未知」处理，不覆盖已有值。
	if len(info.BusinessInfo) > 0 {
		fields["business_info"] = model.JSONMap(info.BusinessInfo)
	}
	if ids := funcIDsOf(resp.AuthorizationInfo.FuncInfo); len(ids) > 0 {
		fields["func_info"] = model.IntSlice(ids)
	}
	if cats := categorySnapshot(info); len(cats) > 0 {
		fields["mini_program_cats"] = model.JSONMap(cats)
	}
	// Sync 不改变授权状态（取消授权由回调负责），但补齐历史缺失的授权时间。
	if current == nil || current.AuthorizedAt == nil {
		fields["authorized_at"] = now
	}
	if err := s.repos().Authorizers.UpdateFields(ctx, appid, fields); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			return nil, core.NotFound("小程序 %s 不存在", appid)
		}
		return nil, core.Internal(err)
	}
	return funcIDsOf(resp.AuthorizationInfo.FuncInfo), nil
}

// GetOption 读取授权方选项信息。
func (s *AuthorizerService) GetOption(ctx context.Context, appid string, params gen.GetAuthorizerOptionParams) (*gen.AuthorizerOption, error) {
	if !params.OptionName.Valid() {
		return nil, core.Validation("optionName 只能是 location_report（地理位置上报）/ voice_recognize（语音识别）/ customer_service（多客服）")
	}
	if _, err := s.requireAuthorized(ctx, appid); err != nil {
		return nil, err
	}
	token, err := s.componentToken(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := s.env.Wx.GetAuthorizerOption(ctx, token, s.componentAppid(), appid, string(params.OptionName))
	if err != nil {
		return nil, wxErr(appid, err)
	}
	name := gen.AuthorizerOptionName(strings.TrimSpace(resp.OptionName))
	if !name.Valid() {
		name = params.OptionName
	}
	return &gen.AuthorizerOption{OptionName: name, OptionValue: resp.OptionValue}, nil
}

// SetOption 设置授权方选项信息（需要授权方已授权对应权限）。
func (s *AuthorizerService) SetOption(ctx context.Context, appid string, req gen.AuthorizerOptionUpdateRequest) (*gen.AuthorizerOption, error) {
	if !req.OptionName.Valid() {
		return nil, core.Validation("optionName 只能是 location_report（地理位置上报）/ voice_recognize（语音识别）/ customer_service（多客服）")
	}
	value := strings.TrimSpace(req.OptionValue)
	if err := validateOptionValue(req.OptionName, value); err != nil {
		return nil, err
	}
	if _, err := s.requireAuthorized(ctx, appid); err != nil {
		return nil, err
	}
	token, err := s.componentToken(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.env.Wx.SetAuthorizerOption(ctx, token, s.componentAppid(), appid, string(req.OptionName), value); err != nil {
		return nil, wxErr(appid, err)
	}
	s.writeOperation(ctx, actionSetOption, targetAuthorizer, appid, model.JSONMap{
		"optionName":  string(req.OptionName),
		"optionValue": value,
	})
	return &gen.AuthorizerOption{OptionName: req.OptionName, OptionValue: value}, nil
}

// OnAuthorized 处理授权成功 / 更新授权事件（由回调业务分发调用）。
//
// 两点官方事实决定了这里的实现：
//   - 授权成功后微信既跳转 redirect_uri?auth_code=xxx（前端回调页），也推送 authorized 事件；
//     两者竞争时事件里可能拿不到可用的授权码，因此 authCode 为空不是错误，只做令牌刷新兜底；
//   - 事件会重复推送，因此落库必须幂等（见 registerAuthorized）。
func (s *AuthorizerService) OnAuthorized(ctx context.Context, appid, authCode, infoType string) error {
	if err := s.ensureWeChat(); err != nil {
		return err
	}
	appid = strings.TrimSpace(appid)
	authCode = strings.TrimSpace(authCode)

	if authCode == "" {
		detail := model.JSONMap{"infoType": infoType, "authCode": ""}
		switch {
		case appid == "":
			detail["note"] = "事件未携带授权方 appid，无法刷新令牌，仅留痕"
		default:
			// 仅尝试用已有 refresh_token 刷新一次令牌，让「授权更新」尽快生效；失败不影响回调。
			if _, err := s.env.Tokens.AuthorizerToken(ctx, appid); err != nil {
				detail["refreshError"] = err.Error()
				detail["note"] = "事件未携带授权码，尝试刷新已有令牌失败（前端回调页可能已先完成换取）"
			} else {
				detail["note"] = "事件未携带授权码，已用已有 refresh_token 刷新令牌"
			}
		}
		s.writeOperation(ctx, actionAuthorizedEvent, targetAuthorizer, appid, detail)
		return nil
	}

	resp, err := s.CompleteAuthorize(ctx, gen.AuthorizeRequest{AuthCode: authCode})
	if err != nil {
		return err
	}
	if appid != "" && appid != resp.Appid {
		// 事件里的 AuthorizerAppid 与授权码换取结果不一致：以换取结果为准，并留痕便于排查。
		s.writeOperation(ctx, actionAuthorizedEvent, targetAuthorizer, resp.Appid, model.JSONMap{
			"infoType":   infoType,
			"eventAppid": appid,
			"note":       "事件 appid 与授权码换取结果不一致，已按换取结果登记",
		})
	}
	return nil
}

// OnUnauthorized 处理取消授权通知。
//
// 按官方语义：记录必须保留（历史作业、审核单与日志仍要可查），只把状态置为 unauthorized、
// 清空令牌缓存与已失效的 refresh_token 密文；备注 / 分组 / 标签等本地维护字段原样保留。
func (s *AuthorizerService) OnUnauthorized(ctx context.Context, appid string) error {
	appid = strings.TrimSpace(appid)
	if appid == "" {
		return core.Validation("取消授权通知缺少授权方 appid")
	}
	if err := s.ensureRepos(); err != nil {
		return err
	}
	now := time.Now()

	current, err := s.repos().Authorizers.Get(ctx, appid)
	if err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			// 平台还没登记过该 appid（例如授权通知早于本地登记）：保持幂等，留痕即可。
			s.writeOperation(ctx, actionUnauthorized, targetAuthorizer, appid, model.JSONMap{
				"note": "本地没有该小程序的记录，仅留痕（不报错，避免微信重推）",
			})
			return nil
		}
		return core.Internal(err)
	}

	if err := s.repos().Authorizers.MarkUnauthorized(ctx, appid, now); err != nil {
		if errors.Is(err, repo.ErrNotFound) {
			s.writeOperation(ctx, actionUnauthorized, targetAuthorizer, appid, model.JSONMap{"note": "标记时记录已不存在，仅留痕"})
			return nil
		}
		return core.Internal(err)
	}
	// 清空令牌：删除 authorizer_access_token 缓存 + 清掉已失效的 refresh_token 密文。
	if s.env.WeChatReady() {
		if err := s.env.Tokens.InvalidateAuthorizer(ctx, appid); err != nil {
			return core.Internal(fmt.Errorf("清空授权方令牌缓存失败: %w", err))
		}
	}
	if err := s.repos().Authorizers.UpdateFields(ctx, appid, map[string]any{
		"refresh_token_cipher":     nil,
		"refresh_token_updated_at": nil,
	}); err != nil && !errors.Is(err, repo.ErrNotFound) {
		return core.Internal(err)
	}

	s.writeOperation(ctx, actionUnauthorized, targetAuthorizer, appid, model.JSONMap{
		"previousStatus": string(current.AuthorizationStatus),
		"remark":         current.Remark,
		"groupName":      current.GroupName,
	})
	return nil
}

// OnMessage 代收的用户消息：本平台只做审计留痕，不自动回复。
func (s *AuthorizerService) OnMessage(ctx context.Context, appid, plainXML string) error {
	if err := s.ensureRepos(); err != nil {
		return err
	}
	s.writeOperation(ctx, actionMessageReceived, targetAuthorizer, strings.TrimSpace(appid), model.JSONMap{
		"length": len(plainXML),
		"xml":    truncate(plainXML, maxMessageLogBytes),
	})
	return nil
}

// parseCategoryIDList 解析逗号分隔的权限集 id 列表（形如 "18,30"）。
func parseCategoryIDList(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == '|' || r == ';' || r == ' '
	})
	out := make([]int, 0, len(parts))
	seen := map[int]bool{}
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		id, err := strconv.Atoi(trimmed)
		if err != nil || id <= 0 {
			return nil, core.Validation("categoryIdList 里的 %q 不是合法的权限集 id：请用逗号分隔的数字，例如 18,30", trimmed)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

// validateOptionValue 校验选项值（取值来自官方「option_name 及 option_value 说明」）。
func validateOptionValue(name gen.AuthorizerOptionName, value string) error {
	switch name {
	case gen.LocationReport:
		if value != "0" && value != "1" && value != "2" {
			return core.Validation("location_report 的取值只能是 0（无上报）/ 1（进入会话时上报）/ 2（每 5 秒上报）")
		}
	case gen.VoiceRecognize, gen.CustomerService:
		if value != "0" && value != "1" {
			return core.Validation("%s 的取值只能是 0（关闭）或 1（开启）", string(name))
		}
	default:
		return core.Validation("optionName 只能是 location_report / voice_recognize / customer_service")
	}
	return nil
}

// describeFuncIDs 把权限集 id 列表转成「id（名称）」形式，便于运维核对。
func describeFuncIDs(ids []int) string {
	if len(ids) == 0 {
		return "无"
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := model.PermissionSetNames[id]; ok {
			parts = append(parts, fmt.Sprintf("%d（%s）", id, name))
			continue
		}
		parts = append(parts, strconv.Itoa(id))
	}
	return strings.Join(parts, "、")
}

// containsInt 判断切片是否包含某个整数。
func containsInt(list []int, want int) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

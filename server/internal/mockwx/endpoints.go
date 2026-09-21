package mockwx

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// mockJPEG 一张最小的合法 JPEG（1x1 像素，b64 解码后为 160 字节），
// 用于 /wxa/get_qrcode —— 官方该接口成功时直接返回二进制图片而不是 JSON。
const mockJPEG = "/9j/4AAQSkZJRgABAQEAYABgAAD/2wBDAAgGBgcGBQgHBwcJCQgKDBQNDAsLDBkSEw8UHRofHh0a" +
	"HBwgJC4nICIsIxwcKDcpLDAxNDQ0Hyc5PTgyPC4zNDL/wAALCAABAAEBAREA/8QAFAABAAAAAAAA" +
	"AAAAAAAAAAAACf/EABQQAQAAAAAAAAAAAAAAAAAAAAD/2gAIAQEAAD8AKp//2Q=="

// reasonScreenshot 审核不通过时的截图示例（竖线分隔的 media_id 列表）。
const reasonScreenshot = "mock_media_shot_1|mock_media_shot_2"

// defaultFailReason 审核不通过时的默认原因。
const defaultFailReason = "1:账号信息不符合规范:<br>(1):包含色情因素<br>2:功能页面设置的部分标签不属于所选的服务类目范围。"

// mockJPEGBytes 解码内置的最小 JPEG（解码失败时返回 nil，调用方仍会返回 200 + image/jpeg）。
func mockJPEGBytes() []byte {
	b, err := base64.StdEncoding.DecodeString(mockJPEG)
	if err != nil {
		return nil
	}
	return b
}

// buildRoutes 组装全部路由。
func (s *Server) buildRoutes() map[string]endpoint {
	routes := map[string]endpoint{}

	// ---- 第三方平台自身（component_access_token）----
	routes["/cgi-bin/component/api_component_token"] = endpoint{method: http.MethodPost, kind: tokenNone, handle: s.handleComponentToken}
	routes["/cgi-bin/component/api_start_push_ticket"] = endpoint{method: http.MethodPost, kind: tokenNone, handle: s.handleStartPushTicket}
	routes["/cgi-bin/component/api_create_preauthcode"] = endpoint{method: http.MethodPost, kind: tokenComponent, handle: s.handleCreatePreauthCode}
	routes["/cgi-bin/component/api_query_auth"] = endpoint{method: http.MethodPost, kind: tokenComponent, handle: s.handleQueryAuth}
	routes["/cgi-bin/component/api_authorizer_token"] = endpoint{method: http.MethodPost, kind: tokenComponent, handle: s.handleAuthorizerToken}
	routes["/cgi-bin/component/api_get_authorizer_list"] = endpoint{method: http.MethodPost, kind: tokenComponent, handle: s.handleGetAuthorizerList}
	routes["/cgi-bin/component/api_get_authorizer_info"] = endpoint{method: http.MethodPost, kind: tokenComponent, handle: s.handleGetAuthorizerInfo}
	// get/set_authorizer_option：中文页写 component_access_token、英文页写 authorizer_access_token（官方自相矛盾），
	// 故按 tokenAny 处理。
	routes["/cgi-bin/component/get_authorizer_option"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleGetAuthorizerOption}
	routes["/cgi-bin/component/set_authorizer_option"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleSetAuthorizerOption}
	// 模板库：仅第三方平台自身调用。
	routes["/wxa/gettemplatedraftlist"] = endpoint{method: http.MethodGet, kind: tokenComponent, handle: s.handleTemplateDraftList}
	routes["/wxa/addtotemplate"] = endpoint{method: http.MethodPost, kind: tokenComponent, handle: s.handleAddToTemplate}
	routes["/wxa/gettemplatelist"] = endpoint{method: http.MethodGet, kind: tokenComponent, handle: s.handleTemplateList}
	routes["/wxa/deletetemplate"] = endpoint{method: http.MethodPost, kind: tokenComponent, handle: s.handleDeleteTemplate}
	// 类目 / 隐私 / 域名 / 服务状态 / 基础库：官方为「代商家调用（权限集 18）」，
	// 但任务口径把它们归到平台侧，故一律按 tokenAny 放宽，且不校验权限集。
	routes["/wxa/get_category"] = endpoint{method: http.MethodGet, kind: tokenAny, handle: s.handleGetCategory}
	routes["/cgi-bin/wxopen/getcategory"] = endpoint{method: http.MethodGet, kind: tokenAny, handle: s.handleGetCategoryWxopen}
	routes["/cgi-bin/component/getprivacysetting"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleGetPrivacySetting}
	routes["/wxa/modify_domain"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleModifyDomain}
	routes["/wxa/modify_domain_directly"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleModifyDomain}
	routes["/wxa/setwebviewdomain"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleSetWebviewDomain}
	routes["/wxa/setwebviewdomain_directly"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleSetWebviewDomain}
	routes["/wxa/get_effective_domain"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleGetEffectiveDomain}
	routes["/wxa/get_effective_webviewdomain"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleGetEffectiveWebviewDomain}
	routes["/wxa/change_visitstatus"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleChangeVisitStatus}
	routes["/wxa/getvisitstatus"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleGetVisitStatus}
	routes["/cgi-bin/wxopen/getweappsupportversion"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleGetSupportVersion}
	routes["/cgi-bin/wxopen/setweappsupportversion"] = endpoint{method: http.MethodPost, kind: tokenAny, handle: s.handleSetSupportVersion}

	// ---- 代商家操作（authorizer_access_token，权限集 18）----
	code := []int{codePermID}
	routes["/wxa/commit"] = endpoint{method: http.MethodPost, kind: tokenAuthorizer, perms: code, handle: s.handleCommit}
	routes["/wxa/get_page"] = endpoint{method: http.MethodGet, kind: tokenAuthorizer, perms: code, handle: s.handleGetPage}
	routes["/wxa/get_qrcode"] = endpoint{method: http.MethodGet, kind: tokenAuthorizer, perms: []int{codePermID, qrPermID}, handle: s.handleGetQRCode}
	routes["/wxa/security/get_code_privacy_info"] = endpoint{method: http.MethodGet, kind: tokenAuthorizer, perms: code, handle: s.handleGetCodePrivacyInfo}
	routes["/wxa/submit_audit"] = endpoint{method: http.MethodPost, kind: tokenAuthorizer, perms: code, handle: s.handleSubmitAudit}
	routes["/wxa/get_auditstatus"] = endpoint{method: http.MethodPost, kind: tokenAuthorizer, perms: code, handle: s.handleGetAuditStatus}
	routes["/wxa/get_latest_auditstatus"] = endpoint{method: http.MethodGet, kind: tokenAuthorizer, perms: code, handle: s.handleGetLatestAuditStatus}
	routes["/wxa/undocodeaudit"] = endpoint{method: http.MethodGet, kind: tokenAuthorizer, perms: code, handle: s.handleUndoCodeAudit}
	routes["/wxa/release"] = endpoint{method: http.MethodPost, kind: tokenAuthorizer, perms: code, handle: s.handleRelease}
	routes["/wxa/getversioninfo"] = endpoint{method: http.MethodPost, kind: tokenAuthorizer, perms: code, handle: s.handleGetVersionInfo}
	routes["/wxa/queryquota"] = endpoint{method: http.MethodGet, kind: tokenAuthorizer, perms: code, handle: s.handleQueryQuota}
	routes["/wxa/speedupaudit"] = endpoint{method: http.MethodPost, kind: tokenAuthorizer, perms: code, handle: s.handleSpeedupAudit}
	routes["/wxa/revertcoderelease"] = endpoint{method: http.MethodGet, kind: tokenAuthorizer, perms: code, handle: s.handleRevertCodeRelease}

	// 类目（全量可选类目，官方接口名 getAllCategories）：任务书未要求，补齐以便上层提审流程使用。
	routes["/cgi-bin/wxopen/getallcategories"] = endpoint{method: http.MethodGet, kind: tokenAny, handle: s.handleGetAllCategories}
	// 分阶段（灰度）发布：任务书未要求，同上。
	routes["/wxa/grayrelease"] = endpoint{method: http.MethodPost, kind: tokenAuthorizer, perms: code, handle: s.handleGrayRelease}
	routes["/wxa/getgrayreleaseplan"] = endpoint{method: http.MethodGet, kind: tokenAuthorizer, perms: code, handle: s.handleGetGrayReleasePlan}
	routes["/wxa/revertgrayrelease"] = endpoint{method: http.MethodGet, kind: tokenAuthorizer, perms: code, handle: s.handleRevertGrayRelease}

	// 提审素材上传（multipart）。任务书未要求，但它是提审链路常见的前置步骤，
	// 这里给一个宽容实现：不校验文件内容，直接返回可用 mediaid，避免上层链路因 404 中断。
	routes["/wxa/uploadmedia"] = endpoint{method: http.MethodPost, kind: tokenAuthorizer, perms: code, handle: s.handleUploadMedia}

	// ---- 平台侧测试驱动辅助接口（非微信官方接口）----
	routes["/mock/authorize"] = endpoint{method: http.MethodPost, kind: tokenNone, handle: s.handleMockAuthorize}

	return routes
}

// ------------- 令牌与授权 -------------

// handleComponentToken 用 component_verify_ticket 换 component_access_token。
// 没有收到过任何 ticket（或传了不认识的 ticket）时按官方语义返回 61005 / 61006。
func (s *Server) handleComponentToken(rc *request) {
	ticket := rc.str("component_verify_ticket")
	s.mu.Lock()
	_, known := s.ticketSet[ticket]
	ever := len(s.ticketSet) > 0
	if known {
		token, _ := s.issueTokenLocked(kindComponent, "")
		s.mu.Unlock()
		rc.ok(field("component_access_token", token), field("expires_in", int(tokenTTL/time.Second)))
		return
	}
	s.mu.Unlock()
	if !ever {
		// 官方 61005：component ticket is expired。模拟器尚未收到过 ticket，等价于「没有可用票据」。
		rc.fail(61005, "component ticket is expired: mock 尚未收到 component_verify_ticket，请先 PushTicket")
		return
	}
	rc.fail(61006, "component ticket is invalid: 该 ticket 不是本模拟器推送过的值")
}

// handleStartPushTicket 启动票据推送服务。模拟器里只是空实现（真实微信会开始每 10 分钟推送 ticket）。
func (s *Server) handleStartPushTicket(rc *request) {
	rc.ok()
}

// handleCreatePreauthCode 获取预授权码。
func (s *Server) handleCreatePreauthCode(rc *request) {
	s.mu.Lock()
	s.authCodeSeq++
	code := fmt.Sprintf("mock_pre_auth_code_%d", s.authCodeSeq)
	s.preAuthCodes[code] = time.Now().Add(preAuthCodeTTL)
	s.mu.Unlock()
	rc.ok(field("pre_auth_code", code), field("expires_in", int(preAuthCodeTTL/time.Second)))
}

// handleQueryAuth 授权码换 authorizer_access_token + authorizer_refresh_token。
func (s *Server) handleQueryAuth(rc *request) {
	code := rc.str("authorization_code")
	s.mu.Lock()
	rec, ok := s.authCodes[code]
	if !ok || time.Now().After(rec.expireAt) {
		s.mu.Unlock()
		// 官方文档未规定「授权码无效」的专用码，按任务书允许自定义可解释的错误码。
		rc.fail(40013, "invalid authorization_code：模拟器中不存在该授权码或已过期")
		return
	}
	a := s.ensureAuthorizerLocked(rec.appid)
	a.authorized = true
	if a.authTime == 0 {
		a.authTime = time.Now().Unix()
	}
	if a.refreshToken == "" {
		a.refreshToken = "mock_refresh_" + a.appid
	}
	token, _ := s.issueTokenLocked(kindAuthorizer, a.appid)
	payload := map[string]any{
		"authorizer_appid":         a.appid,
		"authorizer_access_token":  token,
		"expires_in":               int(tokenTTL / time.Second),
		"authorizer_refresh_token": a.refreshToken,
		"func_info":                funcInfoJSON(a.funcInfoIDs),
	}
	s.mu.Unlock()
	rc.ok(field("authorization_info", payload))
}

// handleAuthorizerToken 用 authorizer_refresh_token 刷新 authorizer_access_token。
func (s *Server) handleAuthorizerToken(rc *request) {
	appid := rc.str("authorizer_appid")
	refresh := rc.str("authorizer_refresh_token")
	s.mu.Lock()
	a, ok := s.auths[appid]
	if !ok || refresh == "" || refresh != a.refreshToken || !a.authorized {
		s.mu.Unlock()
		rc.fail(40013, "invalid authorizer_refresh_token：authorizer_appid 与 refresh_token 不匹配或已取消授权")
		return
	}
	token, _ := s.issueTokenLocked(kindAuthorizer, appid)
	cur := a.refreshToken
	s.mu.Unlock()
	// 小程序的 refresh_token 在有效期内不变，这里原样返回，便于上层「带回就覆盖」的逻辑保持幂等。
	rc.ok(
		field("authorizer_access_token", token),
		field("expires_in", int(tokenTTL/time.Second)),
		field("authorizer_refresh_token", cur),
	)
}

// funcInfoJSON 把权限集 id 列表转成官方 func_info 结构。
func funcInfoJSON(ids []int) []map[string]any {
	out := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		out = append(out, map[string]any{"funcscope_category": map[string]any{"id": id}})
	}
	return out
}

// handleGetAuthorizerList 拉取已授权账号列表（支持 offset / count）。
func (s *Server) handleGetAuthorizerList(rc *request) {
	offset, _ := rc.num("offset")
	count, ok := rc.num("count")
	if !ok || count <= 0 || count > 500 {
		count = 500
	}
	type row struct {
		appid    string
		refresh  string
		authTime int64
	}
	s.mu.Lock()
	var all []row
	for _, appid := range s.authOrder {
		a := s.auths[appid]
		if !a.authorized {
			continue
		}
		all = append(all, row{appid: a.appid, refresh: a.refreshToken, authTime: a.authTime})
	}
	s.mu.Unlock()

	total := len(all)
	start := int(offset)
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := start + int(count)
	if end > total {
		end = total
	}
	list := make([]map[string]any, 0, end-start)
	for _, r := range all[start:end] {
		list = append(list, map[string]any{
			"authorizer_appid": r.appid,
			"refresh_token":    r.refresh,
			"auth_time":        r.authTime,
		})
	}
	rc.ok(field("total_count", total), field("list", list))
}

// handleGetAuthorizerInfo 获取授权账号详情。
func (s *Server) handleGetAuthorizerInfo(rc *request) {
	appid := rc.str("authorizer_appid")
	if appid == "" {
		rc.fail(40013, "invalid appid: body.authorizer_appid 必填")
		return
	}
	s.mu.Lock()
	a, ok := s.auths[appid]
	if !ok {
		s.mu.Unlock()
		rc.fail(40013, "invalid appid: 模拟器未登记该小程序")
		return
	}
	info := map[string]any{
		"nick_name":         a.nickName,
		"head_img":          a.headImg,
		"qrcode_url":        a.qrcodeURL,
		"user_name":         a.userName,
		"alias":             a.alias,
		"principal_name":    a.principalName,
		"service_type_info": map[string]any{"id": 0, "name": "小程序"},
		"verify_type_info":  map[string]any{"id": 0, "name": "未认证"},
		"business_info":     map[string]any{"open_pay": 0, "open_shake": 0, "open_scan": 0, "open_card": 0, "open_store": 0},
		"register_type":     0,
		"account_status":    1,
		"basic_config":      map[string]any{"is_phone_configured": true, "is_email_configured": true},
		"MiniProgramInfo": map[string]any{
			"network": map[string]any{
				"RequestDomain":   orEmpty(a.requestDomain),
				"WsRequestDomain": orEmpty(a.wsRequestDomain),
				"UploadDomain":    orEmpty(a.uploadDomain),
				"DownloadDomain":  orEmpty(a.downloadDomain),
			},
			"categories": []any{},
		},
	}
	auth := map[string]any{
		"authorizer_appid":         a.appid,
		"authorizer_refresh_token": a.refreshToken,
		"func_info":                funcInfoJSON(a.funcInfoIDs),
	}
	s.mu.Unlock()
	rc.ok(field("authorizer_info", info), field("authorization_info", auth))
}

// handleGetAuthorizerOption 获取授权方选项信息。
func (s *Server) handleGetAuthorizerOption(rc *request) {
	optionName := rc.str("option_name")
	if optionName == "" {
		rc.fail(61012, "invalid option name")
		return
	}
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	s.mu.Lock()
	value, exists := a.options[optionName]
	s.mu.Unlock()
	if !exists {
		rc.fail(61012, "invalid option name: 模拟器只支持 location_report / voice_recognize / customer_service")
		return
	}
	rc.ok(field("option_name", optionName), field("option_value", value))
}

// handleSetAuthorizerOption 设置授权方选项信息。
func (s *Server) handleSetAuthorizerOption(rc *request) {
	optionName := rc.str("option_name")
	optionValue := rc.str("option_value")
	if optionName == "" {
		rc.fail(61012, "invalid option name")
		return
	}
	if optionValue == "" {
		rc.fail(61013, "invalid option value")
		return
	}
	allowed, known := map[string][]string{
		"location_report":  {"0", "1", "2"},
		"voice_recognize":  {"0", "1"},
		"customer_service": {"0", "1"},
	}[optionName]
	if !known {
		rc.fail(61012, "invalid option name: 模拟器只支持 location_report / voice_recognize / customer_service")
		return
	}
	valid := false
	for _, v := range allowed {
		if v == optionValue {
			valid = true
			break
		}
	}
	if !valid {
		rc.fail(61013, "invalid option value: "+optionName+" 只接受 "+strings.Join(allowed, "/"))
		return
	}
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	s.mu.Lock()
	a.options[optionName] = optionValue
	s.mu.Unlock()
	rc.ok()
}

// ------------- 模板库 -------------

// handleTemplateDraftList 获取草稿箱列表（模拟器预置若干条草稿，便于走通「草稿 → 模板」链路）。
func (s *Server) handleTemplateDraftList(rc *request) {
	s.mu.Lock()
	ids := make([]int64, 0, len(s.drafts))
	for id := range s.drafts {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	list := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		d := s.drafts[id]
		list = append(list, map[string]any{
			"create_time":              d.createTime,
			"user_version":             d.userVersion,
			"user_desc":                d.userDesc,
			"draft_id":                 d.id,
			"source_miniprogram_appid": d.sourceAppID,
			"source_miniprogram":       d.sourceName,
			"developer":                d.developer,
			"category_list":            []any{},
		})
	}
	s.mu.Unlock()
	rc.ok(field("draft_list", list))
}

// handleAddToTemplate 把草稿添加到模板库（模板库上限 200，超限 85065）。
func (s *Server) handleAddToTemplate(rc *request) {
	draftID, ok := rc.num("draft_id")
	if !ok {
		rc.fail(47001, "data format error: draft_id 必填")
		return
	}
	templateType, _ := rc.num("template_type")

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.templates) >= templateLimit {
		rc.fail(85065, "template list is full")
		return
	}
	d, ok := s.drafts[draftID]
	if !ok {
		rc.fail(85064, "template not found: 草稿不存在")
		return
	}
	s.nextTemplateID++
	t := &templateRec{
		id:           s.nextTemplateID,
		draftID:      d.id,
		createTime:   d.createTime,
		userVersion:  d.userVersion,
		userDesc:     d.userDesc,
		templateType: int(templateType),
		sourceAppID:  d.sourceAppID,
		sourceName:   d.sourceName,
	}
	s.templates[t.id] = t
	rc.ok(field("template_id", t.id))
}

// handleTemplateList 获取模板列表（支持 template_type 过滤：0 普通模板 / 1 标准模板；不填返回全部）。
func (s *Server) handleTemplateList(rc *request) {
	filter := -1
	if raw := rc.r.URL.Query().Get("template_type"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || (n != 0 && n != 1) {
			rc.fail(40097, "invalid args: template_type 只能是 0 或 1")
			return
		}
		filter = n
	}
	s.mu.Lock()
	ids := make([]int64, 0, len(s.templates))
	for id := range s.templates {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	list := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		t := s.templates[id]
		if filter >= 0 && t.templateType != filter {
			continue
		}
		list = append(list, map[string]any{
			"create_time":              t.createTime,
			"user_version":             t.userVersion,
			"user_desc":                t.userDesc,
			"template_id":              t.id,
			"draft_id":                 t.draftID,
			"source_miniprogram_appid": t.sourceAppID,
			"source_miniprogram":       t.sourceName,
			"template_type":            t.templateType,
			"category_list":            []any{},
		})
	}
	s.mu.Unlock()
	rc.ok(field("template_list", list))
}

// handleDeleteTemplate 删除代码模板。
func (s *Server) handleDeleteTemplate(rc *request) {
	id, ok := rc.num("template_id")
	if !ok {
		rc.fail(47001, "data format error: template_id 必填")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.templates[id]; !exists {
		rc.fail(85064, "template not found")
		return
	}
	delete(s.templates, id)
	rc.ok()
}

// ------------- 类目 / 隐私 / 域名 / 服务状态 / 基础库 -------------

// handleGetCategory 获取可选类目（模拟器至少给出 2 组，含三级类目）。
func (s *Server) handleGetCategory(rc *request) {
	list := []map[string]any{
		{"first_class": "工具", "second_class": "信息查询", "third_class": "天气", "first_id": 1, "second_id": 101, "third_id": 1001},
		{"first_class": "教育", "second_class": "教育信息服务", "third_class": "", "first_id": 2, "second_id": 201, "third_id": 0},
		{"first_class": "生活服务", "second_class": "跑腿代购", "third_class": "", "first_id": 3, "second_id": 301, "third_id": 0},
	}
	rc.ok(field("category_list", list))
}

// handleGetAllCategories 获取可以设置的所有类目（官方接口名 getAllCategories，接口不在任务书必做清单内）。
func (s *Server) handleGetAllCategories(rc *request) {
	list := []map[string]any{
		{"first": 1, "first_name": "工具", "second": 101, "second_name": "信息查询", "third": 1001, "third_name": "天气"},
		{"first": 2, "first_name": "教育", "second": 201, "second_name": "教育信息服务", "third": 0, "third_name": ""},
		{"first": 3, "first_name": "生活服务", "second": 301, "second_name": "跑腿代购", "third": 0, "third_name": ""},
	}
	rc.ok(field("categories_list", list))
}

// handleGetCategoryWxopen 获取账号已设置的类目。
func (s *Server) handleGetCategoryWxopen(rc *request) {
	categories := []map[string]any{
		{"first": 1, "first_name": "工具", "second": 101, "second_name": "信息查询", "audit_status": 3, "audit_reason": ""},
		{"first": 2, "first_name": "教育", "second": 201, "second_name": "教育信息服务", "audit_status": 3, "audit_reason": ""},
	}
	rc.ok(
		field("categories", categories),
		field("limit", 5),
		field("quota", 4),
		field("category_limit", 20),
	)
}

// handleGetPrivacySetting 获取小程序用户隐私保护指引。
func (s *Server) handleGetPrivacySetting(rc *request) {
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	s.mu.Lock()
	setting, exists := s.privacySettings[a.appid]
	if !exists {
		setting = &privacySetting{
			codeExist:   1,
			privacyList: []string{"UserInfo"},
			settingList: []any{
				map[string]any{
					"privacy_key":   "UserInfo",
					"privacy_text":  "你的微信昵称、头像、地区及性别",
					"privacy_label": []string{"微信昵称", "微信头像"},
				},
			},
			ownerSetting: map[string]any{
				"contact_email":       "mock@example.com",
				"contact_phone":       "",
				"contact_qq":          "",
				"contact_weixin":      "",
				"notice_method":       "弹窗",
				"store_region":        []string{"中国"},
				"ext_file_media_id":   "",
				"notice_pic_media_id": "",
			},
			updateTime: time.Now().Unix(),
		}
		s.privacySettings[a.appid] = setting
	}
	codeExist := setting.codeExist
	privacyList := setting.privacyList
	settingList := setting.settingList
	ownerSetting := setting.ownerSetting
	updateTime := setting.updateTime
	s.mu.Unlock()

	rc.ok(
		field("code_exist", codeExist),
		field("privacy_list", privacyList),
		field("setting_list", settingList),
		field("owner_setting", ownerSetting),
		field("update_time", updateTime),
	)
}

// domainField 域名接口的四个域名维度。
type domainField struct {
	key string
	set func(a *authorizer, v []string)
	get func(a *authorizer) []string
}

// domainFields 返回四个域名维度（顺序与官方一致）。
func domainFields() []domainField {
	return []domainField{
		{key: "requestdomain", set: func(a *authorizer, v []string) { a.requestDomain = v }, get: func(a *authorizer) []string { return a.requestDomain }},
		{key: "wsrequestdomain", set: func(a *authorizer, v []string) { a.wsRequestDomain = v }, get: func(a *authorizer) []string { return a.wsRequestDomain }},
		{key: "uploaddomain", set: func(a *authorizer, v []string) { a.uploadDomain = v }, get: func(a *authorizer) []string { return a.uploadDomain }},
		{key: "downloaddomain", set: func(a *authorizer, v []string) { a.downloadDomain = v }, get: func(a *authorizer) []string { return a.downloadDomain }},
	}
}

// handleModifyDomain 配置服务器域名（add / delete / set / get）。
// modify_domain_directly 复用本实现：对模拟器而言两者状态一致。
func (s *Server) handleModifyDomain(rc *request) {
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	action := rc.str("action")
	if action == "" {
		action = "get"
	}
	fields := domainFields()
	s.mu.Lock()
	if action == "get" {
		out := make([]kv, 0, len(fields))
		for _, f := range fields {
			out = append(out, field(f.key, orEmpty(f.get(a))))
		}
		s.mu.Unlock()
		rc.ok(out...)
		return
	}
	if action != "add" && action != "delete" && action != "set" {
		s.mu.Unlock()
		rc.fail(40097, "invalid args: action 只能是 add / delete / set / get")
		return
	}
	for _, f := range fields {
		in := rc.strSlice(f.key)
		if in == nil {
			continue
		}
		switch action {
		case "set":
			f.set(a, in)
		case "add":
			f.set(a, mergeStrings(f.get(a), in))
		case "delete":
			f.set(a, removeStrings(f.get(a), in))
		}
	}
	s.mu.Unlock()
	rc.ok()
}

// handleSetWebviewDomain 配置业务域名（add / delete / set / get）。
// setwebviewdomain_directly 复用本实现。
func (s *Server) handleSetWebviewDomain(rc *request) {
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	action := rc.str("action")
	if action == "" {
		action = "get"
	}
	s.mu.Lock()
	switch action {
	case "get":
		domains := orEmpty(a.webviewDomains)
		s.mu.Unlock()
		rc.ok(field("webviewdomain", domains))
		return
	case "set":
		a.webviewDomains = append([]string{}, rc.strSlice("webviewdomain")...)
	case "add":
		a.webviewDomains = mergeStrings(a.webviewDomains, rc.strSlice("webviewdomain"))
	case "delete":
		a.webviewDomains = removeStrings(a.webviewDomains, rc.strSlice("webviewdomain"))
	default:
		s.mu.Unlock()
		rc.fail(40097, "invalid args: action 只能是 add / delete / set / get")
		return
	}
	s.mu.Unlock()
	rc.ok()
}

// handleGetEffectiveDomain 获取生效的服务器域名。官方要求必须 POST 空 JSON `{}`，否则 44002。
func (s *Server) handleGetEffectiveDomain(rc *request) {
	if !rc.requireObject(true) {
		return
	}
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	fields := domainFields()
	s.mu.Lock()
	out := make([]kv, 0, len(fields))
	for _, f := range fields {
		out = append(out, field(f.key, orEmpty(f.get(a))))
	}
	s.mu.Unlock()
	rc.ok(out...)
}

// handleGetEffectiveWebviewDomain 获取生效的业务域名。官方要求必须 POST 空 JSON `{}`，否则 44002。
func (s *Server) handleGetEffectiveWebviewDomain(rc *request) {
	if !rc.requireObject(true) {
		return
	}
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	s.mu.Lock()
	domains := orEmpty(a.webviewDomains)
	s.mu.Unlock()
	rc.ok(field("webviewdomain", domains))
}

// handleChangeVisitStatus 设置小程序服务状态（close 不可见 / open 可见）。
func (s *Server) handleChangeVisitStatus(rc *request) {
	action := rc.str("action")
	if action != "close" && action != "open" {
		rc.fail(40097, "invalid args: action 只能是 close / open")
		return
	}
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	s.mu.Lock()
	if action == "close" {
		a.visitStatus = 0
	} else {
		a.visitStatus = 1
	}
	s.mu.Unlock()
	rc.ok()
}

// handleGetVisitStatus 查询小程序服务状态（0 已暂停 / 1 未暂停）。官方要求 POST 空 JSON。
func (s *Server) handleGetVisitStatus(rc *request) {
	if !rc.requireObject(false) {
		return
	}
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	s.mu.Lock()
	status := a.visitStatus
	s.mu.Unlock()
	rc.ok(field("status", status))
}

// handleGetSupportVersion 查询各版本用户占比。官方要求 POST 空 JSON。
func (s *Server) handleGetSupportVersion(rc *request) {
	if !rc.requireObject(false) {
		return
	}
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	s.mu.Lock()
	now := a.supportVersion
	s.mu.Unlock()
	items := []map[string]any{
		{"version": now, "percentage": 62.5},
		{"version": "2.19.4", "percentage": 25.0},
		{"version": "2.10.4", "percentage": 12.5},
	}
	rc.ok(field("now_version", now), field("uv_info", map[string]any{"items": items}))
}

// versionPattern 基础库版本号形如 2.20.1。
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// handleSetSupportVersion 设置最低基础库版本。
func (s *Server) handleSetSupportVersion(rc *request) {
	version := rc.str("version")
	if !versionPattern.MatchString(version) {
		rc.fail(89014, "support version error: version 形如 2.20.1")
		return
	}
	a, ok := rc.resolveAppid()
	if !ok {
		return
	}
	s.mu.Lock()
	a.supportVersion = version
	s.mu.Unlock()
	rc.ok()
}

// ------------- 代商家操作：代码 / 提审 / 发布 -------------

// handleCommit 上传代码并生成体验版，同时启动隐私检测任务。
func (s *Server) handleCommit(rc *request) {
	if rc.inConcurrentWindow("commit") {
		rc.fail(9402202, "the previous commit task is still in progress, please try again later")
		return
	}
	userVersion := rc.str("user_version")
	if utf8.RuneCountInString(userVersion) > 64 {
		rc.fail(47001, "data format error: user_version 长度不能超过 64 个字符")
		return
	}
	extJSON := rc.str("ext_json")
	if extJSON == "" || !json.Valid([]byte(extJSON)) {
		rc.fail(85048, "parse ext_json fail")
		return
	}
	// 说明：真实微信会用 template_id 校验模板是否存在（85064）；模拟器有意跳过该校验，
	// 让上层不必先跑「草稿 → 模板」链路也能验证提审/发布主链路。
	s.mu.Lock()
	a := rc.auth
	a.expVersion = userVersion
	a.expDesc = rc.str("user_desc")
	a.expTime = time.Now().Unix()
	a.extJSON = extJSON
	a.privacyDoneAt = time.Now().Add(s.privacyDelay)
	s.mu.Unlock()
	rc.ok()
}

// handleUploadMedia 上传提审素材（multipart/form-data，字段名 media）。
// 模拟器不解析文件内容，直接返回一个可用的 mediaid（官方返回体为 type + mediaid）。
func (s *Server) handleUploadMedia(rc *request) {
	s.mu.Lock()
	s.mediaSeq++
	mediaID := fmt.Sprintf("mock_media_%d", s.mediaSeq)
	s.mu.Unlock()
	rc.ok(field("type", "image"), field("mediaid", mediaID))
}

// handleGetPage 获取可配置的页面列表。
func (s *Server) handleGetPage(rc *request) {
	rc.ok(field("page_list", []string{"index", "pages/list/index"}))
}

// handleGetQRCode 获取体验版二维码：成功时直接返回 image/jpeg 二进制（与官方一致）。
// 失败（例如 ForceErrcode 注入）时由统一错误通道返回 JSON。
func (s *Server) handleGetQRCode(rc *request) {
	w := rc.w
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Disposition", `attachment; filename="QRCode.jpg"`)
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(mockJPEGBytes())
}

// handleGetCodePrivacyInfo 获取隐私接口检测结果；检测任务未完成时返回 61039。
func (s *Server) handleGetCodePrivacyInfo(rc *request) {
	s.mu.Lock()
	doneAt := rc.auth.privacyDoneAt
	s.mu.Unlock()
	if !doneAt.IsZero() && time.Now().Before(doneAt) {
		rc.fail(61039, "检查任务未完成，请稍等一分钟再重试")
		return
	}
	rc.ok(field("without_auth_list", []string{}), field("without_conf_list", []string{}))
}

// handleSubmitAudit 提交代码审核：生成自增 auditid，状态置 2（审核中）。
func (s *Server) handleSubmitAudit(rc *request) {
	if rc.inConcurrentWindow("submit_audit") {
		rc.fail(9402202, "the previous submit audit task is still in progress, please try again later")
		return
	}
	s.mu.Lock()
	a := rc.auth
	if a.expVersion == "" {
		s.mu.Unlock()
		rc.fail(85086, "no commit code: 请先调用 /wxa/commit 上传代码")
		return
	}
	for _, rec := range a.audits {
		if rec.status == 2 {
			s.mu.Unlock()
			rc.fail(85009, "audit already in progress: 已有审核中版本")
			return
		}
	}
	if !a.privacyDoneAt.IsZero() && time.Now().Before(a.privacyDoneAt) {
		s.mu.Unlock()
		rc.fail(61039, "检查任务未完成，请稍等一分钟再重试")
		return
	}
	if s.quota.Rest <= 0 {
		s.mu.Unlock()
		rc.fail(85085, "no quota to submit audit")
		return
	}
	a.auditSeq++
	rec := &auditRec{
		id:          a.auditSeq,
		status:      2,
		userVersion: a.expVersion,
		userDesc:    a.expDesc,
		submitTime:  time.Now().Unix(),
	}
	a.audits[rec.id] = rec
	a.latestAuditID = rec.id
	s.quota.Rest--
	s.mu.Unlock()
	rc.ok(field("auditid", rec.id))
}

// handleGetAuditStatus 查询指定审核单状态。
func (s *Server) handleGetAuditStatus(rc *request) {
	id, ok := rc.num("auditid")
	if !ok {
		rc.fail(85012, "invalid audit id")
		return
	}
	s.mu.Lock()
	rec, exists := rc.auth.audits[id]
	s.mu.Unlock()
	if !exists {
		rc.fail(85012, "invalid audit id")
		return
	}
	rc.ok(
		field("status", rec.status),
		field("reason", rec.reason),
		field("screenshot", rec.screenshot),
	)
}

// handleGetLatestAuditStatus 查询最新一次审核单状态。
//
// 注意：截图字段故意用官方返回示例里的大写 ScreenShot（文档正文写作 screenshot），
// 用于覆盖真实存在的大小写不一致。
func (s *Server) handleGetLatestAuditStatus(rc *request) {
	s.mu.Lock()
	a := rc.auth
	rec, exists := a.audits[a.latestAuditID]
	s.mu.Unlock()
	if !exists {
		rc.fail(86001, "no audit record: 该小程序还没有提审记录")
		return
	}
	rc.ok(
		field("auditid", rec.id),
		field("status", rec.status),
		field("reason", rec.reason),
		field("ScreenShot", rec.screenshot),
		field("user_version", rec.userVersion),
		field("user_desc", rec.userDesc),
		field("submit_audit_time", rec.submitTime),
	)
}

// handleUndoCodeAudit 撤回代码审核（每天 5 次、每月 10 次限制，超限 87013）。
func (s *Server) handleUndoCodeAudit(rc *request) {
	s.mu.Lock()
	a := rc.auth
	today := time.Now().Format("2006-01-02")
	month := time.Now().Format("2006-01")
	if a.undoDay != today {
		a.undoDay = today
		a.undoDayCount = 0
	}
	if a.undoMonth != month {
		a.undoMonth = month
		a.undoMonthCount = 0
	}
	if a.undoDayCount >= 5 || a.undoMonthCount >= 10 {
		s.mu.Unlock()
		rc.fail(87013, "no quota to undo code: 每天最多 5 次、每月最多 10 次")
		return
	}
	rec, exists := a.audits[a.latestAuditID]
	if !exists || rec.status != 2 {
		s.mu.Unlock()
		rc.fail(85009, "no audit in progress: 没有审核中的版本，无法撤回")
		return
	}
	rec.status = 3
	a.undoDayCount++
	a.undoMonthCount++
	s.mu.Unlock()
	rc.ok()
}

// handleRelease 发布已通过审核的小程序。官方要求必须 POST 空 JSON `{}`，否则 44002。
func (s *Server) handleRelease(rc *request) {
	if !rc.requireObject(false) {
		return
	}
	s.mu.Lock()
	a := rc.auth
	var target *auditRec
	for id := a.latestAuditID; id >= 1; id-- {
		if rec, exists := a.audits[id]; exists && rec.status == 0 {
			target = rec
			break
		}
	}
	if target == nil {
		s.mu.Unlock()
		rc.fail(85019, "no version is under auditing: 没有审核通过的版本可发布")
		return
	}
	if target.userVersion == a.releaseVersion {
		// 重复发布同一版本：幂等返回 ok（便于测试重放），不再重复入历史。
		a.releaseTime = time.Now().Unix()
		target.released = true
		s.mu.Unlock()
		rc.ok()
		return
	}
	if a.releaseVersion != "" {
		a.history = append(a.history, historyVersion{
			appVersion:  a.releaseAppVersion,
			userVersion: a.releaseVersion,
			userDesc:    a.releaseDesc,
			commitTime:  a.releaseTime,
		})
		// 官方：最多保存最近发布或回退的 5 个版本。
		if len(a.history) > 5 {
			a.history = a.history[len(a.history)-5:]
		}
	}
	a.versionSeq++
	a.releaseAppVersion = a.versionSeq
	a.releaseVersion = target.userVersion
	a.releaseDesc = target.userDesc
	a.releaseTime = time.Now().Unix()
	target.released = true
	s.mu.Unlock()
	rc.ok()
}

// handleGrayRelease 创建/调整分阶段发布计划（灰度比例只能递增）。
func (s *Server) handleGrayRelease(rc *request) {
	percent, hasPercent := rc.num("gray_percentage")
	debugFirst := rc.str("support_debuger_first") == "true" || rc.body["support_debuger_first"] == true
	expFirst := rc.str("support_experiencer_first") == "true" || rc.body["support_experiencer_first"] == true
	s.mu.Lock()
	defer s.mu.Unlock()
	a := rc.auth
	if a.releaseVersion == "" {
		rc.fail(85079, "no released version: 没有线上版本，无法灰度发布")
		return
	}
	if !hasPercent || percent < 0 || percent > 100 {
		rc.fail(85081, "invalid gray percentage: gray_percentage 必须是 0~100 的整数")
		return
	}
	if percent == 0 && !debugFirst && !expFirst {
		rc.fail(85081, "invalid gray percentage: gray_percentage=0 时 support_debuger_first / support_experiencer_first 二选一必填")
		return
	}
	if a.gray != nil && a.gray.status == 1 && int(percent) <= a.gray.grayPercentage {
		rc.fail(85082, "当前灰度比例需比之前设置的比例高")
		return
	}
	status := 1
	if percent == 100 {
		status = 3 // 执行完毕
	}
	plan := &grayPlan{
		status:                  status,
		createTimestamp:         time.Now().Unix(),
		grayPercentage:          int(percent),
		supportDebugerFirst:     debugFirst,
		supportExperiencerFirst: expFirst,
	}
	if a.gray != nil {
		plan.createTimestamp = a.gray.createTimestamp
	}
	a.gray = plan
	rc.ok()
}

// handleGetGrayReleasePlan 获取分阶段发布详情（无计划时 status=0 初始状态）。
func (s *Server) handleGetGrayReleasePlan(rc *request) {
	s.mu.Lock()
	a := rc.auth
	plan := map[string]any{
		"status": 0, "create_timestamp": int64(0), "gray_percentage": 0,
		"support_debuger_first": false, "support_experiencer_first": false,
	}
	if a.gray != nil {
		plan = map[string]any{
			"status":                    a.gray.status,
			"create_timestamp":          a.gray.createTimestamp,
			"gray_percentage":           a.gray.grayPercentage,
			"support_debuger_first":     a.gray.supportDebugerFirst,
			"support_experiencer_first": a.gray.supportExperiencerFirst,
		}
	}
	s.mu.Unlock()
	rc.ok(field("gray_release_plan", plan))
}

// handleRevertGrayRelease 取消分阶段发布。
func (s *Server) handleRevertGrayRelease(rc *request) {
	s.mu.Lock()
	a := rc.auth
	if a.gray != nil {
		a.gray.status = 4 // 被删除
	}
	s.mu.Unlock()
	rc.ok()
}

// handleGetVersionInfo 查询小程序版本信息。官方要求必须 POST 空 JSON `{}`。
func (s *Server) handleGetVersionInfo(rc *request) {
	if !rc.requireObject(false) {
		return
	}
	s.mu.Lock()
	a := rc.auth
	exp := map[string]any{"exp_time": a.expTime, "exp_version": a.expVersion, "exp_desc": a.expDesc}
	release := map[string]any{"release_time": a.releaseTime, "release_version": a.releaseVersion, "release_desc": a.releaseDesc}
	s.mu.Unlock()
	rc.ok(field("exp_info", exp), field("release_info", release))
}

// handleQueryQuota 查询服务商审核额度（所有旗下小程序共用）。
func (s *Server) handleQueryQuota(rc *request) {
	s.mu.Lock()
	q := s.quota
	s.mu.Unlock()
	rc.ok(
		field("rest", q.Rest),
		field("limit", q.Limit),
		field("speedup_rest", q.SpeedupRest),
		field("speedup_limit", q.SpeedupLimit),
	)
}

// handleSpeedupAudit 加急代码审核（额度用尽返回 89405）。
func (s *Server) handleSpeedupAudit(rc *request) {
	id, ok := rc.num("auditid")
	if !ok {
		rc.fail(85012, "invalid audit id")
		return
	}
	s.mu.Lock()
	if s.quota.SpeedupRest <= 0 {
		s.mu.Unlock()
		rc.fail(89405, "本月加急额度已用完，请提高提审质量以获取更多额度")
		return
	}
	a := rc.auth
	rec, exists := a.audits[id]
	if !exists {
		s.mu.Unlock()
		rc.fail(85012, "invalid audit id")
		return
	}
	if rec.status != 2 {
		s.mu.Unlock()
		rc.fail(89402, "该小程序不在待审核队列，请检查是否已提交审核或已审完")
		return
	}
	if rec.speedup {
		s.mu.Unlock()
		rc.fail(89404, "本单已加速成功，请勿重复提交")
		return
	}
	rec.speedup = true
	s.quota.SpeedupRest--
	s.mu.Unlock()
	rc.ok()
}

// handleRevertCodeRelease 小程序版本回退。
//
// action=get_history_version 返回可回退版本列表；带 app_version 则回退到指定版本（不带则回退上一个版本）。
// 回退后，比目标版本更新的历史版本会被丢弃，再次回退同一版本返回 87012（与官方「当前版本回退后不能再回退」一致）。
func (s *Server) handleRevertCodeRelease(rc *request) {
	query := rc.r.URL.Query()
	action := query.Get("action")
	s.mu.Lock()
	defer s.mu.Unlock()
	a := rc.auth
	if action == "get_history_version" {
		list := make([]map[string]any, 0, len(a.history))
		for _, h := range a.history {
			list = append(list, map[string]any{
				"app_version":  h.appVersion,
				"user_version": h.userVersion,
				"user_desc":    h.userDesc,
				"commit_time":  h.commitTime,
			})
		}
		rc.ok(field("version_list", list))
		return
	}
	if action != "" {
		rc.fail(40097, "invalid args: action 只能填 get_history_version")
		return
	}
	if len(a.history) == 0 {
		rc.fail(87012, "forbid revert this version release: 没有上一个线上版本")
		return
	}
	idx := len(a.history) - 1
	if raw := query.Get("app_version"); raw != "" {
		want, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			rc.fail(40097, "invalid args: app_version 必须是数字")
			return
		}
		idx = -1
		for i, h := range a.history {
			if h.appVersion == want {
				idx = i
				break
			}
		}
		if idx < 0 {
			rc.fail(87012, "forbid revert this version release: 该版本不可回退（已回退过的版本或历史版本已超出 5 个）")
			return
		}
	}
	target := a.history[idx]
	a.history = append([]historyVersion{}, a.history[:idx]...)
	a.releaseVersion = target.userVersion
	a.releaseDesc = target.userDesc
	a.releaseAppVersion = target.appVersion
	a.releaseTime = time.Now().Unix()
	rc.ok()
}

// handleMockAuthorize 平台侧辅助接口（非微信官方）：把一个待授权/已登记的小程序置为已授权。
func (s *Server) handleMockAuthorize(rc *request) {
	appid := rc.str("appid")
	if appid == "" {
		rc.fail(40013, "invalid appid: body.appid 必填")
		return
	}
	s.mu.Lock()
	a := s.ensureAuthorizerLocked(appid)
	a.authorized = true
	a.authTime = time.Now().Unix()
	if a.refreshToken == "" {
		a.refreshToken = "mock_refresh_" + appid
	}
	s.mu.Unlock()
	rc.ok(field("appid", appid))
}

// mergeStrings 追加不重复元素。
func mergeStrings(base, add []string) []string {
	out := append([]string{}, base...)
	for _, v := range add {
		found := false
		for _, exists := range out {
			if exists == v {
				found = true
				break
			}
		}
		if !found {
			out = append(out, v)
		}
	}
	return out
}

// removeStrings 移除指定元素。
func removeStrings(base, drop []string) []string {
	out := make([]string, 0, len(base))
	for _, v := range base {
		hit := false
		for _, d := range drop {
			if d == v {
				hit = true
				break
			}
		}
		if !hit {
			out = append(out, v)
		}
	}
	return out
}

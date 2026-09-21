package wxapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"wx-platform/server/internal/model"
)

// ---------------------------------------------------------------------------
// 代码管理（代商家调用）
//
// 本文件所有接口的令牌都是 **authorizer_access_token**（代商家调用），
// 权限集 id 均为 18（「小程序开发与数据分析」）；URL 中的参数名统一为 access_token。
// 注意：/wxa/* 代调用接口不需要额外传 component_appid（见 reference §5.0）。
// ---------------------------------------------------------------------------

// CommitRequest 上传代码并生成体验版。
type CommitRequest struct {
	TemplateID  int64  `json:"template_id"`
	ExtJSON     string `json:"ext_json"`     // 必须是 JSON 字符串（字符串化的 JSON）
	UserVersion string `json:"user_version"` // ≤64 字符
	UserDesc    string `json:"user_desc"`
}

// Commit 上传代码并生成体验版。
//
// 文档：POST /wxa/commit?access_token=TOKEN（code_commit.md）。
// token：authorizer_access_token；权限集 18。
// 限制：user_version ≤ 64 字符；ext_json 必须是转义后的 JSON 字符串；
// 上传后需等检测任务结束（getCodePrivacyInfo）才能提审，否则 61039；
// 不要与提审一起重试（9402202 concurrent limit）；用错模板类型报 9402203。
func (c *Client) Commit(ctx context.Context, token, appid string, req CommitRequest) error {
	return errOrNil(c.postJSON(ctx, "/wxa/commit", model.TokenScopeAuthorizer, appid, token, req, nil))
}

// GetPage 获取已上传代码的页面列表（item_list.address 的来源之一）。
//
// 文档：GET /wxa/get_page?access_token=TOKEN（code_getcodepage.md）。
// —— 文档要求 GET。
// token：authorizer_access_token；权限集 18。
func (c *Client) GetPage(ctx context.Context, token, appid string) ([]string, error) {
	var out struct {
		PageList []string `json:"page_list"`
	}
	if err := c.getJSON(ctx, "/wxa/get_page", model.TokenScopeAuthorizer, appid, token, nil, &out); err != nil {
		return nil, err
	}
	return out.PageList, nil
}

// GetTrialQRCode 获取体验版二维码（成功时直接返回二进制图片，不是 JSON）。
//
// 文档：GET /wxa/get_qrcode?access_token=TOKEN&path=PATH（code_gettrialqrcode.md）。
// —— 文档要求 GET；path 需要 urlencode（如 page/index?action=1 → page%2Findex%3Faction%3D1），
// 这里交给 url.Values.Encode() 完成转义。
// token：authorizer_access_token（文档写「可使用 access_token、authorizer_access_token」）；权限集 18、86。
// 成功时 content-type 形如 image/jpeg 并带 Content-disposition；
// 失败时微信返回 JSON errcode（40001 / 40014 / 43001 等），此时返回 *APIError。
func (c *Client) GetTrialQRCode(ctx context.Context, token, appid, path string) (contentType string, data []byte, err error) {
	q := url.Values{}
	if path != "" {
		q.Set("path", path)
	}
	spec := callSpec{
		method: http.MethodGet,
		path:   "/wxa/get_qrcode",
		scope:  model.TokenScopeAuthorizer,
		appid:  appid,
		token:  token,
		query:  q,
		binary: true,
	}
	res, apiErr := c.do(ctx, spec)
	if apiErr != nil {
		return "", nil, apiErr
	}
	if looksLikeJSON(res.contentType, res.body) {
		// 兜底：微信正常成功时不会返回 JSON；若返回 JSON 且 errcode==0，说明没有图片。
		return res.contentType, nil, &APIError{
			Endpoint:   spec.path,
			Method:     spec.method,
			Appid:      appid,
			Errcode:    0,
			Errmsg:     "微信未返回图片内容（返回了 JSON）",
			HTTPStatus: res.status,
			Raw:        truncateRaw(res.body),
		}
	}
	return res.contentType, res.body, nil
}

// SubmitAuditItem 提审项（item_list 元素）。
type SubmitAuditItem struct {
	Address     string `json:"address,omitempty"`
	Tag         string `json:"tag,omitempty"`
	FirstClass  string `json:"first_class"`
	SecondClass string `json:"second_class"`
	ThirdClass  string `json:"third_class,omitempty"`
	Title       string `json:"title,omitempty"`
	FirstID     int    `json:"first_id"`
	SecondID    int    `json:"second_id"`
	ThirdID     int    `json:"third_id,omitempty"`
}

// SubmitAuditPreview 提审预览信息（截图与操作录屏）。
type SubmitAuditPreview struct {
	VideoIDList []string `json:"video_id_list,omitempty"`
	PicIDList   []string `json:"pic_id_list,omitempty"`
}

// UGCDeclare UGC 信息安全声明。
type UGCDeclare struct {
	Scene          []int  `json:"scene"`
	Method         int    `json:"method,omitempty"`
	OtherSceneDesc string `json:"other_scene_desc,omitempty"`
	HasAuditTeam   int    `json:"has_audit_team,omitempty"`
	AuditDesc      string `json:"audit_desc,omitempty"`
}

// SubmitAuditRequest 提交代码审核请求。
type SubmitAuditRequest struct {
	ItemList         []SubmitAuditItem   `json:"item_list"`
	FeedbackInfo     string              `json:"feedback_info,omitempty"`
	FeedbackStuff    string              `json:"feedback_stuff,omitempty"`
	VersionDesc      string              `json:"version_desc,omitempty"`
	PreviewInfo      *SubmitAuditPreview `json:"preview_info,omitempty"`
	UGCDeclare       *UGCDeclare         `json:"ugc_declare,omitempty"`
	PrivacyAPINotUse *bool               `json:"privacy_api_not_use,omitempty"`
	OrderPath        string              `json:"order_path,omitempty"`
}

// SubmitAuditResponse 提交代码审核响应。
type SubmitAuditResponse struct {
	AuditID int64 `json:"auditid"`
}

// UnmarshalJSON 兼容 auditid 为 number 与 string 两种 JSON 类型。
//
// 文档字段表写 number，而官方示例/部分真实返回是字符串（如 "auditid": "1234567"）。
func (r *SubmitAuditResponse) UnmarshalJSON(b []byte) error {
	var aux struct {
		AuditID json.RawMessage `json:"auditid"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	r.AuditID = flexInt64(aux.AuditID)
	return nil
}

// SubmitAudit 提交代码审核。
//
// 文档：POST /wxa/submit_audit?access_token=TOKEN（code_submitaudit.md）。
// token：authorizer_access_token；权限集 18。
// 限制：item_list 1-5 项（超出报 85023）；类目必须来自 getAllCategoryName；
// title ≤ 32 字、tag ≤ 10 个且每个 ≤ 20 字；version_desc/preview_info 超限报 85051；
// 提审数量达本月上限报 85085（额度为服务商级、旗下小程序共用）；
// 提审前必须先 commit（85086），并等隐私检测任务结束（61039）。
func (c *Client) SubmitAudit(ctx context.Context, token, appid string, req SubmitAuditRequest) (*SubmitAuditResponse, error) {
	var out SubmitAuditResponse
	if err := c.postJSON(ctx, "/wxa/submit_audit", model.TokenScopeAuthorizer, appid, token, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UploadMedia 上传提审素材（截图 / 录屏），返回 mediaid（有效期 3 天）。
//
// 文档：POST /wxa/uploadmedia?access_token=TOKEN（code_uploadmediatocodeaudit.md），
// multipart/form-data，表单字段名固定为 media。
// token：authorizer_access_token；权限集 18。
// 限制：图片 ≤ 2M（PNG/JPEG/JPG/GIF），视频 ≤ 10MB（MP4）；
// 格式不对报 40005，超限报 40006，缺少文件数据报 41005。
func (c *Client) UploadMedia(ctx context.Context, token, appid, filename string, content []byte, contentType string) (string, error) {
	spec := callSpec{
		method: http.MethodPost,
		path:   "/wxa/uploadmedia",
		scope:  model.TokenScopeAuthorizer,
		appid:  appid,
		token:  token,
		multipart: &multipartBody{
			fieldName:   "media",
			fileName:    filename,
			contentType: contentType,
			content:     content,
		},
	}
	var out struct {
		Type    string `json:"type"`
		MediaID string `json:"mediaid"`
	}
	if err := c.jsonCall(ctx, spec, &out); err != nil {
		return "", err
	}
	return out.MediaID, nil
}

// PrivacyInfo 代码隐私接口检测结果。
type PrivacyInfo struct {
	WithoutAuthList []string `json:"without_auth_list"`
	WithoutConfList []string `json:"without_conf_list"`
}

// GetCodePrivacyInfo 获取隐私接口检测结果（提交审核前的必需检查）。
//
// 文档：GET /wxa/security/get_code_privacy_info?access_token=TOKEN（code_getcodeprivacyinfo.md）。
// —— 文档要求 GET。
// token：authorizer_access_token；权限集 18。
// 检测任务未结束时返回 61039（等约 1 分钟再试）；存在未配置/无权限的隐私接口时提交审核报 61040。
func (c *Client) GetCodePrivacyInfo(ctx context.Context, token, appid string) (*PrivacyInfo, error) {
	var out PrivacyInfo
	if err := c.getJSON(ctx, "/wxa/security/get_code_privacy_info", model.TokenScopeAuthorizer, appid, token, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AuditStatusResponse 审核单状态。
//
// 官方字段表用小写 screenshot，而 get_latest_auditstatus 的示例/推送用大写 ScreenShot，
// 两个字段都解析（哪个非空由上层判断）。
type AuditStatusResponse struct {
	AuditID         int64  `json:"auditid"`
	Status          int    `json:"status"` // 0 成功 / 1 被拒 / 2 审核中 / 3 已撤回 / 4 延后
	Reason          string `json:"reason"`
	Screenshot      string `json:"screenshot"` // 官方字段表用小写
	ScreenShotAlt   string `json:"ScreenShot"` // 官方示例用大写，两者都要兼容
	UserVersion     string `json:"user_version"`
	UserDesc        string `json:"user_desc"`
	SubmitAuditTime int64  `json:"submit_audit_time"`
}

// UnmarshalJSON 让 auditid 同样兼容 number 与 string，并容忍 ScreenShot 大小写。
func (r *AuditStatusResponse) UnmarshalJSON(b []byte) error {
	type alias AuditStatusResponse
	var aux struct {
		*alias
		AuditID json.RawMessage `json:"auditid"`
	}
	aux.alias = (*alias)(r)
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	r.AuditID = flexInt64(aux.AuditID)
	return nil
}

// GetAuditStatus 查询指定审核单状态。
//
// 文档：POST /wxa/get_auditstatus?access_token=TOKEN（code_getauditstatus.md）。
// token：authorizer_access_token；权限集 18。auditid 无效返回 85012。
func (c *Client) GetAuditStatus(ctx context.Context, token, appid string, auditID int64) (*AuditStatusResponse, error) {
	body := struct {
		AuditID int64 `json:"auditid"`
	}{AuditID: auditID}
	var out AuditStatusResponse
	if err := c.postJSON(ctx, "/wxa/get_auditstatus", model.TokenScopeAuthorizer, appid, token, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetLatestAuditStatus 查询最新一次审核单状态。
//
// 文档：GET /wxa/get_latest_auditstatus?access_token=TOKEN（code_getlatestauditstatus.md）。
// —— 文档要求 GET。
// token：authorizer_access_token；权限集 18。
func (c *Client) GetLatestAuditStatus(ctx context.Context, token, appid string) (*AuditStatusResponse, error) {
	var out AuditStatusResponse
	if err := c.getJSON(ctx, "/wxa/get_latest_auditstatus", model.TokenScopeAuthorizer, appid, token, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UndoCodeAudit 撤回代码审核。
//
// 文档：GET /wxa/undocodeaudit?access_token=TOKEN（code_undoaudit.md）。
// —— 文档要求 GET。
// token：authorizer_access_token；权限集 18。
// 限制：单账号每天最多 5 次、每月最多 10 次，超限返回 87013。
func (c *Client) UndoCodeAudit(ctx context.Context, token, appid string) error {
	return errOrNil(c.getJSON(ctx, "/wxa/undocodeaudit", model.TokenScopeAuthorizer, appid, token, nil, nil))
}

// Release 发布已通过审核的小程序。
//
// 文档：POST /wxa/release?access_token=TOKEN（code_release.md）。
// —— 文档要求 POST **必须发空 JSON {}**：
// 官方注意事项原文「post 的 data 为空，不等于不需要传 data，否则会报错 {"errcode": 44002, "errmsg": "empty post data"}」。
// token：authorizer_access_token；权限集 18。无可发布版本报 85019 / 85020。
func (c *Client) Release(ctx context.Context, token, appid string) error {
	return errOrNil(c.postEmpty(ctx, "/wxa/release", model.TokenScopeAuthorizer, appid, token, nil))
}

// VersionPart 体验版 / 线上版信息。
type VersionPart struct {
	ExpTime        int64  `json:"exp_time"`
	ExpVersion     string `json:"exp_version"`
	ExpDesc        string `json:"exp_desc"`
	ReleaseTime    int64  `json:"release_time"`
	ReleaseVersion string `json:"release_version"`
	ReleaseDesc    string `json:"release_desc"`
}

// VersionInfoResponse 小程序版本信息。
type VersionInfoResponse struct {
	ExpInfo     VersionPart `json:"exp_info"`
	ReleaseInfo VersionPart `json:"release_info"`
}

// GetVersionInfo 查询小程序版本信息（体验版 + 线上版）。
//
// 文档：POST /wxa/getversioninfo?access_token=TOKEN（code_getversioninfo.md）。
// —— 文档要求 POST **必须发空 JSON {}**（错误码表列了 44002 empty post data）。
// token：authorizer_access_token；权限集 18。
// 注：不存在 /wxa/get_release_status 这个接口，等价能力就是本接口。
func (c *Client) GetVersionInfo(ctx context.Context, token, appid string) (*VersionInfoResponse, error) {
	var out VersionInfoResponse
	if err := c.postEmpty(ctx, "/wxa/getversioninfo", model.TokenScopeAuthorizer, appid, token, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// HistoryVersion 可回退的历史版本。
type HistoryVersion struct {
	AppVersion  int64  `json:"app_version"`
	UserVersion string `json:"user_version"`
	UserDesc    string `json:"user_desc"`
	CommitTime  int64  `json:"commit_time"`
}

// RevertCodeRelease 小程序版本回退。
//
// 文档：GET /wxa/revertcoderelease?access_token=TOKEN[&app_version=123]（code_revertcoderelease.md）。
// —— 文档要求 GET；app_version 是 **URL 参数，非 Body 参数**。
// token：authorizer_access_token；权限集 18。
// 语义：appVersion == nil 时回退到上一个版本；非 nil 时回退到指定版本。
// 限制：无上一个线上版本 / 该版本已回退过 / 版本过旧，都会返回 87012；
// 最多保留最近 5 个发布或回退版本。
func (c *Client) RevertCodeRelease(ctx context.Context, token, appid string, appVersion *int64) error {
	q := url.Values{}
	if appVersion != nil {
		q.Set("app_version", strconv.FormatInt(*appVersion, 10))
	}
	return errOrNil(c.getJSON(ctx, "/wxa/revertcoderelease", model.TokenScopeAuthorizer, appid, token, q, nil))
}

// ListHistoryVersions 查询可回退的小程序版本列表（action=get_history_version）。
//
// 文档：GET /wxa/revertcoderelease?action=get_history_version&access_token=TOKEN
// （code_revertcoderelease.md §5.2）。
// —— 文档要求 GET；action 是 **URL 参数**。
// token：authorizer_access_token；权限集 18。
// 冲突：文档把 version_list 的类型写成 object，但嵌套字段表列的是「列表元素」；
// 真实返回为数组 —— 这里两种都兼容（数组 / 单个对象 / 缺失）。
func (c *Client) ListHistoryVersions(ctx context.Context, token, appid string) ([]HistoryVersion, error) {
	q := url.Values{}
	q.Set("action", "get_history_version")
	var out struct {
		VersionList json.RawMessage `json:"version_list"`
	}
	if err := c.getJSON(ctx, "/wxa/revertcoderelease", model.TokenScopeAuthorizer, appid, token, q, &out); err != nil {
		return nil, err
	}
	return flexHistoryVersions(out.VersionList)
}

// flexHistoryVersions 兼容数组 / 单个对象 / 缺失三种 version_list 写法。
func flexHistoryVersions(raw json.RawMessage) ([]HistoryVersion, error) {
	trimmed := string(raw)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}
	var list []HistoryVersion
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil
	}
	var one HistoryVersion
	if err := json.Unmarshal(raw, &one); err == nil {
		return []HistoryVersion{one}, nil
	}
	return nil, nil
}

// SpeedUpAudit 加急代码审核。
//
// 文档：POST /wxa/speedupaudit?access_token=TOKEN（code_speedupcodeaudit.md）。
// token：authorizer_access_token；权限集 18。
// 限制：加急后预计 2-12 小时审完；89401 系统不稳定 / 89402 不在待审核队列 /
// 89403 不支持加急 / 89404 已加急成功 / 89405 本月加急额度用完。
func (c *Client) SpeedUpAudit(ctx context.Context, token, appid string, auditID int64) error {
	body := struct {
		AuditID int64 `json:"auditid"`
	}{AuditID: auditID}
	return errOrNil(c.postJSON(ctx, "/wxa/speedupaudit", model.TokenScopeAuthorizer, appid, token, body, nil))
}

// QuotaResponse 服务商审核额度。
type QuotaResponse struct {
	Rest         int `json:"rest"`
	Limit        int `json:"limit"`
	SpeedupRest  int `json:"speedup_rest"`
	SpeedupLimit int `json:"speedup_limit"`
}

// QueryQuota 查询服务商审核额度（提审与加急额度，旗下小程序共用）。
//
// 文档：GET /wxa/queryquota?access_token=TOKEN（code_setcodeauditquota.md）。
// —— 文档要求 GET。
// token：authorizer_access_token；权限集 18。额度用尽继续提审返回 85085。
func (c *Client) QueryQuota(ctx context.Context, token, appid string) (*QuotaResponse, error) {
	var out QuotaResponse
	if err := c.getJSON(ctx, "/wxa/queryquota", model.TokenScopeAuthorizer, appid, token, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GrayReleaseRequest 分阶段发布请求。
type GrayReleaseRequest struct {
	GrayPercentage          int  `json:"gray_percentage"`
	SupportDebugerFirst     bool `json:"support_debuger_first,omitempty"`
	SupportExperiencerFirst bool `json:"support_experiencer_first,omitempty"`
}

// GrayRelease 分阶段发布（灰度发布）。
//
// 文档：POST /wxa/grayrelease?access_token=TOKEN（code_grayrelease.md）。
// token：authorizer_access_token；权限集 18。
// 限制：gray_percentage 为 0~100 整数；=0 时 support_experiencer_first / support_debuger_first 二选一必填；
// 比例只能递增（85082）；无线上版本报 85079；提审未通过报 85080；无效比例报 85081。
func (c *Client) GrayRelease(ctx context.Context, token, appid string, req GrayReleaseRequest) error {
	return errOrNil(c.postJSON(ctx, "/wxa/grayrelease", model.TokenScopeAuthorizer, appid, token, req, nil))
}

// GrayPlan 分阶段发布计划。
type GrayPlan struct {
	Status                  int   `json:"status"`
	CreateTimestamp         int64 `json:"create_timestamp"`
	GrayPercentage          int   `json:"gray_percentage"`
	SupportDebugerFirst     bool  `json:"support_debuger_first"`
	SupportExperiencerFirst bool  `json:"support_experiencer_first"`
}

// GetGrayReleasePlan 获取分阶段发布详情。
//
// 文档：GET /wxa/getgrayreleaseplan?access_token=TOKEN（code_getgrayreleaseplan.md）。
// —— 文档要求 GET；返回体为 gray_release_plan 对象。
// token：authorizer_access_token；权限集 18。
func (c *Client) GetGrayReleasePlan(ctx context.Context, token, appid string) (*GrayPlan, error) {
	var out struct {
		GrayReleasePlan GrayPlan `json:"gray_release_plan"`
	}
	if err := c.getJSON(ctx, "/wxa/getgrayreleaseplan", model.TokenScopeAuthorizer, appid, token, nil, &out); err != nil {
		return nil, err
	}
	return &out.GrayReleasePlan, nil
}

// RevertGrayRelease 取消分阶段发布。
//
// 文档：GET /wxa/revertgrayrelease?access_token=TOKEN（code_revertgrayrelease.md）。
// —— 文档要求 GET。
// token：authorizer_access_token；权限集 18。
func (c *Client) RevertGrayRelease(ctx context.Context, token, appid string) error {
	return errOrNil(c.getJSON(ctx, "/wxa/revertgrayrelease", model.TokenScopeAuthorizer, appid, token, nil, nil))
}

// SetVisitStatus 设置小程序服务状态。
//
// 文档：POST /wxa/change_visitstatus?access_token=TOKEN（code_setvisitstatus.md）。
// token：authorizer_access_token；权限集 18。
// action 取 "close"（暂停服务 / 不可见）或 "open"（可见）。
func (c *Client) SetVisitStatus(ctx context.Context, token, appid, action string) error {
	body := struct {
		Action string `json:"action"`
	}{Action: action}
	return errOrNil(c.postJSON(ctx, "/wxa/change_visitstatus", model.TokenScopeAuthorizer, appid, token, body, nil))
}

// GetVisitStatus 查询小程序服务状态：0 已暂停 / 1 未暂停。
//
// 文档：POST /wxa/getvisitstatus?access_token=TOKEN（code_getvisitstatus.md）。
// —— 文档要求 POST **必须发空 JSON {}**（错误码表列了 44002 empty post data）。
// token：authorizer_access_token；权限集 18。
func (c *Client) GetVisitStatus(ctx context.Context, token, appid string) (int, error) {
	var out struct {
		Status int `json:"status"`
	}
	if err := c.postEmpty(ctx, "/wxa/getvisitstatus", model.TokenScopeAuthorizer, appid, token, &out); err != nil {
		return 0, err
	}
	return out.Status, nil
}

// SupportVersionResponse 基础库版本用户占比。
type SupportVersionResponse struct {
	NowVersion string `json:"now_version"`
	UVInfo     struct {
		Items []struct {
			Version    string  `json:"version"`
			Percentage float64 `json:"percentage"`
		} `json:"items"`
	} `json:"uv_info"`
}

// GetSupportVersion 查询各版本基础库用户占比。
//
// 文档：POST /cgi-bin/wxopen/getweappsupportversion?access_token=TOKEN（code_getsupportversion.md）。
// —— 文档要求 POST **必须发空 JSON {}**（与其他空 body 接口同理，否则 44002）。
// token：authorizer_access_token；权限集 18。注意 base path 是 /cgi-bin/wxopen/ 而不是 /wxa/。
func (c *Client) GetSupportVersion(ctx context.Context, token, appid string) (*SupportVersionResponse, error) {
	var out SupportVersionResponse
	if err := c.postEmpty(ctx, "/cgi-bin/wxopen/getweappsupportversion", model.TokenScopeAuthorizer, appid, token, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetSupportVersion 设置小程序最低基础库版本。
//
// 文档：POST /cgi-bin/wxopen/setweappsupportversion?access_token=TOKEN（code_setsupportversion.md）。
// token：authorizer_access_token；权限集 18。版本号必须是已发布的基础库版本，否则 89014。
func (c *Client) SetSupportVersion(ctx context.Context, token, appid, version string) error {
	body := struct {
		Version string `json:"version"`
	}{Version: version}
	return errOrNil(c.postJSON(ctx, "/cgi-bin/wxopen/setweappsupportversion", model.TokenScopeAuthorizer, appid, token, body, nil))
}

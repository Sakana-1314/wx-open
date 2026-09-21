package wxapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"wx-platform/server/internal/model"
)

// ---------------------------------------------------------------------------
// 类目 / 隐私 / 域名
//
// 这一组的 base path 不统一（/wxa/*、/cgi-bin/wxopen/*、/cgi-bin/component/*），
// 且同在 /wxa/ 下也有 GET 与 POST 混用 —— 每个方法都逐个核对过官方单页文档。
// ---------------------------------------------------------------------------

// CategoryName 已设置类目 / 可设置类目。
type CategoryName struct {
	FirstClass  string `json:"first_class"`
	SecondClass string `json:"second_class"`
	ThirdClass  string `json:"third_class"`
	FirstID     int    `json:"first_id"`
	SecondID    int    `json:"second_id"`
	ThirdID     int    `json:"third_id"`
}

// GetAllCategoryName 获取已设置的所有类目名称 + id（提审 item_list 六个类目字段的唯一来源）。
//
// 文档：GET /wxa/get_category?access_token=TOKEN（category-management/api_getallcategoryname.html）。
// —— 文档要求 GET。
// token：authorizer_access_token；权限集 18。
// 限制：返回的都是「已在小程序侧配置好」的类目；当前小程序没有审核通过的类目时提审报 85008。
func (c *Client) GetAllCategoryName(ctx context.Context, token, appid string) ([]CategoryName, error) {
	var out struct {
		CategoryList []CategoryName `json:"category_list"`
	}
	if err := c.getJSON(ctx, "/wxa/get_category", model.TokenScopeAuthorizer, appid, token, nil, &out); err != nil {
		return nil, err
	}
	return out.CategoryList, nil
}

// SettingCategory 已设置的类目条目。
type SettingCategory struct {
	First       int    `json:"first"`
	FirstName   string `json:"first_name"`
	Second      int    `json:"second"`
	SecondName  string `json:"second_name"`
	AuditStatus int    `json:"audit_status"`
	AuditReason string `json:"audit_reason"`
}

// SettingCategoriesResponse 已设置类目查询结果。
type SettingCategoriesResponse struct {
	Categories    []SettingCategory `json:"categories"`
	Limit         int               `json:"limit"`
	Quota         int               `json:"quota"`
	CategoryLimit int               `json:"category_limit"`
}

// GetSettingCategories 获取已设置的所有类目（含审核状态与配额）。
//
// 文档：GET /cgi-bin/wxopen/getcategory?access_token=TOKEN（category-management/api_getsettingcategories.html）。
// —— 文档要求 GET；注意 base path 是 /cgi-bin/wxopen/。
// token：authorizer_access_token；权限集 **30**（类目管理，与代码管理的 18 不同）。
// audit_status：1 审核中 / 2 审核不通过 / 3 审核通过。
func (c *Client) GetSettingCategories(ctx context.Context, token, appid string) (*SettingCategoriesResponse, error) {
	var out SettingCategoriesResponse
	if err := c.getJSON(ctx, "/cgi-bin/wxopen/getcategory", model.TokenScopeAuthorizer, appid, token, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// PrivacySettingResponse 小程序用户隐私保护指引。
type PrivacySettingResponse struct {
	CodeExist   int      `json:"code_exist"`
	PrivacyList []string `json:"privacy_list"`
	SettingList []struct {
		PrivacyKey  string `json:"privacy_key"`
		PrivacyText string `json:"privacy_text"`
	} `json:"setting_list"`
	OwnerSetting map[string]any `json:"owner_setting"`
	UpdateTime   int64          `json:"update_time"`
}

// GetPrivacySetting 获取小程序用户隐私保护指引。
//
// 文档：POST /cgi-bin/component/getprivacysetting?access_token=TOKEN
// （privacy-management/api_getprivacysetting.html）。
// token：authorizer_access_token；权限集 18。
// privacyVer：1 现网版本 / 2 开发版（官方默认值 2）。
// —— 文档要求 POST **必须发 JSON body**：privacyVer 为 nil 时也要发 {}，
// 传具体值时发 {"privacy_ver": N}；完全不带 body 会 44002/47001。
func (c *Client) GetPrivacySetting(ctx context.Context, token, appid string, privacyVer *int) (*PrivacySettingResponse, error) {
	var out PrivacySettingResponse
	if privacyVer == nil {
		if err := c.postEmpty(ctx, "/cgi-bin/component/getprivacysetting", model.TokenScopeAuthorizer, appid, token, &out); err != nil {
			return nil, err
		}
		return &out, nil
	}
	body := struct {
		PrivacyVer int `json:"privacy_ver"`
	}{PrivacyVer: *privacyVer}
	if err := c.postJSON(ctx, "/cgi-bin/component/getprivacysetting", model.TokenScopeAuthorizer, appid, token, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DomainSet 服务器域名集合（action 为 get 时不要传这些字段）。
type DomainSet struct {
	RequestDomain   []string `json:"requestdomain"`
	WSRequestDomain []string `json:"wsrequestdomain"`
	UploadDomain    []string `json:"uploaddomain"`
	DownloadDomain  []string `json:"downloaddomain"`
	UDPDomain       []string `json:"udpdomain"`
	TCPDomain       []string `json:"tcpdomain"`
}

// ModifyDomainResponse 域名配置结果。
//
// 说明：官方 modify_domain 返回体还包含 invalid_wsrequestdomain / invalid_downloaddomain /
// invalid_udpdomain / invalid_tcpdomain 等字段，任务书给定的结构体未列，故此处不解析（详见汇报备注）。
// WebviewDomain 是官方 setwebviewdomain_directly 返回体的字段（业务域名），
// 为不丢失 ModifyJumpDomainDirectly 的返回值而补充；任务书已列字段全部保持原样。
type ModifyDomainResponse struct {
	RequestDomain        []string `json:"requestdomain"`
	WSRequestDomain      []string `json:"wsrequestdomain"`
	UploadDomain         []string `json:"uploaddomain"`
	DownloadDomain       []string `json:"downloaddomain"`
	UDPDomain            []string `json:"udpdomain"`
	TCPDomain            []string `json:"tcpdomain"`
	InvalidRequestDomain []string `json:"invalid_requestdomain"`
	InvalidUploadDomain  []string `json:"invalid_uploaddomain"`
	NoICPDomain          []string `json:"no_icp_domain"`
	WebviewDomain        []string `json:"webviewdomain"`
}

// serverDomainBody modify_domain / modify_domain_directly 的请求体。
type serverDomainBody struct {
	Action          string   `json:"action"`
	RequestDomain   []string `json:"requestdomain,omitempty"`
	WSRequestDomain []string `json:"wsrequestdomain,omitempty"`
	UploadDomain    []string `json:"uploaddomain,omitempty"`
	DownloadDomain  []string `json:"downloaddomain,omitempty"`
	UDPDomain       []string `json:"udpdomain,omitempty"`
	TCPDomain       []string `json:"tcpdomain,omitempty"`
}

// newServerDomainBody 组装请求体；action 为 get 时不带任何域名数组。
func newServerDomainBody(action string, d *DomainSet) serverDomainBody {
	body := serverDomainBody{Action: action}
	if d == nil || strings.EqualFold(action, "get") {
		return body
	}
	body.RequestDomain = d.RequestDomain
	body.WSRequestDomain = d.WSRequestDomain
	body.UploadDomain = d.UploadDomain
	body.DownloadDomain = d.DownloadDomain
	body.UDPDomain = d.UDPDomain
	body.TCPDomain = d.TCPDomain
	return body
}

// ModifyServerDomain 配置小程序服务器域名（旧接口，域名在发布时才生效）。
//
// 文档：POST /wxa/modify_domain?access_token=TOKEN（domain-management/api_modifyserverdomain.html）。
// token：authorizer_access_token；权限集 18。
// action ∈ add/delete/set/get；get 时不传域名数组。
// 限制：域名必须先在第三方平台登记（否则 85017/85018）；每月修改次数有限（超限 86102）；
// 不允许 IP、不允许 api.weixin.qq.com、需 ICP 备案（85301/85302/85303）。
func (c *Client) ModifyServerDomain(ctx context.Context, token, appid, action string, domains *DomainSet) (*ModifyDomainResponse, error) {
	var out ModifyDomainResponse
	body := newServerDomainBody(action, domains)
	if err := c.postJSON(ctx, "/wxa/modify_domain", model.TokenScopeAuthorizer, appid, token, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ModifyJumpDomain 配置小程序业务域名（旧接口，域名在发布时才生效）。
//
// 文档：POST /wxa/setwebviewdomain?access_token=TOKEN（domain-management/api_modifyjumpdomain.html）。
// token：authorizer_access_token；权限集 18。
// action ∈ add/delete/set/get（不传 action 时默认把第三方平台登记的业务域名全部加上）；get 时不传域名数组。
// 限制：业务域名必须先在第三方平台登记；最多 300 个；仅支持 https；不能带端口号；
// 注意 add 与 set 的区别（set 是覆盖）。
func (c *Client) ModifyJumpDomain(ctx context.Context, token, appid, action string, domains []string) error {
	body := struct {
		Action        string   `json:"action,omitempty"`
		WebviewDomain []string `json:"webviewdomain,omitempty"`
	}{Action: action}
	if !strings.EqualFold(action, "get") {
		body.WebviewDomain = domains
	}
	return errOrNil(c.postJSON(ctx, "/wxa/setwebviewdomain", model.TokenScopeAuthorizer, appid, token, body, nil))
}

// ModifyServerDomainDirectly 快速配置小程序服务器域名（直接生效，规则对齐普通小程序）。
//
// 文档：POST /wxa/modify_domain_directly?access_token=TOKEN
// （domain-management/api_modifyserverdomaindirectly.html）。
// token：authorizer_access_token；权限集 18。action ∈ add/delete/set/get。
// 限制：add 前建议先 get；同一小程序配一次即可，旧域名上线成功后不要重新 set；每月修改次数有限。
func (c *Client) ModifyServerDomainDirectly(ctx context.Context, token, appid, action string, domains *DomainSet) error {
	body := newServerDomainBody(action, domains)
	return errOrNil(c.postJSON(ctx, "/wxa/modify_domain_directly", model.TokenScopeAuthorizer, appid, token, body, nil))
}

// ModifyJumpDomainDirectly 快速配置小程序业务域名（直接生效）。
//
// 文档：POST /wxa/setwebviewdomain_directly?access_token=TOKEN
// （domain-management/api_modifyjumpdomaindirectly.html）。
// token：authorizer_access_token；权限集 18。返回体含 webviewdomain（业务域名）。
func (c *Client) ModifyJumpDomainDirectly(ctx context.Context, token, appid, action string, domains []string) (*ModifyDomainResponse, error) {
	body := struct {
		Action        string   `json:"action"`
		WebviewDomain []string `json:"webviewdomain,omitempty"`
	}{Action: action}
	if !strings.EqualFold(action, "get") {
		body.WebviewDomain = domains
	}
	var out ModifyDomainResponse
	if err := c.postJSON(ctx, "/wxa/setwebviewdomain_directly", model.TokenScopeAuthorizer, appid, token, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetEffectiveServerDomain 获取发布后生效的服务器域名列表。
//
// 文档：POST /wxa/get_effective_domain?access_token=TOKEN
// （domain-management/api_geteffectiveserverdomain.html）。
// —— 官方单页文档写的是 **POST**（请求体无字段，示例/既有错误码表要求带空 JSON {}，
// 否则 44002），与任务书「get_effective_domain 是 GET」的说法冲突：**以官方文档 + 本地 errcodes.go 的 44002 提示为准，按 POST {} 实现**。
// token：authorizer_access_token；权限集 18。
// 返回值：把 mp_domain / third_domain / direct_domain / effective_domain 四组域名打平成
// "mp_domain.requestdomain" 形式的 key（每组含 requestdomain/wsrequestdomain/uploaddomain/
// downloaddomain/udpdomain/tcpdomain）。
func (c *Client) GetEffectiveServerDomain(ctx context.Context, token, appid string) (map[string][]string, error) {
	var raw map[string]json.RawMessage
	if apiErr := c.postEmpty(ctx, "/wxa/get_effective_domain", model.TokenScopeAuthorizer, appid, token, &raw); apiErr != nil {
		return nil, apiErr
	}
	// 兼容两种响应形态：
	//   1) 分组形态（真实网关）：{"mp_domain":{"requestdomain":[...]},"effective_domain":{...}}
	//      打平成 "组名.字段名" -> 域名列表；
	//   2) 打平形态（部分网关/模拟器）：{"requestdomain":[...]}，直接以字段名为 key。
	out := map[string][]string{}
	for group, v := range raw {
		if group == "errcode" || group == "errmsg" {
			continue
		}
		var domains map[string][]string
		if err := json.Unmarshal(v, &domains); err == nil {
			for k, list := range domains {
				out[group+"."+k] = list
			}
			continue
		}
		var flat []string
		if err := json.Unmarshal(v, &flat); err == nil {
			out[group] = flat
		}
	}
	return out, nil
}

// GetEffectiveJumpDomain 获取发布后生效的业务域名列表。
//
// 文档：POST /wxa/get_effective_webviewdomain?access_token=TOKEN
// （domain-management/api_geteffectivejumpdomain.html）。
// —— 文档要求 POST 且请求体为 {}（示例即 "{}"）。
// token：authorizer_access_token；权限集 18。
// 返回：mp_webviewdomain / third_webviewdomain / direct_webviewdomain / effective_webviewdomain。
func (c *Client) GetEffectiveJumpDomain(ctx context.Context, token, appid string) (map[string][]string, error) {
	var raw map[string]json.RawMessage
	if err := c.postEmpty(ctx, "/wxa/get_effective_webviewdomain", model.TokenScopeAuthorizer, appid, token, &raw); err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for k, v := range raw {
		if k == "errcode" || k == "errmsg" {
			continue
		}
		var arr []string
		if err := json.Unmarshal(v, &arr); err == nil {
			out[k] = arr
		}
	}
	return out, nil
}

// GetJumpDomainConfirmFile 获取业务域名校验文件（需放到业务域名根目录）。
//
// 文档：POST /wxa/get_webviewdomain_confirmfile?access_token=TOKEN
// （domain-management/api_getjumpdomainconfirmfile.html）。
// —— 文档要求 POST **必须发空 JSON {}**（示例请求体即 {}）。
// token：authorizer_access_token；权限集 18。
// 注意：返回的 file_name / file_content 请勿修改。
func (c *Client) GetJumpDomainConfirmFile(ctx context.Context, token, appid string) (fileName string, fileContent string, err error) {
	var out struct {
		FileName    string `json:"file_name"`
		FileContent string `json:"file_content"`
	}
	if err := c.postEmpty(ctx, "/wxa/get_webviewdomain_confirmfile", model.TokenScopeAuthorizer, appid, token, &out); err != nil {
		return "", "", err
	}
	return out.FileName, out.FileContent, nil
}

// ---------------------------------------------------------------------------
// 第三方平台自身域名（component_access_token；不需要授权方）
// ---------------------------------------------------------------------------

// ModifyThirdPartyServerDomain 设置第三方平台服务器域名（代开发小程序可用的服务器域名池）。
//
// 文档：POST /cgi-bin/component/modify_wxa_server_domain?access_token=TOKEN
// （thirdparty-management/domain-mgnt/api_modifythirdpartyserverdomain.html）。
// token：component_access_token（仅第三方平台自己调用）。
// action ∈ add/delete/set/get；domains 为以 ; 分隔的域名字符串（最多 1000 个，
// 不带 http:// 前缀、不带 URI 路径）；modifyPublishedTogether=true 时同时修改「全网发布版」的值。
// 限制：每月只能修改 50 次（超限 45104）；未登记的域名在代调用时会报 85017。
// 返回值：把响应中的字符串字段（含 published/tested/invalid 域名）原样给出，便于上层展示。
func (c *Client) ModifyThirdPartyServerDomain(ctx context.Context, token, action, domains string, modifyPublishedTogether bool) (map[string]string, error) {
	body := map[string]any{"action": action}
	if domains != "" {
		body["wxa_server_domain"] = domains
		body["is_modify_published_together"] = modifyPublishedTogether
	}
	return c.thirdPartyDomainCall(ctx, "/cgi-bin/component/modify_wxa_server_domain", token, body)
}

// ModifyThirdPartyJumpDomain 设置第三方平台业务域名。
//
// 文档：POST /cgi-bin/component/modify_wxa_jump_domain?access_token=TOKEN
// （thirdparty-management/domain-mgnt/api_modifythirdpartyjumpdomain.html）。
// token：component_access_token；action ∈ add/delete/set/get；
// domains 为以 ; 分隔的业务域名（最多 300 个）。
// 冲突备注：旧版 2.0 文档的路径是 /cgi-bin/component/setwebviewdomain，字段名是 webviewdomain；
// 新版 openApi 文档已改为 modify_wxa_jump_domain + wxa_jump_h5_domain —— 以新版为准。
func (c *Client) ModifyThirdPartyJumpDomain(ctx context.Context, token, action, domains string, modifyPublishedTogether bool) (map[string]string, error) {
	body := map[string]any{"action": action}
	if domains != "" {
		body["wxa_jump_h5_domain"] = domains
		body["is_modify_published_together"] = modifyPublishedTogether
	}
	return c.thirdPartyDomainCall(ctx, "/cgi-bin/component/modify_wxa_jump_domain", token, body)
}

// thirdPartyDomainCall 第三方平台域名接口的公共调用与结果扁平化。
func (c *Client) thirdPartyDomainCall(ctx context.Context, path, token string, body map[string]any) (map[string]string, error) {
	var raw map[string]any
	if err := c.postJSON(ctx, path, model.TokenScopeComponent, "", token, body, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		switch t := v.(type) {
		case nil:
			out[k] = ""
		case string:
			out[k] = t
		case bool:
			out[k] = strconv.FormatBool(t)
		case float64:
			out[k] = strconv.FormatFloat(t, 'f', -1, 64)
		case []any:
			parts := make([]string, 0, len(t))
			for _, item := range t {
				parts = append(parts, fmt.Sprint(item))
			}
			out[k] = strings.Join(parts, ";")
		default:
			out[k] = fmt.Sprint(t)
		}
	}
	return out, nil
}

// GetThirdPartyJumpDomainConfirmFile 获取第三方平台业务域名的校验文件。
//
// 文档：POST /cgi-bin/component/get_domain_confirmfile?access_token=TOKEN
// （thirdparty-management/domain-mgnt/api_getthirdpartyjumpdomainconfirmfile.html）。
// —— 文档要求 POST，请求体无字段；这里按空 JSON {} 发送（与 get_webviewdomain_confirmfile 一致，避免 44002）。
// token：component_access_token。返回的 file_name / file_content 请勿修改。
func (c *Client) GetThirdPartyJumpDomainConfirmFile(ctx context.Context, token string) (fileName string, fileContent string, err error) {
	var out struct {
		FileName    string `json:"file_name"`
		FileContent string `json:"file_content"`
	}
	if err := c.postEmpty(ctx, "/cgi-bin/component/get_domain_confirmfile", model.TokenScopeComponent, "", token, &out); err != nil {
		return "", "", err
	}
	return out.FileName, out.FileContent, nil
}

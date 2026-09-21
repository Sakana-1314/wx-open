package wxapi

import (
	"context"
	"encoding/json"
	"net/http"

	"wx-platform/server/internal/model"
)

// ---------------------------------------------------------------------------
// 第三方平台自身：令牌 / 票据 / 预授权码 / 授权方令牌
// ---------------------------------------------------------------------------

// ComponentTokenRequest 第三方平台令牌请求。
type ComponentTokenRequest struct {
	ComponentAppid        string `json:"component_appid"`
	ComponentAppsecret    string `json:"component_appsecret"`
	ComponentVerifyTicket string `json:"component_verify_ticket"`
}

// ComponentTokenResponse 第三方平台令牌响应。
type ComponentTokenResponse struct {
	ComponentAccessToken string `json:"component_access_token"`
	ExpiresIn            int    `json:"expires_in"`
}

// ComponentToken 获取第三方平台令牌 component_access_token。
//
// 文档：POST /cgi-bin/component/api_component_token（docs/reference/wx-open-platform-api.md §1.1；
// docs/wx-docs 内无该单页，extra8_component_access_token_b3c.md 是抓取失败的占位页）。
// token：无（用 component_appsecret + component_verify_ticket 换令牌，不需要 access_token）。
// 说明：有效期 7200s，官方建议提前 10 分钟刷新；超限返回 45009；IP 未加白名单返回 61004。
func (c *Client) ComponentToken(ctx context.Context, req ComponentTokenRequest) (*ComponentTokenResponse, error) {
	spec := callSpec{
		method:   http.MethodPost,
		path:     "/cgi-bin/component/api_component_token",
		scope:    "",
		appid:    req.ComponentAppid,
		jsonBody: req,
	}
	var out ComponentTokenResponse
	if err := c.jsonCall(ctx, spec, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// StartPushTicketRequest 启动票据推送服务请求。
type StartPushTicketRequest struct {
	ComponentAppid  string `json:"component_appid"`
	ComponentSecret string `json:"component_secret"`
}

// StartPushTicket 启动 component_verify_ticket 推送服务（票据长期收不到时的官方处置手段）。
//
// 文档：POST /cgi-bin/component/api_start_push_ticket（token_startpushticket.md）。
// token：无。返回错误码 0（ok / in a normal state）与 40013（invalid appid）。
func (c *Client) StartPushTicket(ctx context.Context, req StartPushTicketRequest) error {
	spec := callSpec{
		method:   http.MethodPost,
		path:     "/cgi-bin/component/api_start_push_ticket",
		scope:    "",
		appid:    req.ComponentAppid,
		jsonBody: req,
	}
	return errOrNil(c.jsonCall(ctx, spec, nil))
}

// PreAuthCodeResponse 预授权码响应。
type PreAuthCodeResponse struct {
	PreAuthCode string `json:"pre_auth_code"`
	ExpiresIn   int    `json:"expires_in"`
}

// CreatePreAuthCode 获取预授权码。
//
// 文档：POST /cgi-bin/component/api_create_preauthcode?access_token=TOKEN（token_preauthcode.md）。
// token：component_access_token（权限集无关，仅第三方平台自身可调）。
// 限制：预授权码有效期官方正文写 1800 秒，而文档返回示例是 600 —— 以 expires_in 字段为准（注释见 conflicts）。
func (c *Client) CreatePreAuthCode(ctx context.Context, token, componentAppid string) (*PreAuthCodeResponse, error) {
	body := struct {
		ComponentAppid string `json:"component_appid"`
	}{ComponentAppid: componentAppid}
	var out PreAuthCodeResponse
	err := c.postJSON(ctx, "/cgi-bin/component/api_create_preauthcode", model.TokenScopeComponent, componentAppid, token, body, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// FuncScope 授权给第三方平台的权限集。
//
// 官方 func_info 元素是 {"funcscope_category":{id,type,name,desc}}，
// 老版本文档/示例里出现过扁平结构 —— 两种都兼容（见 UnmarshalJSON）。
type FuncScope struct {
	ID   int    `json:"id"`
	Type int    `json:"type"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// UnmarshalJSON 兼容 {"funcscope_category":{...}} 与扁平 {...} 两种写法。
func (f *FuncScope) UnmarshalJSON(b []byte) error {
	type alias FuncScope
	var flat alias
	if err := json.Unmarshal(b, &flat); err != nil {
		return err
	}
	*f = FuncScope(flat)
	if f.ID != 0 || f.Type != 0 || f.Name != "" || f.Desc != "" {
		return nil
	}
	var nested struct {
		FuncscopeCategory *alias `json:"funcscope_category"`
	}
	if err := json.Unmarshal(b, &nested); err != nil {
		return err
	}
	if nested.FuncscopeCategory != nil {
		*f = FuncScope(*nested.FuncscopeCategory)
	}
	return nil
}

// AuthorizationInfo 授权信息。
type AuthorizationInfo struct {
	AuthorizerAppid        string      `json:"authorizer_appid"`
	AuthorizerAccessToken  string      `json:"authorizer_access_token"`
	ExpiresIn              int         `json:"expires_in"`
	AuthorizerRefreshToken string      `json:"authorizer_refresh_token"`
	FuncInfo               []FuncScope `json:"func_info"`
}

// QueryAuthResponse 授权码换令牌的响应。
type QueryAuthResponse struct {
	AuthorizationInfo AuthorizationInfo `json:"authorization_info"`
}

// QueryAuth 用授权码换取授权方令牌与刷新令牌。
//
// 文档：POST /cgi-bin/component/api_query_auth?access_token=TOKEN（token_getauthorizerrefreshtoken.md，
// 接口英文名 getAuthorizerRefreshToken）。
// token：component_access_token。
// 说明：authorization_code 有有效期，过期返回 42003；返回的 func_info 才是商家实际授权的权限集。
func (c *Client) QueryAuth(ctx context.Context, token, componentAppid, authorizationCode string) (*QueryAuthResponse, error) {
	body := struct {
		ComponentAppid    string `json:"component_appid"`
		AuthorizationCode string `json:"authorization_code"`
	}{ComponentAppid: componentAppid, AuthorizationCode: authorizationCode}
	var out QueryAuthResponse
	err := c.postJSON(ctx, "/cgi-bin/component/api_query_auth", model.TokenScopeComponent, componentAppid, token, body, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AuthorizerTokenResponse 授权方令牌响应。
type AuthorizerTokenResponse struct {
	AuthorizerAccessToken  string `json:"authorizer_access_token"`
	ExpiresIn              int    `json:"expires_in"`
	AuthorizerRefreshToken string `json:"authorizer_refresh_token"`
}

// AuthorizerToken 刷新授权方令牌 authorizer_access_token。
//
// 文档：POST /cgi-bin/component/api_authorizer_token?access_token=TOKEN（token_getauthorizeraccesstoken.md，
// 旧版 2.0 路径 token_api_authorizer_token_2.md 用 ?component_access_token=，两个参数名微信侧都接受）。
// token：component_access_token。有效期 7200s，需缓存，避免触发每日限额。
func (c *Client) AuthorizerToken(ctx context.Context, token, componentAppid, authorizerAppid, refreshToken string) (*AuthorizerTokenResponse, error) {
	body := struct {
		ComponentAppid         string `json:"component_appid"`
		AuthorizerAppid        string `json:"authorizer_appid"`
		AuthorizerRefreshToken string `json:"authorizer_refresh_token"`
	}{ComponentAppid: componentAppid, AuthorizerAppid: authorizerAppid, AuthorizerRefreshToken: refreshToken}
	var out AuthorizerTokenResponse
	err := c.postJSON(ctx, "/cgi-bin/component/api_authorizer_token", model.TokenScopeComponent, authorizerAppid, token, body, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// 授权方信息
// ---------------------------------------------------------------------------

// AuthorizerListItem 已授权账号条目。
type AuthorizerListItem struct {
	AuthorizerAppid string `json:"authorizer_appid"`
	RefreshToken    string `json:"refresh_token"`
	AuthTime        int64  `json:"auth_time"`
}

// AuthorizerListResponse 已授权账号列表响应。
type AuthorizerListResponse struct {
	TotalCount int                  `json:"total_count"`
	List       []AuthorizerListItem `json:"list"`
}

// GetAuthorizerList 拉取已授权账号列表。
//
// 文档：POST /cgi-bin/component/api_get_authorizer_list?access_token=TOKEN（auth_getauthorizerlist.md）。
// token：component_access_token。
// 限制：count 最大 500，超出返回 40170；refresh_token 可能为空（此时需用 api_query_auth 重新换取）。
func (c *Client) GetAuthorizerList(ctx context.Context, token, componentAppid string, offset, count int) (*AuthorizerListResponse, error) {
	body := struct {
		ComponentAppid string `json:"component_appid"`
		Offset         int    `json:"offset"`
		Count          int    `json:"count"`
	}{ComponentAppid: componentAppid, Offset: offset, Count: count}
	var out AuthorizerListResponse
	err := c.postJSON(ctx, "/cgi-bin/component/api_get_authorizer_list", model.TokenScopeComponent, componentAppid, token, body, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// TypeIDName 形如 service_type_info / verify_type_info 的 {id, name} 结构。
type TypeIDName struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// MiniProgramInfo 小程序配置信息（字段出现即说明该授权账号是小程序）。
type MiniProgramInfo struct {
	Network    map[string][]string `json:"network"`
	Categories []struct {
		First  string `json:"first"`
		Second string `json:"second"`
	} `json:"categories"`
	VisitStatus int `json:"visit_status"` // 废弃参数，保留以兼容旧响应
}

// AuthorizerInfo 授权账号详情。
type AuthorizerInfo struct {
	NickName        string          `json:"nick_name"`
	HeadImg         string          `json:"head_img"`
	QRCodeURL       string          `json:"qrcode_url"`
	UserName        string          `json:"user_name"`
	Alias           string          `json:"alias"`
	PrincipalName   string          `json:"principal_name"`
	Signature       string          `json:"signature"`
	IDC             int             `json:"idc"`
	RegisterType    int             `json:"register_type"`
	AccountStatus   int             `json:"account_status"`
	ServiceTypeInfo TypeIDName      `json:"service_type_info"`
	VerifyTypeInfo  TypeIDName      `json:"verify_type_info"`
	BusinessInfo    map[string]any  `json:"business_info"`
	MiniProgramInfo MiniProgramInfo `json:"MiniProgramInfo"`
	BasicConfig     map[string]any  `json:"basic_config"`
}

// AuthorizerInfoResponse 授权账号详情响应。
type AuthorizerInfoResponse struct {
	AuthorizerInfo    AuthorizerInfo    `json:"authorizer_info"`
	AuthorizationInfo AuthorizationInfo `json:"authorization_info"`
}

// GetAuthorizerInfo 获取授权账号详情。
//
// 文档：POST /cgi-bin/component/api_get_authorizer_info?access_token=TOKEN（auth_getauthorizerinfo.md）。
// token：component_access_token。注意 official 字段名是 nick_name（不是 nickname）。
func (c *Client) GetAuthorizerInfo(ctx context.Context, token, componentAppid, authorizerAppid string) (*AuthorizerInfoResponse, error) {
	body := struct {
		ComponentAppid  string `json:"component_appid"`
		AuthorizerAppid string `json:"authorizer_appid"`
	}{ComponentAppid: componentAppid, AuthorizerAppid: authorizerAppid}
	var out AuthorizerInfoResponse
	err := c.postJSON(ctx, "/cgi-bin/component/api_get_authorizer_info", model.TokenScopeComponent, authorizerAppid, token, body, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// AuthorizerOptionResponse 授权方选项响应。
type AuthorizerOptionResponse struct {
	OptionName  string `json:"option_name"`
	OptionValue string `json:"option_value"`
}

// GetAuthorizerOption 获取授权方选项信息。
//
// 文档：POST /cgi-bin/component/get_authorizer_option?access_token=TOKEN（auth_getauthorizeroptioninfo.md）。
// token：component_access_token。
// 冲突：新版文档请求体表只列了 option_name，但官方 2.0 页与 FAQ 都要求带 component_appid + authorizer_appid；
// 该页「注意事项」也明确写「需实测」——这里按实际可用形态三个字段都发。
func (c *Client) GetAuthorizerOption(ctx context.Context, token, componentAppid, authorizerAppid, optionName string) (*AuthorizerOptionResponse, error) {
	body := struct {
		ComponentAppid  string `json:"component_appid"`
		AuthorizerAppid string `json:"authorizer_appid"`
		OptionName      string `json:"option_name"`
	}{ComponentAppid: componentAppid, AuthorizerAppid: authorizerAppid, OptionName: optionName}
	var out AuthorizerOptionResponse
	err := c.postJSON(ctx, "/cgi-bin/component/get_authorizer_option", model.TokenScopeComponent, authorizerAppid, token, body, &out)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// SetAuthorizerOption 设置授权方选项信息。
//
// 文档：POST /cgi-bin/component/set_authorizer_option?access_token=TOKEN（auth_setauthorizeroptioninfo.md）。
// token：component_access_token。option_name 仅支持 location_report / voice_recognize / customer_service；
// 设置需要有授权方授权（权限集不足返回 61007/48001）。
func (c *Client) SetAuthorizerOption(ctx context.Context, token, componentAppid, authorizerAppid, optionName, optionValue string) error {
	body := struct {
		ComponentAppid  string `json:"component_appid"`
		AuthorizerAppid string `json:"authorizer_appid"`
		OptionName      string `json:"option_name"`
		OptionValue     string `json:"option_value"`
	}{ComponentAppid: componentAppid, AuthorizerAppid: authorizerAppid, OptionName: optionName, OptionValue: optionValue}
	return errOrNil(c.postJSON(ctx, "/cgi-bin/component/set_authorizer_option", model.TokenScopeComponent, authorizerAppid, token, body, nil))
}

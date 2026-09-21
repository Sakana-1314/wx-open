package core

import (
	"fmt"
	"net/url"
	"strings"
)

// 本文件实现授权链接拼接（官方「授权流程说明」给出的三种形态）。
//
// 官方要点：
//   - PC 版：https://mp.weixin.qq.com/cgi-bin/componentloginpage?component_appid=&pre_auth_code=&redirect_uri=&auth_type=
//   - H5 版：https://open.weixin.qq.com/wxaopen/safe/bindcomponent?action=bindcomponent&no_scan=1&component_appid=&pre_auth_code=&redirect_uri=&auth_type=&biz_appid=#wechat_redirect
//   - auth_type：1 仅公众号 / 2 仅小程序 / 3 公众号+小程序 / 4 小程序推客 / 5 视频号 / 6 全部 / 8 带货助手；
//   - biz_appid 指定唯一账号时优先级高于 auth_type（只有该 appid 的管理员可授权）；
//   - category_id_list 指定权限集 id 列表，多个用中竖线 | 分隔，不传则用已全网发布的权限集；
//   - 授权成功后回调 redirect_uri?auth_code=xxx&expires_in=600，同时微信会推送 authorized 事件携带授权码；
//   - 注意：官方文档未对 `mp.weixin.qq.com/mp/authorize` 之类路径作任何收录，移动端请使用 H5 bindcomponent。
const (
	// PcAuthorizeURL 桌面端授权页（访问后展示二维码）。
	PcAuthorizeURL = "https://mp.weixin.qq.com/cgi-bin/componentloginpage"
	// MobileAuthorizeURL 移动端授权页（no_scan=1 表示无需扫码，直接进入授权确认）。
	MobileAuthorizeURL = "https://open.weixin.qq.com/wxaopen/safe/bindcomponent"
)

// AuthType 授权类型。
type AuthType int

// 官方 auth_type 取值。
const (
	AuthTypeOfficialAccount AuthType = 1
	AuthTypeMiniProgram     AuthType = 2
	AuthTypeBoth            AuthType = 3
	AuthTypeMiniProgramTC   AuthType = 4
	AuthTypeChannel         AuthType = 5
	AuthTypeAll             AuthType = 6
	AuthTypeTalent          AuthType = 8
)

// Valid 校验 auth_type。
func (t AuthType) Valid() bool {
	switch t {
	case AuthTypeOfficialAccount, AuthTypeMiniProgram, AuthTypeBoth,
		AuthTypeMiniProgramTC, AuthTypeChannel, AuthTypeAll, AuthTypeTalent:
		return true
	}
	return false
}

// AuthURLRequest 授权链接参数。
type AuthURLRequest struct {
	ComponentAppid string
	PreAuthCode    string
	RedirectURI    string
	AuthType       AuthType
	BizAppid       string
	CategoryIDs    []int
}

// AuthURLs 生成 PC 与移动端两条授权链接。
//
// 两条链接都带上二维码内容（二维码直接编码 PC 链接即可，与微信后台生成的二维码等价）。
func (r AuthURLRequest) AuthURLs() (pcURL, mobileURL string, err error) {
	if r.ComponentAppid == "" {
		return "", "", ErrNotConfigured
	}
	if r.PreAuthCode == "" {
		return "", "", Validation("预授权码为空，请先生成授权链接")
	}
	if strings.TrimSpace(r.RedirectURI) == "" {
		return "", "", Validation("授权回调地址（redirect_uri）不能为空：需在开放平台后台配置「授权发起页域名」后使用")
	}
	authType := r.AuthType
	if authType == 0 {
		authType = AuthTypeMiniProgram
	}
	if !authType.Valid() {
		return "", "", Validation("auth_type 取值不合法：%d（1 公众号 / 2 小程序 / 3 公众号+小程序 / 4 小程序推客 / 5 视频号 / 6 全部 / 8 带货助手）", int(authType))
	}

	pc := url.Values{}
	pc.Set("component_appid", r.ComponentAppid)
	pc.Set("pre_auth_code", r.PreAuthCode)
	pc.Set("redirect_uri", r.RedirectURI)
	pc.Set("auth_type", fmt.Sprintf("%d", int(authType)))
	if r.BizAppid != "" {
		// biz_appid 优先级高于 auth_type：指定后只有该账号管理员能完成授权。
		pc.Set("biz_appid", r.BizAppid)
	}
	if len(r.CategoryIDs) > 0 {
		pc.Set("category_id_list", joinInts(r.CategoryIDs, "|"))
	}

	mobile := url.Values{}
	mobile.Set("action", "bindcomponent")
	mobile.Set("no_scan", "1")
	mobile.Set("component_appid", r.ComponentAppid)
	mobile.Set("pre_auth_code", r.PreAuthCode)
	mobile.Set("redirect_uri", r.RedirectURI)
	mobile.Set("auth_type", fmt.Sprintf("%d", int(authType)))
	if r.BizAppid != "" {
		mobile.Set("biz_appid", r.BizAppid)
	}
	if len(r.CategoryIDs) > 0 {
		mobile.Set("category_id_list", joinInts(r.CategoryIDs, "|"))
	}

	return PcAuthorizeURL + "?" + pc.Encode(),
		MobileAuthorizeURL + "?" + mobile.Encode() + "#wechat_redirect", nil
}

// BuildRedirectURI 拼接本站授权回调地址（授权完成后微信会跳转 redirect_uri?auth_code=&expires_in=）。
//
// 注意：redirect_uri 的域名必须与开放平台后台的「授权发起页域名」一致，否则商家会看到
// 「确认授权入口页所在域名，与授权后回调页所在域名相同」的报错。
func BuildRedirectURI(publicBaseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if base == "" {
		return "/authorize/callback"
	}
	return base + "/authorize/callback"
}

// CallbackURLs 返回需要填进开放平台后台的两个回调地址。
func CallbackURLs(publicBaseURL string) (authorizationEventURL, messageEventURL string) {
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if base == "" {
		return "/callback/component", "/callback/message/$APPID$"
	}
	return base + "/callback/component", base + "/callback/message/$APPID$"
}

func joinInts(values []int, sep string) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("%d", v))
	}
	return strings.Join(parts, sep)
}

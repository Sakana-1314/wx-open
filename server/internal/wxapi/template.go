package wxapi

import (
	"context"
	"net/url"
	"strconv"

	"wx-platform/server/internal/model"
)

// ---------------------------------------------------------------------------
// 代码模板库（第三方平台侧，令牌一律为 component_access_token）
// ---------------------------------------------------------------------------

// TemplateDraft 草稿箱条目。
type TemplateDraft struct {
	DraftID                int64  `json:"draft_id"`
	CreateTime             int64  `json:"create_time"`
	UserVersion            string `json:"user_version"`
	UserDesc               string `json:"user_desc"`
	SourceMiniProgramAppid string `json:"source_miniprogram_appid"`
	SourceMiniProgram      string `json:"source_miniprogram"`
	Developer              string `json:"developer"`
}

// GetTemplateDraftList 获取草稿箱列表。
//
// 文档：GET /wxa/gettemplatedraftlist?access_token=TOKEN（tpl_gettemplatedraftlist.md）。
// —— 文档要求 GET（3. 错误码里没有 43001，但「5. 代码示例」明确写 GET）。
// token：component_access_token（模板库接口用平台自身令牌，用错令牌返回 61014）。
func (c *Client) GetTemplateDraftList(ctx context.Context, token string) ([]TemplateDraft, error) {
	var out struct {
		DraftList []TemplateDraft `json:"draft_list"`
	}
	if err := c.getJSON(ctx, "/wxa/gettemplatedraftlist", model.TokenScopeComponent, "", token, nil, &out); err != nil {
		return nil, err
	}
	return out.DraftList, nil
}

// AddToTemplate 把草稿添加到代码模板库。
//
// 文档：POST /wxa/addtotemplate?access_token=TOKEN（tpl_addtotemplate.md）。
// token：component_access_token。
// 限制：模板库上限 200 个，已满返回 85065（模板库已满）；草稿不存在返回 85064。
func (c *Client) AddToTemplate(ctx context.Context, token string, draftID int64, templateType int) error {
	body := struct {
		DraftID      int64 `json:"draft_id"`
		TemplateType int   `json:"template_type"`
	}{DraftID: draftID, TemplateType: templateType}
	return errOrNil(c.postJSON(ctx, "/wxa/addtotemplate", model.TokenScopeComponent, "", token, body, nil))
}

// CodeTemplateInfo 代码模板条目。
type CodeTemplateInfo struct {
	TemplateID             int64  `json:"template_id"`
	DraftID                int64  `json:"draft_id"`
	TemplateType           int    `json:"template_type"`
	CreateTime             int64  `json:"create_time"`
	UserVersion            string `json:"user_version"`
	UserDesc               string `json:"user_desc"`
	SourceMiniProgramAppid string `json:"source_miniprogram_appid"`
	SourceMiniProgram      string `json:"source_miniprogram"`
	Developer              string `json:"developer"`
	AuditStatus            int    `json:"audit_status"`
	Reason                 string `json:"reason"`
}

// GetTemplateList 获取代码模板列表。
//
// 文档：GET /wxa/gettemplatelist?access_token=TOKEN（tpl_gettemplatelist.md）。
// —— 文档要求 GET；可选的 template_type 在文档里写在「请求体」表中，但 GET 无 body，
// 因此实现为 URL 查询参数 ?template_type=0/1（不传则返回全部）。
// token：component_access_token。audit_status 枚举：0 未提审核 / 1 审核中 / 2 审核驳回 / 3 审核通过 / 4 提审中 / 5 提审失败。
func (c *Client) GetTemplateList(ctx context.Context, token string, templateType *int) ([]CodeTemplateInfo, error) {
	q := url.Values{}
	if templateType != nil {
		q.Set("template_type", strconv.Itoa(*templateType))
	}
	var out struct {
		TemplateList []CodeTemplateInfo `json:"template_list"`
	}
	if err := c.getJSON(ctx, "/wxa/gettemplatelist", model.TokenScopeComponent, "", token, q, &out); err != nil {
		return nil, err
	}
	return out.TemplateList, nil
}

// DeleteTemplate 删除代码模板。
//
// 文档：POST /wxa/deletetemplate?access_token=TOKEN（tpl_deletetemplate.md）。
// token：component_access_token。模板不存在返回 85064。
func (c *Client) DeleteTemplate(ctx context.Context, token string, templateID int64) error {
	body := struct {
		TemplateID int64 `json:"template_id"`
	}{TemplateID: templateID}
	return errOrNil(c.postJSON(ctx, "/wxa/deletetemplate", model.TokenScopeComponent, "", token, body, nil))
}

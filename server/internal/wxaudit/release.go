package wxaudit

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/wxapi"
)

// ReleaseService 发布台账与发布动作（全量发布、分阶段发布、回退、体验版二维码）。
type ReleaseService struct{ base }

// NewReleaseService 构造发布服务。
func NewReleaseService(env *core.Env) *ReleaseService { return &ReleaseService{base: base{env: env}} }

// releaseHints 发布接口的中文动作建议（覆盖 model 里的通用建议）。
var releaseHints = map[int]string{
	85019: "没有可发布的审核版本（85019）：请先完成 上传代码 → 提交审核 → 审核通过，再执行发布。",
	85020: "审核状态不满足发布条件（85020）：发布的是最后一个审核通过的版本，请先确认该小程序已有审核通过的版本。",
	85021: "审核状态不满足发布条件（85021）：请按 上传代码 → 提审 → 审核通过 → 发布 的状态机顺序操作。",
	85080: "提交的审核未通过（85080）：先在后台重新提审并等待审核通过。",
	86002: "小程序未完成初始化（86002）：先在小程序后台补全昵称/头像/简介。",
}

// grayHints 分阶段发布的中文动作建议（85079~85082 为官方定义的灰度专用错误码）。
var grayHints = map[int]string{
	85079: "小程序没有线上版本（85079）：先全量发布一个线上版本，再开启分阶段发布。",
	85080: "提交的审核未通过（85080）：审核通过后才能发布或灰度。",
	85081: "无效的灰度比例（85081）：gray_percentage 必须是 0-100 的整数；比例为 0 时必须指定 support_debuger_first 或 support_experiencer_first。",
	85082: "灰度比例只能递增（85082）：请传入比当前比例更大的值（当前比例可查「灰度计划」接口）。",
	86002: "小程序未完成初始化（86002）：先在小程序后台补全昵称/头像/简介。",
}

// revertHints 版本回退的中文动作建议。
var revertHints = map[int]string{
	87012: "禁止回退该版本（87012）：可能是没有上一个线上版本、该版本已经回退过，或版本早于回退功能上线时间；官方最多保留最近 5 个发布或回退的版本。",
	40097: "参数不合法（40097）：app_version 必须是可回退版本列表里的正整数。",
	85019: "没有可回退的版本（85019）：先全量发布一个线上版本。",
}

// List 分页查询发布台账（只读本地库）。
func (s *ReleaseService) List(ctx context.Context, params gen.ListReleasesParams) (*gen.ReleaseListResponse, error) {
	if s.env == nil || s.env.Repos == nil {
		return nil, core.Internal(errors.New("服务依赖未初始化"))
	}
	page, pageSize := pagination(params.Page, params.PageSize)
	rows, total, err := s.env.Repos.Releases.List(ctx, derefString(params.Appid), page, pageSize)
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]gen.ReleaseRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, toGenReleaseRecord(row))
	}
	return &gen.ReleaseListResponse{Items: items, Page: page, PageSize: pageSize, Total: int(total)}, nil
}

// VersionInfo 查询小程序版本信息（体验版 + 线上版）。
//
// 官方：POST /wxa/getversioninfo，**必须发空 JSON {}**（否则 44002）。
func (s *ReleaseService) VersionInfo(ctx context.Context, appid string) (*gen.VersionInfo, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return nil, err
	}
	resp, err := s.env.Wx.GetVersionInfo(ctx, token, a.Appid)
	if err != nil {
		return nil, mapError(err)
	}
	out := &gen.VersionInfo{Appid: a.Appid}
	if resp == nil {
		return out, nil
	}
	out.ExpTime = unixTime(resp.ExpInfo.ExpTime)
	out.ExpVersion = strPtr(resp.ExpInfo.ExpVersion)
	out.ExpDesc = strPtr(resp.ExpInfo.ExpDesc)
	out.ReleaseTime = unixTime(resp.ReleaseInfo.ReleaseTime)
	out.ReleaseVersion = strPtr(resp.ReleaseInfo.ReleaseVersion)
	out.ReleaseDesc = strPtr(resp.ReleaseInfo.ReleaseDesc)
	return out, nil
}

// Release 全量发布。
//
// 官方语义：发布的是**最后一个审核通过的版本**，且为全量发布、立即生效
// （没有灰度概念，灰度要用 grayrelease）；没有审核版本返回 85019，
// 审核状态不满足返回 85020/85021。
func (s *ReleaseService) Release(ctx context.Context, appid string) (*gen.AuditActionResult, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return nil, err
	}
	// 官方：POST /wxa/release，body 必须是空 JSON {}（否则 44002 empty post data）。
	if err := s.env.Wx.Release(ctx, token, a.Appid); err != nil {
		return nil, mapWxError(err, releaseHints)
	}

	now := time.Now()
	version := s.latestAuditVersion(ctx, a.Appid)
	rec := &model.ReleaseRecord{
		Appid:       a.Appid,
		Action:      model.ReleaseActionRelease,
		UserVersion: version,
		ReleaseTime: &now,
		Note:        "全量发布，立即生效；发布的是最后一个审核通过的版本",
	}
	if err := s.env.Repos.Releases.Create(ctx, rec); err != nil {
		return nil, core.Internal(fmt.Errorf("微信侧已发布成功，但写发布台账失败：%w", err))
	}
	s.logOperation(ctx, actionRelease, targetApp, a.Appid, model.JSONMap{
		"action":      string(model.ReleaseActionRelease),
		"userVersion": version,
	})

	msg := "发布成功：已发布最后一个审核通过的版本（全量发布、立即生效）。"
	if version != "" {
		msg = fmt.Sprintf("发布成功：已发布最后一个审核通过的版本 %s（全量发布、立即生效）。", version)
	}
	return actionOK(a.Appid, msg), nil
}

// GrayRelease 分阶段发布（灰度）。
//
// 官方要求：gray_percentage 为 0-100 的整数；为 0 时 support_debuger_first 与
// support_experiencer_first 二选一必填；比例只能递增（85082）；无线上版本报 85079。
func (s *ReleaseService) GrayRelease(ctx context.Context, appid string, req gen.GrayReleaseRequest) (*gen.AuditActionResult, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	if req.GrayPercentage < 0 || req.GrayPercentage > 100 {
		return nil, core.Validation("grayPercentage 必须是 0-100 的整数（无效比例微信返回 85081），当前 %d", req.GrayPercentage)
	}
	debugerFirst := derefBool(req.SupportDebugerFirst)
	experiencerFirst := derefBool(req.SupportExperiencerFirst)
	if req.GrayPercentage == 0 && !debugerFirst && !experiencerFirst {
		return nil, core.Validation(
			"灰度比例为 0 时必须把 supportDebugerFirst 或 supportExperiencerFirst 之一设为 true（官方要求：0%% 表示先向项目成员或体验成员开放，否则微信返回 85081）")
	}

	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return nil, err
	}
	err = s.env.Wx.GrayRelease(ctx, token, a.Appid, wxapi.GrayReleaseRequest{
		GrayPercentage:          req.GrayPercentage,
		SupportDebugerFirst:     debugerFirst,
		SupportExperiencerFirst: experiencerFirst,
	})
	if err != nil {
		return nil, mapWxError(err, grayHints)
	}

	now := time.Now()
	percentage := req.GrayPercentage
	version := s.latestAuditVersion(ctx, a.Appid)
	note := fmt.Sprintf("分阶段发布：灰度比例 %d%%", percentage)
	if percentage == 0 {
		scope := "项目成员"
		if experiencerFirst {
			scope = "体验成员"
		}
		if debugerFirst && experiencerFirst {
			scope = "项目成员与体验成员"
		}
		note += "（先向" + scope + "开放）"
	}
	note += "；灰度比例只能递增"
	rec := &model.ReleaseRecord{
		Appid:          a.Appid,
		Action:         model.ReleaseActionGrayRelease,
		UserVersion:    version,
		GrayPercentage: &percentage,
		ReleaseTime:    &now,
		Note:           note,
	}
	if err := s.env.Repos.Releases.Create(ctx, rec); err != nil {
		return nil, core.Internal(fmt.Errorf("微信侧已开启分阶段发布，但写发布台账失败：%w", err))
	}
	s.logOperation(ctx, actionGrayRelease, targetApp, a.Appid, model.JSONMap{
		"action":                  string(model.ReleaseActionGrayRelease),
		"userVersion":             version,
		"grayPercentage":          percentage,
		"supportDebugerFirst":     debugerFirst,
		"supportExperiencerFirst": experiencerFirst,
	})

	return actionOK(a.Appid, fmt.Sprintf(
		"已开启分阶段发布：当前灰度比例 %d%%；灰度比例只能递增（85082），提高比例时请传入更大的值，可在「灰度计划」查看当前比例。",
		percentage)), nil
}

// GrayPlan 查询分阶段发布计划。
func (s *ReleaseService) GrayPlan(ctx context.Context, appid string) (*gen.GrayReleasePlan, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return nil, err
	}
	plan, err := s.env.Wx.GetGrayReleasePlan(ctx, token, a.Appid)
	if err != nil {
		return nil, mapError(err)
	}
	out := &gen.GrayReleasePlan{}
	if plan == nil {
		return out, nil
	}
	out.Status = intPtr(plan.Status)
	out.GrayPercentage = intPtr(plan.GrayPercentage)
	out.SupportDebugerFirst = boolPtr(plan.SupportDebugerFirst)
	out.SupportExperiencerFirst = boolPtr(plan.SupportExperiencerFirst)
	out.CreateTimestamp = unixTime(plan.CreateTimestamp)
	return out, nil
}

// RevertGray 取消分阶段发布（恢复为全量发布）。
func (s *ReleaseService) RevertGray(ctx context.Context, appid string) (*gen.AuditActionResult, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return nil, err
	}
	if err := s.env.Wx.RevertGrayRelease(ctx, token, a.Appid); err != nil {
		return nil, mapError(err)
	}
	s.logOperation(ctx, actionRevertGray, targetApp, a.Appid, nil)
	return actionOK(a.Appid, "已取消分阶段发布：小程序恢复为全量线上版本（不需要重新发布）。"), nil
}

// HistoryVersions 查询可回退的历史版本（官方：最多保留最近 5 个发布或回退的版本）。
func (s *ReleaseService) HistoryVersions(ctx context.Context, appid string) ([]gen.HistoryVersion, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return nil, err
	}
	rows, err := s.env.Wx.ListHistoryVersions(ctx, token, a.Appid)
	if err != nil {
		return nil, mapError(err)
	}
	items := make([]gen.HistoryVersion, 0, len(rows))
	for _, row := range rows {
		items = append(items, gen.HistoryVersion{
			AppVersion:  row.AppVersion,
			UserVersion: strPtr(row.UserVersion),
			UserDesc:    strPtr(row.UserDesc),
			CommitTime:  unixTime(row.CommitTime),
		})
	}
	return items, nil
}

// Revert 版本回退；req.AppVersion 为空表示回退到上一个版本。
//
// 官方限制：最多保留最近 5 个发布/回退的版本；无上一个线上版本、该版本已回退过、
// 或版本早于回退功能上线时间，都会返回 87012。调用前先用 ListHistoryVersions
// 校验版本存在，能把 87012 的三种原因直接翻译成中文；历史列表查询失败时
// 跳过本地校验，交给微信返回 87012 兜底。
func (s *ReleaseService) Revert(ctx context.Context, appid string, req *gen.RevertRequest) (*gen.AuditActionResult, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}

	var appVersion *int64
	if req != nil && req.AppVersion != nil {
		v := *req.AppVersion
		if v <= 0 {
			return nil, core.Validation("appVersion 必须是正整数；留空或不传表示回退到上一个版本")
		}
		appVersion = &v
	}

	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return nil, err
	}

	var targetVersion string
	history, herr := s.env.Wx.ListHistoryVersions(ctx, token, a.Appid)
	switch {
	case herr != nil:
		log.Printf("[wxaudit] 查询可回退版本失败(appid=%s)，跳过本地校验：%v", a.Appid, herr)
	case appVersion == nil && len(history) == 0:
		return nil, core.Conflict(
			"小程序 %s 当前没有可回退的历史版本（微信返回 87012「禁止回退该版本」）：官方最多保留最近 5 个发布或回退的版本，且没有上一个线上版本时无法回退。",
			a.Appid)
	case appVersion == nil:
		targetVersion = history[0].UserVersion
	default:
		found := false
		for _, h := range history {
			if h.AppVersion == *appVersion {
				found, targetVersion = true, h.UserVersion
				break
			}
		}
		if !found {
			return nil, core.Conflict(
				"版本 %d 不在可回退列表里（微信返回 87012）：官方最多保留最近 5 个发布或回退的版本，已回退过的版本也不能再次回退；请用「历史版本」接口重新选择。",
				*appVersion)
		}
	}

	// 官方：GET /wxa/revertcoderelease[&app_version=N]，app_version 是 URL 参数。
	if err := s.env.Wx.RevertCodeRelease(ctx, token, a.Appid, appVersion); err != nil {
		return nil, mapWxError(err, revertHints)
	}

	now := time.Now()
	target := "上一个线上版本"
	if appVersion != nil {
		target = fmt.Sprintf("版本 %d", *appVersion)
	}
	note := "版本回退：回退到" + target
	if targetVersion != "" {
		note += "（user_version=" + targetVersion + "）"
	}
	rec := &model.ReleaseRecord{
		Appid:       a.Appid,
		Action:      model.ReleaseActionRevert,
		UserVersion: targetVersion,
		ReleaseTime: &now,
		Note:        note,
	}
	if err := s.env.Repos.Releases.Create(ctx, rec); err != nil {
		return nil, core.Internal(fmt.Errorf("微信侧已回退成功，但写发布台账失败：%w", err))
	}
	s.logOperation(ctx, actionRevert, targetApp, a.Appid, model.JSONMap{
		"action":      string(model.ReleaseActionRevert),
		"appVersion":  appVersion,
		"userVersion": targetVersion,
	})

	msg := fmt.Sprintf("已回退到%s", target)
	if targetVersion != "" {
		msg += "（user_version=" + targetVersion + "）"
	}
	msg += "；回退后该版本不再出现在可回退列表中，官方最多保留最近 5 个版本。"
	return actionOK(a.Appid, msg), nil
}

// TrialQRCode 获取体验版二维码（微信直接返回二进制 JPEG，这里转成 base64）。
func (s *ReleaseService) TrialQRCode(ctx context.Context, appid string, params gen.GetTrialQRCodeParams) (*gen.QRCodeImage, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	a, err := s.authorizer(ctx, appid)
	if err != nil {
		return nil, err
	}
	token, err := s.token(ctx, a.Appid)
	if err != nil {
		return nil, err
	}
	// 官方：GET /wxa/get_qrcode?path=PATH（path 需要 urlencode，wxapi 已用 url.Values 处理）。
	contentType, data, err := s.env.Wx.GetTrialQRCode(ctx, token, a.Appid, derefString(params.Path))
	if err != nil {
		return nil, mapError(err)
	}
	if len(data) == 0 {
		return nil, core.Internal(fmt.Errorf("小程序 %s：微信未返回体验版二维码图片内容", a.Appid))
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "image/jpeg"
	}
	return &gen.QRCodeImage{
		Appid:       a.Appid,
		ContentType: contentType,
		Base64:      base64.StdEncoding.EncodeToString(data),
	}, nil
}

// boolPtr 返回 bool 指针。
func boolPtr(v bool) *bool { return &v }

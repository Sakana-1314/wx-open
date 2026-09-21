package wxauth

import (
	"context"
	"strings"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// AppService 单个小程序的代调用能力：服务状态、已上传代码的页面列表、基础库版本，
// 以及服务器域名 / 业务域名的批量配置（见 domain.go）。
//
// 本文件所有接口都使用 authorizer_access_token（代商家调用），权限集 18。
type AppService struct {
	base
}

// NewAppService 构造单小程序服务。
func NewAppService(env *core.Env) *AppService {
	return &AppService{base: newBase(env)}
}

// VisitStatus 查询小程序服务状态。
//
// 官方 getvisitstatus 返回 status：0 已暂停服务（含违规被暂停）/ 1 未暂停。
func (s *AppService) VisitStatus(ctx context.Context, appid string) (*gen.VisitStatus, error) {
	if _, err := s.requireAuthorized(ctx, appid); err != nil {
		return nil, err
	}
	token, err := s.authorizerToken(ctx, appid)
	if err != nil {
		return nil, err
	}
	status, err := s.env.Wx.GetVisitStatus(ctx, token, appid)
	if err != nil {
		return nil, wxErr(appid, err)
	}
	return &gen.VisitStatus{Appid: appid, Paused: status == 0}, nil
}

// SetVisitStatus 暂停 / 恢复小程序服务。
//
// 官方 change_visitstatus 只返回 errcode，接口本身没有回显状态，因此这里回显请求值；
// 需要真实状态时再调用 VisitStatus（避免为了回显多打一次微信接口、白撞调用额度）。
func (s *AppService) SetVisitStatus(ctx context.Context, appid string, req gen.VisitStatusUpdateRequest) (*gen.VisitStatus, error) {
	if _, err := s.requireAuthorized(ctx, appid); err != nil {
		return nil, err
	}
	token, err := s.authorizerToken(ctx, appid)
	if err != nil {
		return nil, err
	}
	action := "open"
	if req.Paused {
		action = "close"
	}
	if err := s.env.Wx.SetVisitStatus(ctx, token, appid, action); err != nil {
		return nil, wxErr(appid, err)
	}
	s.writeOperation(ctx, actionSetVisitStatus, targetAuthorizer, appid, model.JSONMap{
		"paused": req.Paused,
		"action": action,
	})
	return &gen.VisitStatus{Appid: appid, Paused: req.Paused}, nil
}

// Pages 已上传代码的页面列表（供提审配置选 address）。
func (s *AppService) Pages(ctx context.Context, appid string) ([]string, error) {
	if _, err := s.requireAuthorized(ctx, appid); err != nil {
		return nil, err
	}
	token, err := s.authorizerToken(ctx, appid)
	if err != nil {
		return nil, err
	}
	list, err := s.env.Wx.GetPage(ctx, token, appid)
	if err != nil {
		return nil, wxErr(appid, err)
	}
	if list == nil {
		// 契约是数组：没有页面时返回 []（而不是 null）。
		list = []string{}
	}
	return list, nil
}

// SupportVersion 查询最低基础库版本与各版本用户占比。
func (s *AppService) SupportVersion(ctx context.Context, appid string) (*gen.SupportVersion, error) {
	if _, err := s.requireAuthorized(ctx, appid); err != nil {
		return nil, err
	}
	token, err := s.authorizerToken(ctx, appid)
	if err != nil {
		return nil, err
	}
	resp, err := s.env.Wx.GetSupportVersion(ctx, token, appid)
	if err != nil {
		return nil, wxErr(appid, err)
	}
	return toSupportVersion(appid, resp.NowVersion, resp.UVInfo.Items), nil
}

// SetSupportVersion 设置小程序最低基础库版本，并回读最新的用户占比。
func (s *AppService) SetSupportVersion(ctx context.Context, appid string, req gen.SupportVersionUpdateRequest) (*gen.SupportVersion, error) {
	version := strings.TrimSpace(req.Version)
	if version == "" {
		return nil, core.Validation("version 不能为空：必须是已发布的基础库版本号（否则微信返回 89014）")
	}
	if _, err := s.requireAuthorized(ctx, appid); err != nil {
		return nil, err
	}
	token, err := s.authorizerToken(ctx, appid)
	if err != nil {
		return nil, err
	}
	if err := s.env.Wx.SetSupportVersion(ctx, token, appid, version); err != nil {
		return nil, wxErr(appid, err)
	}
	s.writeOperation(ctx, actionSetSupportVersion, targetAuthorizer, appid, model.JSONMap{"version": version})

	// 回读用户占比：读失败不影响设置结果（设置本身已成功）。
	if resp, err := s.env.Wx.GetSupportVersion(ctx, token, appid); err == nil {
		return toSupportVersion(appid, resp.NowVersion, resp.UVInfo.Items), nil
	}
	return &gen.SupportVersion{Appid: appid, NowVersion: strPtr(version)}, nil
}

// toSupportVersion 把微信返回的基础库占比转成契约 DTO（percentage 为 float32）。
func toSupportVersion(appid, nowVersion string, items []struct {
	Version    string  `json:"version"`
	Percentage float64 `json:"percentage"`
}) *gen.SupportVersion {
	out := &gen.SupportVersion{Appid: appid}
	if strings.TrimSpace(nowVersion) != "" {
		out.NowVersion = strPtr(nowVersion)
	}
	if len(items) > 0 {
		list := make([]gen.SupportVersionUV, 0, len(items))
		for _, item := range items {
			list = append(list, gen.SupportVersionUV{
				Version:    item.Version,
				Percentage: float32(item.Percentage),
			})
		}
		out.UvItems = &list
	}
	return out
}

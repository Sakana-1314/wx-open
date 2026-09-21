package core

import (
	"context"
	"errors"
	"sort"
	"strings"

	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

// 本文件集中实现「目标小程序选择」这一被作业、体检、批量同步共同复用的逻辑，
// 避免各处重复解释 selection 语义。

// ResolveSelection 依据契约的 AppidSelection 解析出目标小程序清单。
//
// 语义：appids 显式指定优先；其次 filter 走筛选；两者都不传表示全部「已授权且启用」的小程序。
// 结果按 appid 排序，保证批量作业的预览与执行顺序稳定（也便于测试断言）。
func ResolveSelection(ctx context.Context, repos *Repos, sel gen.AppidSelection) ([]model.Authorizer, error) {
	if repos == nil {
		return nil, Internal(errors.New("数据访问未初始化"))
	}
	var list []model.Authorizer
	var err error

	switch {
	case sel.Appids != nil && len(*sel.Appids) > 0:
		appids := normalizeAppids(*sel.Appids)
		if len(appids) == 0 {
			return nil, Validation("未选择任何小程序")
		}
		list, err = repos.Authorizers.ByAppids(ctx, appids)
		if err != nil {
			return nil, Internal(err)
		}
		// 显式指定时按用户给定顺序返回，便于对照勾选结果。
		order := map[string]int{}
		for i, appid := range appids {
			order[appid] = i
		}
		sort.SliceStable(list, func(i, j int) bool { return order[list[i].Appid] < order[list[j].Appid] })
	case sel.Filter != nil:
		f := repo.AuthorizerFilter{
			GroupName:        derefString(sel.Filter.GroupName),
			Tag:              firstTag(sel.Filter.Tags),
			HasDevPermission: sel.Filter.OnlyDevPermission,
			IncludeDisabled:  sel.Filter.IncludeDisabled != nil && *sel.Filter.IncludeDisabled,
			Page:             1,
			PageSize:         1000,
		}
		items, _, err := repos.Authorizers.List(ctx, f)
		if err != nil {
			return nil, Internal(err)
		}
		list = items
		// 批量作业只针对「已授权 + 启用」的小程序，避免对已取消授权的账号白白调用微信。
		kept := make([]model.Authorizer, 0, len(list))
		for i := range list {
			if list[i].AuthorizationStatus != model.AuthStatusAuthorized {
				continue
			}
			if !list[i].Enabled && !f.IncludeDisabled {
				continue
			}
			kept = append(kept, list[i])
		}
		list = kept
	default:
		list, err = repos.Authorizers.All(ctx)
		if err != nil {
			return nil, Internal(err)
		}
		kept := make([]model.Authorizer, 0, len(list))
		for i := range list {
			if list[i].AuthorizationStatus == model.AuthStatusAuthorized && list[i].Enabled {
				kept = append(kept, list[i])
			}
		}
		list = kept
	}

	sort.SliceStable(list, func(i, j int) bool { return list[i].Appid < list[j].Appid })
	if len(list) == 0 {
		return nil, Validation("没有匹配到任何小程序：请确认目标小程序已授权并启用")
	}
	return list, nil
}

func normalizeAppids(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, appid := range in {
		trimmed := strings.TrimSpace(appid)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}

func firstTag(tags *[]string) string {
	if tags == nil || len(*tags) == 0 {
		return ""
	}
	return strings.TrimSpace((*tags)[0])
}

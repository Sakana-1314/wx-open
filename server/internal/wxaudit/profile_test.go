package wxaudit_test

import (
	"errors"
	"strings"
	"testing"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

// completeItemList 一条完整的提审项（类目中文名与数字 id 成对，字段取自 getAllCategoryName）。
func completeItemList() *map[string]interface{} {
	return &map[string]interface{}{
		"address":      "pages/index/index",
		"tag":          "首页 工具",
		"title":        "示例标题",
		"first_class":  "工具",
		"second_class": "信息查询",
		"first_id":     1,
		"second_id":    101,
	}
}

// TestProfilesListSeedsDefault 验证库为空时列表至少返回一条默认配置（与启动种子一致）。
func TestProfilesListSeedsDefault(t *testing.T) {
	f := newFixture(t)
	// audit_profiles 是共享表（其它测试包可能写过配置）：显式清空以保证「空库」这个前提。
	f.clearTable(&model.AuditProfile{})

	resp, err := f.audit.ListProfiles(f.ctx)
	if err != nil {
		t.Fatalf("ListProfiles 失败: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatalf("库为空时应补一条默认配置，实际 %d 条", len(resp.Items))
	}
	item := resp.Items[0]
	if item.Id <= 0 || item.Name != "默认提审配置" || !item.IsDefault {
		t.Fatalf("默认配置内容不对：%+v", item)
	}
	if item.CreatedAt == nil {
		t.Fatalf("应回传创建时间：%+v", item)
	}

	// 再次调用不会重复补默认配置。
	again, err := f.audit.ListProfiles(f.ctx)
	if err != nil {
		t.Fatalf("ListProfiles 失败: %v", err)
	}
	if len(again.Items) != 1 || again.Items[0].Id != item.Id {
		t.Fatalf("重复调用不应新增默认配置：%+v", again.Items)
	}
}

// TestCreateProfileAndNameConflict 验证新建提审配置、名称唯一与「只有一个默认配置」。
func TestCreateProfileAndNameConflict(t *testing.T) {
	f := newFixture(t)
	isDefault := true
	created, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{
		Name:        "标准提审项",
		IsDefault:   &isDefault,
		ItemList:    completeItemList(),
		VersionDesc: strPtr("修复已知问题"),
		Note:        strPtr("备注"),
	})
	if err != nil {
		t.Fatalf("CreateProfile 失败: %v", err)
	}
	if created.Id <= 0 || created.Name != "标准提审项" || !created.IsDefault {
		t.Fatalf("新建结果不对：%+v", created)
	}
	if created.ItemList == nil || (*created.ItemList)["first_class"] != "工具" {
		t.Fatalf("itemList 未落库：%+v", created.ItemList)
	}
	if created.VersionDesc == nil || *created.VersionDesc != "修复已知问题" {
		t.Fatalf("versionDesc 未落库：%+v", created.VersionDesc)
	}

	// 重名 → 409。
	_, err = f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{Name: "标准提审项"})
	if !errors.Is(err, core.ErrConflict) {
		t.Fatalf("重名应返回 ErrConflict，实际 %v", err)
	}

	// 再建一个默认配置：原默认（如果存在）的 is_default 必须被清零。
	if _, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{
		Name: "默认提审配置", IsDefault: &isDefault,
	}); err != nil {
		t.Fatalf("CreateProfile 失败: %v", err)
	}
	items, err := f.audit.ListProfiles(f.ctx)
	if err != nil {
		t.Fatalf("ListProfiles 失败: %v", err)
	}
	defaults := 0
	for _, item := range items.Items {
		if item.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("全局只应有一个默认配置，实际 %d 个：%+v", defaults, items.Items)
	}

	// 合法名称前后空格应被归一化。
	trimmed, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{Name: "  带空格配置  "})
	if err != nil {
		t.Fatalf("CreateProfile 失败: %v", err)
	}
	if trimmed.Name != "带空格配置" {
		t.Fatalf("名称应去掉首尾空格，实际 %q", trimmed.Name)
	}
}

// TestCreateProfileValidation 验证名称必填与类目字段完整性校验。
func TestCreateProfileValidation(t *testing.T) {
	f := newFixture(t)

	if _, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{Name: ""}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("name 为空应返回 ErrValidation，实际 %v", err)
	}

	// 只给了中文名、缺数字 id：提审会报 85008，必须在保存时就拦住。
	incomplete := map[string]interface{}{"first_class": "工具", "second_class": "信息查询"}
	_, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{Name: "类目不完整", ItemList: &incomplete})
	if !errors.Is(err, core.ErrValidation) {
		t.Fatalf("类目字段不完整应返回 ErrValidation，实际 %v", err)
	}
	for _, want := range []string{"first_id", "second_id", "getAllCategoryName"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("校验提示应包含 %q，实际 %q", want, err.Error())
		}
	}

	// 数组写法（item_list 多条）同样被校验。
	nested := map[string]interface{}{
		"item_list": []interface{}{
			map[string]interface{}{"first_class": "工具", "second_class": "信息查询", "first_id": 1},
		},
	}
	if _, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{Name: "数组类目不完整", ItemList: &nested}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("嵌套 item_list 里的类目不完整也应返回 ErrValidation，实际 %v", err)
	}

	// 完全不带类目字段（例如默认配置）允许保存。
	if _, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{
		Name:     "仅占位配置",
		ItemList: &map[string]interface{}{"address": "pages/index/index"},
	}); err != nil {
		t.Fatalf("不带类目字段应允许保存: %v", err)
	}

	// 完整的数组写法可以保存。
	nestedOK := map[string]interface{}{
		"item_list": []interface{}{
			map[string]interface{}{"first_class": "工具", "second_class": "信息查询", "first_id": 1, "second_id": 101},
		},
	}
	if _, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{Name: "数组类目完整", ItemList: &nestedOK}); err != nil {
		t.Fatalf("完整类目应允许保存: %v", err)
	}
}

// TestUpdateProfile 验证编辑：id 不存在 → 404，改名冲突 → 409，正常编辑落库。
func TestUpdateProfile(t *testing.T) {
	f := newFixture(t)
	created, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{Name: "待编辑配置"})
	if err != nil {
		t.Fatalf("CreateProfile 失败: %v", err)
	}
	if _, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{Name: "已占用名称"}); err != nil {
		t.Fatalf("CreateProfile 失败: %v", err)
	}

	if _, err := f.audit.UpdateProfile(f.ctx, 999999, gen.AuditProfileUpsertRequest{Name: "不存在"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("id 不存在应返回 ErrNotFound，实际 %v", err)
	}
	if _, err := f.audit.UpdateProfile(f.ctx, created.Id, gen.AuditProfileUpsertRequest{Name: "已占用名称"}); !errors.Is(err, core.ErrConflict) {
		t.Fatalf("改名冲突应返回 ErrConflict，实际 %v", err)
	}

	isDefault := true
	updated, err := f.audit.UpdateProfile(f.ctx, created.Id, gen.AuditProfileUpsertRequest{
		Name:        "已编辑配置",
		IsDefault:   &isDefault,
		ItemList:    completeItemList(),
		VersionDesc: strPtr("第二个版本"),
		UgcDeclare:  &map[string]interface{}{"scene": []interface{}{1}, "method": 1},
	})
	if err != nil {
		t.Fatalf("UpdateProfile 失败: %v", err)
	}
	if updated.Id != created.Id || updated.Name != "已编辑配置" || !updated.IsDefault {
		t.Fatalf("编辑结果不对：%+v", updated)
	}
	// JSON 列按序列化后的形状断言（数字在 JSON 里都是 number，与 HTTP 响应一致）。
	if got := mustJSON(t, updated.UgcDeclare); got != `{"method":1,"scene":[1]}` {
		t.Fatalf("ugcDeclare 未落库：%s", got)
	}
	// 落到库里再看一遍（确保是真写库，而不是只回显请求体）。
	stored, err := f.env.Repos.Profiles.Get(f.ctx, uint(created.Id))
	if err != nil {
		t.Fatalf("读取配置失败: %v", err)
	}
	if stored.Name != "已编辑配置" || stored.VersionDesc != "第二个版本" || !stored.IsDefault {
		t.Fatalf("库里的配置不对：%+v", stored)
	}
	if !strings.Contains(stored.ItemList["first_class"].(string), "工具") {
		t.Fatalf("库里的 itemList 不对：%+v", stored.ItemList)
	}
}

// TestDeleteProfile 验证删除：被引用 → 409，未被引用 → 删除成功，重复删除 → 404。
func TestDeleteProfile(t *testing.T) {
	const appid = "wx_profile_ref_app"
	f := newFixture(t)
	// 用 appCustom 拿到账号指针：改绑提审配置后直接 plant 回同一行，
	// 这样并发跑测试包时的 seed() 不会把改绑结果抹掉。
	app := f.app(appid, "引用提审配置的小程序", model.PermissionSetDev)

	profile, err := f.audit.CreateProfile(f.ctx, gen.AuditProfileUpsertRequest{Name: "被引用配置"})
	if err != nil {
		t.Fatalf("CreateProfile 失败: %v", err)
	}
	profileID := uint(profile.Id)
	app.AuditProfileID = &profileID
	f.plant(app, true)

	err = f.audit.DeleteProfile(f.ctx, profile.Id)
	if !errors.Is(err, core.ErrConflict) {
		t.Fatalf("被引用时应返回 ErrConflict，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "改绑") {
		t.Fatalf("冲突提示应指导先改绑：%q", err.Error())
	}

	// 解绑后可删除。
	app.AuditProfileID = nil
	f.plant(app, true)
	if err := f.audit.DeleteProfile(f.ctx, profile.Id); err != nil {
		t.Fatalf("解绑后应能删除: %v", err)
	}
	if _, err := f.env.Repos.Profiles.Get(f.ctx, uint(profile.Id)); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("删除后应查不到配置，实际 %v", err)
	}
	if err := f.audit.DeleteProfile(f.ctx, profile.Id); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("重复删除应返回 ErrNotFound，实际 %v", err)
	}
	if err := f.audit.DeleteProfile(f.ctx, 0); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("id 非法应返回 ErrValidation，实际 %v", err)
	}
}

package wxjob

import (
	"errors"
	"strings"
	"testing"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// TestPreviewProblems 预览必须对「模板不存在 / 标准模板 / ext_json 渲染失败 / 版本号超 64 字符 /
// 缺类目」分别给出 invalid + 中文原因。
func TestPreviewProblems(t *testing.T) {
	app := newTestApp(t)
	app.withStub() // 用于断言预览全程不调用微信
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)

	standard := app.seedTemplate(1001, 1)
	normal := app.seedTemplate(1002, 0)
	okProfile := app.seedProfile(model.JSONMap{"item_list": []any{validAuditItem()}})
	noCategoryProfile := app.seedProfile(model.JSONMap{"item_list": []any{map[string]any{"title": "首页"}}})

	cases := []struct {
		name   string
		req    gen.JobCreateRequest
		expect []string
	}{
		{
			name: "模板不存在",
			req: gen.JobCreateRequest{
				Type:      gen.JobType(model.JobTypeCommit),
				Selection: appidSelection(appid),
				Commit:    &gen.CommitOptions{TemplateId: 999999, UserVersionPattern: ptr("{{date}}-{{seq}}")},
			},
			expect: []string{"模板不存在", "85014"},
		},
		{
			name: "标准模板",
			req: gen.JobCreateRequest{
				Type:      gen.JobType(model.JobTypeCommit),
				Selection: appidSelection(appid),
				Commit:    &gen.CommitOptions{TemplateId: standard.TemplateID, UserVersionPattern: ptr("{{date}}-{{seq}}")},
			},
			expect: []string{"标准模板", "9402203"},
		},
		{
			name: "ext_json 渲染失败",
			req: gen.JobCreateRequest{
				Type:      gen.JobType(model.JobTypeCommit),
				Selection: appidSelection(appid),
				Commit: &gen.CommitOptions{
					TemplateId:         normal.TemplateID,
					UserVersionPattern: ptr("{{date}}-{{seq}}"),
					ExtTemplate:        ptr(`{"ext":{"backend":"{{undefined_var}}"}}`),
				},
			},
			expect: []string{"未定义的变量", "undefined_var"},
		},
		{
			name: "版本号超 64 字符",
			req: gen.JobCreateRequest{
				Type:      gen.JobType(model.JobTypeCommit),
				Selection: appidSelection(appid),
				Commit:    &gen.CommitOptions{TemplateId: normal.TemplateID, UserVersionPattern: ptr(strings.Repeat("v", 65))},
			},
			expect: []string{"版本号", "64"},
		},
		{
			name: "提审缺类目",
			req: gen.JobCreateRequest{
				Type:      gen.JobType(model.JobTypeSubmitAudit),
				Selection: appidSelection(appid),
				Audit:     &gen.AuditOptions{AuditProfileId: int64Ptr(int64(noCategoryProfile.ID))},
			},
			expect: []string{"类目", "first_id"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := app.svc.Preview(app.ctx(), tc.req)
			if err != nil {
				t.Fatalf("预览不应返回错误: %v", err)
			}
			if resp.InvalidCount != 1 || resp.ValidCount != 0 {
				t.Fatalf("期望 1 个 invalid、0 个 valid，实际 invalid=%d valid=%d", resp.InvalidCount, resp.ValidCount)
			}
			item := resp.Items[0]
			if item.Valid {
				t.Fatalf("该项不应通过校验：%+v", item)
			}
			if len(item.Problems) == 0 {
				t.Fatalf("必须给出校验原因")
			}
			containsAll(t, strings.Join(item.Problems, "；"), tc.expect...)
		})
	}

	// 对照组：提审配置齐全时该项应通过校验。
	okResp, err := app.svc.Preview(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeSubmitAudit),
		Selection: appidSelection(appid),
		Audit:     &gen.AuditOptions{AuditProfileId: int64Ptr(int64(okProfile.ID))},
	})
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}
	if okResp.ValidCount != 1 {
		t.Fatalf("提审配置齐全时应通过校验，实际问题：%v", okResp.Items[0].Problems)
	}

	if app.stub.callCount() != 0 {
		t.Fatalf("预览不得调用微信接口，实际调用了 %d 次：%v", app.stub.callCount(), app.stub.calls)
	}
}

// TestPreviewValidCommitBody 合法输入下预览给出最终请求体（不调微信）。
func TestPreviewValidCommitBody(t *testing.T) {
	app := newTestApp(t)
	app.withStub()
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	tpl := app.seedTemplate(2001, 0)
	app.setSetting(settingGlobalExtVars, `{"env_name":"production"}`)

	resp, err := app.svc.Preview(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(appid),
		Commit: &gen.CommitOptions{
			TemplateId:         tpl.TemplateID,
			UserVersionPattern: ptr("{{date}}-{{seq}}"),
			UserDescPattern:    ptr("{{nick_name}} 自动上传"),
			ExtOverrides:       &map[string]string{"store_code": "S001"},
		},
	})
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}
	if resp.ValidCount != 1 || resp.InvalidCount != 0 {
		t.Fatalf("期望全部有效，实际 valid=%d invalid=%d（problems=%v）", resp.ValidCount, resp.InvalidCount, resp.Items[0].Problems)
	}
	item := resp.Items[0]
	if item.Endpoint != endpointCommit {
		t.Fatalf("端点应为 %s，实际 %s", endpointCommit, item.Endpoint)
	}
	if got := jsonInt64Of(item.Payload["template_id"]); got != tpl.TemplateID {
		t.Fatalf("template_id 应为 %d，实际 %v", tpl.TemplateID, item.Payload["template_id"])
	}
	extJSON, ok := item.Payload["ext_json"].(string)
	if !ok || !strings.Contains(extJSON, "S001") {
		t.Fatalf("ext_json 必须是含作业覆盖变量的字符串化 JSON，实际 %#v", item.Payload["ext_json"])
	}
	if version, _ := item.Payload["user_version"].(string); !strings.HasPrefix(version, "20") {
		t.Fatalf("user_version 应已渲染日期前缀，实际 %q", version)
	}
	if item.NickName == nil || *item.NickName != "测试小程序" {
		t.Fatalf("应带小程序昵称，实际 %v", item.NickName)
	}
	if app.stub.callCount() != 0 {
		t.Fatalf("预览不得调用微信接口，实际调用了 %d 次", app.stub.callCount())
	}
}

// TestPreviewMultiStepPayload 多步作业（pipeline）预览把每一步的端点与请求体一并给出。
func TestPreviewMultiStepPayload(t *testing.T) {
	app := newTestApp(t)
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)
	tpl := app.seedTemplate(3001, 0)
	profile := app.seedProfile(model.JSONMap{"item_list": []any{validAuditItem()}})

	resp, err := app.svc.Preview(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypePipeline),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}-{{seq}}")},
		Audit:     &gen.AuditOptions{AuditProfileId: int64Ptr(int64(profile.ID))},
	})
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}
	if resp.ValidCount != 1 {
		t.Fatalf("期望 1 个有效项，实际问题：%v", resp.Items[0].Problems)
	}
	item := resp.Items[0]
	if item.Endpoint != endpointCommit {
		t.Fatalf("首个端点应为 %s，实际 %s", endpointCommit, item.Endpoint)
	}
	steps, ok := item.Payload["steps"].([]map[string]any)
	if !ok || len(steps) != 4 {
		t.Fatalf("pipeline 应给出 4 个步骤明细，实际 %#v", item.Payload["steps"])
	}
	order := []string{"commit", "privacy_check", "submit_audit", "release"}
	for i, st := range steps {
		if st["step"] != order[i] {
			t.Fatalf("步骤顺序应为 %v，实际第 %d 步为 %v", order, i+1, st["step"])
		}
	}
}

// TestPreviewRequestLevelValidation 请求级参数缺失直接返回校验错误。
func TestPreviewRequestLevelValidation(t *testing.T) {
	app := newTestApp(t)
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid)

	cases := []struct {
		name string
		req  gen.JobCreateRequest
	}{
		{
			name: "缺少模板",
			req: gen.JobCreateRequest{
				Type:      gen.JobType(model.JobTypeCommit),
				Selection: appidSelection(appid),
				Commit:    &gen.CommitOptions{TemplateId: 0, UserVersionPattern: ptr("{{date}}")},
			},
		},
		{
			name: "缺少版本号模板",
			req: gen.JobCreateRequest{
				Type:      gen.JobType(model.JobTypeCommit),
				Selection: appidSelection(appid),
				Commit:    &gen.CommitOptions{TemplateId: 1},
			},
		},
		{
			name: "未知作业类型",
			req: gen.JobCreateRequest{
				Type:      gen.JobType("unknown_type"),
				Selection: appidSelection(appid),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := app.svc.Preview(app.ctx(), tc.req); !errors.Is(err, core.ErrValidation) {
				t.Fatalf("期望 core.ErrValidation，实际 %v", err)
			}
		})
	}
}

// TestPreviewAuthorizationProblems 未授权 / 停用 / 缺开发权限集的小程序在预览里 invalid。
func TestPreviewAuthorizationProblems(t *testing.T) {
	app := newTestApp(t)
	appid := appidOf(t.Name())
	app.seedAuthorizer(appid, func(a *model.Authorizer) { a.FuncInfo = nil })
	tpl := app.seedTemplate(4001, 0)

	resp, err := app.svc.Preview(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(appid),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}")},
	})
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}
	if resp.ValidCount != 0 || resp.InvalidCount != 1 {
		t.Fatalf("期望全部无效，实际 valid=%d invalid=%d", resp.ValidCount, resp.InvalidCount)
	}
	containsAll(t, strings.Join(resp.Items[0].Problems, "；"), "权限集 18")

	// 显式指定但本地没有授权记录的小程序也要出现在预览里（混在有效选择中时不能被静默丢掉）。
	missing := "wxnotexists0001"
	valid := appidOf(t.Name() + "valid")
	app.seedAuthorizer(valid)
	resp2, err := app.svc.Preview(app.ctx(), gen.JobCreateRequest{
		Type:      gen.JobType(model.JobTypeCommit),
		Selection: appidSelection(valid, missing),
		Commit:    &gen.CommitOptions{TemplateId: tpl.TemplateID, UserVersionPattern: ptr("{{date}}")},
	})
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}
	if resp2.ValidCount != 1 || resp2.InvalidCount != 1 {
		t.Fatalf("期望 1 有效 + 1 无效，实际 valid=%d invalid=%d", resp2.ValidCount, resp2.InvalidCount)
	}
	var missingItem *gen.JobPreviewItem
	for i := range resp2.Items {
		if resp2.Items[i].Appid == missing {
			missingItem = &resp2.Items[i]
		}
	}
	if missingItem == nil {
		t.Fatalf("未授权的 appid 应作为 invalid 项出现，实际 %+v", resp2.Items)
	}
	containsAll(t, strings.Join(missingItem.Problems, "；"), "授权记录")
}

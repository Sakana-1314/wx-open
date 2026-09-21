package wxauth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// testDraft 造一条草稿。
func testDraft(id int64) model.CodeDraft {
	return model.CodeDraft{
		DraftID:                id,
		UserVersion:            "1.0.0",
		UserDesc:               "首个版本",
		SourceMiniProgramAppid: "wxdev0000000001",
		SourceMiniProgram:      "开发者小程序",
		Developer:              "dev",
		CreateTime:             1700000000,
		SyncedAt:               time.Now(),
	}
}

func TestDraftsAndAddToTemplate(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()

	// 有状态的模拟微信：addtotemplate 之后 gettemplatelist 才会返回新模板
	// （官方：addtotemplate 不返回 template_id，必须重新拉列表）。
	var mu sync.Mutex
	templates := []map[string]any{}
	te.wx.jsonRoute("/wxa/gettemplatedraftlist", map[string]any{
		"draft_list": []map[string]any{{
			"draft_id":                 float64(11),
			"user_version":             "1.0.0",
			"user_desc":                "首个版本",
			"source_miniprogram_appid": "wxdev0000000001",
			"source_miniprogram":       "开发者小程序",
			"developer":                "dev",
			"create_time":              float64(1700000000),
		}},
	})
	te.wx.route("/wxa/gettemplatelist", func(w http.ResponseWriter, _ recordedCall) {
		mu.Lock()
		snapshot := append([]map[string]any(nil), templates...)
		mu.Unlock()
		writeMockJSON(w, map[string]any{"template_list": snapshot})
	})
	te.wx.route("/wxa/addtotemplate", func(w http.ResponseWriter, call recordedCall) {
		if int64InBody(call, "draft_id") != 11 {
			writeMockJSON(w, map[string]any{"errcode": 85064, "errmsg": "draft not exist"})
			return
		}
		mu.Lock()
		templates = append(templates, map[string]any{
			"template_id":              float64(9001),
			"draft_id":                 float64(11),
			"template_type":            float64(0),
			"user_version":             "1.0.0",
			"user_desc":                "首个版本",
			"source_miniprogram_appid": "wxdev0000000001",
			"developer":                "dev",
			"create_time":              float64(1700000000),
		})
		mu.Unlock()
		writeMockJSON(w, map[string]any{"errcode": 0, "errmsg": "ok"})
	})

	svc := NewTemplateService(te.env)

	// 本地草稿箱为空 → 自动同步一次。
	drafts, err := svc.ListDrafts(ctx)
	if err != nil {
		t.Fatalf("ListDrafts 失败: %v", err)
	}
	if len(drafts.Items) != 1 || drafts.Items[0].DraftId != 11 {
		t.Fatalf("草稿列表不对: %+v", drafts.Items)
	}
	if drafts.Items[0].UserVersion == nil || *drafts.Items[0].UserVersion != "1.0.0" {
		t.Errorf("草稿字段未映射: %+v", drafts.Items[0])
	}
	if n := te.wx.countTo("/wxa/gettemplatedraftlist"); n != 1 {
		t.Errorf("自动同步应只调用一次 gettemplatedraftlist，实际 %d 次", n)
	}
	// 第二次读库，不再打微信。
	if _, err := svc.ListDrafts(ctx); err != nil {
		t.Fatalf("第二次 ListDrafts 失败: %v", err)
	}
	if n := te.wx.countTo("/wxa/gettemplatedraftlist"); n != 1 {
		t.Errorf("库非空时不应再同步，实际调用 %d 次", n)
	}

	out, err := svc.AddDraftToTemplate(ctx, 11, nil)
	if err != nil {
		t.Fatalf("AddDraftToTemplate 失败: %v", err)
	}
	if out.Limit != model.TemplateLibraryLimit {
		t.Errorf("模板库上限 = %d，期望 %d", out.Limit, model.TemplateLibraryLimit)
	}
	if len(out.Items) != 1 || out.Items[0].TemplateId != 9001 {
		t.Fatalf("添加后应返回最新模板列表: %+v", out.Items)
	}
	if out.Items[0].IsDefault {
		t.Error("新添加的模板不应是默认模板")
	}
	if got := len(te.wx.callsTo("/wxa/gettemplatelist")); got != 1 {
		t.Errorf("addtotemplate 后必须重新拉取模板列表（官方不返回 template_id），gettemplatelist 调用 %d 次", got)
	}
	seq := te.wx.pathSequence()
	if indexOfPath(seq, "/wxa/addtotemplate") > indexOfPath(seq, "/wxa/gettemplatelist") {
		t.Errorf("调用顺序不对：应先 addtotemplate 再 gettemplatelist，实际 %v", seq)
	}
	if body := te.wx.lastCallTo("/wxa/addtotemplate").Body; int64InBody(recordedCall{Body: body}, "draft_id") != 11 {
		t.Errorf("addtotemplate 请求体不对: %v", body)
	}
	rows, err := te.env.Repos.Templates.List(ctx, nil)
	if err != nil || len(rows) != 1 || rows[0].TemplateID != 9001 {
		t.Fatalf("模板未落库: %+v (err=%v)", rows, err)
	}

	// 再次 ListTemplates 应直接读库。
	addCalls := len(te.wx.callsTo("/wxa/gettemplatelist"))
	if _, err := svc.ListTemplates(ctx, gen.ListTemplatesParams{}); err != nil {
		t.Fatalf("ListTemplates 失败: %v", err)
	}
	if got := len(te.wx.callsTo("/wxa/gettemplatelist")); got != addCalls {
		t.Errorf("模板库非空时列表应读库，实际又调用了 %d 次", got-addCalls)
	}

	// 草稿不存在：本地直接 404，不打微信。
	addBefore := te.wx.countTo("/wxa/addtotemplate")
	if _, err := svc.AddDraftToTemplate(ctx, 999, nil); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("不存在的草稿应返回 ErrNotFound，实际 %v", err)
	}
	if got := te.wx.countTo("/wxa/addtotemplate"); got != addBefore {
		t.Errorf("草稿不存在时不应调用微信，实际多调用 %d 次", got-addBefore)
	}

	te.requireAction(actionSyncDrafts)
	te.requireAction(actionSyncTemplates)
	te.requireAction(actionAddToTemplate)
}

func TestAddToTemplateRejectsFullLibrary(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	rows := make([]model.CodeTemplate, 0, model.TemplateLibraryLimit)
	for i := 1; i <= model.TemplateLibraryLimit; i++ {
		rows = append(rows, model.CodeTemplate{TemplateID: int64(1000 + i), SyncedAt: time.Now()})
	}
	if err := te.db.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatalf("填充模板库失败: %v", err)
	}
	if err := te.env.Repos.Drafts.ReplaceAll(ctx, []model.CodeDraft{testDraft(11)}); err != nil {
		t.Fatalf("写入草稿失败: %v", err)
	}

	svc := NewTemplateService(te.env)
	_, err := svc.AddDraftToTemplate(ctx, 11, nil)
	if !errors.Is(err, core.ErrConflict) {
		t.Fatalf("模板库已满应返回冲突，实际 %v", err)
	}
	if !strings.Contains(err.Error(), "200") || !strings.Contains(err.Error(), "删除") {
		t.Errorf("冲突文案应提示上限 200 与先删除模板: %v", err)
	}
	if n := te.wx.countTo("/wxa/addtotemplate"); n != 0 {
		t.Fatalf("模板库已满时不应调用微信，实际 %d 次", n)
	}
}

func TestDeleteTemplateTreats85064AsAlreadyGone(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	if err := te.env.Repos.Templates.ReplaceAll(ctx, []model.CodeTemplate{
		{TemplateID: 9001, SyncedAt: time.Now()},
		{TemplateID: 9002, SyncedAt: time.Now()},
	}); err != nil {
		t.Fatalf("准备模板数据失败: %v", err)
	}
	svc := NewTemplateService(te.env)

	// 微信侧已经不存在（85064）：继续删本地。
	te.wx.failRoute("/wxa/deletetemplate", 85064, "not exist")
	if err := svc.DeleteTemplate(ctx, 9001); err != nil {
		t.Fatalf("85064 应视为已不存在并继续删本地，实际 %v", err)
	}
	left, err := te.env.Repos.Templates.List(ctx, nil)
	if err != nil || len(left) != 1 || left[0].TemplateID != 9002 {
		t.Fatalf("本地模板未删除: %+v (err=%v)", left, err)
	}

	// 其它错误码要冒泡成 WeChatError（带 errcode 与中文处置建议）。
	te.wx.failRoute("/wxa/deletetemplate", 45009, "api freq out of limit")
	err = svc.DeleteTemplate(ctx, 9002)
	var wxErr *core.WeChatError
	if !errors.As(err, &wxErr) || wxErr.Errcode != 45009 {
		t.Fatalf("其它错误应返回 WeChatError，实际 %v", err)
	}
	if wxErr.Class != model.ClassRateLimited {
		t.Errorf("45009 应归类为限流，实际 %s", wxErr.Class)
	}
	if left, _ := te.env.Repos.Templates.List(ctx, nil); len(left) != 1 {
		t.Errorf("删除失败时不应删本地记录: %+v", left)
	}

	if err := svc.DeleteTemplate(ctx, 999999); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("模板不存在应返回 ErrNotFound，实际 %v", err)
	}
	te.requireAction(actionDeleteTemplate)
}

func TestUpdateTemplateIsLocalOnly(t *testing.T) {
	te := newTestEnv(t)
	ctx := context.Background()
	if err := te.env.Repos.Templates.ReplaceAll(ctx, []model.CodeTemplate{
		{TemplateID: 9001, SyncedAt: time.Now()},
		{TemplateID: 9002, SyncedAt: time.Now()},
	}); err != nil {
		t.Fatalf("准备模板数据失败: %v", err)
	}
	svc := NewTemplateService(te.env)

	note := "主力模板"
	isDefault := true
	detail, err := svc.UpdateTemplate(ctx, 9001, gen.TemplateUpdateRequest{IsDefault: &isDefault, Note: &note})
	if err != nil {
		t.Fatalf("UpdateTemplate 失败: %v", err)
	}
	if !detail.IsDefault || detail.Note == nil || *detail.Note != note {
		t.Fatalf("更新结果不对: %+v", detail)
	}

	if _, err := svc.UpdateTemplate(ctx, 9002, gen.TemplateUpdateRequest{IsDefault: &isDefault}); err != nil {
		t.Fatalf("设置第二个默认模板失败: %v", err)
	}
	rows, err := te.env.Repos.Templates.List(ctx, nil)
	if err != nil {
		t.Fatalf("读取模板失败: %v", err)
	}
	defaults := 0
	for _, row := range rows {
		if row.IsDefault {
			defaults++
			if row.TemplateID != 9002 {
				t.Errorf("默认模板应为 9002，实际 %d", row.TemplateID)
			}
		}
	}
	if defaults != 1 {
		t.Fatalf("默认模板必须唯一，实际 %d 个", defaults)
	}
	// 只改本地：不调用任何微信接口。
	if n := te.wx.countTo("/wxa/gettemplatelist") + te.wx.countTo("/wxa/addtotemplate"); n != 0 {
		t.Errorf("更新本地字段不应调用微信，实际 %d 次", n)
	}

	if _, err := svc.UpdateTemplate(ctx, 999999, gen.TemplateUpdateRequest{Note: &note}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("模板不存在应返回 ErrNotFound，实际 %v", err)
	}
	badType := gen.ListTemplatesParamsTemplateType(7)
	if _, err := svc.ListTemplates(ctx, gen.ListTemplatesParams{TemplateType: &badType}); !errors.Is(err, core.ErrValidation) {
		t.Fatalf("非法 templateType 应返回校验错误，实际 %v", err)
	}
}

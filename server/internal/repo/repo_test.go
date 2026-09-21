package repo_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"wx-platform/server/internal/model"
	"wx-platform/server/internal/repo"
)

// 说明：本包是集成测试，需要一个可写的 MySQL / MariaDB 库。
//
//	TEST_DB_DSN='wxplatform:wxplatform_dev_password@tcp(127.0.0.1:3306)/wx_platform_test?charset=utf8mb4&parseTime=True&loc=Local' go test ./internal/repo/ -v
//
// 测试库由仓库根目录的 db/init.sql 创建（mysql -u root < db/init.sql）。
//
// 未设置 TEST_DB_DSN 时全部用例跳过（与团队其它项目的集成测试约定一致），
// 因此本地无数据库时 `go test` 依然可以通过。

// testDB 建立测试库连接并迁移全部表；未设置 TEST_DB_DSN 时跳过测试。
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_DB_DSN"))
	if dsn == "" {
		t.Skip("未设置 TEST_DB_DSN，跳过 repo 集成测试（先 mysql -u root < db/init.sql 建库，再设 TEST_DB_DSN='wxplatform:wxplatform_dev_password@tcp(127.0.0.1:3306)/wx_platform_test?charset=utf8mb4&parseTime=True&loc=Local'）")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("获取底层连接失败: %v", err)
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(testModels()...); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	for _, m := range testModels() {
		if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(m).Error; err != nil {
			t.Fatalf("清空测试表失败: %v", err)
		}
	}
	return db
}

// testModels 需要迁移的全部模型（与 internal/database 中的清单保持一致）。
func testModels() []any {
	return []any{
		&model.PlatformState{},
		&model.Authorizer{},
		&model.TokenCache{},
		&model.CodeDraft{},
		&model.CodeTemplate{},
		&model.AuditProfile{},
		&model.BatchJob{},
		&model.BatchJobItem{},
		&model.AuditRecord{},
		&model.ReleaseRecord{},
		&model.UndoQuotaUsage{},
		&model.ApiCallLog{},
		&model.CallbackEvent{},
		&model.OperationLog{},
		&model.RuntimeSetting{},
	}
}

// sec 截断到秒，避免 DATETIME 精度差异导致比较失败。
func sec(t time.Time) time.Time { return t.Truncate(time.Second) }

// monthKeyOf 与 repo 内部 month_key 口径一致（YYYY-MM），用于构造「上月用量」。
func monthKeyOf(t time.Time) string { return t.Format("2006-01") }

// prevMonth 返回上一个自然月的同一时刻（用于构造跨月用量）。
func prevMonth(t time.Time) time.Time { return t.AddDate(0, -1, 0) }

func intPtr(v int) *int    { return &v }
func boolPtr(v bool) *bool { return &v }

func countRows(t *testing.T, db *gorm.DB, dest any) int64 {
	t.Helper()
	var n int64
	if err := db.Model(dest).Count(&n).Error; err != nil {
		t.Fatalf("统计 %T 失败: %v", dest, err)
	}
	return n
}

// ---- 平台状态 ----

func TestPlatformStateLifecycle(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := repo.NewPlatformState(db)

	st, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("首次 Get 应创建平台状态: %v", err)
	}
	if st.ID != 1 {
		t.Fatalf("平台状态主键应为 1，实际 %d", st.ID)
	}
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("再次 Get 失败: %v", err)
	}
	if n := countRows(t, db, &model.PlatformState{}); n != 1 {
		t.Fatalf("platform_state 应始终只有 1 行，实际 %d", n)
	}

	ticketAt := sec(time.Now().Add(-time.Minute))
	if err := r.SaveTicket(ctx, "ticket-abc", ticketAt); err != nil {
		t.Fatalf("SaveTicket 失败: %v", err)
	}
	expireAt := sec(time.Now().Add(2 * time.Hour))
	if err := r.SaveComponentToken(ctx, "component-token-1", expireAt); err != nil {
		t.Fatalf("SaveComponentToken 失败: %v", err)
	}
	pushAt := sec(time.Now())
	if err := r.MarkPushOK(ctx, pushAt); err != nil {
		t.Fatalf("MarkPushOK 失败: %v", err)
	}

	got, err := r.Get(ctx)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.VerifyTicket != "ticket-abc" || got.TicketReceivedAt == nil || !got.TicketReceivedAt.Equal(ticketAt) {
		t.Fatalf("票据写入不正确: %+v", got)
	}
	if got.ComponentAccessToken != "component-token-1" || got.ComponentTokenExpireAt == nil ||
		!got.ComponentTokenExpireAt.Equal(expireAt) {
		t.Fatalf("component_access_token 写入不正确: %+v", got)
	}
	if got.LastPushOKAt == nil || !got.LastPushOKAt.Equal(pushAt) {
		t.Fatalf("推送成功时间写入不正确: %+v", got)
	}
}

// ---- 令牌缓存 ----

func TestTokensUpsertIdempotentAndDelete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := repo.NewTokens(db)

	exp1 := sec(time.Now().Add(time.Hour))
	if err := r.Put(ctx, model.TokenScopeComponent, "", "token-1", exp1); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}
	exp2 := sec(exp1.Add(time.Hour))
	if err := r.Put(ctx, model.TokenScopeComponent, "", "token-2", exp2); err != nil {
		t.Fatalf("重复 Put 失败: %v", err)
	}
	if n := countRows(t, db, &model.TokenCache{}); n != 1 {
		t.Fatalf("同 (scope,appid) 应 upsert 为 1 行，实际 %d", n)
	}
	got, err := r.Get(ctx, model.TokenScopeComponent, "")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.Token != "token-2" || !sec(got.ExpiresAt).Equal(exp2) {
		t.Fatalf("upsert 后令牌未更新: %+v", got)
	}

	// 不同 scope 互不影响
	if err := r.Put(ctx, model.TokenScopeAuthorizer, "", "token-3", exp1); err != nil {
		t.Fatalf("Put(authorizer) 失败: %v", err)
	}
	if n := countRows(t, db, &model.TokenCache{}); n != 2 {
		t.Fatalf("不同 scope 应各自成行，实际 %d", n)
	}

	if _, err := r.Get(ctx, model.TokenScopeAuthorizer, "wx-not-exist"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("无记录应返回 ErrNotFound，实际 %v", err)
	}
	if err := r.Delete(ctx, model.TokenScopeComponent, ""); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	if _, err := r.Get(ctx, model.TokenScopeComponent, ""); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("删除后 Get 应返回 ErrNotFound，实际 %v", err)
	}
	if err := r.Delete(ctx, model.TokenScopeComponent, ""); err != nil {
		t.Fatalf("重复 Delete 应幂等，实际 %v", err)
	}
}

// ---- 授权小程序 ----

func seedAuthorizer(t *testing.T, db *gorm.DB, a model.Authorizer) model.Authorizer {
	t.Helper()
	if a.Appid == "" {
		t.Fatal("appid 不能为空")
	}
	if a.AuthorizationStatus == "" {
		a.AuthorizationStatus = model.AuthStatusAuthorized
	}
	cp := a
	if err := repo.NewAuthorizers(db).Upsert(context.Background(), &cp); err != nil {
		t.Fatalf("写入授权小程序 %s 失败: %v", a.Appid, err)
	}
	return cp
}

func TestAuthorizersListFilterPagingAndSoftDelete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := repo.NewAuthorizers(db)

	a1 := seedAuthorizer(t, db, model.Authorizer{
		Appid: "wx-a1", NickName: "美食小程序", Alias: "food", GroupName: "组A",
		Tags: model.StringSlice{"core", "vip"}, FuncInfo: model.IntSlice{18, 30}, Enabled: true,
	})
	seedAuthorizer(t, db, model.Authorizer{
		Appid: "wx-a2", NickName: "商城小程序", GroupName: "组A",
		Tags: model.StringSlice{"vip"}, FuncInfo: model.IntSlice{30}, Enabled: true,
	})
	seedAuthorizer(t, db, model.Authorizer{
		Appid: "wx-a3", NickName: "工具小程序", GroupName: "组B",
		AuthorizationStatus: model.AuthStatusUnauthorized,
		Tags:                model.StringSlice{"core"}, FuncInfo: model.IntSlice{18}, Enabled: true,
	})
	seedAuthorizer(t, db, model.Authorizer{
		Appid: "wx-a4", NickName: "停用小程序的", GroupName: "组B",
		FuncInfo: model.IntSlice{18}, Enabled: false,
	})
	seedAuthorizer(t, db, model.Authorizer{
		Appid: "wx-a5", NickName: "无权限小程序", Enabled: true,
	})

	// 默认不含停用：a1 a2 a3 a5
	_, total, err := r.List(ctx, repo.AuthorizerFilter{})
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 4 {
		t.Fatalf("默认应过滤停用小程序，total=%d，期望 4", total)
	}

	// IncludeDisabled
	_, total, err = r.List(ctx, repo.AuthorizerFilter{IncludeDisabled: true})
	if err != nil {
		t.Fatalf("List(IncludeDisabled) 失败: %v", err)
	}
	if total != 5 {
		t.Fatalf("IncludeDisabled 时 total=%d，期望 5", total)
	}

	// 状态过滤
	items, total, err := r.List(ctx, repo.AuthorizerFilter{Status: string(model.AuthStatusAuthorized)})
	if err != nil {
		t.Fatalf("List(Status) 失败: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("已授权且启用的小程序应为 3，实际 total=%d len=%d", total, len(items))
	}

	// 关键字（昵称 / appid / alias）
	for _, kw := range []string{"美食", "wx-a2", "food"} {
		_, total, err = r.List(ctx, repo.AuthorizerFilter{Keyword: kw})
		if err != nil {
			t.Fatalf("List(Keyword=%s) 失败: %v", kw, err)
		}
		if total != 1 {
			t.Fatalf("关键字 %q 应命中 1 条，实际 %d", kw, total)
		}
	}

	// 分组
	if _, total, _ = r.List(ctx, repo.AuthorizerFilter{GroupName: "组A"}); total != 2 {
		t.Fatalf("组A 应有 2 条，实际 %d", total)
	}

	// 标签（JSON 数组包含）
	if _, total, _ = r.List(ctx, repo.AuthorizerFilter{Tag: "core"}); total != 2 {
		t.Fatalf("标签 core 应有 2 条（a1/a3），实际 %d", total)
	}

	// 开发权限集（18）
	if _, total, _ = r.List(ctx, repo.AuthorizerFilter{HasDevPermission: boolPtr(true)}); total != 2 {
		t.Fatalf("启用且有开发权限集应有 2 条（a1/a3），实际 %d", total)
	}
	if _, total, _ = r.List(ctx, repo.AuthorizerFilter{HasDevPermission: boolPtr(true), IncludeDisabled: true}); total != 3 {
		t.Fatalf("含停用且有开发权限集应有 3 条，实际 %d", total)
	}
	if _, total, _ = r.List(ctx, repo.AuthorizerFilter{HasDevPermission: boolPtr(false)}); total != 2 {
		t.Fatalf("无开发权限集应有 2 条（a2/a5），实际 %d", total)
	}

	// 分页：pageSize=2，total 不受分页影响
	page1, total, err := r.List(ctx, repo.AuthorizerFilter{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("List(分页) 失败: %v", err)
	}
	if total != 4 || len(page1) != 2 {
		t.Fatalf("分页结果不正确: total=%d len=%d", total, len(page1))
	}
	page2, _, err := r.List(ctx, repo.AuthorizerFilter{Page: 2, PageSize: 2})
	if err != nil {
		t.Fatalf("List(第 2 页) 失败: %v", err)
	}
	if len(page2) != 2 || page1[0].Appid == page2[0].Appid {
		t.Fatalf("第 2 页应返回另外 2 条: %+v / %+v", page1, page2)
	}

	// 统计与可用 appid
	byStatus, err := r.CountByStatus(ctx)
	if err != nil {
		t.Fatalf("CountByStatus 失败: %v", err)
	}
	if byStatus[model.AuthStatusAuthorized] != 4 || byStatus[model.AuthStatusUnauthorized] != 1 {
		t.Fatalf("状态统计不正确: %+v", byStatus)
	}
	active, err := r.ActiveAppids(ctx)
	if err != nil {
		t.Fatalf("ActiveAppids 失败: %v", err)
	}
	// 已授权 + 启用（a4 停用、a3 未授权被排除）
	if len(active) != 3 {
		t.Fatalf("可用 appid 应为 3 个，实际 %v", active)
	}

	// 软删：列表 / 明细 / 批量查询都不再返回，但物理数据仍在
	if err := r.SoftDelete(ctx, a1.Appid); err != nil {
		t.Fatalf("SoftDelete 失败: %v", err)
	}
	if _, err := r.Get(ctx, a1.Appid); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("软删后 Get 应返回 ErrNotFound，实际 %v", err)
	}
	if byID, err := r.GetByID(ctx, a1.ID); err != nil || byID.DeletedAt == nil {
		t.Fatalf("GetByID 应仍能读到软删记录: %v %+v", err, byID)
	}
	if _, total, _ = r.List(ctx, repo.AuthorizerFilter{IncludeDisabled: true}); total != 4 {
		t.Fatalf("软删后（含停用）应剩 4 条，实际 %d", total)
	}
	all, err := r.All(ctx)
	if err != nil {
		t.Fatalf("All 失败: %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("All 不应包含软删记录，实际 %d", len(all))
	}
	byAppids, err := r.ByAppids(ctx, []string{a1.Appid, "wx-a2"})
	if err != nil {
		t.Fatalf("ByAppids 失败: %v", err)
	}
	if len(byAppids) != 1 || byAppids[0].Appid != "wx-a2" {
		t.Fatalf("ByAppids 结果不正确: %+v", byAppids)
	}
	// 重复软删：记录仍在（未被过滤），因此仍返回 nil
	if err := r.SoftDelete(ctx, a1.Appid); err != nil {
		t.Fatalf("重复 SoftDelete 应幂等: %v", err)
	}
	if err := r.SoftDelete(ctx, "wx-none"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("删除不存在的记录应返回 ErrNotFound，实际 %v", err)
	}
}

func TestAuthorizersUpsertIdempotentAndLocalFields(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := repo.NewAuthorizers(db)

	if err := r.Upsert(ctx, &model.Authorizer{
		Appid: "wx-up", NickName: "旧昵称", AuthorizationStatus: model.AuthStatusAuthorized,
		FuncInfo: model.IntSlice{18}, Tags: model.StringSlice{"a"}, Enabled: true,
	}); err != nil {
		t.Fatalf("首次 Upsert 失败: %v", err)
	}
	if err := r.Upsert(ctx, &model.Authorizer{
		Appid: "wx-up", NickName: "新昵称", AuthorizationStatus: model.AuthStatusAuthorized,
		FuncInfo: model.IntSlice{18, 30}, Enabled: true,
	}); err != nil {
		t.Fatalf("二次 Upsert 失败: %v", err)
	}
	if n := countRows(t, db, &model.Authorizer{}); n != 1 {
		t.Fatalf("Upsert 应按 appid 幂等，实际 %d 行", n)
	}
	got, err := r.Get(ctx, "wx-up")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.NickName != "新昵称" || len(got.FuncInfo) != 2 {
		t.Fatalf("Upsert 未更新字段: %+v", got)
	}

	// refresh_token 密文由独立方法维护，不能被授权同步覆盖
	at := sec(time.Now())
	if err := r.UpdateRefreshToken(ctx, "wx-up", []byte{1, 2, 3}, at); err != nil {
		t.Fatalf("UpdateRefreshToken 失败: %v", err)
	}
	if err := r.Upsert(ctx, &model.Authorizer{
		Appid: "wx-up", NickName: "再同步", AuthorizationStatus: model.AuthStatusAuthorized,
		FuncInfo: model.IntSlice{18}, Enabled: true,
	}); err != nil {
		t.Fatalf("三次 Upsert 失败: %v", err)
	}
	got, err = r.Get(ctx, "wx-up")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if len(got.RefreshTokenCipher) != 3 {
		t.Fatalf("Upsert 不应清空 refresh_token 密文，实际 %q", got.RefreshTokenCipher)
	}
	if got.RefreshTokenUpdatedAt == nil || !got.RefreshTokenUpdatedAt.Equal(at) {
		t.Fatalf("refresh_token 更新时间不正确: %+v", got.RefreshTokenUpdatedAt)
	}

	// 空 appid 报错
	if err := r.Upsert(ctx, &model.Authorizer{}); err == nil {
		t.Fatal("空 appid 的 Upsert 应当报错")
	}

	// 幂等更新（值未变化）不算 ErrNotFound
	if err := r.UpdateFields(ctx, "wx-up", map[string]any{"nick_name": "再同步"}); err != nil {
		t.Fatalf("无变化的 UpdateFields 不应报错: %v", err)
	}
	if err := r.UpdateFields(ctx, "wx-up", map[string]any{"remark": "备注"}); err != nil {
		t.Fatalf("UpdateFields 失败: %v", err)
	}
	if err := r.UpdateFields(ctx, "wx-none", map[string]any{"remark": "x"}); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("更新不存在的记录应返回 ErrNotFound，实际 %v", err)
	}

	// enabled=false 必须真正落库：新增（Insert 分支）与更新（Update 分支）各验一次
	if err := r.Upsert(ctx, &model.Authorizer{
		Appid: "wx-disabled-new", NickName: "停用新增", AuthorizationStatus: model.AuthStatusAuthorized,
		FuncInfo: model.IntSlice{18}, Enabled: false,
	}); err != nil {
		t.Fatalf("Upsert(停用) 失败: %v", err)
	}
	disabled, err := r.Get(ctx, "wx-disabled-new")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if disabled.Enabled {
		t.Fatalf("新增记录不应被 default:true 改成启用: %+v", disabled)
	}
	if err := r.UpdateFields(ctx, "wx-up", map[string]any{"enabled": false}); err != nil {
		t.Fatalf("UpdateFields(enabled=false) 失败: %v", err)
	}
	if got, _ = r.Get(ctx, "wx-up"); got.Enabled {
		t.Fatalf("UpdateFields 未能写入 enabled=false: %+v", got)
	}

	// MarkUnauthorized
	if err := r.MarkUnauthorized(ctx, "wx-up", at); err != nil {
		t.Fatalf("MarkUnauthorized 失败: %v", err)
	}
	got, _ = r.Get(ctx, "wx-up")
	if got.AuthorizationStatus != model.AuthStatusUnauthorized {
		t.Fatalf("取消授权状态未写入: %+v", got)
	}
	if err := r.MarkUnauthorized(ctx, "wx-none", at); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("标记不存在的记录应返回 ErrNotFound，实际 %v", err)
	}

	// 提审前置快照
	if err := r.UpdatePreflightSnapshot(ctx, "wx-up", true,
		model.JSONMap{"requestdomain": "https://a.example.com"}, at); err != nil {
		t.Fatalf("UpdatePreflightSnapshot 失败: %v", err)
	}
	got, _ = r.Get(ctx, "wx-up")
	if got.PrivacyConfigured == nil || !*got.PrivacyConfigured {
		t.Fatalf("隐私配置快照未写入: %+v", got.PrivacyConfigured)
	}
	if got.DomainSnapshot["requestdomain"] != "https://a.example.com" {
		t.Fatalf("域名快照未写入: %+v", got.DomainSnapshot)
	}
	if got.LastPreflightAt == nil {
		t.Fatal("前置检查时间未写入")
	}
}

// ---- 草稿箱与模板库 ----

func TestDraftsReplaceAllSnapshot(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := repo.NewDrafts(db)

	if err := r.ReplaceAll(ctx, []model.CodeDraft{
		{DraftID: 1001, UserVersion: "1.0.0", UserDesc: "首发", Developer: "dev1", SourceMiniProgramAppid: "wx-a"},
		{DraftID: 1002, UserVersion: "1.0.1", Developer: "dev2"},
	}); err != nil {
		t.Fatalf("ReplaceAll 失败: %v", err)
	}
	items, err := r.List(ctx)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("草稿应为 2 条，实际 %d", len(items))
	}
	got, err := r.Get(ctx, 1001)
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.UserVersion != "1.0.0" {
		t.Fatalf("草稿字段不正确: %+v", got)
	}

	// 二次同步是快照替换：旧草稿必须消失（含重复 draft_id 去重）
	if err := r.ReplaceAll(ctx, []model.CodeDraft{
		{DraftID: 1003, UserVersion: "2.0.0"},
		{DraftID: 1003, UserVersion: "2.0.0-dup"},
	}); err != nil {
		t.Fatalf("二次 ReplaceAll 失败: %v", err)
	}
	items, _ = r.List(ctx)
	if len(items) != 1 || items[0].DraftID != 1003 {
		t.Fatalf("草稿箱应被整体替换为 1 条，实际 %+v", items)
	}
	if _, err := r.Get(ctx, 1001); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("旧草稿应已不存在，实际 %v", err)
	}

	// 清空
	if err := r.ReplaceAll(ctx, nil); err != nil {
		t.Fatalf("清空草稿箱失败: %v", err)
	}
	if n := countRows(t, db, &model.CodeDraft{}); n != 0 {
		t.Fatalf("草稿箱应为空，实际 %d", n)
	}
}

func TestTemplatesReplaceAllPreservesLocalFields(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := repo.NewTemplates(db)

	if err := r.ReplaceAll(ctx, []model.CodeTemplate{
		{TemplateID: 100, TemplateType: 0, UserVersion: "1.0.0", UserDesc: "普通模板A"},
		{TemplateID: 101, TemplateType: 0, UserVersion: "1.0.1", UserDesc: "普通模板B"},
		{TemplateID: 102, TemplateType: 1, UserVersion: "1.0.2", UserDesc: "标准模板"},
	}); err != nil {
		t.Fatalf("首次 ReplaceAll 失败: %v", err)
	}
	if n, err := r.Count(ctx); err != nil || n != 3 {
		t.Fatalf("模板数量应为 3，实际 %d (%v)", n, err)
	}
	if err := r.SetDefault(ctx, 101); err != nil {
		t.Fatalf("SetDefault 失败: %v", err)
	}
	if err := r.UpdateNote(ctx, 101, "生产用模板"); err != nil {
		t.Fatalf("UpdateNote 失败: %v", err)
	}

	// 二次同步（微信侧返回全量列表）：本地 is_default / note 必须保留，重复 id 去重，id=0 忽略
	if err := r.ReplaceAll(ctx, []model.CodeTemplate{
		{TemplateID: 100, TemplateType: 0, UserVersion: "2.0.0", UserDesc: "普通模板A2"},
		{TemplateID: 100, TemplateType: 0, UserVersion: "2.0.0-dup", UserDesc: "重复"},
		{TemplateID: 101, TemplateType: 0, UserVersion: "2.0.1", UserDesc: "普通模板B2"},
		{TemplateID: 102, TemplateType: 1, UserVersion: "2.0.2", UserDesc: "标准模板2"},
		{TemplateID: 103, TemplateType: 0, UserVersion: "2.0.3", UserDesc: "新增模板"},
		{TemplateID: 0, TemplateType: 0, UserVersion: "脏数据"},
	}); err != nil {
		t.Fatalf("二次 ReplaceAll 失败: %v", err)
	}
	if n, _ := r.Count(ctx); n != 4 {
		t.Fatalf("二次同步后模板应为 4 条，实际 %d", n)
	}
	tpl, err := r.Get(ctx, 101)
	if err != nil {
		t.Fatalf("Get(101) 失败: %v", err)
	}
	if !tpl.IsDefault || tpl.Note != "生产用模板" {
		t.Fatalf("本地字段 is_default/note 未保留: %+v", tpl)
	}
	if tpl.UserVersion != "2.0.1" {
		t.Fatalf("微信侧字段应被更新: %+v", tpl)
	}
	tpl100, _ := r.Get(ctx, 100)
	if tpl100.IsDefault || tpl100.UserVersion != "2.0.0" {
		t.Fatalf("模板 100 状态不正确: %+v", tpl100)
	}

	// 默认模板唯一
	if err := r.SetDefault(ctx, 103); err != nil {
		t.Fatalf("SetDefault(103) 失败: %v", err)
	}
	var defaults []model.CodeTemplate
	if err := db.Where("is_default = ?", true).Find(&defaults).Error; err != nil {
		t.Fatalf("查询默认模板失败: %v", err)
	}
	if len(defaults) != 1 || defaults[0].TemplateID != 103 {
		t.Fatalf("默认模板应全局唯一且为 103，实际 %+v", defaults)
	}
	if err := r.SetDefault(ctx, 999); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("设置不存在的模板应返回 ErrNotFound，实际 %v", err)
	}

	// 列表过滤
	all, err := r.List(ctx, nil)
	if err != nil || len(all) != 4 {
		t.Fatalf("List(nil) 应为 4 条，实际 %d (%v)", len(all), err)
	}
	normal := 0
	normalList, err := r.List(ctx, &normal)
	if err != nil {
		t.Fatalf("List(0) 失败: %v", err)
	}
	for _, it := range normalList {
		if it.TemplateType != 0 {
			t.Fatalf("模板类型过滤失效: %+v", it)
		}
	}

	// 删除
	if err := r.Delete(ctx, 103); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	if err := r.Delete(ctx, 103); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("重复 Delete 应返回 ErrNotFound，实际 %v", err)
	}
	if n, _ := r.Count(ctx); n != 3 {
		t.Fatalf("删除后模板应为 3 条，实际 %d", n)
	}
	if _, err := r.Get(ctx, 999); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Get 不存在的模板应返回 ErrNotFound，实际 %v", err)
	}
}

// ---- 提审配置 ----

func TestAuditProfilesGetDefault(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := repo.NewAuditProfiles(db)

	if _, err := r.GetDefault(ctx); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("无默认配置应返回 ErrNotFound，实际 %v", err)
	}
	def := &model.AuditProfile{Name: "默认提审配置", IsDefault: true, ItemList: model.JSONMap{"first_class": "工具"}}
	if err := db.Create(def).Error; err != nil {
		t.Fatalf("写入提审配置失败: %v", err)
	}
	if err := db.Create(&model.AuditProfile{Name: "备用配置"}).Error; err != nil {
		t.Fatalf("写入提审配置失败: %v", err)
	}

	got, err := r.GetDefault(ctx)
	if err != nil {
		t.Fatalf("GetDefault 失败: %v", err)
	}
	if got.ID != def.ID || got.ItemList["first_class"] != "工具" {
		t.Fatalf("默认配置内容不正确: %+v", got)
	}
	list, err := r.List(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("List 应为 2 条，实际 %d (%v)", len(list), err)
	}
	if !list[0].IsDefault {
		t.Fatalf("默认配置应排在最前: %+v", list)
	}
	if _, err := r.Get(ctx, def.ID); err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if _, err := r.Get(ctx, 99999); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Get 不存在的配置应返回 ErrNotFound，实际 %v", err)
	}
}

// ---- 作业与子项 ----

func TestJobsCreateTransactional(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	jobs := repo.NewJobs(db)

	job := &model.BatchJob{ID: "job-ok", Type: model.JobTypePipeline, Status: model.JobStatusPending, CreatedBy: "tester"}
	if err := jobs.Create(ctx, job, []model.BatchJobItem{
		{Step: model.StepCommit, Appid: "wx-a"},
		{Step: model.StepCommit, Appid: "wx-b"},
	}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if job.Total != 2 {
		t.Fatalf("Create 应以子项数量补齐 Total，实际 %d", job.Total)
	}
	if n := countRows(t, db, &model.BatchJob{}); n != 1 {
		t.Fatalf("作业数应为 1，实际 %d", n)
	}
	if n := countRows(t, db, &model.BatchJobItem{}); n != 2 {
		t.Fatalf("子项数应为 2，实际 %d", n)
	}
	got, err := jobs.Get(ctx, "job-ok")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.Type != model.JobTypePipeline || got.Status != model.JobStatusPending {
		t.Fatalf("作业字段不正确: %+v", got)
	}

	// 子项违反唯一索引 (job_id, step, appid) → 整单回滚，不能留下「有作业无子项」的僵尸作业
	bad := &model.BatchJob{ID: "job-rollback", Type: model.JobTypeCommit, Status: model.JobStatusPending}
	err = jobs.Create(ctx, bad, []model.BatchJobItem{
		{Step: model.StepCommit, Appid: "wx-x"},
		{Step: model.StepCommit, Appid: "wx-x"},
	})
	if err == nil {
		t.Fatal("重复子项应触发唯一键冲突并回滚")
	}
	if _, err := jobs.Get(ctx, "job-rollback"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("回滚后作业不应存在，实际 %v", err)
	}
	if n := countRows(t, db, &model.BatchJob{}); n != 1 {
		t.Fatalf("回滚后作业数仍应为 1，实际 %d", n)
	}
	if n := countRows(t, db, &model.BatchJobItem{}); n != 2 {
		t.Fatalf("回滚后子项数仍应为 2，实际 %d", n)
	}
	if err := jobs.Create(ctx, nil, nil); err == nil {
		t.Fatal("空作业应报错")
	}
	if _, err := jobs.Get(ctx, "job-none"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Get 不存在的作业应返回 ErrNotFound，实际 %v", err)
	}
}

func TestJobItemsClaimNextSequential(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	jobs := repo.NewJobs(db)
	items := repo.NewJobItems(db)

	job := &model.BatchJob{ID: "job-claim", Type: model.JobTypePipeline, Status: model.JobStatusRunning}
	if err := jobs.Create(ctx, job, []model.BatchJobItem{
		{Step: model.StepCommit, Appid: "wx-c1"},
		{Step: model.StepCommit, Appid: "wx-c2"},
		{Step: model.StepCommit, Appid: "wx-c3"},
	}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	first, err := items.ClaimNext(ctx, "job-claim")
	if err != nil {
		t.Fatalf("首次 ClaimNext 失败: %v", err)
	}
	if first.Appid != "wx-c1" {
		t.Fatalf("应按 id 升序先领最早的子项，实际 %s", first.Appid)
	}
	if first.Status != model.ItemStatusRunning || first.StartedAt == nil {
		t.Fatalf("领取后应为 running 且写入 started_at: %+v", first)
	}
	second, err := items.ClaimNext(ctx, "job-claim")
	if err != nil {
		t.Fatalf("二次 ClaimNext 失败: %v", err)
	}
	if second.ID == first.ID || second.Appid != "wx-c2" {
		t.Fatalf("二次领取应拿到不同的子项: %+v / %+v", first, second)
	}
	if _, err := items.ClaimNext(ctx, "job-claim"); err != nil {
		t.Fatalf("三次 ClaimNext 失败: %v", err)
	}
	if _, err := items.ClaimNext(ctx, "job-claim"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("无可用子项应返回 ErrNotFound，实际 %v", err)
	}
	if _, err := items.ClaimNext(ctx, ""); err == nil {
		t.Fatal("空作业 ID 应报错")
	}

	// 复位残留 running → pending，并可再次领取
	n, err := items.ResetRunningToPending(ctx)
	if err != nil {
		t.Fatalf("ResetRunningToPending 失败: %v", err)
	}
	if n != 3 {
		t.Fatalf("应复位 3 条，实际 %d", n)
	}
	counts, err := items.CountByStatus(ctx, "job-claim")
	if err != nil {
		t.Fatalf("CountByStatus 失败: %v", err)
	}
	if counts[model.ItemStatusPending] != 3 {
		t.Fatalf("复位后应全部为 pending，实际 %+v", counts)
	}
	if _, err := items.ClaimNext(ctx, "job-claim"); err != nil {
		t.Fatalf("复位后应能继续领取: %v", err)
	}
}

// TestJobItemsClaimNextConcurrent 验证多 worker 并发领任务不会重复领取。
func TestJobItemsClaimNextConcurrent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	jobs := repo.NewJobs(db)
	items := repo.NewJobItems(db)

	const total = 24
	const workers = 6
	rows := make([]model.BatchJobItem, 0, total)
	for i := 0; i < total; i++ {
		rows = append(rows, model.BatchJobItem{Step: model.StepCommit, Appid: fmt.Sprintf("wx-conc-%02d", i)})
	}
	job := &model.BatchJob{ID: "job-conc", Type: model.JobTypePipeline, Status: model.JobStatusRunning}
	if err := jobs.Create(ctx, job, rows); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	var (
		mu       sync.Mutex
		claimed  = make(map[uint]int, total)
		failures []error
		wg       sync.WaitGroup
	)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				it, err := items.ClaimNext(ctx, "job-conc")
				if errors.Is(err, repo.ErrNotFound) {
					return
				}
				if err != nil {
					mu.Lock()
					failures = append(failures, err)
					mu.Unlock()
					return
				}
				mu.Lock()
				claimed[it.ID]++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(failures) > 0 {
		t.Fatalf("并发领取出错: %v", failures)
	}
	if len(claimed) != total {
		t.Fatalf("应恰好领取 %d 个子项，实际 %d", total, len(claimed))
	}
	for id, n := range claimed {
		if n != 1 {
			t.Fatalf("子项 %d 被重复领取 %d 次", id, n)
		}
	}

	counts, err := items.CountByStatus(ctx, "job-conc")
	if err != nil {
		t.Fatalf("CountByStatus 失败: %v", err)
	}
	if counts[model.ItemStatusRunning] != total || counts[model.ItemStatusPending] != 0 {
		t.Fatalf("领取后状态统计不正确: %+v", counts)
	}
	if _, err := items.ClaimNext(ctx, "job-conc"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("全部领取完后应返回 ErrNotFound，实际 %v", err)
	}
}

func TestJobItemsRetryFailedAndCancelPending(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	jobs := repo.NewJobs(db)
	items := repo.NewJobItems(db)

	job := &model.BatchJob{ID: "job-retry", Type: model.JobTypePipeline, Status: model.JobStatusPartialFailed}
	if err := jobs.Create(ctx, job, []model.BatchJobItem{
		{Step: model.StepCommit, Appid: "wx-failed", Status: model.ItemStatusFailed, Attempt: 2, Errcode: -1},
		{Step: model.StepCommit, Appid: "wx-ok", Status: model.ItemStatusSucceeded, Attempt: 1},
		{Step: model.StepCommit, Appid: "wx-pending", Status: model.ItemStatusPending},
		{Step: model.StepCommit, Appid: "wx-waiting", Status: model.ItemStatusWaiting},
		{Step: model.StepCommit, Appid: "wx-canceled", Status: model.ItemStatusCanceled},
	}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	n, err := items.RetryFailed(ctx, "job-retry")
	if err != nil {
		t.Fatalf("RetryFailed 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("只应重置 1 条 failed，实际 %d", n)
	}
	all, err := items.AllByJob(ctx, "job-retry")
	if err != nil {
		t.Fatalf("AllByJob 失败: %v", err)
	}
	for _, it := range all {
		switch it.Appid {
		case "wx-failed":
			if it.Status != model.ItemStatusPending {
				t.Fatalf("failed 子项应被重置为 pending: %+v", it)
			}
			if it.Attempt != 2 {
				t.Fatalf("RetryFailed 应保留 attempt，实际 %d", it.Attempt)
			}
			if it.Errcode != -1 {
				t.Fatalf("重试不应清空历史返回码，实际 %d", it.Errcode)
			}
		case "wx-ok":
			if it.Status != model.ItemStatusSucceeded {
				t.Fatalf("succeeded 子项不应被改动: %+v", it)
			}
		}
	}

	n, err = items.CancelPending(ctx, "job-retry")
	if err != nil {
		t.Fatalf("CancelPending 失败: %v", err)
	}
	// wx-pending、wx-waiting、以及刚被重置为 pending 的 wx-failed
	if n != 3 {
		t.Fatalf("应取消 3 条未执行子项，实际 %d", n)
	}
	counts, err := items.CountByStatus(ctx, "job-retry")
	if err != nil {
		t.Fatalf("CountByStatus 失败: %v", err)
	}
	if counts[model.ItemStatusCanceled] != 4 || counts[model.ItemStatusSucceeded] != 1 ||
		counts[model.ItemStatusPending] != 0 || counts[model.ItemStatusWaiting] != 0 {
		t.Fatalf("取消后状态统计不正确: %+v", counts)
	}

	// 不存在失败的子项 → 0
	if n, err := items.RetryFailed(ctx, "job-retry"); err != nil || n != 0 {
		t.Fatalf("无 failed 子项时应返回 0，实际 %d (%v)", n, err)
	}
}

func TestJobsCountersListAndActiveCheck(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	jobs := repo.NewJobs(db)
	items := repo.NewJobItems(db)

	running := &model.BatchJob{ID: "job-run", Type: model.JobTypePipeline, Status: model.JobStatusRunning}
	if err := jobs.Create(ctx, running, []model.BatchJobItem{
		{Step: model.StepCommit, Appid: "wx-ok", Status: model.ItemStatusSucceeded},
		{Step: model.StepSubmitAudit, Appid: "wx-bad", Status: model.ItemStatusFailed},
		{Step: model.StepRelease, Appid: "wx-skip", Status: model.ItemStatusSkipped},
		{Step: model.StepPrivacyCheck, Appid: "wx-wait", Status: model.ItemStatusPending},
	}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if err := jobs.Create(ctx, &model.BatchJob{ID: "job-pend", Type: model.JobTypeCommit, Status: model.JobStatusPending}, nil); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}
	if err := jobs.Create(ctx, &model.BatchJob{ID: "job-done", Type: model.JobTypeRelease, Status: model.JobStatusSucceeded}, nil); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	if err := jobs.UpdateCounters(ctx, "job-run"); err != nil {
		t.Fatalf("UpdateCounters 失败: %v", err)
	}
	got, err := jobs.Get(ctx, "job-run")
	if err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if got.Total != 4 || got.Succeeded != 1 || got.Failed != 1 || got.Skipped != 1 {
		t.Fatalf("作业计数不正确: %+v", got)
	}
	// 计数无变化时也应成功（幂等）
	if err := jobs.UpdateCounters(ctx, "job-run"); err != nil {
		t.Fatalf("重复 UpdateCounters 应成功: %v", err)
	}
	if err := jobs.UpdateCounters(ctx, "job-none"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("统计不存在的作业应返回 ErrNotFound，实际 %v", err)
	}

	// 在途作业
	actives, err := jobs.Running(ctx)
	if err != nil {
		t.Fatalf("Running 失败: %v", err)
	}
	if len(actives) != 2 {
		t.Fatalf("在途作业应为 2 条（running+pending），实际 %d", len(actives))
	}
	byStatus, err := jobs.CountByStatus(ctx)
	if err != nil {
		t.Fatalf("CountByStatus 失败: %v", err)
	}
	if byStatus[model.JobStatusRunning] != 1 || byStatus[model.JobStatusSucceeded] != 1 ||
		byStatus[model.JobStatusPending] != 1 {
		t.Fatalf("作业状态统计不正确: %+v", byStatus)
	}

	// 列表过滤 + 分页
	list, total, err := jobs.List(ctx, model.JobTypePipeline, "", 1, 10)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].ID != "job-run" {
		t.Fatalf("List(type) 结果不正确: total=%d %+v", total, list)
	}
	list, total, err = jobs.List(ctx, "", model.JobStatusPending, 1, 1)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("List(status,pageSize=1) 结果不正确: total=%d %+v", total, list)
	}
	recent, err := jobs.Recent(ctx, 2)
	if err != nil {
		t.Fatalf("Recent 失败: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("Recent 应返回 2 条，实际 %d", len(recent))
	}
	if err := jobs.UpdateFields(ctx, "job-run", map[string]any{"note": "人工暂停", "status": model.JobStatusPaused}); err != nil {
		t.Fatalf("UpdateFields 失败: %v", err)
	}
	if err := jobs.UpdateFields(ctx, "job-none", map[string]any{"note": "x"}); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("更新不存在的作业应返回 ErrNotFound，实际 %v", err)
	}

	// 在途冲突检查
	if ok, err := jobs.HasActiveItem(ctx, "wx-wait", []model.JobStep{model.StepPrivacyCheck}); err != nil || !ok {
		t.Fatalf("pending 子项应判定为在途: %v %v", ok, err)
	}
	if ok, _ := jobs.HasActiveItem(ctx, "wx-ok", []model.JobStep{model.StepCommit}); ok {
		t.Fatal("succeeded 子项不应判定为在途")
	}
	if ok, _ := jobs.HasActiveItem(ctx, "wx-wait", []model.JobStep{model.StepCommit}); ok {
		t.Fatal("步骤不匹配不应判定为在途")
	}
	if ok, _ := jobs.HasActiveItem(ctx, "", []model.JobStep{model.StepCommit}); ok {
		t.Fatal("空 appid 不应判定为在途")
	}

	// 子项分页与按小程序统计
	page1, total, err := items.ListByJob(ctx, "job-run", model.ItemStatusPending, "", 1, 1)
	if err != nil {
		t.Fatalf("ListByJob 失败: %v", err)
	}
	if total != 1 || len(page1) != 1 || page1[0].Appid != "wx-wait" {
		t.Fatalf("ListByJob 结果不正确: total=%d %+v", total, page1)
	}
	byApp, err := items.CountsByApp(ctx, "wx-ok")
	if err != nil {
		t.Fatalf("CountsByApp 失败: %v", err)
	}
	if byApp[model.ItemStatusSucceeded] != 1 {
		t.Fatalf("CountsByApp 结果不正确: %+v", byApp)
	}
	allJob, err := items.AllByJob(ctx, "job-run")
	if err != nil || len(allJob) != 4 {
		t.Fatalf("AllByJob 应为 4 条，实际 %d (%v)", len(allJob), err)
	}
	if _, err := items.Get(ctx, allJob[0].ID); err != nil {
		t.Fatalf("Get 失败: %v", err)
	}
	if _, err := items.Get(ctx, 99999999); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("Get 不存在的子项应返回 ErrNotFound，实际 %v", err)
	}
	if err := items.UpdateFields(ctx, allJob[3].ID, map[string]any{"errmsg": "人工标记"}); err != nil {
		t.Fatalf("UpdateFields 失败: %v", err)
	}
	if err := items.UpdateFields(ctx, 99999999, map[string]any{"errmsg": "x"}); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("更新不存在的子项应返回 ErrNotFound，实际 %v", err)
	}
}

// ---- 审核 / 发布 / 撤回额度 ----

func TestAuditsUpsertAndQueries(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := repo.NewAudits(db)

	rec := &model.AuditRecord{
		Appid: "wx-audit-1", AuditID: 5001, UserVersion: "1.0.0", UserDesc: "首次提审",
		Status: 2, Source: model.AuditSourceAPI, ScreenshotMediaIDs: model.StringSlice{"media-1"},
	}
	if err := r.Upsert(ctx, rec); err != nil {
		t.Fatalf("首次 Upsert 失败: %v", err)
	}
	firstID := rec.ID
	if firstID == 0 {
		t.Fatal("Upsert 应回填主键")
	}

	// 同一 (appid, audit_id) 再次上报（状态变为审核成功）→ 更新而不是新增
	again := &model.AuditRecord{
		Appid: "wx-audit-1", AuditID: 5001, UserVersion: "1.0.0", UserDesc: "首次提审",
		Status: 0, Source: model.AuditSourceEvent, Reason: "审核通过",
	}
	if err := r.Upsert(ctx, again); err != nil {
		t.Fatalf("二次 Upsert 失败: %v", err)
	}
	if n := countRows(t, db, &model.AuditRecord{}); n != 1 {
		t.Fatalf("Upsert 应按 (appid,audit_id) 幂等，实际 %d 行", n)
	}
	if again.ID != firstID {
		t.Fatalf("Upsert 应更新同一行，实际 %d != %d", again.ID, firstID)
	}

	// 审核中的小程序（用于结果轮询）
	if err := r.Upsert(ctx, &model.AuditRecord{
		Appid: "wx-audit-2", AuditID: 5002, UserVersion: "2.0.0", Status: 2, Source: model.AuditSourcePoll,
	}); err != nil {
		t.Fatalf("Upsert 失败: %v", err)
	}
	pending, err := r.PendingAppids(ctx)
	if err != nil {
		t.Fatalf("PendingAppids 失败: %v", err)
	}
	if len(pending) != 1 || pending[0] != "wx-audit-2" {
		t.Fatalf("审核中的 appid 应为 [wx-audit-2]，实际 %v", pending)
	}

	// 列表 / 状态筛选
	_, total, err := r.List(ctx, "wx-audit-1", nil, 1, 10)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 1 {
		t.Fatalf("wx-audit-1 应有 1 条，实际 %d", total)
	}
	list, total, err := r.List(ctx, "", intPtr(2), 1, 10)
	if err != nil {
		t.Fatalf("List(status=2) 失败: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].AuditID != 5002 {
		t.Fatalf("审核中列表不正确: total=%d %+v", total, list)
	}

	latest, err := r.LatestByApp(ctx, "wx-audit-1")
	if err != nil {
		t.Fatalf("LatestByApp 失败: %v", err)
	}
	if latest.AuditID != 5001 || latest.Status != 0 || latest.Reason != "审核通过" {
		t.Fatalf("最新审核记录不正确: %+v", latest)
	}
	if _, err := r.LatestByApp(ctx, "wx-none"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("无记录应返回 ErrNotFound，实际 %v", err)
	}
	history, err := r.HistoryByApp(ctx, "wx-audit-2", 10)
	if err != nil || len(history) != 1 {
		t.Fatalf("HistoryByApp 应返回 1 条，实际 %d (%v)", len(history), err)
	}
}

func TestReleasesAndUndoQuota(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	releases := repo.NewReleases(db)
	gray := 20
	now := time.Now()
	for _, rec := range []*model.ReleaseRecord{
		{Appid: "wx-rel", Action: model.ReleaseActionRelease, UserVersion: "1.0.0", ReleaseTime: &now},
		{Appid: "wx-rel", Action: model.ReleaseActionGrayRelease, UserVersion: "1.0.1", GrayPercentage: &gray},
		{Appid: "wx-other", Action: model.ReleaseActionRelease, UserVersion: "9.9.9"},
	} {
		if err := releases.Create(ctx, rec); err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
	}
	list, total, err := releases.List(ctx, "wx-rel", 1, 10)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Fatalf("发布记录应为 2 条，实际 total=%d len=%d", total, len(list))
	}
	if list[0].GrayPercentage == nil || *list[0].GrayPercentage != 20 {
		t.Fatalf("灰度比例未写入: %+v", list[0])
	}
	if _, total, _ = releases.List(ctx, "", 1, 10); total != 3 {
		t.Fatalf("全部发布记录应为 3 条，实际 %d", total)
	}
	if err := releases.Create(ctx, nil); err == nil {
		t.Fatal("空发布记录应报错")
	}

	// 撤回额度：当天 2 次、昨天 1 次、上月 1 次
	quota := repo.NewUndoQuota(db)
	today := time.Now()
	for _, at := range []time.Time{today, today.Add(-2 * time.Minute), today.AddDate(0, 0, -1), prevMonth(today)} {
		if err := quota.Record(ctx, "wx-quota", at); err != nil {
			t.Fatalf("Record 失败: %v", err)
		}
	}
	todayCount, err := quota.CountToday(ctx, "wx-quota")
	if err != nil {
		t.Fatalf("CountToday 失败: %v", err)
	}
	if todayCount != 2 {
		t.Fatalf("当天用量应为 2，实际 %d", todayCount)
	}
	monthCount, err := quota.CountMonth(ctx, "wx-quota")
	if err != nil {
		t.Fatalf("CountMonth 失败: %v", err)
	}
	// 昨天可能跨月（例如今天是 1 号），因此按实际月份键推断期望值
	wantMonth := int64(2)
	if monthKeyOf(today.AddDate(0, 0, -1)) == monthKeyOf(today) {
		wantMonth = 3
	}
	if monthCount != wantMonth {
		t.Fatalf("当月用量应为 %d，实际 %d", wantMonth, monthCount)
	}
	if other, _ := quota.CountToday(ctx, "wx-other"); other != 0 {
		t.Fatalf("其它小程序当天用量应为 0，实际 %d", other)
	}
}

// ---- 日志 ----

func TestApiCallLogsFilterAndPrune(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := repo.NewApiCallLogs(db)

	now := time.Now()
	old := sec(now.AddDate(0, 0, -30))
	rows := []*model.ApiCallLog{
		{Appid: "wx-log1", Endpoint: "/wxa/commit", Method: "POST", Errcode: 0, OK: true, CreatedAt: now},
		{Appid: "wx-log1", Endpoint: "/wxa/submit_audit", Method: "POST", Errcode: -1, ErrorClass: model.ClassRetryable, CreatedAt: now},
		{Appid: "wx-log2", Endpoint: "/wxa/commit", Method: "POST", Errcode: 45011, JobID: "job-log", ErrorClass: model.ClassRateLimited, CreatedAt: now},
		{Appid: "wx-log1", Endpoint: "/wxa/commit", Method: "POST", Errcode: 0, OK: true, CreatedAt: old},
	}
	for _, row := range rows {
		if err := r.Create(ctx, row); err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
	}
	if err := r.Create(ctx, nil); err == nil {
		t.Fatal("空日志应报错")
	}

	cases := []struct {
		name  string
		f     repo.ApiCallLogFilter
		total int64
	}{
		{"按 appid", repo.ApiCallLogFilter{Appid: "wx-log1"}, 3},
		{"按 appid+endpoint", repo.ApiCallLogFilter{Appid: "wx-log1", Endpoint: "/wxa/commit"}, 2},
		{"按 job", repo.ApiCallLogFilter{JobID: "job-log"}, 1},
		{"按返回码", repo.ApiCallLogFilter{Errcode: intPtr(45011)}, 1},
		{"按返回码 0", repo.ApiCallLogFilter{Errcode: intPtr(0)}, 2},
		{"无条件", repo.ApiCallLogFilter{}, 4},
	}
	for _, c := range cases {
		items, total, err := r.List(ctx, c.f)
		if err != nil {
			t.Fatalf("%s: List 失败: %v", c.name, err)
		}
		if total != c.total {
			t.Fatalf("%s: total=%d，期望 %d", c.name, total, c.total)
		}
		if int64(len(items)) != c.total {
			t.Fatalf("%s: 返回条数 %d 不正确", c.name, len(items))
		}
	}

	// 分页
	page1, total, err := r.List(ctx, repo.ApiCallLogFilter{PageSize: 2})
	if err != nil {
		t.Fatalf("List 分页失败: %v", err)
	}
	if total != 4 || len(page1) != 2 {
		t.Fatalf("分页结果不正确: total=%d len=%d", total, len(page1))
	}

	// 清理 7 天前
	n, err := r.PruneBefore(ctx, now.AddDate(0, 0, -7))
	if err != nil {
		t.Fatalf("PruneBefore 失败: %v", err)
	}
	if n != 1 {
		t.Fatalf("应清理 1 条历史日志，实际 %d", n)
	}
	if left := countRows(t, db, &model.ApiCallLog{}); left != 3 {
		t.Fatalf("清理后应剩 3 条，实际 %d", left)
	}
	if again, err := r.PruneBefore(ctx, now.AddDate(0, 0, -7)); err != nil || again != 0 {
		t.Fatalf("重复清理应为 0 条，实际 %d (%v)", again, err)
	}
}

func TestCallbackEventsCreateIfAbsent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	r := repo.NewCallbackEvents(db)

	ev := &model.CallbackEvent{
		Kind: model.CallbackKindComponent, Appid: "wx-cb", InfoType: "component_verify_ticket",
		Event: "verify_ticket", Encrypted: "cipher", Decrypted: "ticket", DedupeKey: "dedupe-1",
		SignatureOK: true,
	}
	created, isNew, err := r.CreateIfAbsent(ctx, ev)
	if err != nil {
		t.Fatalf("首次 CreateIfAbsent 失败: %v", err)
	}
	if !isNew || created.ID == 0 {
		t.Fatalf("首次写入应返回 isNew=true 且回填主键: %+v isNew=%v", created, isNew)
	}
	if created.ReceivedAt.IsZero() {
		t.Fatal("ReceivedAt 应自动补当前时间")
	}

	dup := &model.CallbackEvent{
		Kind: model.CallbackKindComponent, Appid: "wx-cb", InfoType: "component_verify_ticket",
		Event: "verify_ticket", DedupeKey: "dedupe-1",
	}
	existing, isNew, err := r.CreateIfAbsent(ctx, dup)
	if err != nil {
		t.Fatalf("重复 CreateIfAbsent 失败: %v", err)
	}
	if isNew {
		t.Fatal("重复推送应返回 isNew=false")
	}
	if existing == nil || existing.ID != created.ID || existing.Decrypted != "ticket" {
		t.Fatalf("重复推送应返回库里已有的记录: %+v", existing)
	}
	if n := countRows(t, db, &model.CallbackEvent{}); n != 1 {
		t.Fatalf("幂等写入应只留 1 行，实际 %d", n)
	}

	// 缺 dedupe_key 直接报错（否则无法保证幂等）
	if _, _, err := r.CreateIfAbsent(ctx, &model.CallbackEvent{Kind: model.CallbackKindMessage, Appid: "wx-cb"}); err == nil {
		t.Fatal("缺 dedupe_key 应报错")
	}
	if _, _, err := r.CreateIfAbsent(ctx, nil); err == nil {
		t.Fatal("空事件应报错")
	}

	// 另一条消息类事件
	if _, _, err := r.CreateIfAbsent(ctx, &model.CallbackEvent{
		Kind: model.CallbackKindMessage, Appid: "wx-cb", InfoType: "wxa_media_check",
		Event: "wxa_media_check", DedupeKey: "dedupe-2", ReceivedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateIfAbsent 失败: %v", err)
	}

	list, total, err := r.List(ctx, model.CallbackKindMessage, "wx-cb", "wxa_media_check", 1, 10)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].DedupeKey != "dedupe-2" {
		t.Fatalf("List 结果不正确: total=%d %+v", total, list)
	}
	if _, total, _ := r.List(ctx, "", "", "", 1, 10); total != 2 {
		t.Fatalf("全部事件应为 2 条，实际 %d", total)
	}

	if err := r.MarkProcessed(ctx, created.ID, true, "票据已更新"); err != nil {
		t.Fatalf("MarkProcessed 失败: %v", err)
	}
	// 重复标记同样内容应幂等
	if err := r.MarkProcessed(ctx, created.ID, true, "票据已更新"); err != nil {
		t.Fatalf("重复 MarkProcessed 应成功: %v", err)
	}
	processed, _, _ := r.List(ctx, model.CallbackKindComponent, "wx-cb", "", 1, 10)
	if len(processed) != 1 || !processed[0].Processed || processed[0].ProcessNote != "票据已更新" {
		t.Fatalf("处理结果未写入: %+v", processed)
	}
	if err := r.MarkProcessed(ctx, 99999999, true, "x"); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("标记不存在的事件应返回 ErrNotFound，实际 %v", err)
	}

	// 清理历史事件
	future := time.Now().Add(time.Hour)
	n, err := r.PruneBefore(ctx, future)
	if err != nil {
		t.Fatalf("PruneBefore 失败: %v", err)
	}
	if n != 2 {
		t.Fatalf("应清理 2 条事件，实际 %d", n)
	}
	if left := countRows(t, db, &model.CallbackEvent{}); left != 0 {
		t.Fatalf("清理后应为 0 条，实际 %d", left)
	}
}

func TestOperationLogsAndSettings(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	logs := repo.NewOperationLogs(db)
	for _, l := range []*model.OperationLog{
		{Actor: "admin", Action: "authorize", TargetType: "authorizer", TargetID: "wx-1", Detail: model.JSONMap{"ok": true}},
		{Actor: "admin", Action: "release", TargetType: "job", TargetID: "job-1"},
		{Actor: "admin", Action: "release", TargetType: "job", TargetID: "job-2"},
	} {
		if err := logs.Create(ctx, l); err != nil {
			t.Fatalf("Create 失败: %v", err)
		}
	}
	if err := logs.Create(ctx, nil); err == nil {
		t.Fatal("空操作日志应报错")
	}
	list, total, err := logs.List(ctx, "release", 1, 10)
	if err != nil {
		t.Fatalf("List 失败: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Fatalf("release 操作应为 2 条，实际 total=%d len=%d", total, len(list))
	}
	if list[0].Detail != nil && list[0].TargetID == "" {
		t.Fatalf("操作日志内容不正确: %+v", list[0])
	}
	if _, total, _ := logs.List(ctx, "", 1, 10); total != 3 {
		t.Fatalf("全部操作日志应为 3 条，实际 %d", total)
	}

	settings := repo.NewSettings(db)
	if _, err := settings.Get(ctx, model.SettingJobConcurrency); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("不存在的参数应返回 ErrNotFound，实际 %v", err)
	}
	if err := settings.Put(ctx, model.SettingJobConcurrency, "8"); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}
	if got, err := settings.Get(ctx, model.SettingJobConcurrency); err != nil || got != "8" {
		t.Fatalf("Get 失败: %q %v", got, err)
	}
	if err := settings.Put(ctx, model.SettingJobConcurrency, "4"); err != nil {
		t.Fatalf("重复 Put 失败: %v", err)
	}
	if n := countRows(t, db, &model.RuntimeSetting{}); n != 1 {
		t.Fatalf("同键 Put 应 upsert 为 1 行，实际 %d", n)
	}
	if got, _ := settings.Get(ctx, model.SettingJobConcurrency); got != "4" {
		t.Fatalf("upsert 后取值应为 4，实际 %q", got)
	}
	if err := settings.Put(ctx, model.SettingWxMaxQps, "20"); err != nil {
		t.Fatalf("Put 失败: %v", err)
	}
	if err := settings.Put(ctx, "", "x"); err == nil {
		t.Fatal("空键应报错")
	}
	all, err := settings.All(ctx)
	if err != nil {
		t.Fatalf("All 失败: %v", err)
	}
	if len(all) != 2 || all[model.SettingJobConcurrency] != "4" || all[model.SettingWxMaxQps] != "20" {
		t.Fatalf("All 结果不正确: %+v", all)
	}
}

// TestJobItemsClaimNextPendingAndPromoteWaiting 覆盖按步骤领取与 waiting 到期提升。
func TestJobItemsClaimNextPendingAndPromoteWaiting(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	jobs := repo.NewJobs(db)
	items := repo.NewJobItems(db)

	future := sec(time.Now().Add(time.Hour))
	past := sec(time.Now().Add(-time.Minute))
	job := &model.BatchJob{ID: "job-step", Type: model.JobTypePipeline, Status: model.JobStatusRunning}
	if err := jobs.Create(ctx, job, []model.BatchJobItem{
		{Step: model.StepCommit, Appid: "wx-s1"},
		{Step: model.StepSubmitAudit, Appid: "wx-s1", Status: model.ItemStatusWaiting, NextRunAt: &future},
		{Step: model.StepRelease, Appid: "wx-s1", Status: model.ItemStatusWaiting, NextRunAt: &past},
	}); err != nil {
		t.Fatalf("Create 失败: %v", err)
	}

	// 未到期的 waiting 不能被领取，也不会被提升
	if _, err := items.ClaimNextPending(ctx, "job-step", model.StepSubmitAudit); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("未到期的 waiting 不应被领取，实际 %v", err)
	}
	if n, err := items.PromoteDueWaiting(ctx, "job-step", model.StepSubmitAudit); err != nil || n != 0 {
		t.Fatalf("未到期不应被提升，实际 %d (%v)", n, err)
	}

	// 已到期的 waiting 先提升再领取
	if n, err := items.PromoteDueWaiting(ctx, "job-step", model.StepRelease); err != nil || n != 1 {
		t.Fatalf("到期 waiting 应被提升为 pending，实际 %d (%v)", n, err)
	}
	got, err := items.ClaimNextPending(ctx, "job-step", model.StepRelease)
	if err != nil {
		t.Fatalf("ClaimNextPending(release) 失败: %v", err)
	}
	if got.Step != model.StepRelease || got.Status != model.ItemStatusRunning {
		t.Fatalf("领取结果不正确: %+v", got)
	}
	// 步骤隔离：release 已领完，commit 仍可领
	if _, err := items.ClaimNextPending(ctx, "job-step", model.StepRelease); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("同步骤无可用项应返回 ErrNotFound，实际 %v", err)
	}
	commitItem, err := items.ClaimNextPending(ctx, "job-step", model.StepCommit)
	if err != nil || commitItem.Step != model.StepCommit {
		t.Fatalf("ClaimNextPending(commit) 失败: %+v (%v)", commitItem, err)
	}

	// waiting 子项：把 next_run_at 改成已到期后，空步骤的 PromoteDueWaiting 应把它提升
	waitingList, _, err := items.ListByJob(ctx, "job-step", model.ItemStatusWaiting, "", 1, 10)
	if err != nil || len(waitingList) != 1 {
		t.Fatalf("应恰好有 1 条 waiting 子项，实际 %d (%v)", len(waitingList), err)
	}
	dueAt := sec(time.Now().Add(-time.Second))
	if err := items.UpdateFields(ctx, waitingList[0].ID, map[string]any{"next_run_at": &dueAt}); err != nil {
		t.Fatalf("UpdateFields 失败: %v", err)
	}
	if n, err := items.PromoteDueWaiting(ctx, "job-step", ""); err != nil || n != 1 {
		t.Fatalf("空步骤应提升全部到期 waiting，实际 %d (%v)", n, err)
	}
	waitingDone, err := items.ClaimNextPending(ctx, "job-step", model.StepSubmitAudit)
	if err != nil {
		t.Fatalf("提升后应能领取 submit_audit 子项: %v", err)
	}
	if waitingDone.Step != model.StepSubmitAudit {
		t.Fatalf("领取到错误步骤的子项: %+v", waitingDone)
	}
	// 空步骤等价于不限定步骤：此时仅剩已领取的 running 项
	if _, err := items.ClaimNextPending(ctx, "job-step", ""); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("无可用项应返回 ErrNotFound，实际 %v", err)
	}
	if _, err := items.ClaimNextPending(ctx, "", model.StepCommit); err == nil {
		t.Fatal("空作业 ID 应报错")
	}
	if _, err := items.PromoteDueWaiting(ctx, "", model.StepCommit); err == nil {
		t.Fatal("空作业 ID 应报错")
	}
}

// TestJobsTryLockSingleInstance 验证 GET_LOCK 单实例排他锁（MySQL / MariaDB 环境）。
func TestJobsTryLockSingleInstance(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	name := fmt.Sprintf("wx_platform_test_scheduler_%d", time.Now().UnixNano())

	jobsA := repo.NewJobs(db)
	ok, err := jobsA.TryLock(ctx, name)
	if err != nil {
		t.Fatalf("TryLock 失败: %v", err)
	}
	if !ok {
		t.Fatal("第一个实例应拿到锁")
	}
	if ok, err = jobsA.TryLock(ctx, name); err != nil || !ok {
		t.Fatalf("同一实例重复申请应幂等成功，实际 ok=%v err=%v", ok, err)
	}
	if _, err := jobsA.TryLock(ctx, "   "); err == nil {
		t.Fatal("空锁名应报错")
	}

	// 第二个实例（同进程、不同物理连接，等价于另一台机器）应拿不到同一把锁
	jobsB := repo.NewJobs(db)
	ok, err = jobsB.TryLock(ctx, name)
	if err != nil {
		t.Fatalf("第二个实例 TryLock 出错: %v", err)
	}
	if ok {
		t.Fatal("锁已被占用，第二个实例不应拿到")
	}
	// 不同名称的锁互不影响
	if ok, err = jobsB.TryLock(ctx, name+"_other"); err != nil || !ok {
		t.Fatalf("不同名称的锁应可获取，实际 ok=%v err=%v", ok, err)
	}
}

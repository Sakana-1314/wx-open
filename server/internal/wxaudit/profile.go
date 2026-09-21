package wxaudit

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"wx-platform/server/internal/core"
	"wx-platform/server/internal/gen"
	"wx-platform/server/internal/model"
)

// 本文件实现提审配置（audit_profiles）的 CRUD。
//
// 为什么直连 env.DB 而不是走 repo：repo 侧目前只提供 List/Get/GetDefault 三个只读方法
//（作者已定稿，不在本包职责内改动），而提审配置的写操作只有这一个入口，
// 因此写操作按 core.Env.DB 直连，读仍然走 Repos。
//
// 官方约束（docs/reference/wx-open-platform-api.md §5.3）：
// 提审 item_list 的类目字段必须来自 GET /wxa/get_category（getAllCategoryName）返回的
// category_list —— first_class/second_class 是中文名，first_id/second_id 是数字 id，
// 两边必须成对出现，否则提审报 85008（当前小程序没有已经审核通过的类目）；
// item_list 为 1-5 条（超出报 85023），title ≤32 字。

// 提审配置的操作审计动作（沿用 api/openapi.yaml 的 operationId）。
const (
	actionCreateProfile = "createAuditProfile"
	actionUpdateProfile = "updateAuditProfile"
	actionDeleteProfile = "deleteAuditProfile"
)

// profileTarget 提审配置的操作审计对象类型。
const profileTarget = "audit_profile"

// 默认提审配置的名字与说明：与 database.SeedDefaultAuditProfile 保持一致，
// 库为空时由本包补一条，保证列表至少有一项可供选择。
const (
	defaultProfileName = "默认提审配置"
	defaultProfileNote = "类目字段需按 /wxa/get_category（getAllCategoryName）返回填写：first_class/second_class 为中文名，first_id/second_id 为数字 id"
	// profileNameMaxLen 名称长度上限（模型列为 varchar(128)）。
	profileNameMaxLen = 128
	// profileReferenceSample 冲突提示里最多列举几个引用方。
	profileReferenceSample = 3
)

// profileCategoryRequiredFields 类目必备字段：一旦提审项里出现其中任意一个，
// 四个必须同时提供（third_class/third_id 可选，两级类目很常见）。
var profileCategoryRequiredFields = []string{"first_class", "second_class", "first_id", "second_id"}

// ListProfiles 提审配置列表（默认配置排最前；库为空时补一条默认配置）。
func (s *AuditService) ListProfiles(ctx context.Context) (*gen.AuditProfileListResponse, error) {
	if s.env == nil || s.env.Repos == nil {
		return nil, core.Internal(errors.New("服务依赖未初始化"))
	}
	rows, err := s.env.Repos.Profiles.List(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	if len(rows) == 0 {
		// 正常启动时 database.SeedDefaultAuditProfile 已经写过一条；
		// 这里兜底「全新库 / 测试库直接调接口」的情况，避免前端拿到空列表。
		if s.env.DB == nil {
			return nil, core.Internal(errors.New("提审配置为空且缺少数据库连接，无法补默认配置"))
		}
		created, err := s.createDefaultProfile(ctx)
		if err != nil {
			return nil, err
		}
		rows = []model.AuditProfile{*created}
	}
	items := make([]gen.AuditProfile, 0, len(rows))
	for _, row := range rows {
		items = append(items, toGenProfile(row))
	}
	return &gen.AuditProfileListResponse{Items: items}, nil
}

// CreateProfile 新建提审配置。
//
// name 必填且全局唯一（重名返回 core.ErrConflict）；isDefault=true 时把其它配置的
// is_default 清零（同一时刻只允许一个默认配置）。
func (s *AuditService) CreateProfile(ctx context.Context, req gen.AuditProfileUpsertRequest) (*gen.AuditProfile, error) {
	if err := s.profilesReady(); err != nil {
		return nil, err
	}
	name, err := validProfileName(req)
	if err != nil {
		return nil, err
	}
	itemList := jsonMapFromPtr(req.ItemList)
	if err := validateProfileItemList(itemList); err != nil {
		return nil, err
	}
	if taken, err := s.profileNameTaken(ctx, name, 0); err != nil {
		return nil, err
	} else if taken {
		return nil, profileNameConflict(name)
	}

	rec := &model.AuditProfile{
		Name:             name,
		IsDefault:        derefBool(req.IsDefault),
		ItemList:         itemList,
		UgcDeclare:       jsonMapFromPtr(req.UgcDeclare),
		PrivacyAPINotUse: req.PrivacyApiNotUse,
		VersionDesc:      derefString(req.VersionDesc),
		OrderPath:        derefString(req.OrderPath),
		Note:             derefString(req.Note),
	}
	err = s.env.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := clearDefaultProfile(tx, rec.IsDefault); err != nil {
			return err
		}
		return tx.Create(rec).Error
	})
	if err != nil {
		if isProfileNameDuplicate(err) {
			return nil, profileNameConflict(name)
		}
		return nil, core.Internal(fmt.Errorf("新建提审配置失败：%w", err))
	}
	s.logOperation(ctx, actionCreateProfile, profileTarget, strconv.FormatUint(uint64(rec.ID), 10), model.JSONMap{
		"name":      rec.Name,
		"isDefault": rec.IsDefault,
	})
	out := toGenProfile(*rec)
	return &out, nil
}

// UpdateProfile 编辑提审配置；id 不存在返回 core.ErrNotFound。
func (s *AuditService) UpdateProfile(ctx context.Context, id int64, req gen.AuditProfileUpsertRequest) (*gen.AuditProfile, error) {
	if err := s.profilesReady(); err != nil {
		return nil, err
	}
	if id <= 0 {
		return nil, core.Validation("提审配置 id 必须为正整数")
	}
	name, err := validProfileName(req)
	if err != nil {
		return nil, err
	}
	itemList := jsonMapFromPtr(req.ItemList)
	if err := validateProfileItemList(itemList); err != nil {
		return nil, err
	}

	rec, err := s.env.Repos.Profiles.Get(ctx, uint(id))
	if err != nil {
		return nil, mapError(err) // repo.ErrNotFound → core.ErrNotFound
	}
	if name != rec.Name {
		if taken, err := s.profileNameTaken(ctx, name, rec.ID); err != nil {
			return nil, err
		} else if taken {
			return nil, profileNameConflict(name)
		}
	}

	rec.Name = name
	rec.IsDefault = derefBool(req.IsDefault)
	rec.ItemList = itemList
	rec.UgcDeclare = jsonMapFromPtr(req.UgcDeclare)
	rec.PrivacyAPINotUse = req.PrivacyApiNotUse
	rec.VersionDesc = derefString(req.VersionDesc)
	rec.OrderPath = derefString(req.OrderPath)
	rec.Note = derefString(req.Note)

	err = s.env.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := clearDefaultProfile(tx, rec.IsDefault); err != nil {
			return err
		}
		return tx.Save(rec).Error
	})
	if err != nil {
		if isProfileNameDuplicate(err) {
			return nil, profileNameConflict(name)
		}
		return nil, core.Internal(fmt.Errorf("更新提审配置失败：%w", err))
	}
	s.logOperation(ctx, actionUpdateProfile, profileTarget, strconv.FormatInt(id, 10), model.JSONMap{
		"name":      rec.Name,
		"isDefault": rec.IsDefault,
	})
	out := toGenProfile(*rec)
	return &out, nil
}

// DeleteProfile 删除提审配置。
//
// 仍被小程序引用（authorizers.audit_profile_id）时返回 core.ErrConflict，
// 避免删掉配置后批量提审拿不到 item_list；id 不存在返回 core.ErrNotFound。
func (s *AuditService) DeleteProfile(ctx context.Context, id int64) error {
	if err := s.profilesReady(); err != nil {
		return err
	}
	if id <= 0 {
		return core.Validation("提审配置 id 必须为正整数")
	}
	rec, err := s.env.Repos.Profiles.Get(ctx, uint(id))
	if err != nil {
		return mapError(err)
	}

	var total int64
	if err := s.env.DB.WithContext(ctx).Model(&model.Authorizer{}).
		Where("audit_profile_id = ? AND deleted_at IS NULL", id).
		Count(&total).Error; err != nil {
		return core.Internal(fmt.Errorf("检查提审配置引用失败：%w", err))
	}
	if total > 0 {
		refs := make([]struct {
			Appid    string
			NickName string
		}, 0, profileReferenceSample)
		if err := s.env.DB.WithContext(ctx).Model(&model.Authorizer{}).
			Select("appid, nick_name").
			Where("audit_profile_id = ? AND deleted_at IS NULL", id).
			Order("id ASC").Limit(profileReferenceSample).
			Find(&refs).Error; err != nil {
			return core.Internal(fmt.Errorf("查询提审配置引用方失败：%w", err))
		}
		names := make([]string, 0, len(refs))
		for _, ref := range refs {
			names = append(names, firstNonEmpty(ref.NickName, ref.Appid))
		}
		suffix := ""
		if total > int64(len(names)) {
			suffix = fmt.Sprintf(" 等 %d 个", total)
		}
		return core.Conflict(
			"提审配置 %q 仍被小程序引用（如 %s%s）：请先在「授权管理」里把这些小程序的提审配置改绑到其它配置，再删除。",
			rec.Name, strings.Join(names, "、"), suffix)
	}

	res := s.env.DB.WithContext(ctx).Where("id = ?", id).Delete(&model.AuditProfile{})
	if res.Error != nil {
		return core.Internal(fmt.Errorf("删除提审配置失败：%w", res.Error))
	}
	if res.RowsAffected == 0 {
		return core.NotFound("提审配置 %d 不存在或已被删除", id)
	}
	s.logOperation(ctx, actionDeleteProfile, profileTarget, strconv.FormatInt(id, 10), model.JSONMap{"name": rec.Name})
	return nil
}

// profilesReady 校验提审配置写操作需要的依赖。
func (s *AuditService) profilesReady() error {
	if s.env == nil || s.env.Repos == nil || s.env.DB == nil {
		return core.Internal(errors.New("服务依赖未初始化（提审配置写操作需要 DB 直连）"))
	}
	return nil
}

// validProfileName 校验并归一化配置名。
func validProfileName(req gen.AuditProfileUpsertRequest) (string, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return "", core.Validation("提审配置名称（name）不能为空")
	}
	if runes := len([]rune(name)); runes > profileNameMaxLen {
		return "", core.Validation("提审配置名称最长 %d 个字符，当前 %d 个", profileNameMaxLen, runes)
	}
	return name, nil
}

// profileNameTaken 判断名称是否已被其它配置占用（excludeID 为 0 表示新建）。
func (s *AuditService) profileNameTaken(ctx context.Context, name string, excludeID uint) (bool, error) {
	var n int64
	q := s.env.DB.WithContext(ctx).Model(&model.AuditProfile{}).Where("name = ?", name)
	if excludeID > 0 {
		q = q.Where("id <> ?", excludeID)
	}
	if err := q.Count(&n).Error; err != nil {
		return false, core.Internal(fmt.Errorf("检查提审配置名称失败：%w", err))
	}
	return n > 0, nil
}

// createDefaultProfile 库为空时补一条默认提审配置（内容与 database.SeedDefaultAuditProfile 一致）。
func (s *AuditService) createDefaultProfile(ctx context.Context) (*model.AuditProfile, error) {
	rec := &model.AuditProfile{Name: defaultProfileName, IsDefault: true, Note: defaultProfileNote}
	err := s.env.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := clearDefaultProfile(tx, true); err != nil {
			return err
		}
		return tx.Create(rec).Error
	})
	if err != nil {
		return nil, core.Internal(fmt.Errorf("补默认提审配置失败：%w", err))
	}
	log.Printf("[wxaudit] 提审配置为空，已补默认提审配置（id=%d）", rec.ID)
	return rec, nil
}

// clearDefaultProfile 把其它配置的 is_default 清零（全局只有一个默认配置）。
func clearDefaultProfile(tx *gorm.DB, makeDefault bool) error {
	if !makeDefault {
		return nil
	}
	if err := tx.Model(&model.AuditProfile{}).
		Where("is_default = ?", true).
		Update("is_default", false).Error; err != nil {
		return fmt.Errorf("清除原默认提审配置失败：%w", err)
	}
	return nil
}

// profileNameConflict 构造重名冲突（哨兵错误 + 明确下一步动作）。
func profileNameConflict(name string) error {
	return core.Conflict("提审配置名称 %q 已存在：请换一个名称，或直接编辑已有配置（名称需唯一，便于批量提审时按名称选择）。", name)
}

// isProfileNameDuplicate 判断是否 name 唯一索引冲突（1062，并发新建时可能发生）。
func isProfileNameDuplicate(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// validateProfileItemList 校验提审项里的类目字段。
//
// 只要出现类目字段，first_class/second_class/first_id/second_id 就必须同时提供
// （中文名与数字 id 成对，third_class/third_id 可空），否则提审报 85008。
// 兼容两种写法：单个提审项对象，或 {"item_list": [ ... ]} 这种多项数组。
func validateProfileItemList(itemList model.JSONMap) error {
	if len(itemList) == 0 {
		return nil
	}
	return walkProfileItem(map[string]any(itemList), "itemList")
}

// walkProfileItem 递归检查任意层级的提审项对象。
func walkProfileItem(node any, path string) error {
	switch v := node.(type) {
	case map[string]any:
		if err := checkCategoryFields(v, path); err != nil {
			return err
		}
		for key, child := range v {
			if err := walkProfileItem(child, path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for i, child := range v {
			if err := walkProfileItem(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkCategoryFields 检查单个提审项对象里的类目字段完整性。
func checkCategoryFields(item map[string]any, path string) error {
	present := make([]string, 0, len(profileCategoryRequiredFields))
	missing := make([]string, 0, len(profileCategoryRequiredFields))
	for _, field := range profileCategoryRequiredFields {
		if hasJSONValue(item, field) {
			present = append(present, field)
			continue
		}
		missing = append(missing, field)
	}
	if len(present) == 0 || len(missing) == 0 {
		return nil
	}
	return core.Validation(
		"%s 的类目字段不完整：已填 %s，缺少 %s。类目 6 个字段取自 GET /wxa/get_category（getAllCategoryName）返回的 category_list，"+
			"中文名（first_class/second_class/third_class）与数字 id（first_id/second_id/third_id）必须成对出现，否则提审报 85008。",
		path, strings.Join(present, "/"), strings.Join(missing, "/"))
}

// hasJSONValue 判断 JSON 对象里的字段是否存在且非空（空串/0 视为未填）。
func hasJSONValue(item map[string]any, field string) bool {
	raw, ok := item[field]
	if !ok || raw == nil {
		return false
	}
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v) != ""
	case float64:
		return v != 0
	case int:
		return v != 0
	case int64:
		return v != 0
	default:
		return true
	}
}

// toGenProfile 把本地提审配置转成契约结构。
func toGenProfile(rec model.AuditProfile) gen.AuditProfile {
	out := gen.AuditProfile{
		Id:               int64(rec.ID),
		Name:             rec.Name,
		IsDefault:        rec.IsDefault,
		ItemList:         jsonMapPtr(rec.ItemList),
		UgcDeclare:       jsonMapPtr(rec.UgcDeclare),
		PrivacyApiNotUse: rec.PrivacyAPINotUse,
		VersionDesc:      strPtr(rec.VersionDesc),
		OrderPath:        strPtr(rec.OrderPath),
		Note:             strPtr(rec.Note),
	}
	if !rec.CreatedAt.IsZero() {
		createdAt := rec.CreatedAt
		out.CreatedAt = &createdAt
	}
	if !rec.UpdatedAt.IsZero() {
		updatedAt := rec.UpdatedAt
		out.UpdatedAt = &updatedAt
	}
	return out
}

// jsonMapFromPtr 把契约里的可选对象转成 JSON 列类型（nil 保持 nil）。
func jsonMapFromPtr(in *map[string]interface{}) model.JSONMap {
	if in == nil {
		return nil
	}
	return model.JSONMap(*in)
}

// jsonMapPtr 把 JSON 列转成契约里的可选对象（nil 保持 nil）。
func jsonMapPtr(in model.JSONMap) *map[string]interface{} {
	if in == nil {
		return nil
	}
	out := map[string]interface{}(in)
	return &out
}

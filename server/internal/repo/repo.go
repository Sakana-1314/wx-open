// Package repo 数据访问层：一个聚合类型 + 一组类型化查询方法，禁止在 service 层写 SQL。
//
// 统一约定：
//   - 每个聚合是 `type Xxx struct{ db *gorm.DB }` + `func NewXxx(db *gorm.DB) *Xxx`；
//   - 所有导出方法第一个参数为 context.Context，内部统一使用 r.db.WithContext(ctx)；
//   - 返回值一律是 model 包里的强类型（JSON 列由 model 的自定义类型承载），不返回 map[string]any；
//   - 未查到记录一律返回 ErrNotFound（由 gorm.ErrRecordNotFound 转换），上层 service 据此统一映射 404；
//   - 分页统一 Limit/Offset，并同时返回 total；
//   - 数据库错误统一用 fmt.Errorf("...: %w", err) 包装。
package repo

import (
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// ErrNotFound 记录不存在（上层 service 据此统一映射 404）。
var ErrNotFound = errors.New("记录不存在")

// 分页默认值与上限（防止前端传入 pageSize=100000 拖垮数据库）。
const (
	defaultPageSize = 20
	maxPageSize     = 200
)

// paginate 归一化页码并换算成 Limit/Offset。
func paginate(page, pageSize int) (limit, offset int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return pageSize, (page - 1) * pageSize
}

// existsRow 判断给定条件是否命中至少一行（用于区分「记录不存在」与「更新无变化」）。
func existsRow(db *gorm.DB, dest any, cond string, args ...any) (bool, error) {
	var n int64
	if err := db.Model(dest).Where(cond, args...).Limit(1).Count(&n).Error; err != nil {
		return false, fmt.Errorf("检查记录是否存在失败: %w", err)
	}
	return n > 0, nil
}

// updateWhere 执行条件更新，并把「找不到记录」转换成 ErrNotFound。
//
// MySQL / MariaDB 的 UPDATE 返回的是**实际变更行数**：新值与旧值完全相同时为 0，
// 因此影响 0 行时必须再查一次存在性，否则「幂等的重复更新」会被误判成 404。
func updateWhere(db *gorm.DB, dest any, cond string, args []any, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	res := db.Model(dest).Where(cond, args...).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	ok, err := existsRow(db, dest, cond, args...)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// clearTable 清空整表（快照类表用「先清空再写入」，GORM 默认拒绝无 WHERE 的删除，故显式放行）。
func clearTable(tx *gorm.DB, dest any) error {
	return tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(dest).Error
}

// isDuplicateKey 判断是否 MySQL / MariaDB 唯一键冲突（1062）。
func isDuplicateKey(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// isRetryableLockError 判断是否 InnoDB 死锁（1213）或锁等待超时（1205），这类冲突可安全重试。
func isRetryableLockError(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && (me.Number == 1213 || me.Number == 1205)
}

// isLockFunctionUnavailable 判断错误是否表示「当前服务器没有 GET_LOCK 函数」（1305 函数不存在）。
// 用于把「非 MySQL / 老版本不支持命名锁」与真正的连接错误区分开。
func isLockFunctionUnavailable(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1305
}

// monthKey 返回撤回额度用的月份键（YYYY-MM），与 model.UndoQuotaUsage.MonthKey 一致。
func monthKey(t time.Time) string { return t.Format("2006-01") }

// startOfDay 返回本地时区当天 0 点（撤回额度按账号当天用量限 5 次）。
func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

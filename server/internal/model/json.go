// Package model 定义 GORM 模型、强类型枚举，以及微信返回码的分类规则。
package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSONMap 以 MySQL JSON 列存储的键值对象（如 ext 变量、business_info、请求/响应快照）。
type JSONMap map[string]any

// Value 实现 driver.Valuer。
func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(map[string]any(m))
	if err != nil {
		return nil, fmt.Errorf("序列化 JSON 对象失败: %w", err)
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (m *JSONMap) Scan(src any) error {
	if src == nil {
		*m = nil
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("无法把 %T 解析为 JSON 对象", src)
	}
	if len(raw) == 0 {
		*m = nil
		return nil
	}
	out := map[string]any{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("解析 JSON 对象失败: %w", err)
	}
	*m = out
	return nil
}

// JSONStringMap 以 MySQL JSON 列存储的字符串键值对（如每个小程序的 ext 变量）。
type JSONStringMap map[string]string

// Value 实现 driver.Valuer。
func (m JSONStringMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(map[string]string(m))
	if err != nil {
		return nil, fmt.Errorf("序列化字符串映射失败: %w", err)
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (m *JSONStringMap) Scan(src any) error {
	if src == nil {
		*m = nil
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("无法把 %T 解析为字符串映射", src)
	}
	if len(raw) == 0 {
		*m = nil
		return nil
	}
	out := map[string]string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("解析字符串映射失败: %w", err)
	}
	*m = out
	return nil
}

// StringSlice 以 MySQL JSON 列存储的字符串数组（如标签、域名列表）。
type StringSlice []string

// Value 实现 driver.Valuer。
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	b, err := json.Marshal([]string(s))
	if err != nil {
		return nil, fmt.Errorf("序列化字符串数组失败: %w", err)
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (s *StringSlice) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("无法把 %T 解析为字符串数组", src)
	}
	if len(raw) == 0 {
		*s = nil
		return nil
	}
	out := []string{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("解析字符串数组失败: %w", err)
	}
	*s = out
	return nil
}

// IntSlice 以 MySQL JSON 列存储的整数数组（如权限集 id 列表）。
type IntSlice []int

// Value 实现 driver.Valuer。
func (s IntSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	b, err := json.Marshal([]int(s))
	if err != nil {
		return nil, fmt.Errorf("序列化整数数组失败: %w", err)
	}
	return string(b), nil
}

// Scan 实现 sql.Scanner。
func (s *IntSlice) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("无法把 %T 解析为整数数组", src)
	}
	if len(raw) == 0 {
		*s = nil
		return nil
	}
	out := []int{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("解析整数数组失败: %w", err)
	}
	*s = out
	return nil
}

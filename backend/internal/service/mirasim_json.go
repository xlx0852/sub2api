package service

// Mirasim 只读元数据共用的 JSON 取值小工具（limits 与 available-models 两个解析器共用）。
//
// 关键约束（改动前请先读）：这些函数刻意**不做**类型强转——字符串 "1" 不算数字、
// 数字 1 不算字符串、真值 1 不算布尔。上游字段类型一旦漂移，宁可让该字段被判为
// 「缺失」并按各自的缺省语义处理，也不要在解析层猜类型。

import (
	"encoding/json"
	"math"
)

// mirasimJSONObject 把原始 JSON 解成字段表；非对象（含 null/数组）返回 nil。
func mirasimJSONObject(raw json.RawMessage) map[string]json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil
	}
	return fields
}

// mirasimJSONStringField 只在值确为 JSON 字符串时返回 ok（不做数字/布尔强转）。
func mirasimJSONStringField(fields map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := fields[key]
	if !ok {
		return "", false
	}
	return mirasimJSONStringValue(raw)
}

// mirasimJSONStringValue 只在原始值确为 JSON 字符串时返回 ok。
func mirasimJSONStringValue(raw json.RawMessage) (string, bool) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

// mirasimJSONNumberField 只在值确为有限数字时返回 ok（字符串 "1" 不算）。
func mirasimJSONNumberField(fields map[string]json.RawMessage, key string) (float64, bool) {
	raw, ok := fields[key]
	if !ok {
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

// mirasimJSONBoolField 只在值确为 JSON 布尔时返回 ok。
func mirasimJSONBoolField(fields map[string]json.RawMessage, key string) (bool, bool) {
	raw, ok := fields[key]
	if !ok {
		return false, false
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false
	}
	return value, true
}

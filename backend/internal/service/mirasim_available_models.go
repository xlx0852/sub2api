package service

// Mirasim 账号维度「可用模型」列表（`GET /v1/models`）的解析。
//
// 关键约束（改动前请先读）：
//   - 这个接口是**账号维度**的：只返回本账号当前可用的模型，因此它是模型白名单的
//     唯一来源。全局的 `/v1/model-roster`（agents/cli/pricing）描述的是 relay 的
//     通用目录，与「本账号能用什么」无关，已不再用于白名单；
//   - 响应是 OpenAI 风格：`{"object":"list","data":[{"id":"...","object":"model",
//     "owned_by":"mirasim-router"}]}`；
//   - `id` 必须是 JSON 字符串（不做数字/布尔强转），`trim` 后为空即丢弃；重复 id 只留一个；
//   - 输出做 `sort.Strings`（稳定、确定的顺序），便于前端与测试比对；
//   - 没有任何有效 id 时**返回错误**而不是空列表：静默清空白名单会让账号突然
//     「没有可用模型」，属于比报错更坏的失败模式。

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// mirasimDatedModelSuffix 匹配目录里带发布日期的“双生” id，如 claude-haiku-4-5-20251001。
var mirasimDatedModelSuffix = regexp.MustCompile(`-20\d{6}$`)

// mirasimReservedCatalogIDs 是目录会列、但不指向可服务模型的条目（对齐官方客户端收敛规则）。
var mirasimReservedCatalogIDs = map[string]struct{}{
	"*":                      {},
	"gpt-4o-mini":            {},
	"gpt-4o-mini-openrouter": {},
}

// mirasimNarrowAvailableModels 按官方客户端的方式收敛目录：
//   - 保留保留位（reserved）不指向模型，丢弃；
//   - 带命名空间的 id（含 "/"）不指向模型，丢弃；
//   - 带日期后缀的 id，若不放日期的“原名”也在服务中，则丢弃（避免同一个模型发两次）；
//     没有对应原名的日期 id 必须保留（那是拿到该模型的唯一方式）。
func mirasimNarrowAvailableModels(ids []string) []string {
	undated := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !strings.Contains(id, "/") && !mirasimDatedModelSuffix.MatchString(id) {
			undated[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, reserved := mirasimReservedCatalogIDs[id]; reserved || strings.Contains(id, "/") {
			continue
		}
		if mirasimDatedModelSuffix.MatchString(id) {
			if _, twin := undated[mirasimDatedModelSuffix.ReplaceAllString(id, "")]; twin {
				continue
			}
		}
		out = append(out, id)
	}
	return out
}

// parseMirasimAvailableModels 解析 `GET /v1/models` 响应为可用模型 id 列表（收敛 + 去重 + 升序）。
func parseMirasimAvailableModels(body []byte) ([]string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("mirasim models: 响应解析失败: %w", err)
	}
	if root == nil {
		return nil, errors.New("mirasim models: 响应根不是 JSON 对象")
	}
	rawData, ok := root["data"]
	if !ok {
		return nil, errors.New("mirasim models: 响应缺少 data 字段")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(rawData, &entries); err != nil {
		return nil, errors.New("mirasim models: data 不是数组")
	}

	seen := make(map[string]struct{}, len(entries))
	ids := make([]string, 0, len(entries))
	for _, entryRaw := range entries {
		var entry map[string]json.RawMessage
		if err := json.Unmarshal(entryRaw, &entry); err != nil || entry == nil {
			continue
		}
		var id string
		if err := json.Unmarshal(entry["id"], &id); err != nil {
			continue
		}
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.New("mirasim models: 响应中没有可用的模型 id")
	}
	ids = mirasimNarrowAvailableModels(ids)
	if len(ids) == 0 {
		return nil, errors.New("mirasim models: 收敛后没有可服务的模型 id")
	}
	sort.Strings(ids)
	return ids, nil
}

package handler

import "strings"

// openAITopLevelScalar 是单遍扫描到的顶层标量值。
type openAITopLevelScalar struct {
	kind  byte // 's' 字符串 | 'b' 布尔 | 'o' 其它（数字/对象/数组/null）
	text  string
	truth bool
}

// scanOpenAITopLevelScalars 单遍扫描顶层 JSON 对象，取出指定键的标量值。
//
// 为什么需要它：gjson 的惰性查找对「不存在或位置靠后」的键必须走完整个 JSON。
// 实测 46MiB 的 Responses 包里 `model` / `stream` / `previous_response_id` 三次查找
// 合计约 900ms（见 openspec/OBSERVE-2026-09-25-perf-wave.md）。本函数只做一次线性扫描，
// 且对不需要的嵌套值按括号深度跳过，不构造任何中间对象。
//
// 结构非法时返回已取到的部分，由调用方沿用既有校验（缺失 model → 400）。
func scanOpenAITopLevelScalars(body []byte, keys ...string) map[string]openAITopLevelScalar {
	out := make(map[string]openAITopLevelScalar, len(keys))
	if len(body) == 0 {
		return out
	}
	wanted := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		wanted[key] = struct{}{}
	}

	i := skipJSONSpace(body, 0)
	if i >= len(body) || body[i] != '{' {
		return out
	}
	i++
	for i < len(body) {
		i = skipJSONSpace(body, i)
		if i >= len(body) {
			return out
		}
		switch body[i] {
		case '}':
			return out
		case ',':
			i++
			continue
		case '"':
			key, next, ok := scanJSONString(body, i)
			if !ok {
				return out
			}
			i = skipJSONSpace(body, next)
			if i >= len(body) || body[i] != ':' {
				return out
			}
			i = skipJSONSpace(body, i+1)
			_, needed := wanted[key]
			value, next, ok := scanJSONValue(body, i, needed)
			if !ok {
				return out
			}
			if needed {
				out[key] = value
			}
			i = next
		default:
			return out
		}
	}
	return out
}

// scanJSONValue 读取一个 JSON 值；capture 为 false 时只做定位（不保留内容）。
func scanJSONValue(body []byte, i int, capture bool) (openAITopLevelScalar, int, bool) {
	if i >= len(body) {
		return openAITopLevelScalar{}, i, false
	}
	switch body[i] {
	case '"':
		value, next, ok := scanJSONString(body, i)
		if !ok {
			return openAITopLevelScalar{}, i, false
		}
		if !capture {
			value = ""
		}
		return openAITopLevelScalar{kind: 's', text: value}, next, true
	case 't':
		if hasJSONLiteral(body, i, "true") {
			return openAITopLevelScalar{kind: 'b', truth: true}, i + 4, true
		}
	case 'f':
		if hasJSONLiteral(body, i, "false") {
			return openAITopLevelScalar{kind: 'b'}, i + 5, true
		}
	case 'n':
		if hasJSONLiteral(body, i, "null") {
			return openAITopLevelScalar{kind: 'o'}, i + 4, true
		}
	default:
		if body[i] == '{' || body[i] == '[' {
			next, ok := skipJSONComposite(body, i)
			if !ok {
				return openAITopLevelScalar{}, i, false
			}
			return openAITopLevelScalar{kind: 'o'}, next, true
		}
		if next, ok := skipJSONNumber(body, i); ok {
			return openAITopLevelScalar{kind: 'o'}, next, true
		}
	}
	return openAITopLevelScalar{}, i, false
}

// skipJSONComposite 按括号深度跳过一个对象/数组（字符串感知）。
func skipJSONComposite(body []byte, i int) (int, bool) {
	depth := 0
	for i < len(body) {
		switch body[i] {
		case '"':
			_, next, ok := scanJSONString(body, i)
			if !ok {
				return i, false
			}
			i = next
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				return i + 1, true
			}
		}
		i++
	}
	return i, false
}

// scanJSONString 解析一个 JSON 字符串，返回未转义内容与结束位置。
func scanJSONString(body []byte, i int) (string, int, bool) {
	if i >= len(body) || body[i] != '"' {
		return "", i, false
	}
	i++
	start := i
	escaped := false
	for i < len(body) {
		switch body[i] {
		case '\\':
			escaped = true
			i += 2
			continue
		case '"':
			raw := body[start:i]
			if !escaped {
				return string(raw), i + 1, true
			}
			return unescapeJSONString(raw), i + 1, true
		}
		i++
	}
	return "", i, false
}

func unescapeJSONString(raw []byte) string {
	var builder strings.Builder
	builder.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' || i+1 >= len(raw) {
			builder.WriteByte(raw[i])
			continue
		}
		i++
		switch raw[i] {
		case 'n':
			builder.WriteByte('\n')
		case 't':
			builder.WriteByte('\t')
		case 'r':
			builder.WriteByte('\r')
		case 'b':
			builder.WriteByte('\b')
		case 'f':
			builder.WriteByte('\f')
		case 'u':
			if i+4 < len(raw) {
				if decoded, ok := decodeJSONUnicodeEscape(raw[i+1 : i+5]); ok {
					builder.WriteString(decoded)
					i += 4
					continue
				}
			}
			builder.WriteByte('u')
		default:
			builder.WriteByte(raw[i])
		}
	}
	return builder.String()
}

func decodeJSONUnicodeEscape(hex []byte) (string, bool) {
	var r rune
	for _, c := range hex {
		r <<= 4
		switch {
		case c >= '0' && c <= '9':
			r += rune(c - '0')
		case c >= 'a' && c <= 'f':
			r += rune(c-'a') + 10
		case c >= 'A' && c <= 'F':
			r += rune(c-'A') + 10
		default:
			return "", false
		}
	}
	return string(r), true
}

func skipJSONSpace(body []byte, i int) int {
	for i < len(body) {
		switch body[i] {
		case ' ', '\t', '\n', '\r':
			i++
		default:
			return i
		}
	}
	return i
}

func skipJSONNumber(body []byte, i int) (int, bool) {
	start := i
	if i < len(body) && (body[i] == '-' || body[i] == '+') {
		i++
	}
	digits := 0
	for i < len(body) {
		c := body[i]
		if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
			if c >= '0' && c <= '9' {
				digits++
			}
			i++
			continue
		}
		break
	}
	return i, digits > 0 && i > start
}

func hasJSONLiteral(body []byte, i int, literal string) bool {
	return i+len(literal) <= len(body) && string(body[i:i+len(literal)]) == literal
}

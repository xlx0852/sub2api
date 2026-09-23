package apicompat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// NamespacedToolName records the original ownership of a namespace child tool
// (namespace + bare child name). Shared by native Responses flatten/restore and
// the chat-completions bridge restore path.
type NamespacedToolName struct {
	Namespace string
	Name      string
}

// ResponsesNamespaceName identifies a function child in a Responses namespace.
// It aliases the chat bridge mapping so both native and bridged paths share one
// namespace identity contract.
type ResponsesNamespaceName = NamespacedToolName

// FlattenResponsesNamespaces converts Codex private namespace declarations into
// public Responses function tools and rewrites namespace-qualified request calls.
func FlattenResponsesNamespaces(req map[string]any) (map[string]ResponsesNamespaceName, bool, error) {
	return FlattenResponsesNamespacesExcept(req, nil)
}

// FlattenResponsesNamespacesExcept is FlattenResponsesNamespaces with a set of
// service-owned namespace names that must remain native in the request.
//
// Names come from:
//  1. type=namespace tool declarations (function children become top-level tools)
//  2. namespace-qualified function_call items in input / tool_choice (multi-turn
//     requests that omit the original namespace declaration still need rewrite)
//
// Non-function children inside a flattened namespace are preserved as top-level
// tools with their original type so MCP custom children are not silently dropped.
func FlattenResponsesNamespacesExcept(req map[string]any, preserved map[string]bool) (map[string]ResponsesNamespaceName, bool, error) {
	if req == nil {
		return nil, false, nil
	}

	tools, _ := req["tools"].([]any)
	topLevel := make(map[string]bool)
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		typ := strings.TrimSpace(stringValue(tool["type"]))
		name := strings.TrimSpace(stringValue(tool["name"]))
		if (typ == "function" || typ == "custom") && name != "" {
			topLevel[name] = true
		}
	}

	names := make(map[string]ResponsesNamespaceName)
	register := func(namespace, name string) error {
		namespace = strings.TrimSpace(namespace)
		name = strings.TrimSpace(name)
		if namespace == "" || name == "" || preserved[namespace] {
			return nil
		}
		flat := flattenNamespaceToolName(namespace, name)
		entry := ResponsesNamespaceName{Namespace: namespace, Name: name}
		if topLevel[flat] {
			return fmt.Errorf("namespace tool %q/%q flattens to %q which conflicts with a top-level tool of the same name; this upstream cannot disambiguate them, rename one of the tools", namespace, name, flat)
		}
		if prev, exists := names[flat]; exists && prev != entry {
			return fmt.Errorf("namespace tools %q/%q and %q/%q both flatten to %q; this upstream cannot disambiguate them, rename one of the tools", prev.Namespace, prev.Name, namespace, name, flat)
		}
		names[flat] = entry
		return nil
	}

	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(tool["type"])) != "namespace" {
			continue
		}
		namespace := strings.TrimSpace(stringValue(tool["name"]))
		if namespace == "" || preserved[namespace] {
			continue
		}
		for _, rawChild := range namespaceChildren(tool) {
			child, ok := rawChild.(map[string]any)
			if !ok {
				continue
			}
			childType := strings.TrimSpace(stringValue(child["type"]))
			if childType != "function" && childType != "custom" {
				continue
			}
			if err := register(namespace, stringValue(child["name"])); err != nil {
				return nil, false, err
			}
		}
	}

	// Multi-turn / history-only: collect namespace-qualified calls even when tools
	// no longer declare the original namespace wrapper.
	if err := collectNamespaceQualifiedCallNames(req["input"], preserved, register); err != nil {
		return nil, false, err
	}
	if choice, ok := req["tool_choice"].(map[string]any); ok {
		choiceType := strings.TrimSpace(stringValue(choice["type"]))
		if choiceType == "function" || choiceType == "custom" || choiceType == "" {
			if err := register(stringValue(choice["namespace"]), stringValue(choice["name"])); err != nil {
				return nil, false, err
			}
		}
	}

	if len(names) == 0 {
		// Still need to expand namespace tool declarations that only contain
		// non-function children (no restore mapping), or there is nothing to do.
		if !namespaceToolsNeedStructuralFlatten(tools, preserved) {
			return nil, false, nil
		}
	}

	changed := false
	if len(tools) > 0 {
		flattened := make([]any, 0, len(tools)+len(names))
		seen := make(map[string]bool)
		for _, raw := range tools {
			tool, ok := raw.(map[string]any)
			if !ok || strings.TrimSpace(stringValue(tool["type"])) != "namespace" {
				flattened = append(flattened, raw)
				continue
			}
			namespace := strings.TrimSpace(stringValue(tool["name"]))
			if preserved[namespace] {
				flattened = append(flattened, raw)
				continue
			}
			changed = true
			for _, rawChild := range namespaceChildren(tool) {
				child, ok := rawChild.(map[string]any)
				if !ok {
					continue
				}
				childType := strings.TrimSpace(stringValue(child["type"]))
				name := strings.TrimSpace(stringValue(child["name"]))
				flatChild := make(map[string]any, len(child)+1)
				for key, value := range child {
					flatChild[key] = value
				}
				if childType == "function" {
					if name == "" {
						continue
					}
					flat := flattenNamespaceToolName(namespace, name)
					if seen[flat] {
						continue
					}
					seen[flat] = true
					flatChild["name"] = flat
					flattened = append(flattened, flatChild)
					continue
				}
				// Preserve non-function children (e.g. custom) without silent drop.
				// Prefer a stable flat name when both namespace and child name exist.
				if namespace != "" && name != "" {
					flat := flattenNamespaceToolName(namespace, name)
					if seen["@"+childType+":"+flat] {
						continue
					}
					seen["@"+childType+":"+flat] = true
					flatChild["name"] = flat
				} else if name != "" {
					if seen["@"+childType+":"+name] {
						continue
					}
					seen["@"+childType+":"+name] = true
				}
				flattened = append(flattened, flatChild)
			}
		}
		req["tools"] = flattened
	}

	if rewriteNamespaceQualifiedCalls(req["input"], names) {
		changed = true
	}
	if choice, ok := req["tool_choice"].(map[string]any); ok {
		choiceNamespace := strings.TrimSpace(stringValue(choice["name"]))
		if strings.TrimSpace(stringValue(choice["type"])) == "namespace" && !preserved[choiceNamespace] {
			req["tool_choice"] = "auto"
			changed = true
		} else if rewriteNamespaceQualifiedCall(choice, names) {
			changed = true
		}
	}
	if !changed {
		return names, false, nil
	}
	if len(names) == 0 {
		return nil, true, nil
	}
	return names, true, nil
}

func namespaceToolsNeedStructuralFlatten(tools []any, preserved map[string]bool) bool {
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(tool["type"])) != "namespace" {
			continue
		}
		namespace := strings.TrimSpace(stringValue(tool["name"]))
		if namespace == "" || preserved[namespace] {
			continue
		}
		if len(namespaceChildren(tool)) > 0 {
			return true
		}
	}
	return false
}

func collectNamespaceQualifiedCallNames(value any, preserved map[string]bool, register func(namespace, name string) error) error {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if err := collectNamespaceQualifiedCallNames(item, preserved, register); err != nil {
				return err
			}
		}
	case map[string]any:
		itemType := strings.TrimSpace(stringValue(typed["type"]))
		if itemType == "function_call" || itemType == "custom_tool_call" {
			namespace := strings.TrimSpace(stringValue(typed["namespace"]))
			if namespace != "" && !preserved[namespace] {
				if err := register(namespace, stringValue(typed["name"])); err != nil {
					return err
				}
			}
		}
		for _, child := range typed {
			if err := collectNamespaceQualifiedCallNames(child, preserved, register); err != nil {
				return err
			}
		}
	}
	return nil
}

// RestoreResponsesNamespaceCalls restores flattened function calls in a JSON
// Responses payload to the namespace/name identity expected by Codex, and turns
// calls to downgraded custom tools back into custom_tool_call items.
func RestoreResponsesNamespaceCalls(payload []byte, names map[string]ResponsesNamespaceName, customNames map[string]bool) ([]byte, bool, error) {
	if len(payload) == 0 || (len(names) == 0 && len(customNames) == 0) {
		return payload, false, nil
	}
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return payload, false, err
	}
	changed := restoreResponsesNamespaceValue(value, names, customNames)
	if !changed {
		return payload, false, nil
	}
	var rebuilt bytes.Buffer
	encoder := json.NewEncoder(&rebuilt)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return payload, false, err
	}
	return bytes.TrimSuffix(rebuilt.Bytes(), []byte("\n")), true, nil
}

func namespaceChildren(tool map[string]any) []any {
	if children, ok := tool["tools"].([]any); ok && len(children) > 0 {
		return children
	}
	children, _ := tool["children"].([]any)
	return children
}

func rewriteNamespaceQualifiedCalls(value any, names map[string]ResponsesNamespaceName) bool {
	changed := false
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if rewriteNamespaceQualifiedCalls(item, names) {
				changed = true
			}
		}
	case map[string]any:
		itemType := strings.TrimSpace(stringValue(typed["type"]))
		if itemType == "function_call" || itemType == "custom_tool_call" {
			if rewriteNamespaceQualifiedCall(typed, names) {
				changed = true
			}
		}
		for _, child := range typed {
			if rewriteNamespaceQualifiedCalls(child, names) {
				changed = true
			}
		}
	}
	return changed
}

func rewriteNamespaceQualifiedCall(item map[string]any, names map[string]ResponsesNamespaceName) bool {
	namespace := strings.TrimSpace(stringValue(item["namespace"]))
	name := strings.TrimSpace(stringValue(item["name"]))
	if namespace == "" || name == "" {
		return false
	}
	flat := flattenNamespaceToolName(namespace, name)
	entry, ok := names[flat]
	if !ok || entry.Namespace != namespace || entry.Name != name {
		return false
	}
	item["name"] = flat
	delete(item, "namespace")
	return true
}

func restoreResponsesNamespaceValue(value any, names map[string]ResponsesNamespaceName, customNames map[string]bool) bool {
	changed := false
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			changed = restoreResponsesNamespaceValue(item, names, customNames) || changed
		}
	case map[string]any:
		itemType := strings.TrimSpace(stringValue(typed["type"]))
		if itemType == "function_call" || itemType == "custom_tool_call" {
			// 降级过的 custom 工具：上游只会回 function_call，必须还原成
			// custom_tool_call（codex 只按该类型路由），并把 input 字符串从
			// arguments.input 里取回。
			if itemType == "function_call" && customNames[strings.TrimSpace(stringValue(typed["name"]))] {
				arguments := stringValue(typed["arguments"])
				typed["type"] = "custom_tool_call"
				typed["input"] = extractCustomToolCallInput(arguments)
				delete(typed, "arguments")
				changed = true
			}
			if entry, ok := names[strings.TrimSpace(stringValue(typed["name"]))]; ok {
				typed["name"] = entry.Name
				typed["namespace"] = entry.Namespace
				changed = true
			}
		}
		for _, child := range typed {
			changed = restoreResponsesNamespaceValue(child, names, customNames) || changed
		}
	}
	return changed
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

// ResponsesCustomToolInputSchema 是 custom/freeform 工具降级为 function 工具时的参数
// schema：chat/custom 上游无法表达自由文本与 grammar 约束，退化为单一 input 字符串，
// 回程再由 extractCustomToolCallInput 还原。
const ResponsesCustomToolInputSchema = `{"type":"object","properties":{"input":{"type":"string","description":"The raw input for this tool, passed through verbatim."}},"required":["input"]}`

// DowngradeResponsesCustomTools 把顶层 custom 工具声明降级为 function 工具，并把入参历史
// 里对这些工具的 custom_tool_call 项改写为等价的 function_call。
//
// 背景：codex 的核心执行工具 exec 以 type=custom（自由格式 + lark grammar）声明，部分上游
// （例如 Devin 的 exa.api_server_pb.ApiServerService/GetChatMessage）会直接以
// permission_denied「MCP configuration issue」拒绝整个请求。降级后模型改回 function 调用，
// 回程由 RestoreResponsesNamespaceCalls 还原为 custom_tool_call，对客户端透明。
func DowngradeResponsesCustomTools(req map[string]any) (map[string]bool, bool, error) {
	if req == nil {
		return nil, false, nil
	}
	tools, _ := req["tools"].([]any)
	if len(tools) == 0 {
		return nil, false, nil
	}

	names := make(map[string]bool)
	changed := false
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(tool["type"])) != "custom" {
			continue
		}
		name := strings.TrimSpace(stringValue(tool["name"]))
		if name == "" {
			continue
		}
		tool["type"] = "function"
		tool["parameters"] = json.RawMessage(ResponsesCustomToolInputSchema)
		delete(tool, "format")
		names[name] = true
		changed = true
	}
	if !changed {
		return nil, false, nil
	}

	if input, ok := req["input"].([]any); ok {
		for _, raw := range input {
			item, ok := raw.(map[string]any)
			if !ok || strings.TrimSpace(stringValue(item["type"])) != "custom_tool_call" {
				continue
			}
			if !names[strings.TrimSpace(stringValue(item["name"]))] {
				continue
			}
			arguments, err := json.Marshal(map[string]string{"input": stringValue(item["input"])})
			if err != nil {
				return nil, false, err
			}
			item["type"] = "function_call"
			item["arguments"] = string(arguments)
			delete(item, "input")
		}
	}
	return names, true, nil
}

package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlattenResponsesNamespaces_RewritesDeclarationHistoryAndChoice(t *testing.T) {
	req := map[string]any{
		"model": "gpt-5.5",
		"tools": []any{
			map[string]any{"type": "function", "name": "plain", "description": "keep"},
			map[string]any{
				"type": "namespace",
				"name": "collaboration",
				"tools": []any{
					map[string]any{"type": "function", "name": "spawn_agent", "description": "spawn", "parameters": map[string]any{"type": "object"}},
				},
			},
		},
		"tool_choice": map[string]any{"type": "function", "name": "spawn_agent", "namespace": "collaboration"},
		"input": []any{
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "spawn_agent", "namespace": "collaboration", "arguments": "{}"},
			map[string]any{"type": "message", "role": "user", "content": "hi", "name": "spawn_agent", "namespace": "collaboration"},
		},
	}

	names, changed, err := FlattenResponsesNamespaces(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "collaboration", Name: "spawn_agent"}, names["collaboration__spawn_agent"])

	tools, ok := req["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 2)
	plainTool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "plain", plainTool["name"])
	flatTool, ok := tools[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "collaboration__spawn_agent", flatTool["name"])
	require.Equal(t, "spawn", flatTool["description"])

	choice, ok := req["tool_choice"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "collaboration__spawn_agent", choice["name"])
	require.NotContains(t, choice, "namespace")

	input, ok := req["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 2)
	call, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "collaboration__spawn_agent", call["name"])
	require.NotContains(t, call, "namespace")
	message, ok := input[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "spawn_agent", message["name"])
	require.Equal(t, "collaboration", message["namespace"])
	require.Equal(t, "gpt-5.5", req["model"])
}

func TestFlattenResponsesNamespaces_RejectsFlatNameCollision(t *testing.T) {
	req := map[string]any{"tools": []any{
		map[string]any{"type": "function", "name": "collaboration__spawn_agent"},
		map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{
			map[string]any{"type": "function", "name": "spawn_agent"},
		}},
	}}

	_, _, err := FlattenResponsesNamespaces(req)
	require.ErrorContains(t, err, "conflicts with a top-level tool")
}

func TestFlattenResponsesNamespaces_NamespaceGroupChoiceFallsBackToAuto(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "collaboration", "tools": []any{
				map[string]any{"type": "function", "name": "spawn_agent"},
				map[string]any{"type": "function", "name": "send_message"},
			},
		}},
		"tool_choice": map[string]any{"type": "namespace", "name": "collaboration"},
	}

	_, changed, err := FlattenResponsesNamespaces(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "auto", req["tool_choice"])
}

func TestFlattenResponsesNamespacesExcept_PreservesBuiltInNamespaceAndChoice(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{"type": "namespace", "name": "image_gen", "tools": []any{
				map[string]any{"type": "function", "name": "imagegen"},
			}},
			map[string]any{"type": "namespace", "name": "collaboration", "tools": []any{
				map[string]any{"type": "function", "name": "spawn_agent"},
			}},
		},
		"tool_choice": map[string]any{"type": "namespace", "name": "image_gen"},
	}

	names, changed, err := FlattenResponsesNamespacesExcept(req, map[string]bool{"image_gen": true})
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, names, "collaboration__spawn_agent")
	tools, ok := req["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 2)
	preservedTool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "namespace", preservedTool["type"])
	require.Equal(t, "image_gen", preservedTool["name"])
	flatTool, ok := tools[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function", flatTool["type"])
	require.Equal(t, "collaboration__spawn_agent", flatTool["name"])
	require.Equal(t, map[string]any{"type": "namespace", "name": "image_gen"}, req["tool_choice"])
}

func TestFlattenResponsesNamespaces_RejectsNamespaceCollision(t *testing.T) {
	req := map[string]any{"tools": []any{
		map[string]any{"type": "namespace", "name": "a", "tools": []any{
			map[string]any{"type": "function", "name": "b__c"},
		}},
		map[string]any{"type": "namespace", "name": "a__b", "tools": []any{
			map[string]any{"type": "function", "name": "c"},
		}},
	}}

	_, _, err := FlattenResponsesNamespaces(req)
	require.ErrorContains(t, err, "both flatten")
}

func TestRestoreResponsesNamespaceCalls_RewritesOnlyFunctionCalls(t *testing.T) {
	payload := []byte(`{"type":"response.completed","response":{"output":[{"type":"function_call","name":"collaboration__spawn_agent","call_id":"call_1","arguments":"{}","extra":"keep"},{"type":"function_call","name":"plain","arguments":"{}"},{"type":"message","name":"collaboration__spawn_agent","content":"<tag>&value</tag>"}]}}`)
	names := map[string]ResponsesNamespaceName{
		"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"},
	}

	got, changed, err := RestoreResponsesNamespaceCalls(payload, names, nil)
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"type":"response.completed","response":{"output":[{"type":"function_call","name":"spawn_agent","namespace":"collaboration","call_id":"call_1","arguments":"{}","extra":"keep"},{"type":"function_call","name":"plain","arguments":"{}"},{"type":"message","name":"collaboration__spawn_agent","content":"<tag>&value</tag>"}]}}`, string(got))
	require.Contains(t, string(got), "<tag>&value</tag>")
	require.NotContains(t, string(got), `\u003c`)
}

func TestRestoreResponsesNamespaceCalls_RewritesLifecycleItems(t *testing.T) {
	for _, eventType := range []string{"response.output_item.added", "response.output_item.done"} {
		t.Run(eventType, func(t *testing.T) {
			payload := []byte(`{"type":"` + eventType + `","item":{"type":"function_call","name":"collaboration__spawn_agent","arguments":"{}"}}`)
			got, changed, err := RestoreResponsesNamespaceCalls(payload, map[string]ResponsesNamespaceName{
				"collaboration__spawn_agent": {Namespace: "collaboration", Name: "spawn_agent"},
			}, nil)
			require.NoError(t, err)
			require.True(t, changed)
			require.JSONEq(t, `{"type":"`+eventType+`","item":{"type":"function_call","name":"spawn_agent","namespace":"collaboration","arguments":"{}"}}`, string(got))
		})
	}
}

func TestFlattenResponsesNamespaces_InputOnlyHistoryRewritesWithoutTools(t *testing.T) {
	req := map[string]any{
		"model": "gpt-5.5",
		"input": []any{
			map[string]any{"type": "function_call", "call_id": "call_1", "name": "spawn_agent", "namespace": "collaboration", "arguments": "{}"},
		},
	}

	names, changed, err := FlattenResponsesNamespaces(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "collaboration", Name: "spawn_agent"}, names["collaboration__spawn_agent"])

	input, ok := req["input"].([]any)
	require.True(t, ok)
	call, ok := input[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "collaboration__spawn_agent", call["name"])
	require.NotContains(t, call, "namespace")
}

func TestFlattenResponsesNamespaces_PreservesNonFunctionChildren(t *testing.T) {
	req := map[string]any{
		"tools": []any{
			map[string]any{
				"type": "namespace",
				"name": "mcp_box",
				"tools": []any{
					map[string]any{"type": "function", "name": "run", "parameters": map[string]any{"type": "object"}},
					map[string]any{"type": "custom", "name": "exec", "description": "keep custom"},
				},
			},
		},
	}

	names, changed, err := FlattenResponsesNamespaces(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, names, "mcp_box__run")
	require.Equal(t, ResponsesNamespaceName{Namespace: "mcp_box", Name: "exec"}, names["mcp_box__exec"])

	tools, ok := req["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 2)
	fn, ok := tools[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "function", fn["type"])
	require.Equal(t, "mcp_box__run", fn["name"])
	custom, ok := tools[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "custom", custom["type"])
	require.Equal(t, "mcp_box__exec", custom["name"])
	require.Equal(t, "keep custom", custom["description"])
}

func TestFlattenResponsesNamespaces_RoundTripsCustomCalls(t *testing.T) {
	req := map[string]any{
		"tools": []any{map[string]any{
			"type": "namespace", "name": "mcp_box", "tools": []any{
				map[string]any{"type": "custom", "name": "exec"},
			},
		}},
		"input": []any{
			map[string]any{"type": "custom_tool_call", "name": "exec", "namespace": "mcp_box", "input": "pwd"},
		},
		"tool_choice": map[string]any{"type": "custom", "name": "exec", "namespace": "mcp_box"},
	}

	names, changed, err := FlattenResponsesNamespaces(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ResponsesNamespaceName{Namespace: "mcp_box", Name: "exec"}, names["mcp_box__exec"])

	input := req["input"].([]any)[0].(map[string]any)
	require.Equal(t, "mcp_box__exec", input["name"])
	require.NotContains(t, input, "namespace")
	choice := req["tool_choice"].(map[string]any)
	require.Equal(t, "mcp_box__exec", choice["name"])
	require.NotContains(t, choice, "namespace")

	payload := []byte(`{"type":"response.completed","response":{"output":[{"type":"custom_tool_call","name":"mcp_box__exec","input":"pwd"}]}}`)
	restored, restoredChanged, err := RestoreResponsesNamespaceCalls(payload, names, nil)
	require.NoError(t, err)
	require.True(t, restoredChanged)
	require.JSONEq(t, `{"type":"response.completed","response":{"output":[{"type":"custom_tool_call","name":"exec","namespace":"mcp_box","input":"pwd"}]}}`, string(restored))
}

func TestDowngradeResponsesCustomTools(t *testing.T) {
	req := map[string]any{
		"model": "gpt-6-astra",
		"tools": []any{
			map[string]any{"type": "custom", "name": "functions__exec", "description": "run", "format": map[string]any{"type": "grammar"}},
			map[string]any{"type": "function", "name": "functions__wait", "parameters": map[string]any{}},
		},
		"input": []any{
			map[string]any{"type": "custom_tool_call", "name": "functions__exec", "call_id": "ctc_1", "input": "ls -la"},
			map[string]any{"type": "function_call", "name": "functions__wait", "call_id": "fc_1", "arguments": "{}"},
		},
	}

	names, changed, err := DowngradeResponsesCustomTools(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, map[string]bool{"functions__exec": true}, names)

	tools := req["tools"].([]any)
	execTool := tools[0].(map[string]any)
	require.Equal(t, "function", execTool["type"])
	require.NotNil(t, execTool["parameters"])
	require.NotContains(t, execTool, "format")
	require.Equal(t, "function", tools[1].(map[string]any)["type"])

	input := req["input"].([]any)
	historyCall := input[0].(map[string]any)
	require.Equal(t, "function_call", historyCall["type"])
	require.NotContains(t, historyCall, "input")
	require.JSONEq(t, `{"input":"ls -la"}`, historyCall["arguments"].(string))
	require.Equal(t, "function_call", input[1].(map[string]any)["type"])
}

func TestDowngradeResponsesCustomTools_NoCustomTools(t *testing.T) {
	req := map[string]any{"tools": []any{map[string]any{"type": "function", "name": "shell"}}}
	names, changed, err := DowngradeResponsesCustomTools(req)
	require.NoError(t, err)
	require.False(t, changed)
	require.Empty(t, names)
}

func TestRestoreResponsesNamespaceCalls_RestoresDowngradedCustomCalls(t *testing.T) {
	payload := []byte(`{"type":"response.output_item.done","item":{"type":"function_call","name":"functions__exec","call_id":"ctc_1","arguments":"{\"input\":\"ls -la\"}"}}`)
	got, changed, err := RestoreResponsesNamespaceCalls(payload, map[string]ResponsesNamespaceName{
		"functions__exec": {Namespace: "functions", Name: "exec"},
	}, map[string]bool{"functions__exec": true})
	require.NoError(t, err)
	require.True(t, changed)
	require.JSONEq(t, `{"type":"response.output_item.done","item":{"type":"custom_tool_call","name":"exec","namespace":"functions","call_id":"ctc_1","input":"ls -la"}}`, string(got))
}

func TestRestoreResponsesNamespaceCalls_KeepsPlainFunctionCallsForCustomNames(t *testing.T) {
	payload := []byte(`{"item":{"type":"function_call","name":"shell","call_id":"fc_2","arguments":"{\"cmd\":\"ls\"}"}}`)
	got, changed, err := RestoreResponsesNamespaceCalls(payload, nil, map[string]bool{"functions__exec": true})
	require.NoError(t, err)
	require.False(t, changed)
	require.JSONEq(t, `{"item":{"type":"function_call","name":"shell","call_id":"fc_2","arguments":"{\"cmd\":\"ls\"}"}}`, string(got))
}

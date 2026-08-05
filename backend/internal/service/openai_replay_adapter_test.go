package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeOpenAIReplayItemsConvertsPrivatePairsWithoutSyntheticIDs(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"custom_tool_call","id":"ctc_same","call_id":"call_1","name":"apply_patch","input":"*** Begin Patch"}`),
		json.RawMessage(`{"type":"custom_tool_call_output","id":"ctco_same","call_id":"call_1","output":"Done"}`),
	}
	normalized := normalizeOpenAIReplayItems(items)

	require.Len(t, normalized, 2)
	require.Equal(t, "function_call", gjson.GetBytes(normalized[0], "type").String())
	require.Equal(t, "call_1", gjson.GetBytes(normalized[0], "call_id").String())
	require.JSONEq(t, `{"input":"*** Begin Patch"}`, gjson.GetBytes(normalized[0], "arguments").String())
	require.False(t, gjson.GetBytes(normalized[0], "id").Exists())
	require.Equal(t, "function_call_output", gjson.GetBytes(normalized[1], "type").String())
	require.Equal(t, "call_1", gjson.GetBytes(normalized[1], "call_id").String())
	require.False(t, gjson.GetBytes(normalized[1], "id").Exists())
}

func TestNormalizeOpenAIReplayItemsDropsIncompleteCall(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":""}`),
	}
	require.Empty(t, normalizeOpenAIReplayItems(items))
}

func TestNormalizeOpenAIReplayItemsDropsOutputWhoseCollectedCallWasIncomplete(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":""}`),
		json.RawMessage(`{"type":"function_call_output","call_id":"call_1","output":"must not be orphaned"}`),
	}
	require.Empty(t, normalizeOpenAIReplayItems(items))
}

func TestNormalizeOpenAIReplayItemsKeepsOutputLinkedToEarlierContext(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"function_call_output","call_id":"call_from_prior_input","output":"ok"}`),
	}
	normalized := normalizeOpenAIReplayItems(items)
	require.Len(t, normalized, 1)
	require.Equal(t, "call_from_prior_input", gjson.GetBytes(normalized[0], "call_id").String())
}

func TestNormalizeOpenAIReplayItemsPreservesStructuredOutput(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"custom_tool_call_output","call_id":"call_1","output":[{"type":"input_text","text":"ok"},{"type":"input_image","image_url":"data:image/png;base64,xx"}]}`),
	}
	normalized := normalizeOpenAIReplayItems(items)
	require.Len(t, normalized, 1)
	require.True(t, gjson.GetBytes(normalized[0], "output").IsArray())
	require.Equal(t, "ok", gjson.GetBytes(normalized[0], "output.0.text").String())
	require.Equal(t, "data:image/png;base64,xx", gjson.GetBytes(normalized[0], "output.1.image_url").String())
}

func TestResolveOpenAIResponsesItemDialect(t *testing.T) {
	t.Parallel()

	require.Equal(t, openAIResponsesItemDialectCodex, resolveOpenAIResponsesItemDialect(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}))
	require.Equal(t, openAIResponsesItemDialectAuto, resolveOpenAIResponsesItemDialect(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}))
	require.Equal(t, openAIResponsesItemDialectCodex, resolveOpenAIResponsesItemDialect(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{openAIResponsesItemDialectExtraKey: string(openAIResponsesItemDialectCodex)},
	}))
}

func TestNormalizeOpenAIReplayItemsForAccountHonorsDialect(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"patch","input":"x"}`),
	}
	autoItems := normalizeOpenAIReplayItemsForAccount(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, items)
	standardItems := normalizeOpenAIReplayItemsForAccount(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{openAIResponsesItemDialectExtraKey: string(openAIResponsesItemDialectStandard)},
	}, items)
	oauthItems := normalizeOpenAIReplayItemsForAccount(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, items)

	require.Len(t, autoItems, 1)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(autoItems[0], "type").String())
	require.Len(t, standardItems, 1)
	require.Equal(t, "function_call", gjson.GetBytes(standardItems[0], "type").String())
	require.Len(t, oauthItems, 1)
	require.Equal(t, "custom_tool_call", gjson.GetBytes(oauthItems[0], "type").String())
	require.Equal(t, "x", gjson.GetBytes(oauthItems[0], "input").String())
}

func TestNormalizeOpenAIReplayItemsForAccountFiltersIncompleteNativeCall(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":""}`),
		json.RawMessage(`{"type":"function_call_output","call_id":"call_1","output":"must not be orphaned"}`),
	}
	normalized := normalizeOpenAIReplayItemsForAccount(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, items)
	require.Empty(t, normalized)
}

func TestNormalizeOpenAIReplayItemsResolvesPrivateOutputCallIDFromItemIdentity(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"custom_tool_call","id":"ctc_shared","call_id":"call_1","name":"patch","input":"x"}`),
		json.RawMessage(`{"type":"custom_tool_call_output","id":"ctco_shared","output":"ok"}`),
	}
	normalized := normalizeOpenAIReplayItems(items)

	require.Len(t, normalized, 2)
	require.Equal(t, "call_1", gjson.GetBytes(normalized[1], "call_id").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(normalized[1], "type").String())
}

func TestNormalizeOpenAIReplayItemsForAccountKeepsNativePairWithDuplicateItemID(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"local_shell_call","id":"shell-item-1","call_id":"call_1","name":"shell","action":{"command":["pwd"]}}`),
		json.RawMessage(`{"type":"local_shell_call_output","id":"shell-item-1","call_id":"call_1","output":"ok"}`),
	}
	normalized := normalizeOpenAIReplayItemsForAccount(&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, items)

	require.Len(t, normalized, 2)
	require.Equal(t, "shell-item-1", gjson.GetBytes(normalized[0], "id").String())
	require.False(t, gjson.GetBytes(normalized[1], "id").Exists())
	require.Equal(t, "call_1", gjson.GetBytes(normalized[1], "call_id").String())
}

func TestNormalizeOpenAIReplayItemsDropsJSONWithTrailingGarbage(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{json.RawMessage(`{"type":"message","role":"assistant"} trailing`)}
	require.Empty(t, normalizeOpenAIReplayItems(items))
}

func TestNormalizeOpenAIReplayItemsPreservesCompleteStandardCall(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"pwd\"}","status":"completed"}`),
	}
	normalized := normalizeOpenAIReplayItems(items)
	require.Len(t, normalized, 1)
	require.Equal(t, "fc_1", gjson.GetBytes(normalized[0], "id").String())
	require.Equal(t, "call_1", gjson.GetBytes(normalized[0], "call_id").String())
	require.JSONEq(t, `{"cmd":"pwd"}`, gjson.GetBytes(normalized[0], "arguments").String())
}

func TestAlignOpenAIReplayOutputTypesToCallsRepairsMixedPairOnly(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"function_call","id":"fc_1","call_id":"fc_yxHnti6s5fgUU2ZJISVoSHN6","name":"apply_patch","arguments":"{\"patch\":\"x\"}"}`),
		json.RawMessage(`{"type":"custom_tool_call_output","id":"ctco_1","call_id":"fc_yxHnti6s5fgUU2ZJISVoSHN6","output":"Done"}`),
		json.RawMessage(`{"type":"custom_tool_call","id":"ctc_2","call_id":"call_native","name":"search","input":"q"}`),
		json.RawMessage(`{"type":"custom_tool_call_output","id":"ctco_2","call_id":"call_native","output":"keep"}`),
		json.RawMessage(`{"type":"custom_tool_call_output","id":"ctco_3","call_id":"call_orphan","output":"orphan"}`),
	}
	normalized := alignOpenAIReplayOutputTypesToCalls(items)

	require.Len(t, normalized, 5)
	require.Equal(t, "function_call", gjson.GetBytes(normalized[0], "type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(normalized[1], "type").String())
	require.Equal(t, "fc_yxHnti6s5fgUU2ZJISVoSHN6", gjson.GetBytes(normalized[0], "call_id").String())
	require.Equal(t, "fc_yxHnti6s5fgUU2ZJISVoSHN6", gjson.GetBytes(normalized[1], "call_id").String())
	require.Equal(t, "custom_tool_call", gjson.GetBytes(normalized[2], "type").String())
	require.Equal(t, "custom_tool_call_output", gjson.GetBytes(normalized[3], "type").String())
	require.Equal(t, "custom_tool_call_output", gjson.GetBytes(normalized[4], "type").String())
}

func TestAlignOpenAIReplayOutputTypesToCallsResolvesUniqueItemIDAlias(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"function_call","id":"fc_alias","call_id":"call_real","name":"apply_patch","arguments":"{}"}`),
		json.RawMessage(`{"type":"custom_tool_call_output","call_id":"fc_alias","output":"Done"}`),
	}
	aligned := alignOpenAIReplayOutputTypesToCalls(items)

	require.Len(t, aligned, 2)
	require.Equal(t, "function_call", gjson.GetBytes(aligned[0], "type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(aligned[1], "type").String())
	require.Equal(t, "call_real", gjson.GetBytes(aligned[1], "call_id").String())
}

func TestAlignOpenAIReplayOutputTypesToCallsRefusesAmbiguousItemIDAlias(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"function_call","id":"fc_alias","call_id":"call_1","name":"one","arguments":"{}"}`),
		json.RawMessage(`{"type":"function_call","id":"fc_alias","call_id":"call_2","name":"two","arguments":"{}"}`),
		json.RawMessage(`{"type":"custom_tool_call_output","call_id":"fc_alias","output":"Done"}`),
	}
	aligned := alignOpenAIReplayOutputTypesToCalls(items)

	require.Equal(t, "custom_tool_call_output", gjson.GetBytes(aligned[2], "type").String())
	require.Equal(t, "fc_alias", gjson.GetBytes(aligned[2], "call_id").String())
}

func TestNormalizeOpenAIRejectedReplayPairByCallIDRefusesTrueOrphan(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[{"type":"custom_tool_call_output","call_id":"fc_orphan","output":"Done"}]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairByCallID(body, "fc_orphan")

	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, retryBody)
}

func TestNormalizeOpenAIRejectedReplayPairByCallIDOnlyChangesMatchingPair(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[
		{"type":"function_call","call_id":"fc_target","name":"patch","arguments":"{}"},
		{"type":"custom_tool_call_output","call_id":"fc_target","output":"Done"},
		{"type":"custom_tool_call","call_id":"call_keep","name":"search","input":"q"},
		{"type":"custom_tool_call_output","call_id":"call_keep","output":"keep"}
	]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairByCallID(body, "fc_target")

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function_call", gjson.GetBytes(retryBody, "input.0.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.1.type").String())
	require.Equal(t, "custom_tool_call", gjson.GetBytes(retryBody, "input.2.type").String())
	require.Equal(t, "custom_tool_call_output", gjson.GetBytes(retryBody, "input.3.type").String())
}

func TestNormalizeOpenAIRejectedReplayPairByCallIDResolvesUniqueItemIDAlias(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[
		{"type":"function_call","id":"fc_alias","call_id":"call_real","name":"patch","arguments":"{}"},
		{"type":"custom_tool_call_output","call_id":"fc_alias","output":"Done"}
	]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairByCallID(body, "fc_alias")

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function_call", gjson.GetBytes(retryBody, "input.0.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.1.type").String())
	require.Equal(t, "call_real", gjson.GetBytes(retryBody, "input.1.call_id").String())
}

func TestNormalizeOpenAIRejectedReplayPairByCallIDResolvesCallToFCPrefixAlias(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[
		{"type":"custom_tool_call","id":"ctc_1","call_id":"call_wJfEmRwT5FY7GEbWbYh5K7vq","name":"exec","input":"go test"},
		{"type":"custom_tool_call_output","id":"ctco_1","call_id":"call_wJfEmRwT5FY7GEbWbYh5K7vq","output":"ok"}
	]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairByCallID(body, "fc_wJfEmRwT5FY7GEbWbYh5K7vq")

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function_call", gjson.GetBytes(retryBody, "input.0.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.1.type").String())
	require.Equal(t, "fc_wJfEmRwT5FY7GEbWbYh5K7vq", gjson.GetBytes(retryBody, "input.0.call_id").String())
	require.Equal(t, "fc_wJfEmRwT5FY7GEbWbYh5K7vq", gjson.GetBytes(retryBody, "input.1.call_id").String())
}

// Real session shape for fc_bKQeo...: client still holds call_* on both sides,
// while the upstream rejection message already rewrote the association key to
// fc_*. Repair must rewrite both items even though neither side is exact-match
// on the error call_id until prefix alias resolution.
func TestNormalizeOpenAIRejectedReplayPairByCallIDRepairsSessionShapeCallBPrefix(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[
		{"type":"custom_tool_call","id":"ctc_03ba88029cd0a4ab016a7337381ffc819989120c6843421dc3","call_id":"call_bKQeoEfitPNuvbVgV3DsQMuw","name":"exec","input":"ssh sub2api-prod"},
		{"type":"custom_tool_call_output","id":"ctco_019fd20f-becb-7e22-b7b3-d63f3f82065a","call_id":"call_bKQeoEfitPNuvbVgV3DsQMuw","output":"PUBLIC=0.1.209"}
	]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairByCallID(body, "fc_bKQeoEfitPNuvbVgV3DsQMuw")

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function_call", gjson.GetBytes(retryBody, "input.0.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.1.type").String())
	require.Equal(t, "fc_bKQeoEfitPNuvbVgV3DsQMuw", gjson.GetBytes(retryBody, "input.0.call_id").String())
	require.Equal(t, "fc_bKQeoEfitPNuvbVgV3DsQMuw", gjson.GetBytes(retryBody, "input.1.call_id").String())
}

// Mixed pair: server-collected replay function_call already uses fc_*, while the
// client delta still emits custom_tool_call_output with call_*. Direct call match
// plus prefix-equivalent output must both land on the error-reported call_id.
func TestNormalizeOpenAIRejectedReplayPairByCallIDRepairsDirectCallWithPrefixOutput(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[
		{"type":"function_call","call_id":"fc_bKQeoEfitPNuvbVgV3DsQMuw","name":"exec","arguments":"{\"input\":\"ssh\"}"},
		{"type":"custom_tool_call_output","call_id":"call_bKQeoEfitPNuvbVgV3DsQMuw","output":"ok"}
	]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairByCallID(body, "fc_bKQeoEfitPNuvbVgV3DsQMuw")

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function_call", gjson.GetBytes(retryBody, "input.0.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.1.type").String())
	require.Equal(t, "fc_bKQeoEfitPNuvbVgV3DsQMuw", gjson.GetBytes(retryBody, "input.0.call_id").String())
	require.Equal(t, "fc_bKQeoEfitPNuvbVgV3DsQMuw", gjson.GetBytes(retryBody, "input.1.call_id").String())
}

func TestAlignOpenAIReplayOutputTypesToCallsResolvesCallFCPrefixMismatch(t *testing.T) {
	t.Parallel()

	items := []json.RawMessage{
		json.RawMessage(`{"type":"function_call","call_id":"fc_bKQeoEfitPNuvbVgV3DsQMuw","name":"exec","arguments":"{\"input\":\"ssh\"}"}`),
		json.RawMessage(`{"type":"custom_tool_call_output","call_id":"call_bKQeoEfitPNuvbVgV3DsQMuw","output":"ok"}`),
	}
	aligned := alignOpenAIReplayOutputTypesToCalls(items)

	require.Equal(t, "function_call", gjson.GetBytes(aligned[0], "type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(aligned[1], "type").String())
	require.Equal(t, "fc_bKQeoEfitPNuvbVgV3DsQMuw", gjson.GetBytes(aligned[1], "call_id").String())
}

func TestNormalizeOpenAIRejectedReplayPairByCallIDRefusesAmbiguousPrefixAlias(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[
		{"type":"custom_tool_call","call_id":"call_same","name":"one","input":"x"},
		{"type":"custom_tool_call","call_id":"call_same","name":"two","input":"y"},
		{"type":"custom_tool_call_output","call_id":"call_same","output":"ok"}
	]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairByCallID(body, "fc_same")

	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, retryBody)
}

func TestNormalizeOpenAIRejectedReplayPairByCallIDPrefersDirectCallIDOverAlias(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[
		{"type":"function_call","id":"fc_other","call_id":"fc_target","name":"direct","arguments":"{}"},
		{"type":"function_call","id":"fc_target","call_id":"call_alias","name":"alias","arguments":"{}"},
		{"type":"custom_tool_call_output","call_id":"fc_target","output":"Done"}
	]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairByCallID(body, "fc_target")

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.2.type").String())
	require.Equal(t, "fc_target", gjson.GetBytes(retryBody, "input.2.call_id").String())
}

func TestNormalizeOpenAIRejectedReplayPairAtIndexOnlyChangesCallLinkedItems(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"gpt-5.5","input":[
		{"type":"message","role":"user","content":"keep"},
		{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"patch","input":"x"},
		{"type":"custom_tool_call_output","id":"ctco_1","call_id":"call_1","output":"ok"},
		{"type":"tool_search_call","id":"tsc_2","call_id":"call_2","name":"search","arguments":"{\"q\":\"keep\"}"}
	]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairAtIndex(body, 1)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "keep", gjson.GetBytes(retryBody, "input.0.content").String())
	require.Equal(t, "function_call", gjson.GetBytes(retryBody, "input.1.type").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.2.type").String())
	require.Equal(t, "tool_search_call", gjson.GetBytes(retryBody, "input.3.type").String())
	require.Equal(t, "call_2", gjson.GetBytes(retryBody, "input.3.call_id").String())
}

func TestNormalizeOpenAIRejectedReplayPairAtOutputWithoutCallID(t *testing.T) {
	t.Parallel()

	body := []byte(`{"model":"gpt-5.5","input":[
		{"type":"local_shell_call","id":"lc_shared","call_id":"call_1","name":"shell","action":{"command":["pwd"]}},
		{"type":"local_shell_call_output","id":"ctco_shared","output":"ok"},
		{"type":"message","role":"user","content":"keep"}
	]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairAtIndex(body, 1)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function_call", gjson.GetBytes(retryBody, "input.0.type").String())
	require.Equal(t, "call_1", gjson.GetBytes(retryBody, "input.0.call_id").String())
	require.Equal(t, "function_call_output", gjson.GetBytes(retryBody, "input.1.type").String())
	require.Equal(t, "call_1", gjson.GetBytes(retryBody, "input.1.call_id").String())
	require.Equal(t, "message", gjson.GetBytes(retryBody, "input.2.type").String())
}

func TestNormalizeOpenAIRejectedReplayPairAtIndexRejectsAmbiguousIdentity(t *testing.T) {
	t.Parallel()

	body := []byte(`{"input":[
		{"type":"custom_tool_call","id":"ctc_shared","call_id":"call_1","name":"patch","input":"x"},
		{"type":"custom_tool_call","id":"ctc_shared","call_id":"call_2","name":"patch","input":"y"},
		{"type":"custom_tool_call_output","id":"ctco_shared","output":"ok"}
	]}`)
	retryBody, changed, err := normalizeOpenAIRejectedReplayPairAtIndex(body, 2)

	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, retryBody)
}

func FuzzNormalizeOpenAIReplayItemsProducesValidJSON(f *testing.F) {
	f.Add(`{"type":"custom_tool_call","id":"ctc_1","call_id":"call_1","name":"patch","input":"x"}`)
	f.Add(`{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":""}`)
	f.Add(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}`)
	f.Add(`not-json`)

	f.Fuzz(func(t *testing.T, raw string) {
		for _, normalized := range normalizeOpenAIReplayItems([]json.RawMessage{json.RawMessage(raw)}) {
			require.True(t, json.Valid(normalized))
		}
	})
}

func FuzzNormalizeOpenAIRejectedReplayPairAtIndexNoPanic(f *testing.F) {
	f.Add(`{"input":[{"type":"custom_tool_call","call_id":"call_1","name":"patch","input":"x"}]}`, 0)
	f.Add(`{"input":[]}`, 109)
	f.Add(`not-json`, 0)

	f.Fuzz(func(t *testing.T, body string, index int) {
		retryBody, changed, err := normalizeOpenAIRejectedReplayPairAtIndex([]byte(body), index)
		if err == nil && changed {
			require.True(t, json.Valid(retryBody))
		}
	})
}

package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokWSRequestIDMapperMapsPreviousResponseID(t *testing.T) {
	sessionID := "grok-id-map-session-1"
	defer deleteGrokWSIDState(sessionID)

	state := getGrokWSIDState(sessionID)
	require.NotNil(t, state)
	state.mapDownstreamToUpstream("resp-down-1", "resp-up-1")

	mapper := newGrokWSRequestIDMapper(sessionID, []byte(`{"previous_response_id":"resp-down-1","input":[{"type":"input_text","text":"continue"}]}`))
	require.NotNil(t, mapper)
	require.Equal(t, "resp-up-1", mapper.upstreamPreviousID)

	upstreamPayload := mapper.upstreamRequestPayload([]byte(`{"type":"response.create","previous_response_id":"resp-down-1","input":[{"type":"input_text","text":"continue"}]}`))
	require.Equal(t, "resp-up-1", gjson.GetBytes(upstreamPayload, "previous_response_id").String())
}

func TestGrokWSRequestIDMapperRewritesRepeatedResponseID(t *testing.T) {
	sessionID := "grok-id-map-session-2"
	defer deleteGrokWSIDState(sessionID)

	firstMapper := newGrokWSRequestIDMapper(sessionID, []byte(`{"input":[{"type":"input_text","text":"hello"}]}`))
	require.NotNil(t, firstMapper)
	firstCompleted := []byte(`{"type":"response.completed","response":{"id":"resp-real","previous_response_id":"","output":[{"id":"rs_resp-real","type":"reasoning","status":"completed"}]}}`)
	firstDownstream := firstMapper.downstreamResponsePayload(firstCompleted)
	require.Equal(t, "resp-real", gjson.GetBytes(firstDownstream, "response.id").String())
	require.Equal(t, "rs_resp-real", gjson.GetBytes(firstDownstream, "response.output.0.id").String())
	firstMapper.recordTranscriptTurn([]byte(`{"type":"response.create","input":[{"type":"input_text","text":"hello"}]}`), firstDownstream)

	secondMapper := newGrokWSRequestIDMapper(sessionID, []byte(`{"previous_response_id":"resp-real","input":[{"type":"function_call_output","call_id":"call-1","output":"ok"}]}`))
	require.NotNil(t, secondMapper)
	require.Equal(t, "resp-real", secondMapper.upstreamPreviousID)
	secondUpstream := secondMapper.upstreamRequestPayload([]byte(`{"type":"response.create","previous_response_id":"resp-real","input":[{"type":"function_call_output","call_id":"call-1","output":"ok"}]}`))
	require.Equal(t, "resp-real", gjson.GetBytes(secondUpstream, "previous_response_id").String())

	secondCompleted := []byte(`{"type":"response.completed","response":{"id":"resp-real","previous_response_id":"resp-real","output":[{"id":"rs_resp-real","type":"reasoning","status":"completed"}]}}`)
	secondDownstream := secondMapper.downstreamResponsePayload(secondCompleted)
	secondDownstreamID := gjson.GetBytes(secondDownstream, "response.id").String()
	require.NotEmpty(t, secondDownstreamID)
	require.NotEqual(t, "resp-real", secondDownstreamID)
	require.True(t, strings.HasPrefix(secondDownstreamID, "resp-real-xai-"))
	require.Equal(t, "resp-real", gjson.GetBytes(secondDownstream, "response.previous_response_id").String())
	require.Contains(t, gjson.GetBytes(secondDownstream, "response.output.0.id").String(), secondDownstreamID)

	thirdMapper := newGrokWSRequestIDMapper(sessionID, []byte(fmt.Sprintf(`{"previous_response_id":%q,"input":[{"type":"function_call_output","call_id":"call-2","output":"ok"}]}`, secondDownstreamID)))
	require.NotNil(t, thirdMapper)
	require.Equal(t, "resp-real", thirdMapper.upstreamPreviousID)
}

func TestGrokWSRequestIDMapperPassesThroughUnknownPreviousResponseID(t *testing.T) {
	sessionID := "grok-id-map-session-3"
	defer deleteGrokWSIDState(sessionID)

	mapper := newGrokWSRequestIDMapper(sessionID, []byte(`{"previous_response_id":"resp-unknown","input":[{"type":"input_text","text":"continue"}]}`))
	require.NotNil(t, mapper)
	require.Equal(t, "resp-unknown", mapper.upstreamPreviousID)

	upstreamPayload := mapper.upstreamRequestPayload([]byte(`{"type":"response.create","previous_response_id":"resp-unknown","input":[{"type":"input_text","text":"continue"}]}`))
	require.Equal(t, "resp-unknown", gjson.GetBytes(upstreamPayload, "previous_response_id").String())
	require.Len(t, gjson.GetBytes(upstreamPayload, "input").Array(), 1)
}
package cursor

import (
	"bytes"
	"io"
	"testing"

	cursorpb "github.com/Wei-Shaw/sub2api/internal/pkg/cursor/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestEnvelopeRoundtrip(t *testing.T) {
	req := &cursorpb.InferenceStreamRequest{
		ModelId: proto.String("grok-4.5"),
		Messages: []*cursorpb.InferenceCoreMessage{
			{
				Role: cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
				Content: &cursorpb.InferenceCoreMessage_Text{
					Text: "Hello",
				},
			},
		},
	}

	frame, err := EncodeEnvelope(req)
	require.NoError(t, err)
	assert.Greater(t, len(frame), 5)
	assert.Equal(t, ConnectFlagData, frame[0])

	r := bytes.NewReader(frame)
	flag, payload, err := ReadEnvelope(r)
	require.NoError(t, err)
	assert.Equal(t, ConnectFlagData, flag)

	decoded := &cursorpb.InferenceStreamRequest{}
	err = proto.Unmarshal(payload, decoded)
	require.NoError(t, err)
	assert.Equal(t, "grok-4.5", decoded.GetModelId())
	assert.Len(t, decoded.GetMessages(), 1)
	assert.Equal(t, "Hello", decoded.GetMessages()[0].GetText())

	// Next read should be io.EOF
	_, _, err = ReadEnvelope(r)
	assert.Equal(t, io.EOF, err)
}

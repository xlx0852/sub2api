package cursor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	cursorpb "github.com/Wei-Shaw/sub2api/internal/pkg/cursor/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestClient_StreamInference_Mock(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/aiserver.v1.InferenceService/Stream", r.URL.Path)
		assert.Equal(t, "application/connect+proto", r.Header.Get("Content-Type"))
		assert.NotEmpty(t, r.Header.Get("x-cursor-checksum"))
		assert.Equal(t, "sand", r.Header.Get("x-cursor-client-type"))

		w.Header().Set("Content-Type", "application/connect+proto")
		w.WriteHeader(http.StatusOK)

		// Frame 1: TextPart "Hello"
		f1, _ := EncodeEnvelope(&cursorpb.InferenceStreamResponse{
			Response: &cursorpb.InferenceStreamResponse_TextPart{
				TextPart: &cursorpb.InferenceTextStreamPart{Text: "Hello "},
			},
		})
		w.Write(f1)

		// Frame 2: TextPart "World"
		f2, _ := EncodeEnvelope(&cursorpb.InferenceStreamResponse{
			Response: &cursorpb.InferenceStreamResponse_TextPart{
				TextPart: &cursorpb.InferenceTextStreamPart{Text: "World"},
			},
		})
		w.Write(f2)

		// Frame 3: EndStream
		endBuf := []byte{ConnectFlagEndStream, 0, 0, 0, 0}
		w.Write(endBuf)
	}))
	defer ts.Close()

	client := NewClient("test-token", "mach-1", ts.Client(), ts.URL)
	stream, err := client.StreamInference(context.Background(), &cursorpb.InferenceStreamRequest{
		ModelId: proto.String("default"),
	})
	require.NoError(t, err)
	defer stream.Close()

	msg1, err := stream.Recv()
	require.NoError(t, err)
	assert.Equal(t, "Hello ", msg1.GetTextPart().GetText())

	msg2, err := stream.Recv()
	require.NoError(t, err)
	assert.Equal(t, "World", msg2.GetTextPart().GetText())

	_, err = stream.Recv()
	assert.Equal(t, "EOF", err.Error())
}

func TestClient_Live_Stream(t *testing.T) {
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJncm9rfHVzZXJfMDFNMVhLRU5DOVdNUkpRVE5QRVhISFJYVDIiLCJ0aW1lIjoiMTc4ODg4NzI2OSIsInJhbmRvbW5lc3MiOiIzNmQzZmQ2Ni05MWMyLTRlODkiLCJleHAiOjE3OTQwNzEyNjksImlzcyI6Imh0dHBzOi8vYXV0aGVudGljYXRpb24uY3Vyc29yLnNoIiwic2NvcGUiOiJvcGVuaWQgcHJvZmlsZSBlbWFpbCBvZmZsaW5lX2FjY2VzcyIsImF1ZCI6Imh0dHBzOi8vY3Vyc29yLmNvbSIsInR5cGUiOiJzZXNzaW9uIn0.j4_oQ_7Ws9hh8UB6CrBLLOKyH21qwuzbPI57EBQiCE0"
	machineID := "fd065226-77db-45bf-abda-3e11d4f26c67"

	client := NewClient(token, machineID, nil, DefaultBackendURL)
	req := &cursorpb.InferenceStreamRequest{
		ModelId: proto.String("default"),
		RequestedModel: &cursorpb.InferenceRequestedModel{
			ModelId: "default",
			MaxMode: false,
		},
		Messages: []*cursorpb.InferenceCoreMessage{
			{
				Role: cursorpb.InferenceMessageRole_INFERENCE_MESSAGE_ROLE_USER,
				Content: &cursorpb.InferenceCoreMessage_Text{
					Text: "Say 'Go Client Works!' and nothing else.",
				},
			},
		},
	}

	stream, err := client.StreamInference(context.Background(), req)
	require.NoError(t, err)
	defer stream.Close()

	var collected string
	for {
		msg, err := stream.Recv()
		if err != nil {
			t.Logf("Stream finished with: %v", err)
			break
		}
		t.Logf("Received response: %+v", msg)
		if msg.GetTextPart() != nil {
			collected += msg.GetTextPart().GetText()
		}
		if msg.GetThinkingPart() != nil {
			collected += msg.GetThinkingPart().GetText()
		}
	}

	t.Logf("Collected output from live Cursor: %q", collected)
	assert.NotEmpty(t, collected)
}

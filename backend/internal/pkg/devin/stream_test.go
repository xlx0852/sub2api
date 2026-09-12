package devin

import (
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	devinpb "github.com/Wei-Shaw/sub2api/internal/pkg/devin/proto"
	"google.golang.org/protobuf/proto"
)

func TestStreamUsesDevinConnectPathAndAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/exa.api_server_pb.ApiServerService/GetChatMessage" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Basic devin-token$test-devin-token$test" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/connect+proto" {
			t.Fatalf("content type = %q", got)
		}
		_, _ = io.Copy(io.Discard, r.Body)
		payload, err := proto.Marshal(&devinpb.GetChatMessageResponse{DeltaText: "hello"})
		if err != nil {
			t.Fatal(err)
		}
		trailer := []byte(`{}`)
		frame := make([]byte, 5+len(payload)+5+len(trailer))
		frame[0] = 0
		binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
		copy(frame[5:], payload)
		frame[5+len(payload)] = 2
		binary.BigEndian.PutUint32(frame[5+len(payload)+1:], uint32(len(trailer)))
		copy(frame[5+len(payload)+5:], trailer)
		w.Header().Set("Content-Type", "application/connect+proto")
		_, _ = w.Write(frame)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "devin-token$test", "glm-5-2", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Stream(t.Context(), &devinpb.GetChatMessageRequest{ChatModelUid: "glm-5-2"})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetDeltaText() != "hello" {
		t.Fatalf("delta text = %q", resp.GetDeltaText())
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("second recv error = %v, want EOF", err)
	}
}

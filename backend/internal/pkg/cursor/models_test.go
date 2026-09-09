package cursor

import (
	"context"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAvailableModelsWireFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/aiserver.v1.AiService/AvailableModels", r.URL.Path)
		require.Equal(t, "application/proto", r.Header.Get("Content-Type"))
		b, _ := io.ReadAll(r.Body)
		require.Contains(t, string(b), string([]byte{0x50, 1}))
		require.Contains(t, string(b), string([]byte{0x10, 1}))
		// Captured schema: response models field 2, name 1, display name 17, aliases 37.
		m := []byte{0x0a, 1, 'x', 0x8a, 1, 1, 'X', 0xaa, 2, 1, 'y'}
		w.Write(append([]byte{0x12, byte(len(m))}, m...))
	}))
	defer srv.Close()
	result, err := NewClient("token", "machine", srv.Client(), srv.URL).AvailableModels(context.Background())
	require.NoError(t, err)
	require.Len(t, result.Models, 1)
	require.Equal(t, "x", result.Models[0].Name)
	require.Equal(t, "X", result.Models[0].ClientDisplayName)
	require.Equal(t, []string{"y"}, result.Models[0].IdAliases)
}

package repository

import (
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/imroc/req/v3"
	"github.com/stretchr/testify/require"
)

func forceHTTPVersion(t *testing.T, client *req.Client) string {
	t.Helper()
	transport := client.GetTransport()
	field := reflect.ValueOf(transport).Elem().FieldByName("forceHttpVersion")
	require.True(t, field.IsValid(), "forceHttpVersion field not found")
	require.True(t, field.CanAddr(), "forceHttpVersion field not addressable")
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().String()
}

func TestGetSharedReqClient_ForceHTTP2SeparatesCache(t *testing.T) {
	pool := NewReqClientPool()
	base := reqClientOptions{
		ProxyURL: "http://proxy.local:8080",
		Timeout:  time.Second,
	}
	clientDefault, err := pool.get(base)
	require.NoError(t, err)

	force := base
	force.ForceHTTP2 = true
	clientForce, err := pool.get(force)
	require.NoError(t, err)

	require.NotSame(t, clientDefault, clientForce)
	require.NotEqual(t, buildReqClientKey(base), buildReqClientKey(force))
}

func TestGetSharedReqClient_ReuseCachedClient(t *testing.T) {
	pool := NewReqClientPool()
	opts := reqClientOptions{
		ProxyURL: "http://proxy.local:8080",
		Timeout:  2 * time.Second,
	}
	first, err := pool.get(opts)
	require.NoError(t, err)
	second, err := pool.get(opts)
	require.NoError(t, err)
	require.Same(t, first, second)
}

func TestGetSharedReqClient_IgnoresNonClientCache(t *testing.T) {
	pool := NewReqClientPool()
	opts := reqClientOptions{
		ProxyURL: " http://proxy.local:8080 ",
		Timeout:  3 * time.Second,
	}
	key := buildReqClientKey(opts)
	pool.clients.Store(key, "invalid")

	client, err := pool.get(opts)
	require.NoError(t, err)

	require.NotNil(t, client)
	loaded, ok := pool.clients.Load(key)
	require.True(t, ok)
	require.IsType(t, "invalid", loaded)
}

func TestGetSharedReqClient_ImpersonateAndProxy(t *testing.T) {
	pool := NewReqClientPool()
	opts := reqClientOptions{
		ProxyURL:    "  http://proxy.local:8080  ",
		Timeout:     4 * time.Second,
		Impersonate: true,
	}
	client, err := pool.get(opts)
	require.NoError(t, err)

	require.NotNil(t, client)
	require.Equal(t, "http://proxy.local:8080|4s|true|false", buildReqClientKey(opts))
}

func TestGetSharedReqClient_InvalidProxyURL(t *testing.T) {
	_, err := NewReqClientPool().get(reqClientOptions{
		ProxyURL: "://missing-scheme",
		Timeout:  time.Second,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid proxy URL")
}

func TestGetSharedReqClient_ProxyURLMissingHost(t *testing.T) {
	_, err := NewReqClientPool().get(reqClientOptions{
		ProxyURL: "http://",
		Timeout:  time.Second,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "proxy URL missing host")
}

func TestCreateOpenAIReqClient_Timeout120Seconds(t *testing.T) {
	client, err := createOpenAIReqClient(NewReqClientPool(), "http://proxy.local:8080")
	require.NoError(t, err)
	require.Equal(t, 120*time.Second, client.GetClient().Timeout)
}

func TestCreateGeminiReqClient_ForceHTTP2Disabled(t *testing.T) {
	client, err := createGeminiReqClient(NewReqClientPool(), "http://proxy.local:8080")
	require.NoError(t, err)
	require.Equal(t, "", forceHTTPVersion(t, client))
}

func TestReqClientPool_NilReceiverDoesNotCache(t *testing.T) {
	opts := reqClientOptions{ProxyURL: "http://proxy.local:8080", Timeout: time.Second}
	first, err := (*ReqClientPool)(nil).get(opts)
	require.NoError(t, err)
	second, err := (*ReqClientPool)(nil).get(opts)
	require.NoError(t, err)
	require.NotSame(t, first, second)
}

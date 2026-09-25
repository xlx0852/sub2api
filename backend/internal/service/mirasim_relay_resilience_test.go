//go:build unit

package service

// mirasim relay 健壮性单测：device ticket 的 404/501 降级与退避、access token 的
// 提前刷新窗口（15m lead / 30s stale）、目录收敛、客户端版本默认值。
//
// 关键约束（改动前请先读）：
//   - 静默期内必须改用 access token 作为签名凭据，且**不再**向上游申领 ticket；
//   - 退避期内若有未过期 ticket 必须继续用它，而不是把一次上游抖动升级成请求失败；
//   - 只有距过期 30s 以内才允许把 refresh 失败暴露给调用方。

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// mirasimStatusUpstream 按端点 + 指定状态序列回答，用于 ticket 降级/退避场景。
type mirasimStatusUpstream struct {
	sessionStatus int
	sessionCalls  int
	sessionJSON   string
	refreshCalls  int
	refreshStatus int
	refreshJSON   string
}

func (u *mirasimStatusUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	switch {
	case strings.HasSuffix(req.URL.Path, mirasimAccessRefreshPath):
		u.refreshCalls++
		if u.refreshStatus != 0 && u.refreshStatus != http.StatusOK {
			return mirasimJSONResponse(u.refreshStatus, `{"error":"synthetic"}`), nil
		}
		payload := u.refreshJSON
		if payload == "" {
			payload = `{"access_token":"at","refresh_token":"rt2","expires_in":3600}`
		}
		return mirasimJSONResponse(http.StatusOK, payload), nil
	case strings.HasSuffix(req.URL.Path, mirasimDeviceSessionPath):
		u.sessionCalls++
		status := u.sessionStatus
		if status == 0 {
			status = http.StatusOK
		}
		payload := u.sessionJSON
		if payload == "" {
			payload = `{"ticket":"t","expiresIn":600}`
		}
		if status != http.StatusOK {
			payload = `{"error":"synthetic"}`
		}
		return mirasimJSONResponse(status, payload), nil
	}
	return mirasimJSONResponse(http.StatusOK, `{}`), nil
}

func (u *mirasimStatusUpstream) DoWithTLS(req *http.Request, proxyURL string, accountID int64, concurrency int, _ *domain.TLSFingerprintRuntimeProfile) (*http.Response, error) {
	return u.Do(req, proxyURL, accountID, concurrency)
}

func mirasimJSONResponse(status int, payload string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(payload)),
	}
}

func mirasimResilienceClient(t *testing.T, up HTTPUpstream) *mirasimRelayClient {
	t.Helper()
	priv, err := mirasimGenerateDeviceKey()
	require.NoError(t, err)
	deviceID, devicePub := mirasimDeviceIdentity(priv)
	sealPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	require.NoError(t, err)
	return &mirasimRelayClient{
		accountID:     77,
		relayURL:      "https://relay.test",
		authURL:       "https://auth.test",
		upstream:      up,
		refreshToken:  "rt",
		priv:          priv,
		deviceID:      deviceID,
		devicePubB64:  devicePub,
		clientVersion: mirasimDefaultClientVersion,
		sealPubkey:    base64.StdEncoding.EncodeToString(sealPriv.PublicKey().Bytes()),
	}
}

func TestMirasimTicketDegradesWhenRelayHasNoDeviceSessionRoute(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		quiet  time.Duration
	}{
		{"404 route absent", http.StatusNotFound, mirasimTicketRouteAbsentQuiet},
		{"501 unimplemented", http.StatusNotImplemented, mirasimTicketUnimplementedQuiet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			up := &mirasimStatusUpstream{sessionStatus: tc.status}
			c := mirasimResilienceClient(t, up)

			// 第一次：申领失败 → 降级为 access token 作为签名凭据。
			cred, err := c.ticket(context.Background())
			require.NoError(t, err, "静默期内必须降级而不是让请求失败")
			require.Equal(t, "at", cred, "降级凭据应当是 access token")
			require.Equal(t, 1, up.sessionCalls)

			// 静默期内再次调用：不得再申领 ticket。
			cred2, err := c.ticket(context.Background())
			require.NoError(t, err)
			require.Equal(t, "at", cred2)
			require.Equal(t, 1, up.sessionCalls, "静默期内不得再次申领 device ticket")
			require.WithinDuration(t, time.Now().Add(tc.quiet), c.ticketUnmintableUntil, 5*time.Second)
		})
	}
}

func TestMirasimTicketMintBackoffKeepsStaleTicket(t *testing.T) {
	up := &mirasimStatusUpstream{sessionStatus: http.StatusInternalServerError}
	c := mirasimResilienceClient(t, up)

	// 先放一张进入提前刷新窗口、但离过期仍有 90s 的 ticket
	// （ticketRefreshLead=2m，所以会尝试申领；staleLead=30s，所以退避期间仍可用）。
	c.mu.Lock()
	c.cachedTicket = "stale-ticket"
	c.ticketExp = time.Now().Add(90 * time.Second)
	c.mu.Unlock()

	// 第一次失败：进入退避，但仍返回未过期 ticket（不让请求失败）。
	cred, err := c.ticket(context.Background())
	require.NoError(t, err)
	require.Equal(t, "stale-ticket", cred)
	require.Equal(t, 1, up.sessionCalls)
	require.Greater(t, c.ticketBackoff, time.Duration(0))
	firstBackoff := c.ticketBackoff

	// 退避窗口内：不再申领。
	cred2, err := c.ticket(context.Background())
	require.NoError(t, err)
	require.Equal(t, "stale-ticket", cred2)
	require.Equal(t, 1, up.sessionCalls, "退避窗口内不得再次申领")
	require.Equal(t, firstBackoff, c.ticketBackoff)

	// 退避结束 + ticket 已过期：必须报错（调用方可 failover），且退避翻倍。
	c.mu.Lock()
	c.ticketRetryAt = time.Now().Add(-time.Second)
	c.ticketExp = time.Now().Add(-time.Minute)
	c.mu.Unlock()
	_, err = c.ticket(context.Background())
	require.Error(t, err)
	require.Equal(t, 2, up.sessionCalls)
	require.Greater(t, c.ticketBackoff, firstBackoff, "失败退避必须指数增长")
}

func TestMirasimAccessRefreshDefersFailureInsideLeadWindow(t *testing.T) {
	up := &mirasimStatusUpstream{}
	c := mirasimResilienceClient(t, up)

	// token 仍在 15 分钟 lead 之外：直接用缓存，不打 refresh。
	c.mu.Lock()
	c.cachedAccess = "cached-at"
	c.accessExp = time.Now().Add(time.Hour)
	c.mu.Unlock()
	tok, err := c.accessToken(context.Background())
	require.NoError(t, err)
	require.Equal(t, "cached-at", tok)
	require.Equal(t, 0, up.refreshCalls)

	// 进入 lead 窗口但离过期还有 10 分钟：refresh 失败也必须继续用旧 token。
	c.mu.Lock()
	c.accessExp = time.Now().Add(10 * time.Minute)
	c.mu.Unlock()
	up.refreshCalls = 0
	up.refreshStatus = http.StatusInternalServerError
	tok, err = c.accessToken(context.Background())
	require.NoError(t, err)
	require.Equal(t, "cached-at", tok, "lead 窗口内 refresh 失败不得让请求失败")

	// 距过期 5 秒：必须刷新成功才继续。
	up.refreshStatus = 0
	c.mu.Lock()
	c.accessExp = time.Now().Add(5 * time.Second)
	c.mu.Unlock()
	tok, err = c.accessToken(context.Background())
	require.NoError(t, err)
	require.Equal(t, "at", tok, "stale 窗口内必须用刷新后的 token")
}

func TestMirasimAvailableModelsNarrowing(t *testing.T) {
	raw := []byte(`{"object":"list","data":[
		{"id":"*","object":"model"},
		{"id":"gpt-4o-mini","object":"model"},
		{"id":"gpt-4o-mini-openrouter","object":"model"},
		{"id":"anthropic/claude-sonnet-5","object":"model"},
		{"id":"claude-haiku-4-5","object":"model"},
		{"id":"claude-haiku-4-5-20251001","object":"model"},
		{"id":"deepseek-v4-flash-0731","object":"model"},
		{"id":"deepseek-flash","object":"model"}
	]}`)
	got, err := parseMirasimAvailableModels(raw)
	require.NoError(t, err)
	require.Equal(t, []string{
		"claude-haiku-4-5",
		"deepseek-flash",
		"deepseek-v4-flash-0731", // 没有无日期同名的孪生，必须保留
	}, got)
}

func TestMirasimDefaultClientVersionMatchesOfficialClient(t *testing.T) {
	require.Equal(t, "0.0.354", mirasimDefaultClientVersion,
		"x-mirasim-client 是明文可检测字段，默认值必须对齐官方客户端")
}

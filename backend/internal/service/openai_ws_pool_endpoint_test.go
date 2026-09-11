package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIWSConnPool_EndpointChangeMustNotReuseOldConnection(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 2
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 2
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCountingDialer{})
	account := &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	first, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{Account: account, WSURL: "wss://old.example/v1/responses"})
	require.NoError(t, err)
	id := first.ConnID()
	first.Release()
	second, err := pool.Acquire(context.Background(), openAIWSAcquireRequest{Account: account, WSURL: "wss://new.example/v1/responses"})
	require.NoError(t, err)
	defer second.Release()
	require.NotEqual(t, id, second.ConnID(), "a changed upstream endpoint must not reuse the old connection")
}

func TestOpenAIWSConnPool_RouteCompatibility(t *testing.T) {
	for _, mode := range []string{"idle", "pristine", "preferred", "strict"} {
		t.Run(mode, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(&openAIWSCountingDialer{})
			req := openAIWSAcquireRequest{Account: &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, WSURL: "wss://example.com/v1/responses", ProxyURL: "http://proxy-a:8080"}
			first, err := pool.Acquire(context.Background(), req)
			require.NoError(t, err)
			id := first.ConnID()
			first.Release()
			req.ProxyURL = "http://proxy-b:8080"
			req.RequirePristine = mode == "pristine"
			if mode == "preferred" || mode == "strict" {
				req.PreferredConnID = id
			}
			req.ForcePreferredConn = mode == "strict"
			second, err := pool.Acquire(context.Background(), req)
			if mode == "strict" {
				require.ErrorIs(t, err, errOpenAIWSPreferredConnUnavailable)
				return
			}
			require.NoError(t, err)
			defer second.Release()
			require.NotEqual(t, id, second.ConnID())
		})
	}
}

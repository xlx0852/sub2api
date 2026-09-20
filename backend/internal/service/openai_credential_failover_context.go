package service

import (
	"context"
	"net/http"
	"sync/atomic"
)

type openAICredentialFailoverKey struct{}
type openAICredentialFailoverState struct {
	class atomic.Pointer[OpenAICredentialClass]
}

// WithOpenAICredentialFailoverState installs an isolated request-lifetime guard.
// No client-supplied header can set this state; nothing is persisted in Redis.
func WithOpenAICredentialFailoverState(ctx context.Context) context.Context {
	return context.WithValue(ctx, openAICredentialFailoverKey{}, &openAICredentialFailoverState{})
}

func recordOpenAICredentialFailover(ctx context.Context, account *Account, status int) {
	if ctx == nil || account == nil || !account.IsOpenAI() || (status != http.StatusUnauthorized && status != http.StatusForbidden) {
		return
	}
	state, _ := ctx.Value(openAICredentialFailoverKey{}).(*openAICredentialFailoverState)
	if state == nil {
		return
	}
	class := OpenAIAccountCredentialClass(account)
	if class != OpenAICredentialClassUnspecified {
		state.class.CompareAndSwap(nil, &class)
	}
}

func openAICredentialFailoverClass(ctx context.Context) OpenAICredentialClass {
	if ctx != nil {
		if state, ok := ctx.Value(openAICredentialFailoverKey{}).(*openAICredentialFailoverState); ok && state != nil {
			if class := state.class.Load(); class != nil {
				return *class
			}
		}
	}
	return OpenAICredentialClassUnspecified
}

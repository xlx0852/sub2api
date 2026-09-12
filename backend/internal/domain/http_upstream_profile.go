// Package domain —— 由 OpenSpec change refactor-architecture P1 自 internal/service 机械搬移。
package domain

import (
	"context"
)

// HTTPUpstreamProfile marks HTTP upstream requests that need provider-specific
// transport policy.
type HTTPUpstreamProfile string

const (
	HTTPUpstreamProfileDefault     HTTPUpstreamProfile = ""
	HTTPUpstreamProfileOpenAI      HTTPUpstreamProfile = "openai"
	HTTPUpstreamProfileGrok        HTTPUpstreamProfile = "grok"
	HTTPUpstreamProfileAntigravity HTTPUpstreamProfile = "antigravity"
)

type httpUpstreamProfileContextKey struct{}

type httpUpstreamDisableRedirectsContextKey struct{}
type httpUpstreamAllowPrivateHostsContextKey struct{}

// WithHTTPUpstreamProfile injects an upstream transport profile into ctx.
func WithHTTPUpstreamProfile(ctx context.Context, profile HTTPUpstreamProfile) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if profile == HTTPUpstreamProfileDefault {
		return ctx
	}
	return context.WithValue(ctx, httpUpstreamProfileContextKey{}, profile)
}

// HTTPUpstreamProfileFromContext resolves the upstream transport profile from ctx.
func HTTPUpstreamProfileFromContext(ctx context.Context) HTTPUpstreamProfile {
	if ctx == nil {
		return HTTPUpstreamProfileDefault
	}
	profile, ok := ctx.Value(httpUpstreamProfileContextKey{}).(HTTPUpstreamProfile)
	if !ok {
		return HTTPUpstreamProfileDefault
	}
	switch profile {
	case HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileGrok, HTTPUpstreamProfileAntigravity:
		return profile
	default:
		return HTTPUpstreamProfileDefault
	}
}

// WithHTTPUpstreamRedirectsDisabled prevents credential-bearing probes from
// following redirects through the shared upstream client.
func WithHTTPUpstreamRedirectsDisabled(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, httpUpstreamDisableRedirectsContextKey{}, true)
}

// HTTPUpstreamRedirectsDisabled reports whether the context opts out of redirect
// following (used by probes that carry upstream credentials).
func HTTPUpstreamRedirectsDisabled(ctx context.Context) bool {
	return ctx != nil && ctx.Value(httpUpstreamDisableRedirectsContextKey{}) == true
}

// WithHTTPUpstreamPrivateHostsAllowed opts a trusted, operator-managed bridge
// request into the private sidecar network. Callers must only set this for an
// explicitly marked external bridge account.
func WithHTTPUpstreamPrivateHostsAllowed(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, httpUpstreamAllowPrivateHostsContextKey{}, true)
}

func HTTPUpstreamPrivateHostsAllowed(ctx context.Context) bool {
	return ctx != nil && ctx.Value(httpUpstreamAllowPrivateHostsContextKey{}) == true
}

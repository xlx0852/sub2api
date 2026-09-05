package service

import "context"

type skipAccountAcquireContextKey struct{}

func withSkipAccountAcquire(ctx context.Context) context.Context {
	return context.WithValue(ctx, skipAccountAcquireContextKey{}, true)
}

func shouldSkipAccountAcquire(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	skip, _ := ctx.Value(skipAccountAcquireContextKey{}).(bool)
	return skip
}

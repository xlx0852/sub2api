//go:build unit

package service

import "context"

// These compatibility shims keep the historical unit tests buildable while the
// production gateway uses AccountSelector and selectGatewayAccountOnce.
// They are test-only; new production code MUST use the unified selector.

func (s *GatewayService) selectAccountForModelWithPlatform(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	platform string,
) (*Account, error) {
	ctx = context.WithValue(ctx, gatewayPlatformHintContextKey{}, platform)
	result, err := s.selectAccountWithLoadAwarenessStrategy(
		ctx, groupID, sessionHash, requestedModel, excludedIDs, "", 0,
	)
	if result != nil {
		result.Release()
		return result.Account, err
	}
	return nil, err
}

func (s *GatewayService) selectAccountWithMixedScheduling(
	ctx context.Context,
	groupID *int64,
	sessionHash string,
	requestedModel string,
	excludedIDs map[int64]struct{},
	platform string,
) (*Account, error) {
	return s.selectAccountForModelWithPlatform(ctx, groupID, sessionHash, requestedModel, excludedIDs, platform)
}

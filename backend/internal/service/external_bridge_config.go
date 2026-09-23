package service

import (
	"net/http"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// validateExternalBridgeConfiguration guards administrative writes, not runtime
// reads: a marked account must use the credential type the bridge forwarder reads.
// Unknown, non-empty kinds remain supported for private generic bridges.
func validateExternalBridgeConfiguration(platform, accountType string, extra map[string]any) error {
	account := &Account{Platform: platform, Type: accountType, Extra: extra}
	if !account.IsExternalBridge() {
		return nil
	}
	if platform != PlatformOpenAI || accountType != AccountTypeAPIKey {
		return infraerrors.New(http.StatusBadRequest, "INVALID_EXTERNAL_BRIDGE_TYPE",
			"external bridges require an OpenAI API-key account")
	}
	if account.ExternalBridgeKind() == "" {
		return infraerrors.New(http.StatusBadRequest, "EXTERNAL_BRIDGE_KIND_REQUIRED",
			"external bridge kind is required")
	}
	return nil
}

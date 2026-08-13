package service

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

const (
	anthropicAPIKeyAuthSchemeExtraKey = domain.AnthropicAPIKeyAuthSchemeExtraKey

	AnthropicAPIKeyAuthSchemeXAPIKey             = domain.AnthropicAPIKeyAuthSchemeXAPIKey
	AnthropicAPIKeyAuthSchemeAuthorizationBearer = domain.AnthropicAPIKeyAuthSchemeAuthorizationBearer
)

func setAnthropicAPIKeyAuthHeader(header http.Header, account *Account, token string) {
	if account.GetAnthropicAPIKeyAuthScheme() == AnthropicAPIKeyAuthSchemeAuthorizationBearer {
		header.Set("Authorization", "Bearer "+token)
		return
	}
	header.Set("x-api-key", token)
}

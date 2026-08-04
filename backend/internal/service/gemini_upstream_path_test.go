package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeGeminiModelPathSegment(t *testing.T) {
	got, err := sanitizeGeminiModelPathSegment("gemini-2.5-pro")
	require.NoError(t, err)
	require.Equal(t, "gemini-2.5-pro", got)

	got, err = sanitizeGeminiModelPathSegment("models/gemini-2.5-flash")
	require.NoError(t, err)
	require.Equal(t, "gemini-2.5-flash", got)

	_, err = sanitizeGeminiModelPathSegment("../evil")
	require.Error(t, err)

	_, err = sanitizeGeminiModelPathSegment("gemini/../x")
	require.Error(t, err)

	_, err = sanitizeGeminiModelPathSegment("gemini:generateContent")
	require.Error(t, err)
}

func TestNormalizeGeminiAIStudioGETPath(t *testing.T) {
	got, err := normalizeGeminiAIStudioGETPath("/v1beta/models")
	require.NoError(t, err)
	require.Equal(t, "/v1beta/models", got)

	got, err = normalizeGeminiAIStudioGETPath("/v1beta/models/gemini-2.5-pro")
	require.NoError(t, err)
	require.Equal(t, "/v1beta/models/gemini-2.5-pro", got)

	got, err = normalizeGeminiAIStudioGETPath("/v1beta/models/gemini-2.5-pro/")
	require.NoError(t, err)
	require.Equal(t, "/v1beta/models/gemini-2.5-pro", got)

	_, err = normalizeGeminiAIStudioGETPath("/v1beta/models/../admin")
	require.Error(t, err)

	_, err = normalizeGeminiAIStudioGETPath("/v1beta/models/gemini/extra")
	require.Error(t, err)

	_, err = normalizeGeminiAIStudioGETPath("/v1beta/models/gemini-2.5-pro:generateContent")
	require.Error(t, err)

	_, err = normalizeGeminiAIStudioGETPath("/v1/files")
	require.Error(t, err)
}

func TestBuildGeminiAIStudioGetModelPath(t *testing.T) {
	got, err := BuildGeminiAIStudioGetModelPath("gemini-2.5-pro")
	require.NoError(t, err)
	require.Equal(t, "/v1beta/models/gemini-2.5-pro", got)

	_, err = BuildGeminiAIStudioGetModelPath("..%2fadmin")
	require.Error(t, err)
}

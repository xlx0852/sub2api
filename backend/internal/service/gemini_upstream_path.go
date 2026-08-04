package service

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// geminiModelPathPattern allows typical Gemini model IDs used in URL path segments.
// Rejects path separators / traversal / query fragments.
var geminiModelPathPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

var allowedGeminiModelActions = map[string]struct{}{
	"generateContent":       {},
	"streamGenerateContent": {},
	"countTokens":           {},
	"batchGenerateContent":  {},
}

func sanitizeGeminiModelPathSegment(model string) (string, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return "", fmt.Errorf("gemini model is required")
	}
	// SDKs sometimes pass "models/<id>"; strip a single safe prefix only.
	if strings.HasPrefix(model, "models/") {
		model = strings.TrimSpace(strings.TrimPrefix(model, "models/"))
	}
	if model == "" {
		return "", fmt.Errorf("gemini model is required")
	}
	if strings.ContainsAny(model, "/\\?#:") || strings.Contains(model, "..") {
		return "", fmt.Errorf("invalid gemini model path segment")
	}
	if !geminiModelPathPattern.MatchString(model) {
		return "", fmt.Errorf("invalid gemini model path segment")
	}
	return model, nil
}

func sanitizeGeminiModelAction(action string) (string, error) {
	action = strings.TrimSpace(action)
	if _, ok := allowedGeminiModelActions[action]; !ok {
		return "", fmt.Errorf("unsupported gemini action: %s", action)
	}
	return action, nil
}

// buildGeminiAIStudioModelURL builds POST .../v1beta/models/{model}:{action}
// with closed-set action and sanitized model path segment.
func buildGeminiAIStudioModelURL(baseURL, model, action string, stream bool) (string, error) {
	model, err := sanitizeGeminiModelPathSegment(model)
	if err != nil {
		return "", err
	}
	action, err = sanitizeGeminiModelAction(action)
	if err != nil {
		return "", err
	}
	fullURL := fmt.Sprintf(
		"%s/v1beta/models/%s:%s",
		strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		url.PathEscape(model),
		action,
	)
	if stream {
		fullURL += "?alt=sse"
	}
	return fullURL, nil
}

// buildGeminiAIStudioGetModelPath builds GET path /v1beta/models/{model}.
func buildGeminiAIStudioGetModelPath(model string) (string, error) {
	model, err := sanitizeGeminiModelPathSegment(model)
	if err != nil {
		return "", err
	}
	return "/v1beta/models/" + url.PathEscape(model), nil
}

// normalizeGeminiAIStudioGETPath is a closed allowlist for AI Studio GET proxy paths.
// Allowed:
//   - /v1beta/models
//   - /v1beta/models/{sanitizedModel}
func normalizeGeminiAIStudioGETPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("invalid path")
	}
	// Drop query/fragment if a caller accidentally included them.
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "", fmt.Errorf("invalid path")
	}
	if strings.Contains(path, "//") || strings.Contains(path, "\\") || strings.Contains(path, "..") {
		return "", fmt.Errorf("invalid path")
	}

	const listPath = "/v1beta/models"
	if path == listPath {
		return listPath, nil
	}
	const prefix = listPath + "/"
	if !strings.HasPrefix(path, prefix) {
		return "", fmt.Errorf("disallowed gemini GET path")
	}
	modelPart := strings.TrimPrefix(path, prefix)
	if modelPart == "" || strings.Contains(modelPart, "/") {
		return "", fmt.Errorf("disallowed gemini GET path")
	}
	// Reject action-style segments (model:generateContent) on GET get-model.
	if strings.Contains(modelPart, ":") {
		return "", fmt.Errorf("disallowed gemini GET path")
	}
	return buildGeminiAIStudioGetModelPath(modelPart)
}

// SanitizeGeminiModelPathSegment is the exported form of sanitizeGeminiModelPathSegment.
func SanitizeGeminiModelPathSegment(model string) (string, error) {
	return sanitizeGeminiModelPathSegment(model)
}

// BuildGeminiAIStudioGetModelPath is the exported form of buildGeminiAIStudioGetModelPath.
func BuildGeminiAIStudioGetModelPath(model string) (string, error) {
	return buildGeminiAIStudioGetModelPath(model)
}

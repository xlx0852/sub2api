package service

import (
	"fmt"
	"regexp"
	"strings"
)

var contentModerationSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>，。；、]+`),
	regexp.MustCompile(`(?i)\b((?:api[_-]?key|apikey|access[_-]?token|refresh[_-]?token|id[_-]?token|session[_-]?token|token|session|cookie|set[_-]?cookie|authorization|bearer|password|passwd|pwd|secret|client[_-]?secret|private[_-]?key)\s*[:=]\s*)(["']?)[^"'\s,;，。；、]{6,}`),
	regexp.MustCompile(`(?i)\b(Bearer\s+)[A-Za-z0-9._~+/=-]{12,}`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`),
	regexp.MustCompile(`(?i)\b(?:sk|sk-proj|sk-ant|sess|rk|pk|ak|api|key|token|secret)[_-][A-Za-z0-9._~+/=-]{12,}\b`),
	regexp.MustCompile(`\b[0-9a-fA-F]{32,}\b`),
	regexp.MustCompile(`\b[A-Za-z0-9_-]{48,}\b`),
	regexp.MustCompile(`\b[A-Za-z0-9+/]{48,}={0,2}\b`),
	regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`),
}

var (
	retrospectivePEMSecretPattern   = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*(?:PRIVATE KEY|SECRET)[A-Z0-9 ]*-----.*?-----END [A-Z0-9 ]*(?:PRIVATE KEY|SECRET)[A-Z0-9 ]*-----`)
	retrospectiveURLAuthPattern     = regexp.MustCompile(`(?i)\b(https?://)([^/@\s]+)@`)
	retrospectiveBearerPattern      = regexp.MustCompile(`(?i)\bBearer[ \t]+([A-Za-z0-9._~+/=-]{8,})`)
	retrospectiveQuerySecretPattern = regexp.MustCompile(`(?i)([?&])(api[_-]?key|apikey|access[_-]?token|refresh[_-]?token|id[_-]?token|session[_-]?token|token|password|passwd|pwd|secret|client[_-]?secret)=([^&#\s,;，。；、]{4,})`)
	retrospectiveKeyValuePattern    = regexp.MustCompile(`(?i)\b(api[_-]?key|apikey|access[_-]?token|refresh[_-]?token|id[_-]?token|session[_-]?token|token|session|cookie|set[_-]?cookie|authorization|password|passwd|pwd|secret|client[_-]?secret|private[_-]?key)\s*[:=]\s*(?:"([^"]+)"|'([^']+)'|([^\s,;，。；、]{4,}))`)
	retrospectiveJWTSecretPattern   = regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)
	retrospectiveKnownTokenPattern  = regexp.MustCompile(`(?i)\b(?:sk|sk-proj|sk-ant|sess|rk|pk|ak|api|key|token|secret)[_-][A-Za-z0-9._~+/=-]{12,}\b`)
	retrospectiveCloudKeyPattern    = regexp.MustCompile(`\b(?:AKIA|ASIA|LTAI)[A-Z0-9]{12,}\b`)
)

type retrospectiveSecretPlaceholders struct {
	byValue map[string]string
	next    int
}

func newRetrospectiveSecretPlaceholders() *retrospectiveSecretPlaceholders {
	return &retrospectiveSecretPlaceholders{byValue: make(map[string]string)}
}

func (p *retrospectiveSecretPlaceholders) placeholder(kind, value string) string {
	if existing := p.byValue[value]; existing != "" {
		return existing
	}
	p.next++
	kind = normalizeRetrospectiveSecretKind(kind)
	placeholder := fmt.Sprintf("<SECRET:%s#%d>", kind, p.next)
	p.byValue[value] = placeholder
	return placeholder
}

func normalizeRetrospectiveSecretKind(label string) string {
	label = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(label), "-", "_"))
	switch {
	case strings.Contains(label, "private"):
		return "private_key"
	case strings.Contains(label, "password") || label == "passwd" || label == "pwd":
		return "password"
	case strings.Contains(label, "cookie"):
		return "cookie"
	case strings.Contains(label, "authorization"):
		return "authorization"
	case strings.Contains(label, "refresh"):
		return "refresh_token"
	case strings.Contains(label, "access"):
		return "access_token"
	case strings.Contains(label, "api") || label == "ak":
		return "api_key"
	case strings.Contains(label, "session"):
		return "session"
	case strings.Contains(label, "secret"):
		return "secret"
	case strings.Contains(label, "token") || label == "bearer" || label == "jwt":
		return label
	case label == "url_credentials":
		return label
	default:
		return "credential"
	}
}

// sanitizeRetrospectiveReviewContent removes credential values without
// destroying the evidence's semantic context. Unlike log redaction, it keeps
// URLs, hashes, UUIDs, code, and natural-language intent intact.
func sanitizeRetrospectiveReviewContent(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	placeholders := newRetrospectiveSecretPlaceholders()
	out := replaceRetrospectiveSecretGroup(text, retrospectivePEMSecretPattern, 0, "private_key", placeholders, nil)
	out = replaceRetrospectiveSecretGroup(out, retrospectiveURLAuthPattern, 2, "url_credentials", placeholders, nil)
	out = replaceRetrospectiveSecretGroup(out, retrospectiveBearerPattern, 1, "bearer", placeholders, nil)
	out = replaceRetrospectiveQuerySecrets(out, placeholders)
	out = replaceRetrospectiveKeyValues(out, placeholders)
	out = replaceRetrospectiveSecretGroup(out, retrospectiveJWTSecretPattern, 0, "jwt", placeholders, nil)
	out = replaceRetrospectiveSecretGroup(out, retrospectiveKnownTokenPattern, 0, "token", placeholders, nil)
	out = replaceRetrospectiveSecretGroup(out, retrospectiveCloudKeyPattern, 0, "access_key", placeholders, nil)
	return out
}

func replaceRetrospectiveKeyValues(text string, placeholders *retrospectiveSecretPlaceholders) string {
	matches := retrospectiveKeyValuePattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, match := range matches {
		if match[0] > 0 && text[match[0]-1] == '<' {
			continue
		}
		label := retrospectiveSubmatch(text, match, 1)
		group := 2
		if match[group*2] < 0 {
			group = 3
		}
		if match[group*2] < 0 {
			group = 4
		}
		start, end := match[group*2], match[group*2+1]
		if start < 0 || end < start {
			continue
		}
		value := text[start:end]
		if strings.Contains(value, "<SECRET:") || (strings.EqualFold(label, "authorization") && strings.EqualFold(value, "Bearer")) {
			continue
		}
		out.WriteString(text[last:start])
		out.WriteString(placeholders.placeholder(label, value))
		last = end
	}
	if last == 0 {
		return text
	}
	out.WriteString(text[last:])
	return out.String()
}

func replaceRetrospectiveQuerySecrets(text string, placeholders *retrospectiveSecretPlaceholders) string {
	matches := retrospectiveQuerySecretPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, match := range matches {
		start, end := match[6], match[7]
		if start < 0 || end < start {
			continue
		}
		value := text[start:end]
		if strings.Contains(value, "<SECRET:") {
			continue
		}
		out.WriteString(text[last:start])
		out.WriteString(placeholders.placeholder(retrospectiveSubmatch(text, match, 2), value))
		last = end
	}
	if last == 0 {
		return text
	}
	out.WriteString(text[last:])
	return out.String()
}

func replaceRetrospectiveSecretGroup(text string, pattern *regexp.Regexp, group int, kind string, placeholders *retrospectiveSecretPlaceholders, skip func(string) bool) string {
	matches := pattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return text
	}
	var out strings.Builder
	last := 0
	for _, match := range matches {
		idx := group * 2
		if idx+1 >= len(match) || match[idx] < 0 {
			continue
		}
		start, end := match[idx], match[idx+1]
		value := text[start:end]
		if strings.Contains(value, "<SECRET:") || (skip != nil && skip(value)) {
			continue
		}
		out.WriteString(text[last:start])
		out.WriteString(placeholders.placeholder(kind, value))
		last = end
	}
	if last == 0 {
		return text
	}
	out.WriteString(text[last:])
	return out.String()
}

func retrospectiveSubmatch(text string, match []int, group int) string {
	idx := group * 2
	if idx+1 >= len(match) || match[idx] < 0 {
		return ""
	}
	return text[match[idx]:match[idx+1]]
}

func redactContentModerationSecrets(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	out := text
	for idx, pattern := range contentModerationSecretPatterns {
		switch idx {
		case 1:
			out = pattern.ReplaceAllString(out, `${1}${2}[已脱敏]`)
		case 2:
			out = pattern.ReplaceAllString(out, `${1}[已脱敏]`)
		default:
			out = pattern.ReplaceAllString(out, `[已脱敏]`)
		}
	}
	return out
}

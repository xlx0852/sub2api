package service

import appbilling "github.com/Wei-Shaw/sub2api/internal/app/billing"

func normalizeOpenAIUsageForBilling(usage *OpenAIUsage) {
	if usage == nil {
		return
	}
	normalized := appbilling.NormalizeUsage(appbilling.Usage{
		InputTokens:         usage.InputTokens,
		OutputTokens:        usage.OutputTokens,
		CacheCreationTokens: usage.CacheCreationInputTokens,
		CacheReadTokens:     usage.CacheReadInputTokens,
		ImageInputTokens:    usage.ImageInputTokens,
		ImageOutputTokens:   usage.ImageOutputTokens,
	}, appbilling.UsageNormalizationOpenAI)
	usage.InputTokens = normalized.InputTokens
	usage.OutputTokens = normalized.OutputTokens
	usage.CacheCreationInputTokens = normalized.CacheCreationTokens
	usage.CacheReadInputTokens = normalized.CacheReadTokens
	usage.ImageInputTokens = normalized.ImageInputTokens
	usage.ImageOutputTokens = normalized.ImageOutputTokens
}

func normalizeClaudeUsageForBilling(usage *ClaudeUsage) {
	if usage == nil {
		return
	}
	normalized := appbilling.NormalizeUsage(appbilling.Usage{
		InputTokens:           usage.InputTokens,
		OutputTokens:          usage.OutputTokens,
		CacheCreationTokens:   usage.CacheCreationInputTokens,
		CacheReadTokens:       usage.CacheReadInputTokens,
		CacheCreation5mTokens: usage.CacheCreation5mTokens,
		CacheCreation1hTokens: usage.CacheCreation1hTokens,
		ImageOutputTokens:     usage.ImageOutputTokens,
	}, appbilling.UsageNormalizationGateway)
	usage.InputTokens = normalized.InputTokens
	usage.OutputTokens = normalized.OutputTokens
	usage.CacheCreationInputTokens = normalized.CacheCreationTokens
	usage.CacheReadInputTokens = normalized.CacheReadTokens
	usage.CacheCreation5mTokens = normalized.CacheCreation5mTokens
	usage.CacheCreation1hTokens = normalized.CacheCreation1hTokens
	usage.ImageOutputTokens = normalized.ImageOutputTokens
}

package service

func normalizeOpenAIUsageForBilling(usage *OpenAIUsage) {
	if usage == nil {
		return
	}
	usage.InputTokens = max(usage.InputTokens, 0)
	usage.OutputTokens = max(usage.OutputTokens, 0)
	usage.CacheReadInputTokens = min(max(usage.CacheReadInputTokens, 0), usage.InputTokens)
	usage.CacheCreationInputTokens = min(max(usage.CacheCreationInputTokens, 0), usage.InputTokens-usage.CacheReadInputTokens)
	usage.ImageInputTokens = min(max(usage.ImageInputTokens, 0), usage.InputTokens)
	usage.ImageOutputTokens = min(max(usage.ImageOutputTokens, 0), usage.OutputTokens)
}

func normalizeClaudeUsageForBilling(usage *ClaudeUsage) {
	if usage == nil {
		return
	}
	usage.InputTokens = max(usage.InputTokens, 0)
	usage.OutputTokens = max(usage.OutputTokens, 0)
	usage.CacheCreationInputTokens = max(usage.CacheCreationInputTokens, 0)
	usage.CacheReadInputTokens = max(usage.CacheReadInputTokens, 0)
	usage.CacheCreation5mTokens = min(max(usage.CacheCreation5mTokens, 0), usage.CacheCreationInputTokens)
	usage.CacheCreation1hTokens = min(max(usage.CacheCreation1hTokens, 0), usage.CacheCreationInputTokens-usage.CacheCreation5mTokens)
	usage.ImageOutputTokens = min(max(usage.ImageOutputTokens, 0), usage.OutputTokens)
}

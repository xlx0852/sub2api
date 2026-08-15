package billing

type UsageNormalizationPolicy uint8

const (
	UsageNormalizationGateway UsageNormalizationPolicy = iota
	UsageNormalizationOpenAI
)

// NormalizeUsage preserves each provider family's existing token-bucket
// contract while keeping the normalization implementation in one place.
func NormalizeUsage(usage Usage, policy UsageNormalizationPolicy) Usage {
	usage.InputTokens = max(usage.InputTokens, 0)
	usage.OutputTokens = max(usage.OutputTokens, 0)
	usage.CacheCreationTokens = max(usage.CacheCreationTokens, 0)
	usage.CacheReadTokens = max(usage.CacheReadTokens, 0)
	usage.ImageInputTokens = max(usage.ImageInputTokens, 0)
	usage.ImageOutputTokens = min(max(usage.ImageOutputTokens, 0), usage.OutputTokens)

	switch policy {
	case UsageNormalizationOpenAI:
		usage.CacheReadTokens = min(usage.CacheReadTokens, usage.InputTokens)
		usage.CacheCreationTokens = min(usage.CacheCreationTokens, usage.InputTokens-usage.CacheReadTokens)
		usage.ImageInputTokens = min(usage.ImageInputTokens, usage.InputTokens)
		usage.CacheCreation5mTokens = 0
		usage.CacheCreation1hTokens = 0
	default:
		usage.CacheCreation5mTokens = min(max(usage.CacheCreation5mTokens, 0), usage.CacheCreationTokens)
		usage.CacheCreation1hTokens = min(max(usage.CacheCreation1hTokens, 0), usage.CacheCreationTokens-usage.CacheCreation5mTokens)
		usage.ImageInputTokens = 0
	}
	return usage
}

// ExclusiveInputTokens returns the non-cache input bucket. OpenAI reports
// total input including cache details; Gateway providers already report
// independent buckets and therefore keep their input unchanged.
func ExclusiveInputTokens(usage Usage, policy UsageNormalizationPolicy) int {
	if policy != UsageNormalizationOpenAI {
		return usage.InputTokens
	}
	return max(usage.InputTokens-usage.CacheReadTokens-usage.CacheCreationTokens, 0)
}

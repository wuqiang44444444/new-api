package openai

import "github.com/QuantumNous/new-api/relaykit/dto"

// normalizeImageUsageWithEvidence uses the native normalization for every
// synchronous and durable image boundary, preserving optional meter presence.
func normalizeImageUsageWithEvidence(usage *dto.Usage, body []byte) {
	if usage == nil {
		return
	}
	recordImageCacheReadEvidence(usage, body)
	normalizeOpenAIUsage(usage)
}

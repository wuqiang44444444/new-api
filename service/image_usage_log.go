package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func appendImageUsageForLog(other *model.LogOther, usage *dto.Usage) {
	if usage != nil && usage.CacheReadTokensReported != nil {
		other.SetPublic("cache_read_tokens_reported", *usage.CacheReadTokensReported)
	}
	if usage != nil && usage.CacheWriteTokensReported != nil {
		other.SetPublic("cache_write_tokens_reported", *usage.CacheWriteTokensReported)
		if *usage.CacheWriteTokensReported {
			other.SetPublic("cache_write_tokens", usage.PromptTokensDetails.CacheCreationTokensTotal())
		}
	}
	if usage != nil && usage.InputImages != nil {
		other.SetPublic("input_images", *usage.InputImages)
	}
}

package openai

import (
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/tidwall/gjson"
	"math"
)

// Preserve meter presence before normalization. Optional image cache meters
// are not a claim that every image endpoint reports reads or writes.
func recordImageCacheReadEvidence(usage *dto.Usage, body []byte) {
	path := "usage.prompt_tokens_details."
	if usage.InputTokensDetails != nil {
		path = "usage.input_tokens_details."
	}
	read := gjson.GetBytes(body, path+"cached_tokens")
	readReported := read.Type == gjson.Number && read.Num >= 0 && math.Trunc(read.Num) == read.Num
	usage.CacheReadTokensReported = &readReported
	writeReported := false
	for _, field := range []string{"cache_write_tokens", "cached_creation_tokens"} {
		value := gjson.GetBytes(body, path+field)
		if value.Type == gjson.Number && value.Num >= 0 && math.Trunc(value.Num) == value.Num {
			writeReported = true
		}
	}
	usage.CacheWriteTokensReported = &writeReported
}

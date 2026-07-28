package dto

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGPTImage2TokenCountMetaKeepsExistingBillingDimensions(t *testing.T) {
	request := ImageRequest{
		Model:   "gpt-image-2",
		Prompt:  "a production image",
		N:       common.GetPointer(uint(2)),
		Size:    "1536x1024",
		Quality: "high",
	}

	meta := request.GetTokenCountMeta()

	require.NotNil(t, meta)
	assert.Equal(t, "a production image", meta.CombineText)
	assert.Equal(t, 1.0, meta.ImagePriceRatio)
	assert.Equal(t, map[string]float64{"n": 2}, meta.BillingRatios)
}

package helper

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaImageTieredPreConsumeUsesSafeUpperBoundAndCompactProbe(t *testing.T) {
	const modelName = "media-image-tiered"
	const upperBound = 100
	loadTaskPricingConfig(t, map[string]string{
		modelName: `tier("image", c + img * 2 + img_o * 3)`,
	}, map[string]int{modelName: upperBound})
	n := uint(2)
	info := &relaycommon.RelayInfo{
		OriginModelName: modelName,
		UserGroup:       "default",
		UsingGroup:      "default",
		Request: &dto.ImageRequest{
			Model: modelName, Prompt: "must-not-be-frozen", Size: "2K", N: &n,
		},
	}

	price, err := ModelPriceHelperMediaImageTaskTiered(taskPriceContext(), info)
	require.NoError(t, err)
	expected, clamp := common.QuotaRoundChecked(float64(upperBound*6) / 1_000_000 * common.QuotaPerUnit)
	require.Nil(t, clamp)
	assert.Equal(t, expected, price.QuotaToPreConsume)
	require.NotNil(t, info.BillingRequestInput)
	probe := string(info.BillingRequestInput.Body)
	assert.Contains(t, probe, `"_task"`)
	assert.Contains(t, probe, `"size":"2K"`)
	assert.Contains(t, probe, `"n":2`)
	assert.False(t, strings.Contains(probe, "must-not-be-frozen"))
}

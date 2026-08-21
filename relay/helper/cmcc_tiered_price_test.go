package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cmccSeedance20USDExpression = `param("_task.has_video_input") == true
  ? (param("_task.resolution") == "1080p" ? tier("1080p_video", c * 9.090909090909) : tier("480p720p_video", c * 8.211143695015))
  : (param("_task.resolution") == "1080p" ? tier("1080p", c * 14.956011730205) : tier("480p720p", c * 13.489736070381))`

func TestCMCCSeedanceTieredExpressionUsesFixed682USDConversion(t *testing.T) {
	const model = "seedance-2.0-cmcc"
	const estimatedTokens = 1460025
	loadTaskPricingConfig(t, map[string]string{model: cmccSeedance20USDExpression}, map[string]int{model: estimatedTokens})

	tests := []struct {
		resolution string
		hasVideo   bool
		tier       string
		rmbRate    float64
	}{
		{resolution: "720p", hasVideo: true, tier: "480p720p_video", rmbRate: 56},
		{resolution: "1080p", hasVideo: true, tier: "1080p_video", rmbRate: 62},
		{resolution: "720p", tier: "480p720p", rmbRate: 92},
		{resolution: "1080p", tier: "1080p", rmbRate: 102},
	}
	for _, test := range tests {
		t.Run(test.tier, func(t *testing.T) {
			info := &relaycommon.RelayInfo{OriginModelName: model, UserGroup: "default", UsingGroup: "default"}
			price, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{
				"resolution": test.resolution, "has_video_input": test.hasVideo,
			})
			require.NoError(t, err)
			usdRate := test.rmbRate / 6.82
			expected, clamp := common.QuotaRoundChecked(float64(estimatedTokens) * usdRate / 1_000_000 * common.QuotaPerUnit)
			require.Nil(t, clamp)
			assert.InDelta(t, expected, price.Quota, 1)
			require.NotNil(t, info.TieredBillingSnapshot)
			assert.Equal(t, test.tier, info.TieredBillingSnapshot.EstimatedTier)
		})
	}
}

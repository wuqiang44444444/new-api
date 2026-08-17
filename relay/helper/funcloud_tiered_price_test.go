package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const funCloudStandardTokenPriceExpression = `param("_task.has_video_input") == true
  ? (param("_task.resolution") == "1080p" ? tier("1080p_video", c * 4.55) : tier("480p720p_video", c * 4.11))
  : (param("_task.resolution") == "1080p" ? tier("1080p", c * 7.48) : tier("480p720p", c * 6.74))`

const funCloudFastTokenPriceExpression = `param("_task.has_video_input") == true
  ? tier("480p720p_video", c * 3.23)
  : tier("480p720p", c * 5.43)`

const funCloudMiniTokenPriceExpression = `param("_task.has_video_input") == true
  ? tier("480p720p_video", c * 2.05)
  : tier("480p720p", c * 3.37)`

const funCloud25TokenPriceExpression = `param("_task.has_video_input") == true
  ? tier("480p720p_video", c * 6.16)
  : tier("480p720p", c * 10.26)`

func TestFunCloudTieredExpressionsUseUSDTokenPrices(t *testing.T) {
	expressions := map[string]string{
		"seedance-2-funcloud":      funCloudStandardTokenPriceExpression,
		"seedance-2-fast-funcloud": funCloudFastTokenPriceExpression,
		"seedance-2-mini-funcloud": funCloudMiniTokenPriceExpression,
		"seedance-2-5-funcloud":    funCloud25TokenPriceExpression,
	}
	estimates := map[string]int{
		"seedance-2-funcloud": 730000, "seedance-2-fast-funcloud": 324000,
		"seedance-2-mini-funcloud": 324000, "seedance-2-5-funcloud": 648000,
	}
	loadTaskPricingConfig(t, expressions, estimates)

	tests := []struct {
		model      string
		resolution string
		hasVideo   bool
		rate       float64
		tier       string
	}{
		{model: "seedance-2-funcloud", resolution: "720p", rate: 6.74, tier: "480p720p"},
		{model: "seedance-2-funcloud", resolution: "1080p", rate: 7.48, tier: "1080p"},
		{model: "seedance-2-funcloud", resolution: "720p", hasVideo: true, rate: 4.11, tier: "480p720p_video"},
		{model: "seedance-2-funcloud", resolution: "1080p", hasVideo: true, rate: 4.55, tier: "1080p_video"},
		{model: "seedance-2-fast-funcloud", resolution: "720p", rate: 5.43, tier: "480p720p"},
		{model: "seedance-2-fast-funcloud", resolution: "720p", hasVideo: true, rate: 3.23, tier: "480p720p_video"},
		{model: "seedance-2-mini-funcloud", resolution: "720p", rate: 3.37, tier: "480p720p"},
		{model: "seedance-2-mini-funcloud", resolution: "720p", hasVideo: true, rate: 2.05, tier: "480p720p_video"},
		{model: "seedance-2-5-funcloud", resolution: "720p", rate: 10.26, tier: "480p720p"},
		{model: "seedance-2-5-funcloud", resolution: "720p", hasVideo: true, rate: 6.16, tier: "480p720p_video"},
	}
	for _, test := range tests {
		t.Run(test.model+"/"+test.tier, func(t *testing.T) {
			info := &relaycommon.RelayInfo{OriginModelName: test.model, UserGroup: "default", UsingGroup: "default"}
			price, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{
				"resolution": test.resolution, "has_video_input": test.hasVideo,
			})
			require.NoError(t, err)
			expected, clamp := common.QuotaRoundChecked(float64(estimates[test.model]) * test.rate / 1_000_000 * common.QuotaPerUnit)
			require.Nil(t, clamp)
			assert.Equal(t, expected, price.Quota)
			require.NotNil(t, info.TieredBillingSnapshot)
			assert.Equal(t, test.tier, info.TieredBillingSnapshot.EstimatedTier)
		})
	}
}

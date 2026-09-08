package helper

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSynlinkPricingRequiresAnEstimateOnlyForUsageExpressions(t *testing.T) {
	loadTaskPricingConfig(t, map[string]string{"fixed": `tier("fixed", 1000000)`, "usage": `tier("usage", c)`}, nil)
	info := &relaycommon.RelayInfo{OriginModelName: "fixed", UserGroup: "default", UsingGroup: "default", ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType:          constant.ChannelTypeSeedanceLink,
		ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolSynlinkVideoV1},
	}}
	price, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{})
	require.NoError(t, err)
	assert.Positive(t, price.Quota)
	require.NotNil(t, info.TieredBillingSnapshot)
	assert.Zero(t, info.TieredBillingSnapshot.EstimatedCompletionTokens)
	info.OriginModelName = "usage"
	_, err = ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{})
	require.ErrorContains(t, err, "pre-consume token upper bound is not configured")
}

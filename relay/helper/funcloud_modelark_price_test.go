package helper

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFunCloudModelArkSecondsPricingDoesNotInventTokens(t *testing.T) {
	const name = "v3-seconds-test"
	loadTaskPricingConfig(t, map[string]string{name: `tier("seconds", u("duration_seconds") * 0.1)`}, map[string]int{})
	info := &relaycommon.RelayInfo{OriginModelName: name, UserGroup: "default", UsingGroup: "default", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3}}}
	price, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{"duration_seconds": 4})
	require.NoError(t, err)
	assert.Equal(t, common.QuotaRound(0.4*common.QuotaPerUnit), price.Quota)
	require.NotNil(t, info.TieredBillingSnapshot)
	assert.Zero(t, info.TieredBillingSnapshot.EstimatedCompletionTokens)
}

func TestFunCloudModelArkTokenPricingStillRequiresBound(t *testing.T) {
	const name = "v3-token-test"
	loadTaskPricingConfig(t, map[string]string{name: `tier("tokens", u("tokens") * 10 / 1000000)`}, map[string]int{})
	info := &relaycommon.RelayInfo{OriginModelName: name, UserGroup: "default", UsingGroup: "default", ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3}}}
	_, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{})
	require.ErrorContains(t, err, "pre-consume token upper bound is not configured")
}

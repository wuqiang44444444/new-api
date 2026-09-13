package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	rc "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedanceNewAcceptanceUsesUSDContract(t *testing.T) {
	for _, tc := range []struct {
		name, expression string
		quota            int
		invalid          bool
	}{
		{"fixed", `tier("base", 0.5)`, common.QuotaRound(0.5 * common.QuotaPerUnit), false},
		{"free", `tier("base", 0)`, 0, false},
		{"legacy", `tier("base", c * 5)`, 0, true},
		{"legacy probe", `tier("base", param("_task.duration_seconds") * 400000)`, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loadTaskPricingConfig(t, map[string]string{"price-regression": tc.expression}, map[string]int{"price-regression": 300000})
			info := &rc.RelayInfo{OriginModelName: "price-regression", UserGroup: "default", UsingGroup: "default", ChannelMeta: &rc.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3CMCC}}}
			price, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{"duration_seconds": 5})
			if tc.invalid {
				require.Error(t, err)
				assert.Nil(t, info.TieredBillingSnapshot)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.quota, price.Quota)
			require.NotNil(t, info.TieredBillingSnapshot)
			assert.True(t, info.TieredBillingSnapshot.TaskUsageBilling)
		})
	}
}

func TestSeedanceUSDPreconsumePreservesGroupAndContract(t *testing.T) {
	for _, tc := range []struct {
		name, groupConfig string
		quota             int
	}{
		{"contract", `{"default":0.87}`, 522000},
		{"free group", `{"default":0}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loadTaskPricingConfig(t, map[string]string{"contract-video": `tier("base", u("tokens") * 5 / 1000000)`}, map[string]int{"contract-video": 300000})
			require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(tc.groupConfig))
			info := &rc.RelayInfo{OriginModelName: "contract-video", UserGroup: "default", UsingGroup: "default", ChannelMeta: &rc.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3CMCC}}, ContractBillingFact: &types.ContractBillingFact{RatioUnits: 80_000_000}}
			price, err := ModelPriceHelperTaskTiered(taskPriceContext(), info, fixedTaskProbe{})
			require.NoError(t, err)
			assert.Equal(t, tc.quota, price.Quota)
			require.NotNil(t, info.TieredBillingSnapshot)
			assert.True(t, info.TieredBillingSnapshot.TaskUsageBilling)
		})
	}
}

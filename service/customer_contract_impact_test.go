package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviewCustomerContractRatioImpactCountsOnlyChangedActiveContracts(t *testing.T) {
	truncate(t)
	previousGroups := ratio_setting.GroupRatio2JSONString()
	previousSpecialGroups := ratio_setting.GroupGroupRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-a":0.87,"contract-b":1}`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(previousSpecialGroups))
	})

	active := &model.User{Username: "impact-active", AffCode: "impact-active-aff", Group: "default", Status: common.UserStatusEnabled}
	inactive := &model.User{Username: "impact-inactive", AffCode: "impact-inactive-aff", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, model.DB.Create(active).Error)
	require.NoError(t, model.DB.Create(inactive).Error)
	// One enabled contract with two rules and one disabled contract; only the
	// enabled contract's rules are affected by native group ratio changes.
	activeContract := &model.CustomerContract{UserId: active.Id, Name: "Impact Active", Enabled: true, Version: 1}
	inactiveContract := &model.CustomerContract{UserId: inactive.Id, Name: "Impact Inactive", Enabled: false, Version: 1}
	require.NoError(t, model.DB.Create(activeContract).Error)
	require.NoError(t, model.DB.Create(inactiveContract).Error)
	require.NoError(t, model.DB.Create([]model.CustomerContractEntityRule{
		{ContractId: activeContract.Id, PublicModel: "model-a", ChannelId: 1, RouteGroup: "contract-a", RatioUnits: 80_000_000},
		{ContractId: activeContract.Id, PublicModel: "model-b", ChannelId: 1, RouteGroup: "contract-b", RatioUnits: 80_000_000},
		{ContractId: inactiveContract.Id, PublicModel: "model-c", ChannelId: 1, RouteGroup: "contract-a", RatioUnits: 80_000_000},
	}).Error)

	impact, err := PreviewCustomerContractRatioImpact(
		`{"default":1,"contract-a":0.5,"contract-b":1}`,
		`{}`,
	)

	require.NoError(t, err)
	assert.Equal(t, 1, impact.AffectedContracts)
	assert.Equal(t, 1, impact.AffectedRules)
	assert.Equal(t, []string{"contract-a"}, impact.AffectedGroups)
}

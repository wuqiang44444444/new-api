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

	active := &model.User{Username: "impact-active", AffCode: "impact-active-aff", Group: "default", Status: common.UserStatusEnabled, ContractMode: true, ContractVersion: 1}
	inactive := &model.User{Username: "impact-inactive", AffCode: "impact-inactive-aff", Group: "default", Status: common.UserStatusEnabled, ContractMode: false, ContractVersion: 1}
	require.NoError(t, model.DB.Create(active).Error)
	require.NoError(t, model.DB.Create(inactive).Error)
	require.NoError(t, model.DB.Create([]model.CustomerModelContract{
		{UserId: active.Id, PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 80_000_000},
		{UserId: active.Id, PublicModel: "model-b", RouteGroup: "contract-b", RatioUnits: 80_000_000},
		{UserId: inactive.Id, PublicModel: "model-c", RouteGroup: "contract-a", RatioUnits: 80_000_000},
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

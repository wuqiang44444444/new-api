package model

import (
	"errors"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestContractAvailabilityBatchesRulesWithoutChangingSources(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	first := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	second := createCustomerContractAbility(t, db, "contract-b", "model-a", common.ChannelStatusEnabled)
	disabled := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusManuallyDisabled)
	inactiveAbility := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", inactiveAbility.Id).Update("enabled", false).Error)
	typed := Channel{Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled, Group: "contract-a,contract-b", Models: " model-a ,model-b"}
	require.NoError(t, db.Create(&typed).Error)
	batch := Channel{Type: constant.ChannelTypeAzureBatch, Status: common.ChannelStatusEnabled, Group: "contract-b", Models: "model-a"}
	require.NoError(t, db.Create(&batch).Error)
	snapshot := ContractEntitySnapshot{Rules: []ContractEntityRule{
		{ChannelId: first.Id, RouteGroup: "contract-a", PublicModel: "model-a"},
		{ChannelId: second.Id, RouteGroup: "contract-b", PublicModel: "model-a"},
		{ChannelId: disabled.Id, RouteGroup: "contract-a", PublicModel: "model-a"},
		{ChannelId: inactiveAbility.Id, RouteGroup: "contract-a", PublicModel: "model-a"},
		{ChannelId: first.Id, RouteGroup: "contract-b", PublicModel: "model-a"},
		{ChannelId: first.Id, RouteGroup: "contract-a", PublicModel: "Model-A"},
		{ChannelId: 999, RouteGroup: "contract-a", PublicModel: "model-a"},
		{ChannelId: typed.Id, RouteGroup: "contract-a", PublicModel: "model-a"},
		{ChannelId: typed.Id, RouteGroup: "contract-b", PublicModel: "model-b"},
		{ChannelId: typed.Id, RouteGroup: "default", PublicModel: "model-a"},
		{ChannelId: typed.Id, RouteGroup: "contract-a", PublicModel: "other"},
		{ChannelId: batch.Id, RouteGroup: "contract-b", PublicModel: "model-a"},
		{ChannelId: first.Id, RouteGroup: "removed", PublicModel: "model-a"},
	}}
	queries := 0
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:count_route_queries", func(tx *gorm.DB) { queries++ }))
	require.NoError(t, RefreshContractEntityAvailability(&snapshot))
	got := make([]bool, len(snapshot.Rules))
	for i, rule := range snapshot.Rules {
		got[i] = rule.Available
	}
	assert.Equal(t, []bool{true, true, false, false, false, false, false, true, true, false, false, true, false}, got)
	assert.LessOrEqual(t, queries, 2, "source reads must be batched rather than repeated for every rule")
}

func TestContractAvailabilityReadFailureDoesNotPublishPartialResult(t *testing.T) {
	for _, table := range []string{"channels", "abilities"} {
		t.Run(table, func(t *testing.T) {
			db := setupCustomerContractTestDB(t)
			channel := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
			snapshot := ContractEntitySnapshot{Rules: []ContractEntityRule{{ChannelId: channel.Id, RouteGroup: "contract-a", PublicModel: "model-a", Available: true}}}
			before := append([]ContractEntityRule(nil), snapshot.Rules...)
			failure := errors.New("route source unavailable")
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:fail_route_read", func(tx *gorm.DB) {
				if tx.Statement.Table == table {
					tx.AddError(failure)
				}
			}))
			require.ErrorIs(t, RefreshContractEntityAvailability(&snapshot), failure)
			assert.Equal(t, before, snapshot.Rules, "a failed read must not replace the previous snapshot with partial availability")
		})
	}
}

func TestContractAvailabilityCrossesBatchBoundaryWithoutPartialResults(t *testing.T) {
	for _, failSecondBatch := range []bool{false, true} {
		t.Run(fmt.Sprintf("second_batch_failure=%t", failSecondBatch), func(t *testing.T) {
			db := setupCustomerContractTestDB(t)
			channel := Channel{Status: common.ChannelStatusEnabled, Group: "contract-a"}
			require.NoError(t, db.Create(&channel).Error)
			// 201 distinct sources cross the documented 200-rule query boundary.
			rules := make([]ContractEntityRule, 201)
			abilities := make([]Ability, len(rules))
			for i := range rules {
				name := fmt.Sprintf("batch-model-%03d", i)
				rules[i] = ContractEntityRule{ChannelId: channel.Id, RouteGroup: "contract-a", PublicModel: name}
				abilities[i] = Ability{ChannelId: channel.Id, Group: "contract-a", Model: name, Enabled: true}
			}
			require.NoError(t, db.CreateInBatches(abilities, 100).Error)
			queries, abilityQueries := 0, 0
			failure := errors.New("second source batch unavailable")
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:second_source_batch", func(tx *gorm.DB) {
				queries++
				if tx.Statement.Table == "abilities" {
					abilityQueries++
					if failSecondBatch && abilityQueries == 2 {
						tx.AddError(failure)
					}
				}
			}))
			result, err := GetContractRouteAvailability(rules)
			if failSecondBatch {
				require.ErrorIs(t, err, failure)
				assert.Nil(t, result)
				assert.False(t, rules[0].Available, "the caller's frozen rules must not be mutated")
				return
			}
			require.NoError(t, err)
			require.Len(t, result, len(rules))
			for i, rule := range result {
				assert.True(t, rule.Available, rule.PublicModel)
				assert.Equal(t, rules[i].PublicModel, rule.PublicModel, "source order must be retained")
			}
			assert.LessOrEqual(t, queries, 4)
		})
	}
}

func TestContractTypedGroupsAreMembershipsNotListSearchFilters(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-a":1,"all":1,"null":1,"literal_%":1}`))
	channel := Channel{Type: constant.ChannelTypeSeedanceLink, Status: common.ChannelStatusEnabled, Group: "contract-a,literal_%", Models: "model-a"}
	require.NoError(t, db.Create(&channel).Error)
	for _, tc := range []struct {
		group     string
		available bool
	}{{"contract-a", true}, {"literal_%", true}, {"all", false}, {"null", false}} {
		t.Run(tc.group, func(t *testing.T) {
			rule := ContractEntityRule{ChannelId: channel.Id, RouteGroup: tc.group, PublicModel: "model-a"}
			result, err := GetContractRouteAvailability([]ContractEntityRule{rule})
			require.NoError(t, err)
			require.Len(t, result, 1)
			assert.Equal(t, tc.available, result[0].Available)
			err = validateCustomerContractEntityChannel(db, channel.Id, tc.group, "model-a")
			if tc.available {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrCustomerContractEntityInvalidChannel)
			}
		})
	}
}

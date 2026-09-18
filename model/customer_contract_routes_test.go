package model

import (
	"fmt"
	"math/rand"
	"slices"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomerContractRoutesPreserveNativeSamplingAcrossTwoPlusOneGroups(t *testing.T) {
	// Fixed random inputs compare the two candidate sources against the same
	// native algorithm, rather than checking statistical frequencies.
	t.Setenv("GODEBUG", "randseednop=0")
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cache=%t", cached), func(t *testing.T) {
			db := setupCustomerContractTestDB(t)
			oldMemory := common.MemoryCacheEnabled
			common.MemoryCacheEnabled = cached
			t.Cleanup(func() { common.MemoryCacheEnabled = oldMemory })
			initCol()
			routes := map[int]string{}
			for i, group := range []string{"contract-a", "contract-a", "contract-b"} {
				priority, weight := int64(1), uint(100)
				channel := Channel{Name: fmt.Sprintf("candidate-%d", i), Group: group + ",reference", Models: "sample", Status: common.ChannelStatusEnabled, Priority: &priority, Weight: &weight}
				require.NoError(t, db.Create(&channel).Error)
				for _, g := range []string{group, "reference"} {
					require.NoError(t, db.Create(&Ability{Group: g, Model: "sample", ChannelId: channel.Id, Enabled: true, Priority: &priority, Weight: weight}).Error)
				}
				routes[channel.Id] = group
			}
			InitChannelCache()
			filter := []dto.ChannelFilter{{Kind: dto.FilterContractRoutes, Routes: routes}}
			var abilities []Ability
			require.NoError(t, channelRouteAbilityQuery("ignored", "sample", filter).Find(&abilities).Error)
			require.Len(t, abilities, 3, "all combinations reach native priority/weight selection exactly once")
			var nativeAbilities, scopedAbilities []Ability
			require.NoError(t, channelRouteAbilityQuery("reference", "sample", nil).Order("priority DESC, weight DESC").Find(&nativeAbilities).Error)
			require.NoError(t, channelRouteAbilityQuery("ignored", "sample", filter).Order("priority DESC, weight DESC").Find(&scopedAbilities).Error)
			for _, seed := range []int64{1, 7, 42} {
				rand.Seed(seed)
				expected, err := GetRandomSatisfiedChannel("reference", "sample", 0, nil)
				require.NoError(t, err)
				rand.Seed(seed)
				actual, err := GetRandomSatisfiedChannel("ignored", "sample", 0, filter)
				require.NoError(t, err)
				require.NotNil(t, actual)
				if cached {
					assert.Equal(t, expected.Id, actual.Id)
				} else {
					// SQL leaves tie ordering unspecified; the same draw must land
					// on the same weighted slot in each complete candidate list.
					assert.Equal(t, slices.IndexFunc(nativeAbilities, func(a Ability) bool { return a.ChannelId == expected.Id }), slices.IndexFunc(scopedAbilities, func(a Ability) bool { return a.ChannelId == actual.Id }))
				}
			}
			empty, err := GetRandomSatisfiedChannel("reference", "sample", 0, []dto.ChannelFilter{{Kind: dto.FilterContractRoutes, Routes: map[int]string{}}})
			require.NoError(t, err)
			assert.Nil(t, empty)
		})
	}
}

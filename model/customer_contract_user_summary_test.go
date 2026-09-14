package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserListsSummarizeCurrentContractEntities(t *testing.T) {
	for _, search := range []bool{false, true} {
		name := "list"
		if search {
			name = "search"
		}
		t.Run(name, func(t *testing.T) {
			db := setupCustomerContractTestDB(t)
			_, user := createCustomerContractFixture(t, db)
			// Old flags deliberately disagree with the new entities.
			require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).
				Updates(map[string]any{"contract_mode": true, "contract_version": 9}).Error)
			contracts := []CustomerContract{
				{UserId: user.Id, Name: "enabled-empty", Enabled: true, Version: 1},
				{UserId: user.Id, Name: "disabled", Enabled: false, Version: 2},
			}
			require.NoError(t, db.Create(&contracts).Error)
			// Multiple rules must not multiply the count of contracts.
			require.NoError(t, db.Create(&[]CustomerContractEntityRule{
				{ContractId: contracts[1].Id, PublicModel: "one", RatioUnits: 80_000_000},
				{ContractId: contracts[1].Id, PublicModel: "two", RatioUnits: 80_000_000},
			}).Error)
			for _, state := range []struct {
				enabled bool
				count   int
			}{{true, 1}, {false, 0}} {
				require.NoError(t, db.Model(&CustomerContract{}).Where("id = ?", contracts[0].Id).Update("enabled", state.enabled).Error)
				var users []*User
				var err error
				if search {
					users, _, err = SearchUsers("contract-", "", nil, nil, 0, 20)
				} else {
					users, _, err = GetAllUsers(&common.PageInfo{Page: 1, PageSize: 20})
				}
				require.NoError(t, err)
				require.Len(t, users, 2)
				for _, listed := range users {
					require.NotNil(t, listed.ContractSummary)
					if listed.Id == user.Id {
						assert.Equal(t, UserContractSummary{Total: 2, Enabled: state.count}, *listed.ContractSummary)
						assert.True(t, listed.ContractMode, "projection must not rewrite legacy migration state")
						assert.EqualValues(t, 9, listed.ContractVersion)
					} else {
						assert.Equal(t, UserContractSummary{}, *listed.ContractSummary, "other users own no entities")
					}
				}
			}
		})
	}
}

func TestUserSummaryAfterLegacyContractMigration(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	channel := createCustomerContractAbility(t, db, "contract-a", "migrated-model", common.ChannelStatusEnabled)
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("contract_mode", true).Error)
	require.NoError(t, db.Create(&CustomerModelContract{
		UserId: user.Id, PublicModel: "migrated-model", RouteGroup: "contract-a", RatioUnits: 75_000_000,
	}).Error)
	token := Token{UserId: user.Id, Key: "summary-migration-key", Name: "existing", Group: "default"}
	require.NoError(t, db.Create(&token).Error)
	snapshot, err := MigrateLegacyCustomerContractForUser(MigrateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Reason: "summary regression",
	})
	require.NoError(t, err)
	require.Len(t, snapshot.Rules, 1)
	assert.Equal(t, channel.Id, snapshot.Rules[0].ChannelId)
	users, _, err := SearchUsers(user.Username, "", nil, nil, 0, 20)
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.NotNil(t, users[0].ContractSummary)
	assert.Equal(t, UserContractSummary{Total: 1, Enabled: 1}, *users[0].ContractSummary)
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, snapshot.Id, token.ContractId)
}

package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyCustomerContractMigrationBindsKeysAndKeepsLegacyRows(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&CustomerContract{},
		&CustomerContractEntityRule{},
		&CustomerContractEntityAudit{},
		&Token{},
	))
	admin, user := createCustomerContractFixture(t, db)
	channel := createCustomerContractAbility(t, db, "contract-a", "legacy-model", common.ChannelStatusEnabled)

	// Legacy user-level contract state.
	user.ContractMode = true
	require.NoError(t, db.Model(&User{}).Where("id = ?", user.Id).Update("contract_mode", true).Error)
	require.NoError(t, db.Create([]CustomerModelContract{
		{UserId: user.Id, PublicModel: "legacy-model", RouteGroup: "contract-a", RatioUnits: 75_000_000},
	}).Error)
	tokens := []Token{
		{UserId: user.Id, Key: "legacy-key-1", Name: "k1", CreatedTime: 1, AccessedTime: 1, ExpiredTime: -1, Group: "default"},
		{UserId: user.Id, Key: "legacy-key-2", Name: "k2", CreatedTime: 1, AccessedTime: 1, ExpiredTime: -1, Group: "pro"},
	}
	require.NoError(t, db.Create(&tokens).Error)

	previews, err := PreviewLegacyCustomerContractMigration()
	require.NoError(t, err)
	var preview *CustomerContractMigrationPreview
	for i := range previews {
		if previews[i].UserId == user.Id {
			preview = &previews[i]
		}
	}
	require.NotNil(t, preview, "the user must appear in the migration preview")
	assert.False(t, preview.AlreadyMigrated)
	assert.True(t, preview.ContractEnabled)
	require.Len(t, preview.Rules, 1)
	assert.False(t, preview.Rules[0].NeedsDecision, "a single qualified channel is resolved automatically")
	assert.Equal(t, channel.Id, preview.Rules[0].ResolvedChannelId)

	snapshot, err := MigrateLegacyCustomerContractForUser(MigrateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ContractName: "Migrated", Reason: "upgrade to contract entities",
	})
	require.NoError(t, err)
	assert.Equal(t, "Migrated", snapshot.Name)
	assert.True(t, snapshot.Enabled, "the legacy enabled state is preserved")
	require.Len(t, snapshot.Rules, 1)
	assert.Equal(t, "legacy-model", snapshot.Rules[0].PublicModel)
	assert.Equal(t, channel.Id, snapshot.Rules[0].ChannelId)
	assert.Equal(t, "contract-a", snapshot.Rules[0].RouteGroup)
	assert.EqualValues(t, 75_000_000, snapshot.Rules[0].RatioUnits, "discount precision is preserved")

	var bound []Token
	require.NoError(t, db.Where("user_id = ?", user.Id).Order("key").Find(&bound).Error)
	require.Len(t, bound, 2)
	groupByKey := map[string]string{"legacy-key-1": "default", "legacy-key-2": "pro"}
	for _, token := range bound {
		assert.Equal(t, snapshot.Id, token.ContractId, "every existing key binds to the migrated contract")
		assert.Equal(t, groupByKey[token.Key], token.Group, "original key groups are never rewritten")
	}

	// The migration is idempotent.
	_, err = MigrateLegacyCustomerContractForUser(MigrateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Reason: "second attempt",
	})
	require.ErrorIs(t, err, ErrCustomerContractMigrationAlreadyDone)

	// Legacy rule rows are retained as history.
	var legacyCount int64
	require.NoError(t, db.Model(&CustomerModelContract{}).Where("user_id = ?", user.Id).Count(&legacyCount).Error)
	assert.EqualValues(t, 1, legacyCount)

	audits, total, err := GetContractEntityAudits(snapshot.Id, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, audits, 1)
	assert.Equal(t, "migrate", audits[0].Operation)
}

func TestLegacyCustomerContractMigrationRequiresExplicitAmbiguousChannel(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&CustomerContract{},
		&CustomerContractEntityRule{},
		&CustomerContractEntityAudit{},
		&Token{},
	))
	admin, user := createCustomerContractFixture(t, db)
	first := createCustomerContractAbility(t, db, "contract-b", "ambiguous-model", common.ChannelStatusEnabled)
	second := createCustomerContractAbility(t, db, "contract-b", "ambiguous-model", common.ChannelStatusEnabled)
	require.NotEqual(t, first.Id, second.Id)

	require.NoError(t, db.Create(&CustomerModelContract{
		UserId: user.Id, PublicModel: "ambiguous-model", RouteGroup: "contract-b", RatioUnits: 80_000_000,
	}).Error)

	previews, err := PreviewLegacyCustomerContractMigration()
	require.NoError(t, err)
	var preview *CustomerContractMigrationPreview
	for i := range previews {
		if previews[i].UserId == user.Id {
			preview = &previews[i]
		}
	}
	require.NotNil(t, preview)
	require.Len(t, preview.Rules, 1)
	assert.True(t, preview.Rules[0].NeedsDecision, "multiple candidates require an explicit admin decision")

	_, err = MigrateLegacyCustomerContractForUser(MigrateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Reason: "no override given",
	})
	require.ErrorIs(t, err, ErrCustomerContractMigrationChannelRequired)

	// An override outside the candidate list is rejected.
	_, err = MigrateLegacyCustomerContractForUser(MigrateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Reason: "wrong override",
		ChannelOverrides: map[string]int{"ambiguous-model": first.Id + 10_000},
	})
	require.Error(t, err)

	snapshot, err := MigrateLegacyCustomerContractForUser(MigrateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Reason: "explicit decision",
		ChannelOverrides: map[string]int{"ambiguous-model": second.Id},
	})
	require.NoError(t, err)
	require.Len(t, snapshot.Rules, 1)
	assert.Equal(t, second.Id, snapshot.Rules[0].ChannelId)
}

func TestLegacyCustomerContractMigrationWithoutRulesIsRejected(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&CustomerContract{},
		&CustomerContractEntityRule{},
		&CustomerContractEntityAudit{},
		&Token{},
	))
	admin, user := createCustomerContractFixture(t, db)
	_, err := MigrateLegacyCustomerContractForUser(MigrateCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, Reason: "nothing to migrate",
	})
	require.ErrorIs(t, err, ErrCustomerContractMigrationNothingToDo)
}

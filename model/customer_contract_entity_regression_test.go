package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestContractEntityKeepsAcceptedSourcesAfterGroupRemoval(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	channel := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	rules := []CustomerContractEntityRuleInput{{PublicModel: "model-a", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80000000}}
	snapshot, err := CreateCustomerContractEntity(CreateCustomerContractParams{UserId: user.Id, AdminUserId: admin.Id, Name: "Retained", Enabled: true, Reason: "create", Rules: rules})
	require.NoError(t, err)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1}`))
	for _, enabled := range []bool{true, false, true} {
		rules[0].RatioUnits = 70000000
		snapshot, err = ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{ContractId: snapshot.Id, AdminUserId: admin.Id, ExpectedVersion: snapshot.Version, Name: "Retained", Enabled: &enabled, Reason: "edit after group removal", Rules: rules})
		require.NoError(t, err)
		assert.Equal(t, enabled, snapshot.Enabled)
		assert.EqualValues(t, 70000000, snapshot.Rules[0].RatioUnits)
	}
	_, err = CreateCustomerContractEntity(CreateCustomerContractParams{UserId: user.Id, AdminUserId: admin.Id, Name: "New source", Enabled: true, Reason: "missing group", Rules: rules})
	require.ErrorIs(t, err, ErrCustomerContractInvalidRule)
}

func TestContractEntityAcceptsTypedBatchWithoutGenericAbilities(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	channel := Channel{Type: constant.ChannelTypeAzureBatch, Status: common.ChannelStatusEnabled, Models: "batch", Group: "contract-a"}
	require.NoError(t, db.Create(&channel).Error)
	snapshot, err := CreateCustomerContractEntity(CreateCustomerContractParams{UserId: user.Id, AdminUserId: admin.Id, Name: "Batch", Enabled: true, Reason: "typed rule", Rules: []CustomerContractEntityRuleInput{{PublicModel: "batch", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80000000}}})
	require.NoError(t, err)
	require.Len(t, snapshot.Rules, 1)
	assert.True(t, snapshot.Rules[0].Available)
	var count int64
	require.NoError(t, db.Model(&Ability{}).Where("channel_id = ?", channel.Id).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, db.Model(&channel).Update("status", common.ChannelStatusManuallyDisabled).Error)
	enabled := false
	rules := []CustomerContractEntityRuleInput{{PublicModel: "batch", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80000000}}
	disabled, err := ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{ContractId: snapshot.Id, AdminUserId: admin.Id, ExpectedVersion: snapshot.Version, Name: "Batch", Reason: "disable unavailable channel", Enabled: &enabled, Rules: rules})
	require.NoError(t, err)
	assert.False(t, disabled.Enabled)
	// Re-enabling with the unchanged rule source succeeds: an unavailable
	// channel never cancels the discount nor blocks re-enable.
	enabled = true
	reenabled, err := ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{ContractId: snapshot.Id, AdminUserId: admin.Id, ExpectedVersion: disabled.Version, Name: "Batch", Reason: "reenable unchanged source", Enabled: &enabled, Rules: rules})
	require.NoError(t, err)
	assert.True(t, reenabled.Enabled)

	// Adding a NEW rule whose channel cannot serve the model is rejected.
	_, err = ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{ContractId: snapshot.Id, AdminUserId: admin.Id, ExpectedVersion: reenabled.Version, Name: "Batch", Reason: "add unavailable source", Enabled: &enabled, Rules: append(rules, CustomerContractEntityRuleInput{PublicModel: "batch", ChannelId: channel.Id + 5_000, RouteGroup: "contract-a", RatioUnits: 80000000})})
	require.Error(t, err)
}
func TestContractEntityInvalidReplacementWritesNothing(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	channel := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	rules := []CustomerContractEntityRuleInput{{PublicModel: "model-a", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80000000}}
	before, err := CreateCustomerContractEntity(CreateCustomerContractParams{UserId: user.Id, AdminUserId: admin.Id, Name: "original", Enabled: true, Reason: "initial", Rules: rules})
	require.NoError(t, err)
	rules = append(rules, CustomerContractEntityRuleInput{PublicModel: "unavailable", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 70000000})
	_, err = ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{ContractId: before.Id, AdminUserId: admin.Id, ExpectedVersion: before.Version, Name: "modified", Reason: "invalid replacement", Rules: rules})
	require.Error(t, err)
	after, err := GetContractEntitySnapshot(before.Id, true)
	require.NoError(t, err)
	assert.Equal(t, before, after)
	var count int64
	require.NoError(t, db.Model(&CustomerContractEntityAudit{}).Where("contract_id = ?", before.Id).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, db.First(&user, user.Id).Error)
	assert.EqualValues(t, 1, user.AuthVersion)
}
func TestContractEntityRedisFenceFailurePreventsMutation(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	channel := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	rules := []CustomerContractEntityRuleInput{{PublicModel: "model-a", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80000000}}
	before, err := CreateCustomerContractEntity(CreateCustomerContractParams{UserId: user.Id, AdminUserId: admin.Id, Name: "original", Enabled: true, Reason: "initial", Rules: rules})
	require.NoError(t, err)
	redis := useUserCacheMiniRedis(t)
	redis.SetError("fence unavailable")
	disabled := false
	_, err = ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{ContractId: before.Id, AdminUserId: admin.Id, ExpectedVersion: before.Version, Name: "original", Enabled: &disabled, Reason: "disable", Rules: rules})
	require.Error(t, err)
	after, err := GetContractEntitySnapshot(before.Id, true)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestContractMigrationInvalidatesCachedTokenBindingAndFencesRefill(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	require.NoError(t, db.Model(&user).Updates(map[string]any{"contract_mode": true, "contract_version": 7}).Error)
	require.NoError(t, db.Create(&CustomerModelContract{UserId: user.Id, PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 80000000}).Error)
	token := Token{UserId: user.Id, Key: "migration-cache-test", Group: "obsolete", RemainQuota: 100}
	require.NoError(t, db.Create(&token).Error)
	redis := useUserCacheMiniRedis(t)
	_, err := cacheInitToken(token)
	require.NoError(t, err)
	snapshot, err := MigrateLegacyCustomerContractForUser(MigrateCustomerContractParams{UserId: user.Id, AdminUserId: admin.Id, Reason: "cache migration"})
	require.NoError(t, err)
	assert.EqualValues(t, 7, snapshot.Version)
	assert.False(t, redis.Exists(getTokenCacheKey(token.Key)))
	result, err := cacheInitToken(token)
	require.NoError(t, err)
	assert.Zero(t, result, "pre-migration snapshots cannot refill the token hash")
	require.NoError(t, db.First(&token, token.Id).Error)
	assert.Equal(t, snapshot.Id, token.ContractId)
	assert.Equal(t, "obsolete", token.Group)
}

func TestContractEntityAcceptsSameModelOnSeveralChannelsWithOneDiscount(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	channelA := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	channelB := createCustomerContractAbility(t, db, "contract-b", "model-a", common.ChannelStatusEnabled)
	// 0.8 and 80% normalize to the same fixed point value.
	rules := []CustomerContractEntityRuleInput{
		{PublicModel: "model-a", ChannelId: channelA.Id, RouteGroup: "contract-a", RatioUnits: 80000000},
		{PublicModel: "model-a", ChannelId: channelB.Id, RouteGroup: "contract-b", RatioUnits: 80000000},
	}
	snapshot, err := CreateCustomerContractEntity(CreateCustomerContractParams{UserId: user.Id, AdminUserId: admin.Id, Name: "Multi channel", Enabled: true, Reason: "same discount", Rules: rules})
	require.NoError(t, err)
	require.Len(t, snapshot.Rules, 2)

	reload, err := GetContractEntitySnapshot(snapshot.Id, false)
	require.NoError(t, err)
	require.Len(t, reload.Rules, 2)

	// A conflicting discount for one of the channels is rejected and nothing
	// is committed.
	conflict := []CustomerContractEntityRuleInput{
		{PublicModel: "model-a", ChannelId: channelA.Id, RouteGroup: "contract-a", RatioUnits: 80000000},
		{PublicModel: "model-a", ChannelId: channelB.Id, RouteGroup: "contract-b", RatioUnits: 90000000},
	}
	_, err = ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{ContractId: snapshot.Id, AdminUserId: admin.Id, ExpectedVersion: reload.Version, Name: "Multi channel", Reason: "conflicting discount", Rules: conflict})
	require.Error(t, err)
	kept, err := GetContractEntitySnapshot(snapshot.Id, false)
	require.NoError(t, err)
	require.Len(t, kept.Rules, 2, "a failed submission changes nothing")

	// The same model+channel pair is a duplicate even with another group.
	duplicate := []CustomerContractEntityRuleInput{
		{PublicModel: "model-a", ChannelId: channelA.Id, RouteGroup: "contract-a", RatioUnits: 80000000},
		{PublicModel: "model-a", ChannelId: channelA.Id, RouteGroup: "contract-b", RatioUnits: 80000000},
	}
	_, err = ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{ContractId: snapshot.Id, AdminUserId: admin.Id, ExpectedVersion: kept.Version, Name: "Multi channel", Reason: "duplicate source", Rules: duplicate})
	require.Error(t, err)

	// Names differing only by letter case stay rejected.
	caseOnly := append(rules, CustomerContractEntityRuleInput{PublicModel: "Model-A", ChannelId: channelA.Id + 1, RouteGroup: "contract-a", RatioUnits: 80000000})
	_, err = ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{ContractId: snapshot.Id, AdminUserId: admin.Id, ExpectedVersion: kept.Version + 1, Name: "Multi channel", Reason: "case duplicate", Rules: caseOnly})
	require.Error(t, err)
}

func TestContractEntityRuleIndexMigrationDropsLegacyConstraint(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&User{}, &Channel{}, &Ability{}, &Token{}, &CustomerModelContract{}, &CustomerContract{},
	))
	createCustomerContractFixture(t, db)
	channel := createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)

	// Rebuild the legacy table shape: one row per (contract, public_model)
	// enforced by the historical unique index.
	require.NoError(t, db.Migrator().DropTable(&CustomerContractEntityRule{}))
	require.NoError(t, db.Exec(`CREATE TABLE customer_contract_entity_rules (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		contract_id INTEGER,
		public_model TEXT,
		channel_id INTEGER,
		route_group TEXT,
		ratio_units INTEGER,
		created_at BIGINT,
		updated_at BIGINT
	)`).Error)
	require.NoError(t, db.Exec(`CREATE UNIQUE INDEX idx_cc_entity_rule_contract_model
		ON customer_contract_entity_rules (contract_id, public_model)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO customer_contract_entity_rules
		(contract_id, public_model, channel_id, route_group, ratio_units, created_at, updated_at)
		VALUES (0, 'model-a', ?, 'contract-a', 80000000, 1, 1)`, channel.Id).Error)

	var original CustomerContractEntityRule
	require.NoError(t, db.First(&original).Error)

	require.NoError(t, migrateCustomerContractEntityRuleIndex(db))
	require.NoError(t, db.AutoMigrate(&CustomerContractEntityRule{}))

	assert.False(t, db.Migrator().HasIndex(&CustomerContractEntityRule{}, "idx_cc_entity_rule_contract_model"),
		"the legacy unique index must be gone")
	assert.True(t, db.Migrator().HasIndex(&CustomerContractEntityRule{}, "idx_cc_entity_rule_contract_model_channel"),
		"the new unique index must exist")

	// The historical single-channel rule survives the migration unchanged.
	var kept CustomerContractEntityRule
	require.NoError(t, db.First(&kept).Error)
	assert.Equal(t, "model-a", kept.PublicModel)
	assert.Equal(t, original, kept, "all persisted rule facts must survive upgrade")

	// A second rule for the same model on another channel is now legal; the
	// same model+channel pair stays rejected.
	other := createCustomerContractAbility(t, db, "contract-b", "model-a", common.ChannelStatusEnabled)
	require.NoError(t, db.Create(&CustomerContractEntityRule{
		ContractId: kept.ContractId, PublicModel: "model-a", ChannelId: other.Id,
		RouteGroup: "contract-b", RatioUnits: 80000000,
	}).Error)
	require.Error(t, db.Create(&CustomerContractEntityRule{
		ContractId: kept.ContractId, PublicModel: "model-a", ChannelId: channel.Id,
		RouteGroup: "contract-b", RatioUnits: 80000000,
	}).Error)

	// The migration is idempotent on repeated starts.
	require.NoError(t, migrateCustomerContractEntityRuleIndex(db))
	require.NoError(t, db.AutoMigrate(&CustomerContractEntityRule{}))
	var preserved CustomerContractEntityRule
	require.NoError(t, db.First(&preserved, original.Id).Error)
	assert.Equal(t, original, preserved)
}

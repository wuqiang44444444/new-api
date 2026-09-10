package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

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
	enabled = true
	_, err = ReplaceCustomerContractEntity(ReplaceCustomerContractEntityParams{ContractId: snapshot.Id, AdminUserId: admin.Id, ExpectedVersion: disabled.Version, Name: "Batch", Reason: "invalid reenable", Enabled: &enabled, Rules: rules})
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

package model

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupCustomerContractTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB := DB
	previousDatabaseType := common.MainDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	previousGroupRatios := ratio_setting.GroupRatio2JSONString()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&User{},
		&Channel{},
		&Ability{},
		&CustomerModelContract{},
		&CustomerContractAudit{},
	))
	DB = db
	common.RedisEnabled = false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	initCol()
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"contract-a":0.87,"contract-b":1}`))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB = previousDB
		common.RedisEnabled = previousRedisEnabled
		common.SetMainDatabaseType(previousDatabaseType)
		initCol()
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroupRatios))
		_ = sqlDB.Close()
	})
	return db
}

func createCustomerContractFixture(t *testing.T, db *gorm.DB) (User, User) {
	t.Helper()
	admin := User{Username: "contract-admin", AffCode: "contract-admin-aff", Role: common.RoleAdminUser, AuthVersion: 1}
	user := User{Username: "contract-user", AffCode: "contract-user-aff", Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&admin).Error)
	require.NoError(t, db.Create(&user).Error)
	return admin, user
}

func createCustomerContractAbility(t *testing.T, db *gorm.DB, group string, modelName string, status int) Channel {
	t.Helper()
	channel := Channel{Name: group + "-channel", Group: group, Models: modelName, Key: "test-key", Status: status}
	require.NoError(t, db.Create(&channel).Error)
	priority := int64(0)
	require.NoError(t, db.Create(&Ability{
		Group: group, Model: modelName, ChannelId: channel.Id, Enabled: true, Priority: &priority,
	}).Error)
	return channel
}

func TestCustomerContractReplaceIsAtomicVersionedAndAudited(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	createCustomerContractAbility(t, db, "contract-a", "claude-sonnet-5", common.ChannelStatusEnabled)

	native, err := GetCustomerContractSnapshot(user.Id)
	require.NoError(t, err)
	assert.False(t, native.Enabled)
	assert.Zero(t, native.Version)
	assert.Empty(t, native.Rules)

	snapshot, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "signed customer contract",
		Rules: []CustomerContractRule{{PublicModel: "claude-sonnet-5", RouteGroup: "contract-a", RatioUnits: 80_000_000}},
	})
	require.NoError(t, err)
	assert.True(t, snapshot.Enabled)
	assert.EqualValues(t, 1, snapshot.Version)
	require.Len(t, snapshot.Rules, 1)
	assert.Equal(t, "claude-sonnet-5", snapshot.Rules[0].PublicModel)
	assert.True(t, snapshot.Rules[0].Available)

	var saved User
	require.NoError(t, db.First(&saved, user.Id).Error)
	assert.True(t, saved.ContractMode)
	assert.EqualValues(t, 1, saved.ContractVersion)
	assert.EqualValues(t, 2, saved.AuthVersion)

	audits, total, err := GetCustomerContractAudits(user.Id, 0, 20)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, audits, 1)
	assert.Equal(t, "create", audits[0].Operation)
	assert.Equal(t, "signed customer contract", audits[0].Reason)
	assert.NotEmpty(t, audits[0].BeforeState)
	assert.NotEmpty(t, audits[0].AfterState)
	var auditState customerContractAuditState
	require.NoError(t, common.Unmarshal([]byte(audits[0].AfterState), &auditState))
	require.Len(t, auditState.PricingFacts, 1)
	assert.Equal(t, "0.8", auditState.PricingFacts[0].ContractDiscount)
	assert.Equal(t, "0.87", auditState.PricingFacts[0].NativeGroupRatio)
	assert.Equal(t, "0.696", auditState.PricingFacts[0].EffectiveMultiplier)
}

func TestCustomerContractRejectsInvalidRulesWithoutPartialWrite(t *testing.T) {
	tests := []struct {
		name  string
		rules []CustomerContractRule
	}{
		{name: "duplicate exact model", rules: []CustomerContractRule{
			{PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 80_000_000},
			{PublicModel: "model-a", RouteGroup: "contract-b", RatioUnits: 50_000_000},
		}},
		{name: "case-only duplicate model", rules: []CustomerContractRule{
			{PublicModel: "Model-A", RouteGroup: "contract-a", RatioUnits: 80_000_000},
			{PublicModel: "model-a", RouteGroup: "contract-b", RatioUnits: 50_000_000},
		}},
		{name: "empty model", rules: []CustomerContractRule{{PublicModel: " ", RouteGroup: "contract-a", RatioUnits: 80_000_000}}},
		{name: "auto group", rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "auto", RatioUnits: 80_000_000}}},
		{name: "all group selector", rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "all", RatioUnits: 80_000_000}}},
		{name: "null group selector", rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "null", RatioUnits: 80_000_000}}},
		{name: "unknown group", rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "missing", RatioUnits: 80_000_000}}},
		{name: "zero ratio", rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 0}}},
		{name: "ratio above one", rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: hosttypes.CustomerContractRatioScale + 1}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := setupCustomerContractTestDB(t)
			admin, user := createCustomerContractFixture(t, db)
			_, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
				UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: false,
				Reason: "invalid rule must roll back", Rules: test.rules,
			})
			require.Error(t, err)

			var ruleCount, auditCount int64
			require.NoError(t, db.Model(&CustomerModelContract{}).Count(&ruleCount).Error)
			require.NoError(t, db.Model(&CustomerContractAudit{}).Count(&auditCount).Error)
			assert.Zero(t, ruleCount)
			assert.Zero(t, auditCount)
			var saved User
			require.NoError(t, db.First(&saved, user.Id).Error)
			assert.False(t, saved.ContractMode)
			assert.Zero(t, saved.ContractVersion)
			assert.EqualValues(t, 1, saved.AuthVersion)
		})
	}
}

func TestCustomerContractActivationFailsClosedWhenRouteIsUnavailable(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusManuallyDisabled)

	_, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "cannot activate unavailable route",
		Rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 80_000_000}},
	})
	require.ErrorIs(t, err, ErrCustomerContractRuleUnavailable)

	draft, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: false, Reason: "save disabled draft",
		Rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 80_000_000}},
	})
	require.NoError(t, err)
	assert.False(t, draft.Enabled)
	require.Len(t, draft.Rules, 1)
	assert.False(t, draft.Rules[0].Available)
}

func TestCustomerContractAvailabilityUsesExactCaseAndEnabledChannels(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	createCustomerContractAbility(t, db, "contract-a", "Model-A", common.ChannelStatusEnabled)
	createCustomerContractAbility(t, db, "contract-a", "disabled-model", common.ChannelStatusManuallyDisabled)

	models, err := GetCustomerContractAvailableModelsForGroup("contract-a")
	require.NoError(t, err)
	assert.Contains(t, models, "Model-A")
	assert.NotContains(t, models, "model-a")
	assert.NotContains(t, models, "disabled-model")

	_, err = ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "case mismatch must fail",
		Rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 80_000_000}},
	})
	require.ErrorIs(t, err, ErrCustomerContractRuleUnavailable)
}

func TestCustomerContractEmptyRulesRemainRestrictedUntilExplicitDisable(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)

	created, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "activate contract",
		Rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 80_000_000}},
	})
	require.NoError(t, err)

	empty, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: created.Version, Enabled: true, Reason: "remove last model",
	})
	require.NoError(t, err)
	assert.True(t, empty.Enabled)
	assert.Empty(t, empty.Rules)

	disabled, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: empty.Version, Enabled: false, Reason: "restore native access",
	})
	require.NoError(t, err)
	assert.False(t, disabled.Enabled)
	assert.Empty(t, disabled.Rules)
}

func TestCustomerContractVersionConflictPreservesWinningState(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	createCustomerContractAbility(t, db, "contract-b", "model-b", common.ChannelStatusEnabled)

	winning, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "first editor wins",
		Rules: []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 80_000_000}},
	})
	require.NoError(t, err)
	_, err = ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "stale editor",
		Rules: []CustomerContractRule{{PublicModel: "model-b", RouteGroup: "contract-b", RatioUnits: 50_000_000}},
	})
	require.ErrorIs(t, err, ErrCustomerContractVersionConflict)

	current, err := GetCustomerContractSnapshot(user.Id)
	require.NoError(t, err)
	assert.Equal(t, winning, current)
	var auditCount int64
	require.NoError(t, db.Model(&CustomerContractAudit{}).Count(&auditCount).Error)
	assert.EqualValues(t, 1, auditCount)
}

func TestCustomerContractExactModelIdentityAndPerUserPrices(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, userA := createCustomerContractFixture(t, db)
	userB := User{Username: "contract-user-b", AffCode: "contract-user-b-aff", Group: "default", AuthVersion: 1}
	require.NoError(t, db.Create(&userB).Error)
	createCustomerContractAbility(t, db, "contract-a", "Model-A", common.ChannelStatusEnabled)
	createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)

	for _, item := range []struct {
		user      User
		modelName string
		ratio     int64
	}{
		{user: userA, modelName: "Model-A", ratio: 80_000_000},
		{user: userB, modelName: "model-a", ratio: 50_000_000},
	} {
		_, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
			UserId: item.user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "customer-specific price",
			Rules: []CustomerContractRule{{PublicModel: item.modelName, RouteGroup: "contract-a", RatioUnits: item.ratio}},
		})
		require.NoError(t, err)
	}

	snapshotA, err := GetCustomerContractSnapshot(userA.Id)
	require.NoError(t, err)
	snapshotB, err := GetCustomerContractSnapshot(userB.Id)
	require.NoError(t, err)
	require.Len(t, snapshotA.Rules, 1)
	require.Len(t, snapshotB.Rules, 1)
	assert.Equal(t, "Model-A", snapshotA.Rules[0].PublicModel)
	assert.Equal(t, "model-a", snapshotB.Rules[0].PublicModel)
	assert.EqualValues(t, 80_000_000, snapshotA.Rules[0].RatioUnits)
	assert.EqualValues(t, 50_000_000, snapshotB.Rules[0].RatioUnits)
}

func TestCustomerContractSeedanceAvailabilityUsesDedicatedChannelGroup(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	channel := seedanceTestChannel("seedance-contract", common.ChannelStatusEnabled)
	channel.Group = "contract-a"
	require.NoError(t, db.Create(channel).Error)

	_, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "activate link contract",
		Rules: []CustomerContractRule{{PublicModel: "seedance-contract", RouteGroup: "contract-b", RatioUnits: 60_000_000}},
	})
	require.ErrorIs(t, err, ErrCustomerContractRuleUnavailable)

	snapshot, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true, Reason: "activate correct link group",
		Rules: []CustomerContractRule{{PublicModel: "seedance-contract", RouteGroup: "contract-a", RatioUnits: 60_000_000}},
	})
	require.NoError(t, err)
	require.Len(t, snapshot.Rules, 1)
	assert.True(t, snapshot.Rules[0].Available)
}

func TestCustomerContractRequiresChangeReason(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin, user := createCustomerContractFixture(t, db)
	_, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true,
	})
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrCustomerContractVersionConflict))

	var auditCount int64
	require.NoError(t, db.Model(&CustomerContractAudit{}).Count(&auditCount).Error)
	assert.Zero(t, auditCount)
}

func TestUserListIncludesCustomerContractRuleCount(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	_, user := createCustomerContractFixture(t, db)
	require.NoError(t, db.Create([]CustomerModelContract{
		{UserId: user.Id, PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 80_000_000},
		{UserId: user.Id, PublicModel: "model-b", RouteGroup: "contract-b", RatioUnits: 80_000_000},
	}).Error)

	users, _, err := GetAllUsers(&common.PageInfo{Page: 1, PageSize: 20}, NewUserSortOptions("id", "asc"))
	require.NoError(t, err)
	var listed *User
	for _, candidate := range users {
		if candidate.Id == user.Id {
			listed = candidate
			break
		}
	}
	require.NotNil(t, listed)
	assert.Equal(t, 2, listed.ContractRuleCount)
}

func TestCustomerContractCommitFencesEveryCachedKeyImmediately(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	server := useUserCacheMiniRedis(t)
	admin, user := createCustomerContractFixture(t, db)
	createCustomerContractAbility(t, db, "contract-a", "model-a", common.ChannelStatusEnabled)
	require.NoError(t, populateUserCache(user))
	stale := *user.ToBaseUser()

	snapshot, err := ReplaceCustomerContract(ReplaceCustomerContractParams{
		UserId: user.Id, AdminUserId: admin.Id, ExpectedVersion: 0, Enabled: true,
		Reason: "activate all existing keys",
		Rules:  []CustomerContractRule{{PublicModel: "model-a", RouteGroup: "contract-a", RatioUnits: 80_000_000}},
	})
	require.NoError(t, err)
	assert.EqualValues(t, 1, snapshot.Version)

	cached, err := cacheGetUserBase(user.Id)
	require.NoError(t, err)
	assert.True(t, cached.ContractMode)
	assert.EqualValues(t, 1, cached.ContractVersion)
	assert.EqualValues(t, 2, cached.AuthVersion)
	assert.False(t, server.Exists(getUserAuthFenceKey(user.Id)))

	err = writeUserCache(&stale, true)
	require.ErrorIs(t, err, ErrUserAuthCachePending)
	committed, err := common.RDB.Get(t.Context(), getUserAuthVersionKey(user.Id)).Result()
	require.NoError(t, err)
	assert.Equal(t, "2", committed)
}

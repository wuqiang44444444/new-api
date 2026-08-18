package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createCustomerContractAdminListUser(t *testing.T, db *gorm.DB, username string, role int, enabled bool, version int64) User {
	t.Helper()
	user := User{
		Username: username, DisplayName: username + " display", AffCode: username + "-aff",
		Role: role, Group: "default", AuthVersion: 1,
		ContractMode: enabled, ContractVersion: version,
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func TestCustomerContractAdminListPreservesStatusScopeAndAvailability(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin := createCustomerContractAdminListUser(t, db, "list-admin", common.RoleAdminUser, false, 0)
	active := createCustomerContractAdminListUser(t, db, "active-customer", common.RoleCommonUser, true, 1)
	zero := createCustomerContractAdminListUser(t, db, "zero-customer", common.RoleCommonUser, true, 2)
	inactive := createCustomerContractAdminListUser(t, db, "inactive-customer", common.RoleCommonUser, false, 3)
	_ = createCustomerContractAdminListUser(t, db, "native-customer", common.RoleCommonUser, false, 0)
	_ = createCustomerContractAdminListUser(t, db, "peer-admin", common.RoleAdminUser, true, 1)
	deleted := createCustomerContractAdminListUser(t, db, "deleted-customer", common.RoleCommonUser, true, 1)
	require.NoError(t, db.Delete(&deleted).Error)

	createCustomerContractAbility(t, db, "contract-a", "available-model", common.ChannelStatusEnabled)
	require.NoError(t, db.Create([]CustomerModelContract{
		{UserId: active.Id, PublicModel: "available-model", RouteGroup: "contract-a", RatioUnits: 80_000_000},
		{UserId: active.Id, PublicModel: "missing-model", RouteGroup: "contract-a", RatioUnits: 60_000_000},
		{UserId: inactive.Id, PublicModel: "available-model", RouteGroup: "contract-a", RatioUnits: 50_000_000},
	}).Error)
	require.NoError(t, db.Create([]CustomerContractAudit{
		{UserId: active.Id, ContractVersion: 1, AdminUserId: admin.Id, Operation: "create", Reason: "active", CreatedAt: 100},
		{UserId: zero.Id, ContractVersion: 2, AdminUserId: admin.Id, Operation: "update", Reason: "zero", CreatedAt: 300},
		{UserId: inactive.Id, ContractVersion: 3, AdminUserId: admin.Id, Operation: "disable", Reason: "inactive", CreatedAt: 200},
	}).Error)

	items, total, summary, err := GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Limit: 20,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)
	assert.Equal(t, CustomerContractAdminSummary{Total: 3, Active: 1, ZeroAccess: 1, Inactive: 1}, summary)
	require.Len(t, items, 3)
	assert.Equal(t, zero.Id, items[0].UserId)
	assert.Equal(t, CustomerContractAdminStatusZeroAccess, items[0].ContractStatus)
	assert.Equal(t, admin.Username, items[0].AdminUsername)

	byUser := make(map[int]CustomerContractAdminListItem, len(items))
	for _, item := range items {
		byUser[item.UserId] = item
	}
	assert.Equal(t, CustomerContractAdminStatusActive, byUser[active.Id].ContractStatus)
	assert.Equal(t, 2, byUser[active.Id].RuleCount)
	assert.Equal(t, 1, byUser[active.Id].UnavailableRuleCount)
	assert.Equal(t, CustomerContractAdminStatusInactive, byUser[inactive.Id].ContractStatus)
	assert.Equal(t, 0, byUser[inactive.Id].UnavailableRuleCount)
	_, containsDeleted := byUser[deleted.Id]
	assert.False(t, containsDeleted)
}

func TestCustomerContractAdminListFiltersWithoutChangingSummary(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin := createCustomerContractAdminListUser(t, db, "filter-admin", common.RoleAdminUser, false, 0)
	active := createCustomerContractAdminListUser(t, db, "alpha-customer", common.RoleCommonUser, true, 1)
	zero := createCustomerContractAdminListUser(t, db, "zero-customer", common.RoleCommonUser, true, 2)
	_ = createCustomerContractAdminListUser(t, db, "inactive-customer", common.RoleCommonUser, false, 3)
	createCustomerContractAbility(t, db, "contract-a", "model-alpha", common.ChannelStatusEnabled)
	require.NoError(t, db.Create(&CustomerModelContract{
		UserId: active.Id, PublicModel: "model-alpha", RouteGroup: "contract-a", RatioUnits: 80_000_000,
	}).Error)
	require.NoError(t, db.Create([]CustomerContractAudit{
		{UserId: active.Id, ContractVersion: 1, AdminUserId: admin.Id, Operation: "create", Reason: "active", CreatedAt: 100},
		{UserId: zero.Id, ContractVersion: 2, AdminUserId: admin.Id, Operation: "update", Reason: "zero", CreatedAt: 200},
	}).Error)

	items, total, summary, err := GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Keyword: "MODEL-ALPHA", Limit: 20,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, active.Id, items[0].UserId)
	assert.EqualValues(t, 1, total)
	assert.EqualValues(t, 3, summary.Total)

	items, total, summary, err = GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Keyword: "zero-customer", Status: CustomerContractAdminStatusZeroAccess, Limit: 20,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, zero.Id, items[0].UserId)
	assert.EqualValues(t, 1, total)
	assert.EqualValues(t, 1, summary.ZeroAccess)

	items, total, _, err = GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Keyword: strconv.Itoa(zero.Id), Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, zero.Id, items[0].UserId)
	assert.EqualValues(t, 1, total)
}

func TestCustomerContractAdminListEnforcesRoleBoundary(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	_ = createCustomerContractAdminListUser(t, db, "role-admin", common.RoleAdminUser, false, 0)
	_ = createCustomerContractAdminListUser(t, db, "role-customer", common.RoleCommonUser, true, 1)
	_ = createCustomerContractAdminListUser(t, db, "role-peer", common.RoleAdminUser, true, 1)
	_ = createCustomerContractAdminListUser(t, db, "role-root", common.RoleRootUser, true, 1)

	adminItems, adminTotal, _, err := GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Limit: 20,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 1, adminTotal)
	require.Len(t, adminItems, 1)
	assert.Equal(t, "role-customer", adminItems[0].Username)

	rootItems, rootTotal, _, err := GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleRootUser, Limit: 20,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 3, rootTotal)
	assert.Len(t, rootItems, 3)
}

func TestCustomerContractAdminListRejectsUnknownStatus(t *testing.T) {
	setupCustomerContractTestDB(t)
	_, _, _, err := GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleRootUser, Status: "unknown", Limit: 20,
	})
	require.Error(t, err)
}

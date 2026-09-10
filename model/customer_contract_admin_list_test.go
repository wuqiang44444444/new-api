package model

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func createCustomerContractAdminListUser(t *testing.T, db *gorm.DB, username string, role int) User {
	t.Helper()
	user := User{
		Username: username, DisplayName: username + " display", AffCode: username + "-aff",
		Role: role, Group: "default", AuthVersion: 1,
	}
	require.NoError(t, db.Create(&user).Error)
	return user
}

// createCustomerContractAdminListContract inserts one contract entity row
// directly, including zero-rule contracts that the write API rejects but the
// read-only overview must still be able to display.
func createCustomerContractAdminListContract(t *testing.T, db *gorm.DB, userId int, name string, enabled bool, version int64, adminUserId int, createdAt int64) CustomerContract {
	t.Helper()
	contract := CustomerContract{UserId: userId, Name: name, Enabled: enabled, Version: version}
	require.NoError(t, db.Create(&contract).Error)
	require.NoError(t, db.Create(&CustomerContractEntityAudit{
		ContractId: contract.Id, UserId: userId, ContractVersion: version,
		AdminUserId: adminUserId, Operation: "create", Reason: "list fixture", CreatedAt: createdAt,
	}).Error)
	return contract
}

func TestCustomerContractAdminListPreservesStatusScopeAndAvailability(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin := createCustomerContractAdminListUser(t, db, "list-admin", common.RoleAdminUser)
	active := createCustomerContractAdminListUser(t, db, "active-customer", common.RoleCommonUser)
	zero := createCustomerContractAdminListUser(t, db, "zero-customer", common.RoleCommonUser)
	inactive := createCustomerContractAdminListUser(t, db, "inactive-customer", common.RoleCommonUser)
	_ = createCustomerContractAdminListUser(t, db, "native-customer", common.RoleCommonUser)
	_ = createCustomerContractAdminListUser(t, db, "peer-admin", common.RoleAdminUser)
	deleted := createCustomerContractAdminListUser(t, db, "deleted-customer", common.RoleCommonUser)
	require.NoError(t, db.Delete(&deleted).Error)

	channel := createCustomerContractAbility(t, db, "contract-a", "available-model", common.ChannelStatusEnabled)
	activeContract := createCustomerContractAdminListContract(t, db, active.Id, "Active Contract", true, 1, admin.Id, 100)
	zeroContract := createCustomerContractAdminListContract(t, db, zero.Id, "Zero Contract", true, 1, admin.Id, 300)
	inactiveContract := createCustomerContractAdminListContract(t, db, inactive.Id, "Inactive Contract", false, 1, admin.Id, 200)
	deletedContract := createCustomerContractAdminListContract(t, db, deleted.Id, "Deleted Owner", true, 1, admin.Id, 150)
	_ = zeroContract
	require.NoError(t, db.Create([]CustomerContractEntityRule{
		{ContractId: activeContract.Id, PublicModel: "available-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80_000_000},
		{ContractId: activeContract.Id, PublicModel: "missing-model", ChannelId: channel.Id + 5_000, RouteGroup: "contract-a", RatioUnits: 60_000_000},
		{ContractId: inactiveContract.Id, PublicModel: "available-model", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 50_000_000},
	}).Error)

	items, total, summary, err := GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Limit: 20,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 3, total, "contracts of deleted owners are excluded")
	assert.Equal(t, CustomerContractAdminSummary{Total: 3, Active: 1, ZeroAccess: 1, Inactive: 1}, summary)
	require.Len(t, items, 3)
	// All contracts share one update second in the fixture, so the newest id
	// leads; the deleted owner's contract is gone.
	assert.Equal(t, inactiveContract.Id, items[0].ContractId)
	assert.Equal(t, CustomerContractAdminStatusInactive, items[0].ContractStatus)
	assert.Equal(t, admin.Username, items[0].AdminUsername)

	byContract := make(map[int]CustomerContractAdminListItem, len(items))
	for _, item := range items {
		byContract[item.ContractId] = item
	}
	activeItem := byContract[activeContract.Id]
	assert.Equal(t, CustomerContractAdminStatusActive, activeItem.ContractStatus)
	assert.Equal(t, 2, activeItem.RuleCount)
	assert.Equal(t, 1, activeItem.UnavailableRuleCount)
	inactiveItem := byContract[inactiveContract.Id]
	assert.Equal(t, CustomerContractAdminStatusInactive, inactiveItem.ContractStatus)
	assert.Equal(t, 0, inactiveItem.UnavailableRuleCount)
	_, containsDeleted := byContract[deletedContract.Id]
	assert.False(t, containsDeleted)
}

func TestCustomerContractAdminListFiltersWithoutChangingSummary(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	admin := createCustomerContractAdminListUser(t, db, "filter-admin", common.RoleAdminUser)
	active := createCustomerContractAdminListUser(t, db, "alpha-customer", common.RoleCommonUser)
	zero := createCustomerContractAdminListUser(t, db, "zero-customer", common.RoleCommonUser)
	inactive := createCustomerContractAdminListUser(t, db, "inactive-customer", common.RoleCommonUser)
	channel := createCustomerContractAbility(t, db, "contract-a", "model-alpha", common.ChannelStatusEnabled)
	activeContract := createCustomerContractAdminListContract(t, db, active.Id, "Alpha Contract", true, 1, admin.Id, 100)
	zeroContract := createCustomerContractAdminListContract(t, db, zero.Id, "Zero Contract", true, 1, admin.Id, 200)
	createCustomerContractAdminListContract(t, db, inactive.Id, "Inactive Contract", false, 1, admin.Id, 300)
	require.NoError(t, db.Create(&CustomerContractEntityRule{
		ContractId: activeContract.Id, PublicModel: "model-alpha", ChannelId: channel.Id, RouteGroup: "contract-a", RatioUnits: 80_000_000,
	}).Error)

	items, total, summary, err := GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Keyword: "MODEL-ALPHA", Limit: 20,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, activeContract.Id, items[0].ContractId)
	assert.EqualValues(t, 1, total)
	assert.EqualValues(t, 3, summary.Total)

	items, total, summary, err = GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Keyword: "zero-customer", Status: CustomerContractAdminStatusZeroAccess, Limit: 20,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, zeroContract.Id, items[0].ContractId)
	assert.EqualValues(t, 1, total)
	assert.EqualValues(t, 1, summary.ZeroAccess)

	items, total, _, err = GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Keyword: strconv.Itoa(zero.Id), Limit: 1,
	})
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, zeroContract.Id, items[0].ContractId)
	assert.EqualValues(t, 1, total)

	items, total, _, err = GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Keyword: "Alpha Contract", Status: CustomerContractAdminStatusInactive, Limit: 20,
	})
	require.NoError(t, err)
	assert.Len(t, items, 0)
	assert.EqualValues(t, 0, total)
}

func TestCustomerContractAdminListEnforcesRoleBoundary(t *testing.T) {
	db := setupCustomerContractTestDB(t)
	_ = createCustomerContractAdminListUser(t, db, "role-admin", common.RoleAdminUser)
	customer := createCustomerContractAdminListUser(t, db, "role-customer", common.RoleCommonUser)
	peer := createCustomerContractAdminListUser(t, db, "role-peer", common.RoleAdminUser)
	root := createCustomerContractAdminListUser(t, db, "role-root", common.RoleRootUser)
	createCustomerContractAdminListContract(t, db, customer.Id, "Customer Contract", true, 1, customer.Id, 100)
	createCustomerContractAdminListContract(t, db, peer.Id, "Peer Contract", true, 1, peer.Id, 100)
	createCustomerContractAdminListContract(t, db, root.Id, "Root Contract", true, 1, root.Id, 100)

	adminItems, adminTotal, _, err := GetCustomerContractAdminList(CustomerContractAdminListFilter{
		AdminRole: common.RoleAdminUser, Limit: 20,
	})
	require.NoError(t, err)
	assert.EqualValues(t, 1, adminTotal)
	require.Len(t, adminItems, 1)
	assert.Equal(t, "Customer Contract", adminItems[0].ContractName)

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

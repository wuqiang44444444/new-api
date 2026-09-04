package model

import (
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// withLegacyCustomerContractDB creates the contract table via AutoMigrate and
// then drops the (user_id, public_model) unique index — matching the legacy
// deployments whose startup AutoMigrate fails on duplicate rows.
func withLegacyCustomerContractDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}, &CustomerModelContract{}))
	require.True(t, db.Migrator().HasIndex(&CustomerModelContract{}, "idx_customer_model_contract_user_model"))
	require.NoError(t, db.Migrator().DropIndex(&CustomerModelContract{}, "idx_customer_model_contract_user_model"))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })
	return db
}

func TestCustomerModelContractUniquenessKeepsLatestRowAndUnblocksIndex(t *testing.T) {
	db := withLegacyCustomerContractDB(t)

	seed := func(userId int, publicModel string, routeGroup string, ratioUnits int64, updatedAt int64) {
		require.NoError(t, db.Create(&CustomerModelContract{
			UserId: userId, PublicModel: publicModel, RouteGroup: routeGroup,
			RatioUnits: ratioUnits, CreatedAt: updatedAt, UpdatedAt: updatedAt,
		}).Error)
	}
	// Historical duplicate rows for the same (user, model): the latest
	// updated_at must survive the dedup.
	seed(7, "seedance-pro", "vip", 100, 1000)
	seed(7, "seedance-pro", "default", 300, 2000)
	seed(7, "seedance-pro", "vip", 150, 1500)
	// Unrelated rows must survive untouched.
	seed(8, "seedance-pro", "vip", 200, 3000)

	require.NoError(t, migrateCustomerModelContractUniqueness(db))
	require.NoError(t, migrateCustomerModelContractUniqueness(db)) // idempotent

	var rows []CustomerModelContract
	require.NoError(t, db.Where("user_id = ?", 7).Order("public_model").Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "default", rows[0].RouteGroup)
	assert.Equal(t, int64(300), rows[0].RatioUnits)

	var total int64
	require.NoError(t, db.Model(&CustomerModelContract{}).Where("user_id = ?", 8).Count(&total).Error)
	assert.EqualValues(t, 1, total)

	var marker Option
	require.NoError(t, db.Where(&Option{Key: customerModelContractUniquenessKey}).First(&marker).Error)
	assert.Equal(t, "done", marker.Value)

	// AutoMigrate can now materialize the unique index without a constraint
	// failure — the exact startup path the dedup unblocks.
	require.NoError(t, db.AutoMigrate(&CustomerModelContract{}))
	require.True(t, db.Migrator().HasIndex(&CustomerModelContract{}, "idx_customer_model_contract_user_model"))
}

func TestCustomerModelContractUniquenessNoopWithoutTable(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	// Fresh database: the contract table does not exist yet; the migration
	// must not fail (AutoMigrate creates the table with the unique index).
	require.NoError(t, migrateCustomerModelContractUniqueness(db))
}

package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func seedLegacyModelDiscount(t *testing.T, db *gorm.DB, periodStart int64, channelId int, providerModel string, billingMode string, discount string, copiedFrom int64, reason string, operatorId int) ProviderBillingDiscount {
	t.Helper()
	row := ProviderBillingDiscount{
		PeriodStart: periodStart, ChannelId: channelId, ProviderModel: providerModel, BillingMode: billingMode,
		Discount: decimal.RequireFromString(discount), CopiedFromPeriod: copiedFrom, Version: 1,
		Reason: reason, CreatedBy: operatorId, UpdatedBy: operatorId,
	}
	require.NoError(t, db.Create(&row).Error)
	require.NoError(t, createProviderBillingAudit(db, "discount", providerBillingEntityKey(periodStart, channelId, providerModel, billingMode), "create", nil, &row, reason, operatorId))
	// The model-level record alone cannot prove the historical channel scope.
	// Seed the persisted usage that this fixture's discount actually covered.
	other, err := common.Marshal(map[string]interface{}{"upstream_model_name": providerModel, "statement_snapshot": map[string]string{"billing_mode": billingMode}})
	require.NoError(t, err)
	require.NoError(t, LOG_DB.Create(&Log{ChannelId: channelId, ModelName: providerModel, Type: LogTypeConsume, CreatedAt: periodStart + 1, Other: string(other)}).Error)
	return row
}

func correctLegacyModelDiscount(t *testing.T, db *gorm.DB, row ProviderBillingDiscount, newDiscount string, reason string, operatorId int) {
	t.Helper()
	before := row
	row.Discount = decimal.RequireFromString(newDiscount)
	row.Reason = reason
	row.Version++
	row.UpdatedBy = operatorId
	require.NoError(t, db.Model(&ProviderBillingDiscount{}).Where("id = ?", row.Id).Updates(map[string]interface{}{"discount": row.Discount, "reason": row.Reason, "version": row.Version}).Error)
	require.NoError(t, createProviderBillingAudit(db, "discount", providerBillingEntityKey(row.PeriodStart, row.ChannelId, row.ProviderModel, row.BillingMode), "update", &before, &row, reason, operatorId))
}

func channelDiscountPeriod(month time.Month, year int) int64 {
	location := time.FixedZone("Asia/Shanghai", 8*60*60)
	return time.Date(year, month, 1, 0, 0, 0, 0, location).Unix()
}

func requireSingleChannelDiscount(t *testing.T, db *gorm.DB, periodStart int64, channelId int) ProviderChannelBillingDiscount {
	t.Helper()
	var rows []ProviderChannelBillingDiscount
	require.NoError(t, db.Where("period_start = ? AND channel_id = ?", periodStart, channelId).Find(&rows).Error)
	require.Len(t, rows, 1)
	return rows[0]
}

func requireNoChannelDiscount(t *testing.T, db *gorm.DB, periodStart int64, channelId int) {
	t.Helper()
	var rows []ProviderChannelBillingDiscount
	require.NoError(t, db.Where("period_start = ? AND channel_id = ?", periodStart, channelId).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.NotEmpty(t, rows[0].PendingReason)
	assert.Zero(t, rows[0].Version)
}

// Multi-month manual chains converge; October inherits from the migrated
// September through the regular runtime initialization.
func TestMigrateProviderChannelDiscountsConvergesManualInheritanceChain(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	july, august, september, october := channelDiscountPeriod(time.July, 2026), channelDiscountPeriod(time.August, 2026), channelDiscountPeriod(time.September, 2026), channelDiscountPeriod(time.October, 2026)
	require.NoError(t, db.Create(&Channel{Id: 61, Name: "chain"}).Error)
	julyRow := seedLegacyModelDiscount(t, db, july, 61, "model-a", BillingReconciliationModeToken, "0.85", 0, "supplier contract", 9)
	seedLegacyModelDiscount(t, db, august, 61, "model-a", BillingReconciliationModeToken, "0.85", july, "automatic copy from previous billing period", 10)
	seedLegacyModelDiscount(t, db, september, 61, "model-a", BillingReconciliationModeToken, "0.85", august, "automatic copy from previous billing period", 10)

	require.NoError(t, migrateProviderModelDiscountsToChannel())
	for _, period := range []int64{july, august, september} {
		record := requireSingleChannelDiscount(t, db, period, 61)
		assert.True(t, record.Discount.Equal(decimal.RequireFromString("0.85")))
	}
	outcomes, err := InitializeProviderChannelBillingDiscounts(october, []int{61}, 11)
	require.NoError(t, err)
	assert.Equal(t, "created", outcomes[0].Outcome)
	assert.True(t, requireSingleChannelDiscount(t, db, october, 61).Discount.Equal(decimal.RequireFromString("0.85")))
	_ = julyRow

	// Re-running the migration never duplicates or overwrites.
	require.NoError(t, migrateProviderModelDiscountsToChannel())
	requireSingleChannelDiscount(t, db, july, 61)
}

// A later correction of the source month keeps the already-made copy valid
// with the value it actually copied.
func TestMigrateProviderChannelDiscountsKeepsHistoricalCopyAfterSourceCorrection(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	july, august := channelDiscountPeriod(time.July, 2026), channelDiscountPeriod(time.August, 2026)
	require.NoError(t, db.Create(&Channel{Id: 62, Name: "corrected"}).Error)
	julyRow := seedLegacyModelDiscount(t, db, july, 62, "model-a", BillingReconciliationModeToken, "0.85", 0, "supplier contract", 9)
	seedLegacyModelDiscount(t, db, august, 62, "model-a", BillingReconciliationModeToken, "0.85", july, "automatic copy from previous billing period", 10)
	correctLegacyModelDiscount(t, db, julyRow, "0.7", "late July correction", 9)

	require.NoError(t, migrateProviderModelDiscountsToChannel())
	julyRecord := requireSingleChannelDiscount(t, db, july, 62)
	augustRecord := requireSingleChannelDiscount(t, db, august, 62)
	assert.True(t, julyRecord.Discount.Equal(decimal.RequireFromString("0.7")))
	assert.True(t, augustRecord.Discount.Equal(decimal.RequireFromString("0.85")))
}

// A system default 1 never masquerades as a confirmed no-discount, while a
// manually confirmed 1 keeps its inheritance chain valid.
func TestMigrateProviderChannelDiscountsPreservesDefaultAndManualNoDiscount(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	july, august := channelDiscountPeriod(time.July, 2026), channelDiscountPeriod(time.August, 2026)
	require.NoError(t, db.Create(&Channel{Id: 63, Name: "default-only"}).Error)
	require.NoError(t, db.Create(&Channel{Id: 64, Name: "manual-one"}).Error)
	seedLegacyModelDiscount(t, db, july, 63, "model-a", BillingReconciliationModeToken, "1", 0, "automatic monthly default", 9)
	seedLegacyModelDiscount(t, db, august, 63, "model-a", BillingReconciliationModeToken, "1", july, "automatic copy from previous billing period", 10)
	seedLegacyModelDiscount(t, db, july, 64, "model-a", BillingReconciliationModeToken, "1", 0, "confirmed no discount", 9)
	seedLegacyModelDiscount(t, db, august, 64, "model-a", BillingReconciliationModeToken, "1", july, "automatic copy from previous billing period", 10)

	require.NoError(t, migrateProviderModelDiscountsToChannel())
	assert.True(t, requireSingleChannelDiscount(t, db, july, 63).Discount.Equal(decimal.NewFromInt(1)))
	assert.True(t, requireSingleChannelDiscount(t, db, august, 63).Discount.Equal(decimal.NewFromInt(1)))
	assert.True(t, requireSingleChannelDiscount(t, db, july, 64).Discount.Equal(decimal.NewFromInt(1)))
	assert.True(t, requireSingleChannelDiscount(t, db, august, 64).Discount.Equal(decimal.NewFromInt(1)))

	// The default coefficient is also a valid inheritance source.
	september := channelDiscountPeriod(time.September, 2026)
	outcomes, err := InitializeProviderChannelBillingDiscounts(september, []int{63}, 9)
	require.NoError(t, err)
	assert.Equal(t, "created", outcomes[0].Outcome)
}

// Conflicting effective coefficients, and mixtures of confirmed values with
// unconfirmed defaults or unknown evidence, stay pending without picking or
// averaging values.
func TestMigrateProviderChannelDiscountsLeavesConflictsPending(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	july, august := channelDiscountPeriod(time.July, 2026), channelDiscountPeriod(time.August, 2026)
	require.NoError(t, db.Create(&Channel{Id: 65, Name: "conflict"}).Error)
	require.NoError(t, db.Create(&Channel{Id: 66, Name: "mixed"}).Error)
	// Same month, two models with different manual coefficients.
	seedLegacyModelDiscount(t, db, july, 65, "model-a", BillingReconciliationModeToken, "0.8", 0, "manual a", 9)
	seedLegacyModelDiscount(t, db, july, 65, "model-b", BillingReconciliationModeToken, "0.9", 0, "manual b", 9)
	// One confirmed value plus one system default cannot prove channel scope.
	seedLegacyModelDiscount(t, db, july, 66, "model-a", BillingReconciliationModeToken, "0.8", 0, "manual a", 9)
	seedLegacyModelDiscount(t, db, july, 66, "model-b", BillingReconciliationModeToken, "1", 0, "automatic monthly default", 9)
	// An August record pointing at a month without any ancestor evidence stays
	// unknown instead of trusting the copied value.
	june := channelDiscountPeriod(time.June, 2026)
	seedLegacyModelDiscount(t, db, august, 65, "model-a", BillingReconciliationModeToken, "0.8", june, "automatic copy from previous billing period", 10)

	require.NoError(t, migrateProviderModelDiscountsToChannel())
	requireNoChannelDiscount(t, db, july, 65)
	requireNoChannelDiscount(t, db, july, 66)
	requireNoChannelDiscount(t, db, august, 65)
}

// A manual update of an inherited record re-confirms its value.
func TestMigrateProviderChannelDiscountsTreatsManualUpdateAsConfirmation(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	july := channelDiscountPeriod(time.July, 2026)
	require.NoError(t, db.Create(&Channel{Id: 67, Name: "updated"}).Error)
	row := seedLegacyModelDiscount(t, db, july, 67, "model-a", BillingReconciliationModeToken, "1", 0, "automatic monthly default", 9)
	correctLegacyModelDiscount(t, db, row, "0.75", "renegotiated", 9)

	require.NoError(t, migrateProviderModelDiscountsToChannel())
	record := requireSingleChannelDiscount(t, db, july, 67)
	assert.True(t, record.Discount.Equal(decimal.RequireFromString("0.75")))
}

func TestMigratePendingChannelDiscountCannotBeReplacedByInitialization(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	july, august := channelDiscountPeriod(time.July, 2026), channelDiscountPeriod(time.August, 2026)
	require.NoError(t, db.Create(&Channel{Id: 95, Name: "conflicting"}).Error)
	seedLegacyModelDiscount(t, db, july, 95, "a", BillingReconciliationModeToken, "0.8", 0, "confirmed", 7)
	seedLegacyModelDiscount(t, db, august, 95, "a", BillingReconciliationModeToken, "0.7", 0, "confirmed", 7)
	seedLegacyModelDiscount(t, db, august, 95, "b", BillingReconciliationModeToken, "0.6", 0, "confirmed", 7)
	require.NoError(t, migrateProviderModelDiscountsToChannel())
	for range 2 {
		outcomes, err := InitializeProviderChannelBillingDiscounts(august, []int{95}, 7)
		require.NoError(t, err)
		assert.Equal(t, "exists", outcomes[0].Outcome)
		values, err := GetProviderChannelBillingDiscounts(august, []int{95})
		require.NoError(t, err)
		assert.Empty(t, values)
		require.NoError(t, migrateProviderModelDiscountsToChannel())
	}
	row := ProviderChannelBillingDiscount{PeriodStart: august, ChannelId: 95, Discount: decimal.RequireFromString("0.65"), Reason: "manual conflict resolution"}
	require.NoError(t, SaveProviderChannelBillingDiscount(&row, 0, 7))
	assert.EqualValues(t, 1, row.Version)
	values, err := GetProviderChannelBillingDiscounts(august, []int{95})
	require.NoError(t, err)
	require.Contains(t, values, 95)
	assert.True(t, values[95].Discount.Equal(row.Discount))
	assert.Empty(t, values[95].PendingReason)
}

func TestMigrateChannelDiscountRejectsCurrentValueWithoutMatchingAudit(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	period := channelDiscountPeriod(time.July, 2026)
	row := seedLegacyModelDiscount(t, db, period, 99, "model", BillingReconciliationModeToken, "0.8", 0, "confirmed", 7)
	require.NoError(t, db.Model(&row).Update("discount", decimal.RequireFromString("0.6")).Error)
	require.NoError(t, migrateProviderModelDiscountsToChannel())
	requireNoChannelDiscount(t, db, period, 99)
}

func TestMigrateChannelDiscountRequiresCompleteRecordedModelScope(t *testing.T) {
	for _, tc := range []struct {
		name           string
		other          string
		removeEvidence bool
	}{
		{name: "another model without a discount", other: `{"upstream_model_name":"model-b","statement_snapshot":{"billing_mode":"token"}}`},
		{name: "another billing mode without a discount", other: `{"upstream_model_name":"model-a","statement_snapshot":{"billing_mode":"per_call"}}`},
		{name: "unknown upstream identity", other: `{"is_model_mapped":true,"statement_snapshot":{"billing_mode":"token"}}`},
		{name: "no retained usage evidence", removeEvidence: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupBillingReconciliationTestDB(t)
			period := channelDiscountPeriod(time.July, 2026)
			require.NoError(t, db.Create(&Channel{Id: 100, Name: "partially configured"}).Error)
			seedLegacyModelDiscount(t, db, period, 100, "model-a", BillingReconciliationModeToken, "0.8", 0, "confirmed", 7)
			if tc.removeEvidence {
				require.NoError(t, LOG_DB.Where("channel_id = ?", 100).Delete(&Log{}).Error)
			} else {
				require.NoError(t, LOG_DB.Create(&Log{ChannelId: 100, ModelName: "customer-model", Type: LogTypeConsume, CreatedAt: period + 2, Other: tc.other}).Error)
			}
			require.NoError(t, migrateProviderModelDiscountsToChannel())
			requireNoChannelDiscount(t, db, period, 100)
			outcomes, err := InitializeProviderChannelBillingDiscounts(period, []int{100}, 7)
			require.NoError(t, err)
			assert.Equal(t, "exists", outcomes[0].Outcome)
			requireNoChannelDiscount(t, db, period, 100)
		})
	}
}

func TestMigrateChannelDiscountUsesSeparateLogDatabase(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	logDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := logDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, logDB.AutoMigrate(&Log{}))
	LOG_DB = logDB
	period := channelDiscountPeriod(time.July, 2026)
	seedLegacyModelDiscount(t, db, period, 101, "model-a", BillingReconciliationModeToken, "0.8", 0, "confirmed", 7)
	seedLegacyModelDiscount(t, db, period, 102, "model-a", BillingReconciliationModeToken, "0.8", 0, "confirmed", 7)
	require.NoError(t, logDB.Create(&Log{ChannelId: 102, ModelName: "model-b", Type: LogTypeConsume, CreatedAt: period + 2, Other: `{"upstream_model_name":"model-b","statement_snapshot":{"billing_mode":"token"}}`}).Error)
	require.NoError(t, migrateProviderModelDiscountsToChannel())
	assert.True(t, requireSingleChannelDiscount(t, db, period, 101).Discount.Equal(decimal.RequireFromString("0.8")))
	requireNoChannelDiscount(t, db, period, 102)
}

func TestMigrateChannelDiscountReadFailureDoesNotWriteConfiguration(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	period := channelDiscountPeriod(time.July, 2026)
	seedLegacyModelDiscount(t, db, period, 103, "model-a", BillingReconciliationModeToken, "0.8", 0, "confirmed", 7)
	require.NoError(t, LOG_DB.Migrator().DropTable(&Log{}))
	require.Error(t, migrateProviderModelDiscountsToChannel())
	var count int64
	require.NoError(t, db.Model(&ProviderChannelBillingDiscount{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, db.Model(&ProviderBillingAudit{}).Where("entity_type = ?", providerChannelDiscountEntity).Count(&count).Error)
	assert.Zero(t, count)
}

func TestProviderChannelDiscountMigrationWaitsForLogInitialization(t *testing.T) {
	db := setupBillingReconciliationTestDB(t)
	period := channelDiscountPeriod(time.July, 2026)
	seedLegacyModelDiscount(t, db, period, 104, "model-a", BillingReconciliationModeToken, "0.8", 0, "confirmed", 7)
	LOG_DB = nil
	require.NoError(t, migrateBillingReconciliationDB(), "schema migration must not read an uninitialized log DB")
	LOG_DB = db
	previousMaster := common.IsMasterNode
	t.Cleanup(func() { common.IsMasterNode = previousMaster })
	common.IsMasterNode = false
	require.NoError(t, InitProviderChannelBillingDiscounts())
	var count int64
	require.NoError(t, db.Model(&ProviderChannelBillingDiscount{}).Count(&count).Error)
	assert.Zero(t, count, "replicas do not migrate configuration")
	common.IsMasterNode = true
	require.NoError(t, InitProviderChannelBillingDiscounts())
	assert.True(t, requireSingleChannelDiscount(t, db, period, 104).Discount.Equal(decimal.RequireFromString("0.8")))
}

package main

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupRepairMaintenance(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.BillingStatementMaintenance{}, &model.BillingStatementAudit{}, &model.Option{}))
	require.NoError(t, db.Create(&model.BillingStatementMaintenance{ID: 1, Enabled: true, Generation: 2, Reason: "verified fixture"}).Error)
	require.NoError(t, db.Create(&model.Option{Key: model.BillingStatementVersionEnabledKey, Value: "false"}).Error)
}

func TestRepairRequiresCurrentMaintenanceBeforeAnyLogWrites(t *testing.T) {
	for _, scenario := range []string{"not maintaining", "wrong generation", "confirmation enabled", "missing control row"} {
		t.Run(scenario, func(t *testing.T) {
			db, s := setupTaskLogRepair(t)
			var before []model.Log
			require.NoError(t, db.Order("id").Find(&before).Error)
			switch scenario {
			case "not maintaining":
				require.NoError(t, db.Model(&model.BillingStatementMaintenance{}).Where("id = 1").Update("enabled", false).Error)
			case "wrong generation":
				s.MaintenanceGeneration--
			case "confirmation enabled":
				require.NoError(t, db.Model(&model.Option{}).Where("key = ?", model.BillingStatementVersionEnabledKey).Update("value", "true").Error)
			case "missing control row":
				require.NoError(t, db.Where("id = 1").Delete(&model.BillingStatementMaintenance{}).Error)
			}
			_, err := repair(db, s, false)
			require.NoError(t, err, "preview remains read-only and does not need maintenance")
			_, err = repair(db, s, true)
			require.Error(t, err)
			var after []model.Log
			require.NoError(t, db.Order("id").Find(&after).Error)
			assert.Equal(t, before, after)
		})
	}
}

func TestRepairMaintenanceFailureRemainsFencedAndEndRequiresEvidence(t *testing.T) {
	db, s := setupTaskLogRepair(t)
	oldDB, oldLog := model.DB, model.LOG_DB
	oldMainType, oldLogType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() { model.DB, model.LOG_DB = oldDB, oldLog; common.SetDatabaseTypes(oldMainType, oldLogType) })
	require.NoError(t, db.Create(&model.User{Id: 99, Username: "operator", AffCode: "operator", Role: common.RoleRootUser, Status: common.UserStatusEnabled}).Error)
	require.NoError(t, db.Model(&model.BillingStatementMaintenance{}).Where("id = 1").Update("enabled", false).Error)
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", model.BillingStatementVersionEnabledKey).Update("value", "true").Error)
	state, err := repairMaintenance(db, "begin", "approved repair plan", 0, 99)
	require.NoError(t, err)
	s.MaintenanceGeneration = state.Generation
	require.NoError(t, db.Model(&model.TaskCreateAttempt{}).Where("attempt_id = ?", "attempt").Update("billing_hold_state", model.TaskCreateAttemptBillingReleased).Error)
	_, err = repair(db, s, true)
	require.Error(t, err)
	require.NoError(t, db.First(&state, 1).Error)
	assert.True(t, state.Enabled)
	_, err = repairMaintenance(db, "end", "", state.Generation, 99)
	require.Error(t, err)
	_, err = repairMaintenance(db, "end", "verified no source writes; failed repair rolled back", state.Generation-1, 99)
	require.Error(t, err)
	ended, err := repairMaintenance(db, "end", "verified no source writes; failed repair rolled back", state.Generation, 99)
	require.NoError(t, err)
	assert.False(t, ended.Enabled)
	var option model.Option
	require.NoError(t, db.Where("key = ?", model.BillingStatementVersionEnabledKey).First(&option).Error)
	assert.Equal(t, "false", option.Value)
	var audits []model.BillingStatementAudit
	require.NoError(t, db.Order("id").Find(&audits).Error)
	require.Len(t, audits, 2)
	assert.Equal(t, "maintenance_begin", audits[0].Action)
	assert.Equal(t, "maintenance_end", audits[1].Action)
}

func TestMaintenanceDiagnosticsDoNotExposeDriverDetails(t *testing.T) {
	assert.Contains(t, maintenanceFailureMessage(fmt.Errorf("billing statement maintenance not in progress")), "generation")
	assert.Contains(t, maintenanceFailureMessage(fmt.Errorf("no such table: secret_query_argument")), "migrated tables")
	assert.NotContains(t, maintenanceFailureMessage(fmt.Errorf("database failure with private credentials")), "private credentials")
}

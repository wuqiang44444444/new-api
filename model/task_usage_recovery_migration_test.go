package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskUsageRecoveryUpgradePreservesExistingHold(t *testing.T) {
	// Existing rows have no recovery columns. Test the additive migration and
	// discovery of a pre-upgrade awaiting row, not a fresh-table-only fixture.
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	type previousTask struct {
		ID           int64 `gorm:"primaryKey"`
		TaskID       string
		Status       TaskStatus
		Quota        int
		BillingState TaskBillingState
		FinishTime   int64
		PrivateData  TaskPrivateData `gorm:"type:json"`
	}
	require.NoError(t, db.Table("tasks").AutoMigrate(&previousTask{}))
	now := common.GetTimestamp()
	old := previousTask{ID: 1, TaskID: "upgrade-awaiting", Status: TaskStatusSuccess, Quota: 700, BillingState: TaskBillingStateAwaitingUsage, FinishTime: now - 60,
		PrivateData: TaskPrivateData{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3, VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3,
			AsyncBilling: &TaskAsyncBillingContext{State: TaskBillingStateAwaitingUsage}}}
	require.NoError(t, db.Table("tasks").Create(&old).Error)
	require.NoError(t, db.AutoMigrate(&Task{}))
	previous := DB
	DB = db
	t.Cleanup(func() { DB = previous })
	require.True(t, HasDueTaskUsageChecks(now))
	task, review, err := ClaimTaskUsageCheck(1, now)
	require.NoError(t, err)
	require.NotNil(t, task)
	assert.False(t, review)
	assert.Equal(t, 700, task.Quota)
	assert.Equal(t, now-60, task.UsageCheckStartedAt)
	assert.Equal(t, 1, task.UsageCheckAttempts)
	require.NoError(t, db.AutoMigrate(&Task{}))
	var saved Task
	require.NoError(t, db.First(&saved, 1).Error)
	assert.Equal(t, task.UsageCheckNextAt, saved.UsageCheckNextAt)
	assert.Equal(t, 700, saved.Quota)
}

package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestImportLegacyEvidenceRetentionPreservesExistingWindow(t *testing.T) {
	if dialect := os.Getenv("TEST_BILLING_STATEMENT_DIALECT"); dialect != "" {
		verifyLegacyEvidenceRetentionUpgrade(t, setupBillingStatementServerDB(t, dialect))
		return
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "upgrade.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	verifyLegacyEvidenceRetentionUpgrade(t, db)
}

func verifyLegacyEvidenceRetentionUpgrade(t *testing.T, db *gorm.DB) {
	t.Helper()
	previousDB := DB
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
	})
	require.NoError(t, db.AutoMigrate(&Option{}, &TaskRequestEvidence{}))
	const now int64 = 1794600000
	const day int64 = 86400
	evidence := TaskRequestEvidence{CreatedAt: now - 3*day, RetainUntil: now + 4*day}
	require.NoError(t, CreateTaskRequestEvidence(&evidence))
	key := system_setting.TaskRequestEvidenceRetentionOptionKey
	t.Setenv("TASK_REQUEST_EVIDENCE_RETENTION_DAYS", " 7 ")
	require.NoError(t, ImportLegacyEvidenceRetention())
	policy := system_setting.GetTaskRequestEvidenceRetentionPolicy()
	require.Equal(t, system_setting.TaskRequestEvidenceRetentionPolicy{Days: 7, Source: "database"}, policy)
	retention := int64(policy.Days) * day
	closed, err := TaskRequestEvidenceBodyClosedForAppend(evidence.Id, now, retention)
	require.NoError(t, err)
	assert.False(t, closed)
	expired, err := TaskRequestEvidenceExpiredBodies(now, retention, 0, 0, 10)
	require.NoError(t, err)
	assert.Empty(t, expired)

	require.NoError(t, db.Model(&Option{}).Where(&Option{Key: key}).Update("value", "invalid").Error)
	require.NoError(t, ImportLegacyEvidenceRetention())
	assert.Equal(t, policy, system_setting.GetTaskRequestEvidenceRetentionPolicy())
	require.NoError(t, UpdateOption(key, "14"))
	require.NoError(t, ImportLegacyEvidenceRetention())
	t.Setenv("TASK_REQUEST_EVIDENCE_RETENTION_DAYS", "")
	require.NoError(t, ImportLegacyEvidenceRetention())
	assert.Equal(t, system_setting.TaskRequestEvidenceRetentionPolicy{Days: 14, Source: "database"}, system_setting.GetTaskRequestEvidenceRetentionPolicy())
	expired, err = TaskRequestEvidenceExpiredBodies(now+4*day+1, 14*day, 0, 0, 10)
	require.NoError(t, err)
	require.Len(t, expired, 1)
	assert.Equal(t, evidence.Id, expired[0].Id)
}

func TestRequestEvidenceRetentionOptionPersistenceAndRefresh(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "retention.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB := DB
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		assert.NoError(t, sqlDB.Close())
	})

	key := system_setting.TaskRequestEvidenceRetentionOptionKey
	getPolicy := system_setting.GetTaskRequestEvidenceRetentionPolicy
	defaultPolicy := system_setting.TaskRequestEvidenceRetentionPolicy{Days: 2, Source: "default"}
	assert.Equal(t, defaultPolicy, getPolicy())
	require.Error(t, UpdateOption(key, "30"), "missing table must report failure")
	assert.Equal(t, defaultPolicy, getPolicy())

	require.NoError(t, db.AutoMigrate(&Option{}))
	for _, invalid := range []string{"0", "31", "1.5", "invalid"} {
		require.Error(t, UpdateOption(key, invalid))
	}
	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, UpdateOption(key, "30"))
	assert.Equal(t, system_setting.TaskRequestEvidenceRetentionPolicy{Days: 30, Source: "database"}, getPolicy())
	var persisted Option
	require.NoError(t, db.First(&persisted, "key = ?", key).Error)
	assert.Equal(t, "30", persisted.Value)

	persisted.Value = "1"
	require.NoError(t, db.Save(&persisted).Error)
	loadOptionsFromDatabase()
	assert.Equal(t, system_setting.TaskRequestEvidenceRetentionPolicy{Days: 1, Source: "database"}, getPolicy())
	persisted.Value = "invalid"
	require.NoError(t, db.Save(&persisted).Error)
	loadOptionsFromDatabase()
	assert.Equal(t, defaultPolicy, getPolicy())
	require.NoError(t, UpdateOption(key, "14"))
	require.NoError(t, db.Delete(&persisted).Error)
	loadOptionsFromDatabase()
	assert.Equal(t, defaultPolicy, getPolicy())
}

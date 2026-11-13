package model

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRequestEvidenceRetentionOptionPersistenceAndRefresh(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "retention.db")), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	previousDB := DB
	previousConfig := system_setting.GetTaskRequestEvidenceConfig()
	common.OptionMapRWMutex.Lock()
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	DB = db
	t.Cleanup(func() {
		DB = previousDB
		system_setting.SetTaskRequestEvidenceConfig(previousConfig)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptions
		common.OptionMapRWMutex.Unlock()
		assert.NoError(t, sqlDB.Close())
	})

	key := system_setting.TaskRequestEvidenceRetentionOptionKey
	getPolicy := system_setting.GetTaskRequestEvidenceRetentionPolicy
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{})
	assert.Equal(t, system_setting.TaskRequestEvidenceRetentionPolicy{Days: 2, Source: "default"}, getPolicy())
	t.Setenv(system_setting.TaskRequestEvidenceRetentionDaysEnv, "3")
	system_setting.SetTaskRequestEvidenceConfig(system_setting.LoadTaskRequestEvidenceConfig())
	envPolicy := system_setting.TaskRequestEvidenceRetentionPolicy{Days: 3, Source: "environment"}
	assert.Equal(t, envPolicy, getPolicy())
	require.Error(t, UpdateOption(key, "7"), "missing table must report failure")
	assert.Equal(t, envPolicy, getPolicy())

	require.NoError(t, db.AutoMigrate(&Option{}))
	for _, invalid := range []string{"0", "8", "1.5", "invalid"} {
		require.Error(t, UpdateOption(key, invalid))
	}
	var count int64
	require.NoError(t, db.Model(&Option{}).Count(&count).Error)
	assert.Zero(t, count)
	require.NoError(t, UpdateOption(key, "7"))
	assert.Equal(t, system_setting.TaskRequestEvidenceRetentionPolicy{Days: 7, Source: "database"}, getPolicy())
	var persisted Option
	require.NoError(t, db.First(&persisted, "key = ?", key).Error)
	assert.Equal(t, "7", persisted.Value)

	persisted.Value = "1"
	require.NoError(t, db.Save(&persisted).Error)
	loadOptionsFromDatabase()
	assert.Equal(t, system_setting.TaskRequestEvidenceRetentionPolicy{Days: 1, Source: "database"}, getPolicy())
	persisted.Value = "invalid"
	require.NoError(t, db.Save(&persisted).Error)
	loadOptionsFromDatabase()
	assert.Equal(t, envPolicy, getPolicy())
}

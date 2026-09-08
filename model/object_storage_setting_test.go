package model

import (
	"bytes"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestObjectStorageCredentialWritesStayPrivateWithDebugLogging(t *testing.T) {
	previousDB, previousDebug, previousObserver := DB, common.DebugEnabled, objectStorageSettingObserver
	t.Cleanup(func() {
		DB, common.DebugEnabled, objectStorageSettingObserver = previousDB, previousDebug, previousObserver
	})
	common.DebugEnabled = true
	objectStorageSettingObserver = nil
	var logs bytes.Buffer
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: newGormLogger(&logs)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	DB = db.Debug() // Match InitDB: DEBUG changes Warn to Info.
	require.NoError(t, DB.AutoMigrate(&Option{}))
	logs.Reset()

	initial := `{"backend":"s3","bucket":"artifacts","credential":"fixture-initial-secret"}`
	rotated := `{"backend":"s3","bucket":"artifacts","credential":"fixture-rotated-secret"}`
	require.NoError(t, SaveObjectStorageSetting(initial))
	require.NoError(t, SaveObjectStorageSetting(rotated))
	stored, err := GetObjectStorageSetting()
	require.NoError(t, err)
	assert.Equal(t, rotated, stored)
	assert.NotContains(t, logs.String(), "fixture-initial-secret")
	assert.NotContains(t, logs.String(), "fixture-rotated-secret")

	// A deterministic database failure whose driver message also contains a secret.
	require.NoError(t, DB.Exec(`CREATE TRIGGER reject_storage_update BEFORE UPDATE ON options BEGIN SELECT RAISE(ABORT, 'fixture-driver-secret'); END`).Error)
	logs.Reset()
	require.Error(t, SaveObjectStorageSetting(initial))
	assert.NotContains(t, logs.String(), "fixture-initial-secret")
	assert.NotContains(t, logs.String(), "fixture-driver-secret")
	stored, err = GetObjectStorageSetting()
	require.NoError(t, err)
	assert.Equal(t, rotated, stored, "failed rotation must preserve the saved credential")

	require.NoError(t, DB.Exec("SELECT ?", "ordinary-query-marker").Error)
	assert.Contains(t, logs.String(), "ordinary-query-marker", "unrelated DEBUG queries must still be logged")
}

func TestObjectStorageSettingNamespaceIsolation(t *testing.T) {
	require.NoError(t, DB.AutoMigrate(&Option{}))

	previousObserver := objectStorageSettingObserver
	t.Cleanup(func() { objectStorageSettingObserver = previousObserver })

	var received []string
	SetObjectStorageSettingObserver(func(value string) {
		received = append(received, value)
	})

	settingJSON := `{"backend":"azure_blob","bucket":"task-artifacts","revision":"rev-1"}`
	require.NoError(t, SaveObjectStorageSetting(settingJSON))

	// 配置持久化到数据库。
	stored, err := GetObjectStorageSetting()
	require.NoError(t, err)
	assert.Equal(t, settingJSON, stored)

	// 观察者收到更新（装载即初始化与多节点刷新的唯一通路）。
	require.Len(t, received, 1)
	assert.Equal(t, settingJSON, received[0])

	// 该命名空间不进入通用 OptionMap，普通选项接口因此读不到。
	common.OptionMapRWMutex.RLock()
	_, exists := common.OptionMap[system_setting.ObjectStorageSettingOptionKey]
	common.OptionMapRWMutex.RUnlock()
	assert.False(t, exists)
}

func TestObjectStorageLocationRequiresOfflineMaintenance(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	DB = db
	t.Cleanup(func() { DB = previousDB })
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&Option{}))
	original := system_setting.ObjectStorageConfig{Backend: "azure_blob", Endpoint: "https://account.blob.core.windows.net", Bucket: "artifacts", AccountName: "account", Prefix: "prod", Credential: "old"}
	encoded, err := common.Marshal(original)
	require.NoError(t, err)
	require.NoError(t, SaveObjectStorageSetting(string(encoded)))
	for _, field := range []string{"backend", "endpoint", "bucket", "prefix", "region", "account", "disable"} {
		t.Run(field, func(t *testing.T) {
			changed := original
			switch field {
			case "backend":
				changed.Backend = "s3"
			case "endpoint":
				changed.Endpoint = "https://another.example.com"
			case "bucket":
				changed.Bucket = "another"
			case "prefix":
				changed.Prefix = "another"
			case "region":
				changed.Region = "another"
			case "account":
				changed.AccountName = "another"
			case "disable":
				changed.Backend = "upstream"
			}
			data, err := common.Marshal(changed)
			require.NoError(t, err)
			require.ErrorContains(t, SaveObjectStorageSetting(string(data)), "offline maintenance")
			raw, err := GetObjectStorageSetting()
			require.NoError(t, err)
			assert.Equal(t, string(encoded), raw)
		})
	}
	original.Credential = "rotated"
	encoded, err = common.Marshal(original)
	require.NoError(t, err)
	require.NoError(t, SaveObjectStorageSetting(string(encoded)))
	raw, err := GetObjectStorageSetting()
	require.NoError(t, err)
	assert.Equal(t, string(encoded), raw)
}

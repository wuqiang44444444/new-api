package controller

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestObjectStorageEditRetainsCredential(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Setenv("CRYPTO_SECRET", "")
	t.Setenv("SESSION_SECRET", "")
	t.Cleanup(func() { model.DB = previousDB })
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	for _, backend := range []string{"azure_blob", "s3"} {
		t.Run(backend, func(t *testing.T) {
			stored := system_setting.ObjectStorageConfig{Backend: backend, AccountName: "account", Endpoint: "https://storage.example.com", Bucket: "artifacts", Region: "us-east-1", Credential: "test-storage-secret"}
			encoded, err := common.Marshal(stored)
			require.NoError(t, err)
			require.NoError(t, db.Save(&model.Option{Key: system_setting.ObjectStorageSettingOptionKey, Value: string(encoded)}).Error)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			GetObjectStorageSetting(c)
			var response struct {
				Data struct {
					AccountName string `json:"account_name"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
			assert.Equal(t, stored.AccountName, response.Data.AccountName)
			assert.NotContains(t, w.Body.String(), "test-storage-secret")
			config, credential, err := normalizeObjectStorageSettingRequest(&objectStorageSettingRequest{Backend: backend, AccountName: response.Data.AccountName, Endpoint: stored.Endpoint, Bucket: stored.Bucket, Region: stored.Region, InputMode: "manual"})
			require.NoError(t, err)
			assert.Equal(t, "test-storage-secret", credential)
			assert.Equal(t, stored.AccountName, config.AccountName)
		})
	}
}

func TestObjectStorageSaveWithoutEnvironmentSecret(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "")
	t.Setenv("SESSION_SECRET", "")
	setupGenericTaskTest(t)
	previousLogDB := model.LOG_DB
	model.LOG_DB = model.DB
	t.Cleanup(func() { model.LOG_DB = previousLogDB })
	require.NoError(t, model.DB.AutoMigrate(&model.Log{}))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	require.NoError(t, model.DB.Where("key = ?", system_setting.ObjectStorageSettingOptionKey).Delete(&model.Option{}).Error)
	var mu sync.Mutex
	var data []byte
	exists := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPut:
			data, _ = io.ReadAll(r.Body)
			exists = true
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet, http.MethodHead:
			if !exists {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(data)))
			if r.Method == http.MethodGet {
				_, _ = w.Write(data)
			}
		case http.MethodDelete:
			exists = false
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(server.Close)
	body, err := common.Marshal(objectStorageSettingRequest{Backend: "s3", Endpoint: server.URL, Bucket: "artifacts", Region: "us-east-1", AccountName: "account", Credential: "test-save-secret", InputMode: "manual"})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/option/object_storage", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	UpdateObjectStorageSetting(c)
	var response struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.True(t, response.Success)
	assert.NotContains(t, w.Body.String(), "test-save-secret")
	var option model.Option
	require.NoError(t, model.DB.Where("key = ?", system_setting.ObjectStorageSettingOptionKey).First(&option).Error)
	var stored system_setting.ObjectStorageConfig
	require.NoError(t, common.UnmarshalJsonStr(option.Value, &stored))
	assert.Equal(t, "test-save-secret", stored.Credential)
	assert.Equal(t, "passed", stored.LastTestStatus)
	var logs []model.Log
	require.NoError(t, model.LOG_DB.Find(&logs).Error)
	require.NotEmpty(t, logs)
	audit, err := common.Marshal(logs)
	require.NoError(t, err)
	assert.NotContains(t, string(audit), "test-save-secret")
}

package controller

import (
	"net/http/httptest"
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
	previousDB, previousSecret := model.DB, common.CryptoSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB, common.CryptoSecret = db, "test-master-key"
	t.Setenv("CRYPTO_SECRET", "test-master-key")
	t.Cleanup(func() { model.DB, common.CryptoSecret = previousDB, previousSecret })
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	for _, backend := range []string{"azure_blob", "s3"} {
		t.Run(backend, func(t *testing.T) {
			ciphertext, err := common.EncryptObjectStorageCredential("test-storage-secret")
			require.NoError(t, err)
			stored := system_setting.ObjectStorageConfig{Backend: backend, AccountName: "account", Endpoint: "https://storage.example.com", Bucket: "artifacts", Region: "us-east-1", CredentialCiphertext: ciphertext}
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
			assert.NotContains(t, w.Body.String(), ciphertext)
			config, credential, err := normalizeObjectStorageSettingRequest(&objectStorageSettingRequest{Backend: backend, AccountName: response.Data.AccountName, Endpoint: stored.Endpoint, Bucket: stored.Bucket, Region: stored.Region, InputMode: "manual"})
			require.NoError(t, err)
			assert.Equal(t, "test-storage-secret", credential)
			assert.Equal(t, stored.AccountName, config.AccountName)
		})
	}
}

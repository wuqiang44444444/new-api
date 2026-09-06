package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestImageAdmissionStorageFailurePrecedesTaskAndDebit(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "image-admission-test-secret")
	previousDB, previousSecret := model.DB, common.CryptoSecret
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		model.DB = previousDB
		common.CryptoSecret = previousSecret
		model.NotifyObjectStorageSettingUpdate("")
		_ = sqlDB.Close()
	})
	model.DB = db
	common.CryptoSecret = "image-admission-test-secret"
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.User{}))
	require.NoError(t, db.Create(&model.User{Id: 771, Username: "image-admission", Quota: 1000}).Error)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPut, r.Method)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(server.Close)
	cipher, err := common.EncryptObjectStorageCredential("test-secret")
	require.NoError(t, err)
	config := system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: server.URL, Bucket: "images", AccountName: "account", Region: "us-east-1", CredentialCiphertext: cipher, Revision: "image-admission"}
	encoded, err := common.Marshal(config)
	require.NoError(t, err)
	for index, raw := range []string{"", string(encoded), "", string(encoded)} {
		model.NotifyObjectStorageSettingUpdate(raw)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
		c.Request.Header.Set("Prefer", "respond-async")
		c.Set("token_id", 7)
		request := &dto.ImageRequest{Model: "gemini-3.1-flash-image", Prompt: "draw a cat"}
		if index >= 2 {
			request.ResponseFormat = "url"
		}
		info := &relaycommon.RelayInfo{UserId: 771, OriginModelName: request.Model, Request: request,
			RelayMode:   relayconstant.RelayModeImagesGenerations,
			ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeGemini, UpstreamModelName: request.Model},
		}
		apiErr := imageAsyncHelper(c, info)
		require.NotNil(t, apiErr)
		assert.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
		var count int64
		require.NoError(t, db.Model(&model.Task{}).Count(&count).Error)
		assert.Zero(t, count)
		var user model.User
		require.NoError(t, db.First(&user, 771).Error)
		assert.Equal(t, 1000, user.Quota)
	}
}

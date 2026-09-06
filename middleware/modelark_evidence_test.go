package middleware

import (
	"bytes"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestEvidenceModelArkCapturesBeforeConversion(t *testing.T) {
	oldDB := model.DB
	oldConfig := system_setting.GetTaskRequestEvidenceConfig()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "evidence.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}))
	model.DB = db
	config := system_setting.TaskRequestEvidenceConfig{Enabled: true, StorageDir: t.TempDir(), EncryptionKeyHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f", MaxBodyBytes: 1 << 20, MaxResponseBytes: 1 << 20, WriteTimeoutSeconds: 5}
	system_setting.SetTaskRequestEvidenceConfig(config)
	require.NoError(t, service.InitTaskRequestEvidenceStore(config))
	t.Cleanup(func() {
		model.DB = oldDB
		system_setting.SetTaskRequestEvidenceConfig(oldConfig)
		_ = service.InitTaskRequestEvidenceStore(oldConfig)
	})
	original := `{"model":"seedance-2-0-m","content":[{"type":"text","text":"cat"}],"duration":4,"ratio":"16:9","resolution":"480p","generate_audio":false}`
	engine := gin.New()
	engine.POST("/api/v3/contents/generations/tasks", func(c *gin.Context) { c.Set("id", 7); c.Set(common.RequestIdKey, "original-request"); c.Next() }, ModelArkVideoCreateConvert(), func(c *gin.Context) {
		assert.Equal(t, "/v1/video/generations", c.Request.URL.Path)
		require.NoError(t, service.BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
		c.Status(http.StatusNoContent)
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(original))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusNoContent, recorder.Code)
	var events []model.TaskRequestEvidenceEvent
	require.NoError(t, db.Find(&events).Error)
	require.Len(t, events, 2)
	assert.Equal(t, "/api/v3/contents/generations/tasks", events[0].Target)
	payload, err := service.GetTaskRequestEvidenceStore().Get(events[0].ObjectKey)
	require.NoError(t, err)
	assert.JSONEq(t, original, string(payload))
}

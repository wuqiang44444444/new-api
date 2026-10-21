package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupModelArkEvidenceErrorFixture(t *testing.T, migrateEvents bool, storageDir string, config system_setting.TaskRequestEvidenceConfig) {
	t.Helper()
	oldDB, oldConfig := model.DB, system_setting.GetTaskRequestEvidenceConfig()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "evidence.db")), &gorm.Config{})
	require.NoError(t, err)
	migrations := []any{&model.TaskRequestEvidence{}}
	if migrateEvents {
		migrations = append(migrations, &model.TaskRequestEvidenceEvent{})
	}
	require.NoError(t, db.AutoMigrate(migrations...))
	model.DB = db
	system_setting.SetTaskRequestEvidenceConfig(config)
	if storageDir != "" {
		require.NoError(t, service.InitTaskRequestEvidenceStore(config))
	}
	t.Cleanup(func() {
		model.DB = oldDB
		system_setting.SetTaskRequestEvidenceConfig(oldConfig)
		_ = service.InitTaskRequestEvidenceStore(oldConfig)
	})
}

func newModelArkEvidenceErrorEngine(calls *int) *gin.Engine {
	engine := gin.New()
	engine.POST(
		"/api/v3/contents/generations/tasks",
		func(c *gin.Context) {
			c.Set("id", 7)
			c.Set(common.RequestIdKey, "req-evidence-classified")
			c.Next()
		},
		TaskCreateResponseContract(),
		ModelArkVideoCreateConvert(),
		func(c *gin.Context) {
			*calls++
			c.Status(http.StatusNoContent)
		},
	)
	return engine
}

func doModelArkEvidenceErrorRequest(t *testing.T, engine *gin.Engine, contentType, body string) (int, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", contentType)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	return recorder.Code, recorder.Body.String()
}

func modelArkEvidenceErrorConfig(t *testing.T, maxBodyBytes int64, storageDir string) system_setting.TaskRequestEvidenceConfig {
	t.Helper()
	return system_setting.TaskRequestEvidenceConfig{
		Enabled:             true,
		RetentionDays:       7,
		StorageDir:          storageDir,
		EncryptionKeyHex:    "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		MaxBodyBytes:        maxBodyBytes,
		MaxResponseBytes:    1 << 20,
		WriteTimeoutSeconds: 5,
	}
}

func TestModelArkEvidenceFailuresPreserveValidationAndDispatch(t *testing.T) {
	for _, scenario := range []string{"capture limit", "content type", "event database", "object storage", "configuration", "disabled"} {
		t.Run(scenario, func(t *testing.T) {
			config := modelArkEvidenceErrorConfig(t, 1<<20, t.TempDir())
			contentType := "application/json"
			migrateEvents := true
			storageDir := config.StorageDir
			switch scenario {
			case "capture limit":
				config.MaxBodyBytes = 16
			case "content type":
				contentType = "json;;="
			case "event database":
				migrateEvents = false
			case "object storage":
				storageDir = filepath.Join(t.TempDir(), "occupied")
				require.NoError(t, os.WriteFile(storageDir, []byte("occupied"), 0600))
				config.StorageDir = storageDir
			case "configuration":
				config.MaxBodyBytes = 0
				storageDir = ""
			case "disabled":
				config.Enabled = false
				storageDir = ""
			}
			setupModelArkEvidenceErrorFixture(t, migrateEvents, storageDir, config)
			calls := 0
			status, body := doModelArkEvidenceErrorRequest(t, newModelArkEvidenceErrorEngine(&calls), contentType, `{"model":"seedance-2-0-m","content":[{"type":"text","text":"cat"}]}`)
			if scenario == "content type" {
				assert.Equal(t, http.StatusBadRequest, status)
				assert.Contains(t, body, `"code":"invalid_request"`)
				assert.Zero(t, calls)
			} else {
				assert.Equal(t, http.StatusNoContent, status)
				assert.Empty(t, body)
				assert.Equal(t, 1, calls)
			}
		})
	}
}

func TestModelArkInvalidJSONStillFailsBusinessConversion(t *testing.T) {
	config := modelArkEvidenceErrorConfig(t, 1<<20, t.TempDir())
	setupModelArkEvidenceErrorFixture(t, true, config.StorageDir, config)
	calls := 0
	status, body := doModelArkEvidenceErrorRequest(t, newModelArkEvidenceErrorEngine(&calls), "application/json", `{"model":`)
	assert.Equal(t, http.StatusBadRequest, status)
	assert.Contains(t, body, `"code":"invalid_request"`)
	assert.NotContains(t, body, `{"model":`)
	assert.Zero(t, calls)
}

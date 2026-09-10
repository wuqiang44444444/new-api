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
		model.DB = nil
		system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
		_ = service.InitTaskRequestEvidenceStore(system_setting.TaskRequestEvidenceConfig{Enabled: false})
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
		StorageDir:          storageDir,
		EncryptionKeyHex:    "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		MaxBodyBytes:        maxBodyBytes,
		MaxResponseBytes:    1 << 20,
		WriteTimeoutSeconds: 5,
	}
}

func TestModelArkEvidenceFailureClassifications(t *testing.T) {
	t.Run("north body too large returns 413 request_body_too_large", func(t *testing.T) {
		config := modelArkEvidenceErrorConfig(t, 16, t.TempDir())
		setupModelArkEvidenceErrorFixture(t, true, config.StorageDir, config)
		calls := 0
		engine := newModelArkEvidenceErrorEngine(&calls)
		status, body := doModelArkEvidenceErrorRequest(t, engine, "application/json", `{"model":"seedance-2-0-m","content":[{"type":"text","text":"this body is longer than sixteen bytes"}]}`)
		require.Equal(t, http.StatusRequestEntityTooLarge, status)
		assert.Contains(t, body, `"code":"request_body_too_large"`)
		assert.Contains(t, body, "16 byte")
		assert.Contains(t, body, "req-evidence-classified")
		assert.Zero(t, calls, "evidence rejection must stop before the relay handler")
	})

	t.Run("proven invalid north json under json contract returns 400 invalid_request", func(t *testing.T) {
		config := modelArkEvidenceErrorConfig(t, 1<<20, t.TempDir())
		setupModelArkEvidenceErrorFixture(t, true, config.StorageDir, config)
		calls := 0
		engine := newModelArkEvidenceErrorEngine(&calls)
		// 非法 JSON 穿过证据脱敏失败分支，被证明为客户语法违例。
		status, body := doModelArkEvidenceErrorRequest(t, engine, "application/json", `{"model":`)
		require.Equal(t, http.StatusBadRequest, status)
		assert.Contains(t, body, `"code":"invalid_request"`)
		assert.NotContains(t, body, `{"model":`, "raw client input must not be echoed")
		assert.Zero(t, calls)
	})

	t.Run("unparseable content type stays a processing failure", func(t *testing.T) {
		config := modelArkEvidenceErrorConfig(t, 1<<20, t.TempDir())
		setupModelArkEvidenceErrorFixture(t, true, config.StorageDir, config)
		calls := 0
		engine := newModelArkEvidenceErrorEngine(&calls)
		status, body := doModelArkEvidenceErrorRequest(t, engine, "json;;=", `{"model":"seedance-2-0-m","content":[{"type":"text","text":"cat"}]}`)
		require.Equal(t, http.StatusServiceUnavailable, status)
		assert.Contains(t, body, `"code":"evidence_processing_failed"`)
		assert.Zero(t, calls)
	})

	t.Run("event reservation database failure returns 503 evidence_database_unavailable", func(t *testing.T) {
		config := modelArkEvidenceErrorConfig(t, 1<<20, t.TempDir())
		// 不建事件表：证据索引可建，事件预留必然失败。
		setupModelArkEvidenceErrorFixture(t, false, config.StorageDir, config)
		calls := 0
		engine := newModelArkEvidenceErrorEngine(&calls)
		status, body := doModelArkEvidenceErrorRequest(t, engine, "application/json", `{"model":"seedance-2-0-m","content":[{"type":"text","text":"cat"}]}`)
		require.Equal(t, http.StatusServiceUnavailable, status)
		assert.Contains(t, body, `"code":"evidence_database_unavailable"`)
		assert.Zero(t, calls)
	})

	t.Run("object storage failure returns 503 evidence_storage_unavailable", func(t *testing.T) {
		unwritable := filepath.Join(t.TempDir(), "file.txt")
		require.NoError(t, os.WriteFile(unwritable, []byte("occupied"), 0o600))
		config := modelArkEvidenceErrorConfig(t, 1<<20, unwritable)
		setupModelArkEvidenceErrorFixture(t, true, config.StorageDir, config)
		calls := 0
		err := service.InitTaskRequestEvidenceStore(config)
		require.NoError(t, err)
		engine := newModelArkEvidenceErrorEngine(&calls)
		status, body := doModelArkEvidenceErrorRequest(t, engine, "application/json", `{"model":"seedance-2-0-m","content":[{"type":"text","text":"cat"}]}`)
		require.Equal(t, http.StatusServiceUnavailable, status)
		assert.Contains(t, body, `"code":"evidence_storage_unavailable"`)
		assert.Zero(t, calls)
	})

	t.Run("invalid configuration returns 503 evidence_configuration_error", func(t *testing.T) {
		brokenConfig := system_setting.TaskRequestEvidenceConfig{Enabled: true, StorageDir: t.TempDir(), MaxBodyBytes: 0, MaxResponseBytes: 1 << 20, WriteTimeoutSeconds: 5}
		setupModelArkEvidenceErrorFixture(t, true, "", brokenConfig)
		calls := 0
		engine := newModelArkEvidenceErrorEngine(&calls)
		status, body := doModelArkEvidenceErrorRequest(t, engine, "application/json", `{"model":"seedance-2-0-m","content":[{"type":"text","text":"cat"}]}`)
		require.Equal(t, http.StatusServiceUnavailable, status)
		assert.Contains(t, body, `"code":"evidence_configuration_error"`)
		assert.Zero(t, calls)
	})

	t.Run("evidence disabled keeps the request flowing to conversion", func(t *testing.T) {
		disabled := system_setting.TaskRequestEvidenceConfig{Enabled: false}
		setupModelArkEvidenceErrorFixture(t, true, "", disabled)
		calls := 0
		engine := newModelArkEvidenceErrorEngine(&calls)
		status, _ := doModelArkEvidenceErrorRequest(t, engine, "application/json", `{"model":"seedance-2-0-m","content":[{"type":"text","text":"cat"}]}`)
		require.Equal(t, http.StatusNoContent, status)
		assert.Equal(t, 1, calls, "handler runs once when evidence is disabled")
	})
}

package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type evidenceTestEnv struct {
	db       *gorm.DB
	storeDir string
	oldDB    *gorm.DB
}

func setupEvidenceTestEnv(t testing.TB) *evidenceTestEnv {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "evidence.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}, &model.TaskRequestEvidenceAccessLog{}, &model.EvidenceTaskLink{}))
	oldDB, oldStore, oldConfig := model.DB, evidenceObjectStore, system_setting.GetTaskRequestEvidenceConfig()
	oldPressure := evidenceDiskPressure.Load()
	model.DB = db
	evidenceDiskPressure.Store(false)
	dir := t.TempDir()
	config := system_setting.TaskRequestEvidenceConfig{Enabled: true, RetentionDays: 2, StorageDir: dir, EncryptionKeyHex: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"}
	system_setting.SetTaskRequestEvidenceConfig(config)
	require.NoError(t, InitTaskRequestEvidenceStore(config))
	t.Cleanup(func() {
		drainImageEvidence(t)
		model.DB = oldDB
		evidenceObjectStore = oldStore
		system_setting.SetTaskRequestEvidenceConfig(oldConfig)
		evidenceDiskPressure.Store(oldPressure)
	})
	return &evidenceTestEnv{db: db, storeDir: dir, oldDB: oldDB}
}
func drainImageEvidence(t testing.TB) {
	t.Helper()
	require.NoError(t, FlushRequestEvidence(context.Background()))
}
func newEvidenceTestContext(t *testing.T, body []byte) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech?model=tts-1", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.RequestIdKey, "request-fixture")
	c.Set("id", 7)
	c.Set("app_id", 3)
	c.Set("token_id", 42)
	return c
}
func evidenceEventRows(t *testing.T, id int64) []*model.TaskRequestEvidenceEvent {
	t.Helper()
	drainImageEvidence(t)
	events, err := model.ListTaskRequestEvidenceEvents(id)
	require.NoError(t, err)
	return events
}
func evidenceCapturedBody(t *testing.T, event *model.TaskRequestEvidenceEvent) []byte {
	t.Helper()
	content, status := ReadEvidenceEventContent(event, false)
	require.Equal(t, "available", status)
	return content.Body
}
func evidenceIndexes(t *testing.T, filter model.TaskRequestEvidenceQueryParams) []*model.TaskRequestEvidence {
	t.Helper()
	drainImageEvidence(t)
	items, _, err := model.QueryTaskRequestEvidence(filter)
	require.NoError(t, err)
	return items
}

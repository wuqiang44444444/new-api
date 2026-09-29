package controller

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceDisabledVideoContentStillDelivers(t *testing.T) {
	old := system_setting.GetTaskRequestEvidenceConfig()
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	t.Cleanup(func() { system_setting.SetTaskRequestEvidenceConfig(old) })
	TestLegacyVideoArtifactContentUsesGetResultURL(t)
	var recorder *taskContentDeliveryRecorder
	require.NotPanics(t, func() { recorder.Done(200, nil) })
}

func setupEvidenceViewFixture(t *testing.T) *model.TaskRequestEvidence {
	t.Helper()
	setupGenericTaskTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.TaskRequestEvidence{}, &model.TaskRequestEvidenceEvent{}, &model.TaskRequestEvidenceAccessLog{}))
	old := system_setting.GetTaskRequestEvidenceConfig()
	config := system_setting.TaskRequestEvidenceConfig{Enabled: true, StorageDir: t.TempDir(), EncryptionKeyHex: strings.Repeat("01", 32), MaxBodyBytes: 1024, MaxResponseBytes: 1024, WriteTimeoutSeconds: 5}
	system_setting.SetTaskRequestEvidenceConfig(config)
	require.NoError(t, service.InitTaskRequestEvidenceStore(config))
	t.Cleanup(func() {
		system_setting.SetTaskRequestEvidenceConfig(old)
		_ = service.InitTaskRequestEvidenceStore(old)
	})
	evidence := &model.TaskRequestEvidence{RequestID: "req-1", UpstreamModel: "provider-model", UpstreamRequestID: "upstream-1"}
	require.NoError(t, model.CreateTaskRequestEvidence(evidence))
	payload := []byte(`{"content":{"video_url":"https://cdn.example/video?x=1&sig=secret"}}`)
	require.NoError(t, service.GetTaskRequestEvidenceStore().Put("test/body.bin", payload))
	event := &model.TaskRequestEvidenceEvent{EvidenceId: evidence.Id, ObjectKey: "test/body.bin", ContentType: "application/json", Sha256: service.EvidenceSha256Hex(payload)}
	require.NoError(t, model.CreateTaskRequestEvidenceEvent(event))
	return evidence
}

func callEvidenceHandler(t *testing.T, handler gin.HandlerFunc, role int, path string, params ...gin.Param) *httptest.ResponseRecorder {
	t.Helper()
	writer := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(writer)
	c.Set("id", 7)
	c.Set("role", role)
	c.Params = params
	c.Request = httptest.NewRequest("GET", path, nil)
	handler(c)
	return writer
}

func evidenceAccessRows(t *testing.T, evidenceId int64) []model.TaskRequestEvidenceAccessLog {
	t.Helper()
	var rows []model.TaskRequestEvidenceAccessLog
	require.NoError(t, model.DB.Model(&model.TaskRequestEvidenceAccessLog{}).Where("evidence_id = ?", evidenceId).Order("id").Find(&rows).Error)
	return rows
}

func TestEvidenceDetailMasksNestedSignedURLForEveryRole(t *testing.T) {
	evidence := setupEvidenceViewFixture(t)
	for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
		writer := callEvidenceHandler(t, GetTaskRequestEvidenceDetail, role, "/api/task_request_evidence/1", gin.Param{Key: "id", Value: "1"})
		require.Equal(t, 200, writer.Code)
		// The default preview masks signed URLs for every role; Root keeps the
		// upstream diagnostic projection in the evidence index view.
		assert.NotContains(t, writer.Body.String(), "secret")
		assert.NotContains(t, writer.Body.String(), "test/body.bin")
		if role == common.RoleAdminUser {
			assert.NotContains(t, writer.Body.String(), "provider-model")
		} else {
			assert.Contains(t, writer.Body.String(), "provider-model")
		}
	}
	rows := evidenceAccessRows(t, evidence.Id)
	require.Len(t, rows, 2)
	for _, row := range rows {
		assert.Equal(t, "view", row.Action)
		assert.Equal(t, "redacted", row.Scope)
	}
}

func TestEvidenceOriginalViewAndDownloadAuditSeparately(t *testing.T) {
	evidence := setupEvidenceViewFixture(t)
	root := common.RoleRootUser

	// The masked detail preview is the only pre-click exposure.
	writer := callEvidenceHandler(t, GetTaskRequestEvidenceDetail, root, "/api/task_request_evidence/1", gin.Param{Key: "id", Value: "1"})
	require.Equal(t, 200, writer.Code)

	// The explicit original view returns the unmasked body inline as text.
	writer = callEvidenceHandler(t, GetTaskRequestEvidenceContent, root, "/api/task_request_evidence/1/events/1/content",
		gin.Param{Key: "id", Value: "1"}, gin.Param{Key: "event_id", Value: "1"})
	require.Equal(t, 200, writer.Code)
	assert.Equal(t, "text/plain; charset=utf-8", writer.Header().Get("Content-Type"))
	assert.Contains(t, writer.Body.String(), "sig=secret")

	// The download delivers the same stored bytes as an attachment.
	writer = callEvidenceHandler(t, GetTaskRequestEvidenceObject, root, "/api/task_request_evidence/1/events/1/object",
		gin.Param{Key: "id", Value: "1"}, gin.Param{Key: "event_id", Value: "1"})
	require.Equal(t, 200, writer.Code)
	assert.Equal(t, "application/octet-stream", writer.Header().Get("Content-Type"))
	assert.Contains(t, writer.Body.String(), "sig=secret")

	// A missing event is a 404 and is not audited as a successful original view.
	writer = callEvidenceHandler(t, GetTaskRequestEvidenceContent, root, "/api/task_request_evidence/1/events/99/content",
		gin.Param{Key: "id", Value: "1"}, gin.Param{Key: "event_id", Value: "99"})
	require.Equal(t, 404, writer.Code)

	rows := evidenceAccessRows(t, evidence.Id)
	require.Len(t, rows, 3)
	assert.Equal(t, "view", rows[0].Action)
	assert.Equal(t, "redacted", rows[0].Scope)
	assert.Equal(t, "view", rows[1].Action)
	assert.Equal(t, "original", rows[1].Scope)
	assert.Equal(t, "download", rows[2].Action)
	assert.Equal(t, "original", rows[2].Scope)
}

func TestEvidenceDetailDecryptFailureKeepsStatusAndAudit(t *testing.T) {
	setupEvidenceViewFixture(t)
	wrongConfig := system_setting.GetTaskRequestEvidenceConfig()
	wrongConfig.EncryptionKeyHex = strings.Repeat("02", 32)
	require.NoError(t, service.InitTaskRequestEvidenceStore(wrongConfig))
	for _, handler := range []struct {
		name   string
		call   func(c *gin.Context)
		params []gin.Param
	}{
		{name: "detail", call: GetTaskRequestEvidenceDetail, params: []gin.Param{{Key: "id", Value: "1"}}},
		{name: "content", call: GetTaskRequestEvidenceContent, params: []gin.Param{{Key: "id", Value: "1"}, {Key: "event_id", Value: "1"}}},
		{name: "download", call: GetTaskRequestEvidenceObject, params: []gin.Param{{Key: "id", Value: "1"}, {Key: "event_id", Value: "1"}}},
	} {
		t.Run(handler.name, func(t *testing.T) {
			writer := callEvidenceHandler(t, handler.call, common.RoleRootUser, "/api/task_request_evidence/1", handler.params...)
			if handler.name == "detail" {
				assert.Equal(t, 200, writer.Code)
			} else {
				assert.Equal(t, 410, writer.Code)
			}
			assert.Contains(t, writer.Body.String(), `"body_status":"decrypt_failed"`)
			assert.NotContains(t, writer.Body.String(), "test/body.bin")
			assert.NotContains(t, writer.Body.String(), "sig=secret")
		})
	}
	rows, total := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{UpstreamRequestID: "upstream-1"})
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
}

func TestEvidenceOriginalUnavailableNeverReturnsBody(t *testing.T) {
	for _, scenario := range []string{"expired", "missing", "cross_evidence_event"} {
		t.Run(scenario, func(t *testing.T) {
			evidence := setupEvidenceViewFixture(t)
			expectedStatus, expectedBodyStatus := 410, scenario
			switch scenario {
			case "expired":
				require.NoError(t, model.DB.Model(evidence).Update("body_expired", true).Error)
			case "missing":
				require.NoError(t, service.GetTaskRequestEvidenceStore().Delete("test/body.bin"))
			case "cross_evidence_event":
				other := &model.TaskRequestEvidence{RequestID: "other-request"}
				require.NoError(t, model.CreateTaskRequestEvidence(other))
				evidence = other
				expectedStatus, expectedBodyStatus = 404, ""
			}
			for _, handler := range []gin.HandlerFunc{GetTaskRequestEvidenceContent, GetTaskRequestEvidenceObject} {
				response := callEvidenceHandler(t, handler, common.RoleRootUser, "/api/task_request_evidence",
					gin.Param{Key: "id", Value: strconv.FormatInt(evidence.Id, 10)}, gin.Param{Key: "event_id", Value: "1"})
				assert.Equal(t, expectedStatus, response.Code)
				assert.NotContains(t, response.Body.String(), "sig=secret")
				if expectedBodyStatus != "" {
					assert.Contains(t, response.Body.String(), `"body_status":"`+expectedBodyStatus+`"`)
				}
			}
			assert.Empty(t, evidenceAccessRows(t, evidence.Id))
		})
	}
}

func TestEvidenceOriginalAuditFailureDoesNotChangeDelivery(t *testing.T) {
	setupEvidenceViewFixture(t)
	require.NoError(t, model.DB.Migrator().DropTable(&model.TaskRequestEvidenceAccessLog{}))
	response := callEvidenceHandler(t, GetTaskRequestEvidenceContent, common.RoleRootUser, "/api/task_request_evidence/1/events/1/content",
		gin.Param{Key: "id", Value: "1"}, gin.Param{Key: "event_id", Value: "1"})
	assert.Equal(t, 200, response.Code)
	assert.Contains(t, response.Body.String(), "sig=secret")
}

package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEvidenceDisabledVideoContentStillDelivers(t *testing.T) {
	old := system_setting.GetTaskRequestEvidenceConfig()
	system_setting.SetTaskRequestEvidenceConfig(system_setting.TaskRequestEvidenceConfig{Enabled: false})
	t.Cleanup(func() { system_setting.SetTaskRequestEvidenceConfig(old) })
	TestLegacyVideoArtifactContentUsesGetResultURL(t)
	var recorder *taskContentDeliveryRecorder
	require.NotPanics(t, func() { recorder.Done(200, nil) })
}

func TestEvidenceDetailMasksNestedSignedURLForAdmin(t *testing.T) {
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
	for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
		writer := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(writer)
		c.Set("id", 7)
		c.Set("role", role)
		c.Params = gin.Params{{Key: "id", Value: "1"}}
		c.Request = httptest.NewRequest("GET", "/api/task_request_evidence/1", nil)
		GetTaskRequestEvidenceDetail(c)
		require.Equal(t, 200, writer.Code)
		if role == common.RoleAdminUser {
			assert.NotContains(t, writer.Body.String(), "secret")
			assert.NotContains(t, writer.Body.String(), "provider-model")
		} else {
			assert.Contains(t, writer.Body.String(), "secret")
		}
	}
	rows, total := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{UpstreamRequestID: "upstream-1"})
	require.EqualValues(t, 1, total)
	require.Len(t, rows, 1)
}

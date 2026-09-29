package router

import (
	"net/http"
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

func TestTaskRequestEvidenceRoutesPermissionsAndCache(t *testing.T) {
	setupRelayRouterTestDB(t)
	old := system_setting.GetTaskRequestEvidenceConfig()
	config := system_setting.TaskRequestEvidenceConfig{Enabled: true, StorageDir: t.TempDir(), EncryptionKeyHex: strings.Repeat("01", 32), MaxBodyBytes: 1024, MaxResponseBytes: 1024, WriteTimeoutSeconds: 5}
	system_setting.SetTaskRequestEvidenceConfig(config)
	require.NoError(t, service.InitTaskRequestEvidenceStore(config))
	t.Cleanup(func() {
		system_setting.SetTaskRequestEvidenceConfig(old)
		require.NoError(t, service.InitTaskRequestEvidenceStore(old))
	})
	evidence := &model.TaskRequestEvidence{RequestID: "evidence-fixture", UpstreamModel: "private-provider-model"}
	require.NoError(t, model.CreateTaskRequestEvidence(evidence))
	payload := []byte(`{"url":"https://example.test/video?sig=fixture","api_key":"[REDACTED]"}`)
	require.NoError(t, service.GetTaskRequestEvidenceStore().Put("fixture/body", payload))
	event := &model.TaskRequestEvidenceEvent{EvidenceId: evidence.Id, ObjectKey: "fixture/body", ContentType: "application/json", Sha256: service.EvidenceSha256Hex(payload), Complete: true}
	require.NoError(t, model.CreateTaskRequestEvidenceEvent(event))
	base := "/api/task_request_evidence/" + strconv.FormatInt(evidence.Id, 10)
	eventPath := base + "/events/" + strconv.FormatInt(event.Id, 10)
	engine := gin.New()
	registerTaskRequestEvidenceRoutes(engine.Group("/api"))
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser} {
		pat := ""
		if role != 0 {
			pat = "evidence-fixture-" + strconv.Itoa(role)
			require.NoError(t, model.DB.Create(&model.User{Username: pat, Status: common.UserStatusEnabled, Role: role, Group: "default", AccessToken: &pat, AuthVersion: 1, AffCode: pat}).Error)
		}
		for _, route := range []struct {
			path     string
			original bool
		}{
			{"/api/task_request_evidence?request_id=evidence-fixture", false}, {base, false}, {eventPath + "/content", true}, {eventPath + "/object", true},
		} {
			t.Run(strconv.Itoa(role)+route.path, func(t *testing.T) {
				request := httptest.NewRequest(http.MethodGet, route.path, nil)
				if pat != "" {
					request.Header.Set("Authorization", "Bearer "+pat)
				}
				response := httptest.NewRecorder()
				engine.ServeHTTP(response, request)
				expected := http.StatusOK
				if role == 0 {
					expected = http.StatusUnauthorized
				} else if role < common.RoleAdminUser || (route.original && role < common.RoleRootUser) {
					expected = http.StatusForbidden
				}
				require.Equal(t, expected, response.Code)
				assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
				if expected == http.StatusOK && route.original {
					assert.Equal(t, string(payload), response.Body.String())
					assert.Contains(t, response.Header().Get("Content-Type"), map[bool]string{true: "text/plain", false: "application/octet-stream"}[strings.HasSuffix(route.path, "/content")])
				} else {
					assert.NotContains(t, response.Body.String(), "sig=fixture")
				}
			})
		}
	}
	var rows []model.TaskRequestEvidenceAccessLog
	require.NoError(t, model.DB.Where("evidence_id = ?", evidence.Id).Order("id").Find(&rows).Error)
	require.Len(t, rows, 4)
	assert.Equal(t, []string{"redacted", "redacted", "original", "original"}, []string{rows[0].Scope, rows[1].Scope, rows[2].Scope, rows[3].Scope})
	assert.Equal(t, "view", rows[2].Action)
	assert.Equal(t, "download", rows[3].Action)
}

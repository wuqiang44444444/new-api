package router

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestEvidenceDownloadRequiresFinalAuthenticationAndHistoricalDigest(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.AuditLog{}))
	old := system_setting.GetTaskRequestEvidenceConfig()
	config := system_setting.TaskRequestEvidenceConfig{StorageDir: t.TempDir(), EncryptionKeyHex: strings.Repeat("01", 32)}
	system_setting.SetTaskRequestEvidenceConfig(config)
	require.NoError(t, service.InitTaskRequestEvidenceStore(config))
	t.Cleanup(func() {
		system_setting.SetTaskRequestEvidenceConfig(old)
		require.NoError(t, service.InitTaskRequestEvidenceStore(old))
	})
	token := "evidence-download-test"
	require.NoError(t, model.DB.Create(&model.User{
		Username: token, Status: common.UserStatusEnabled, Role: common.RoleAdminUser,
		Group: "default", AccessToken: &token, AuthVersion: 1, AffCode: token,
	}).Error)
	index := &model.TaskRequestEvidence{RequestID: "evidence-download"}
	require.NoError(t, model.CreateTaskRequestEvidence(index))
	payload := bytes.Repeat([]byte("opaque evidence bytes\n"), 10000)
	digest := service.EvidenceSha256Hex(payload)
	const firstPacket = len("NEVSIO3\n") + (64 << 10) + 32
	tests := []struct {
		name, digest string
		payload      []byte
		truncate     bool
		wantError    bool
	}{
		{name: "blank padded digest", digest: strings.Repeat(" ", 64), payload: payload},
		{name: "matching digest", digest: digest, payload: payload},
		{name: "incorrect digest", digest: strings.Repeat("0", 64), payload: payload, wantError: true},
		{name: "incorrect digest before any bytes are sent", digest: digest, payload: []byte("x"), wantError: true},
		{name: "missing final packet", payload: payload, truncate: true, wantError: true},
	}

	engine := gin.New()
	SetApiRouter(engine)
	for _, protocol := range []string{"HTTP/1.1", "HTTP/2.0"} {
		server := httptest.NewUnstartedServer(engine)
		if protocol == "HTTP/2.0" {
			server.EnableHTTP2 = true
			server.StartTLS()
		} else {
			server.Start()
		}
		t.Cleanup(server.Close)
		for _, tc := range tests {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				event := &model.TaskRequestEvidenceEvent{EvidenceId: index.Id, Sha256: tc.digest}
				require.NoError(t, model.CreateTaskRequestEvidenceEvent(event))
				require.NoError(t, service.GetTaskRequestEvidenceStore().PutEventBody(index.Id, event.Id, tc.payload))
				if tc.truncate {
					path := filepath.Join(config.StorageDir, service.EvidenceObjectKey(index.Id, event.Id))
					require.NoError(t, os.Truncate(path, int64(firstPacket)))
				}

				path := "/api/task_request_evidence/" + strconv.FormatInt(index.Id, 10) +
					"/events/" + strconv.FormatInt(event.Id, 10) + "/object"
				request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
				require.NoError(t, err)
				request.Header.Set("Authorization", "Bearer "+token)
				request.Header.Set("Accept-Encoding", "gzip")
				response, err := server.Client().Do(request)
				require.NoError(t, err)
				defer response.Body.Close()
				require.Equal(t, protocol, response.Proto)
				require.Empty(t, response.Header.Get("Content-Encoding"))
				body, readErr := io.ReadAll(response.Body)
				if tc.wantError && len(tc.payload) == 1 {
					require.NoError(t, readErr)
					assert.Equal(t, http.StatusGone, response.StatusCode)
					assert.Contains(t, string(body), "integrity_failed")
					assert.Empty(t, response.Header.Get("Content-Disposition"))
					return
				}
				require.Equal(t, http.StatusOK, response.StatusCode)
				if tc.wantError {
					assert.ErrorIs(t, readErr, io.ErrUnexpectedEOF)
					assert.NotEmpty(t, body)
					assert.Less(t, int64(len(body)), response.ContentLength)
					return
				}
				require.NoError(t, readErr)
				assert.Equal(t, int64(len(tc.payload)), response.ContentLength)
				assert.True(t, bytes.Equal(tc.payload, body), "download must preserve the original bytes")
			})
		}
	}
}

func TestTaskRequestEvidenceRoutesPermissionsAndCache(t *testing.T) {
	setupRelayRouterTestDB(t)
	old := system_setting.GetTaskRequestEvidenceConfig()
	config := system_setting.TaskRequestEvidenceConfig{Enabled: true, StorageDir: t.TempDir(), EncryptionKeyHex: strings.Repeat("01", 32)}
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
				} else if role < common.RoleAdminUser {
					expected = http.StatusForbidden
				}
				require.Equal(t, expected, response.Code)
				assert.Contains(t, response.Header().Get("Cache-Control"), "no-store")
				if expected == http.StatusOK && route.original {
					if strings.HasSuffix(route.path, "/content") {
						var result struct {
							Data struct {
								Text string `json:"text"`
							}
						}
						require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
						assert.Equal(t, string(payload), result.Data.Text)
					} else {
						assert.Equal(t, string(payload), response.Body.String())
					}
					assert.Contains(t, response.Header().Get("Content-Type"), map[bool]string{true: "application/json", false: "application/octet-stream"}[strings.HasSuffix(route.path, "/content")])
				} else {
					assert.NotContains(t, response.Body.String(), "sig=fixture")
				}
			})
		}
	}
	var rows []model.TaskRequestEvidenceAccessLog
	require.NoError(t, model.DB.Where("evidence_id = ?", evidence.Id).Order("id").Find(&rows).Error)
	require.Len(t, rows, 6)
	for _, i := range []int{0, 3} {
		assert.Equal(t, "directory", rows[i].Scope)
		assert.Equal(t, "view", rows[i+1].Action)
		assert.Equal(t, "original", rows[i+1].Scope)
		assert.Equal(t, "download", rows[i+2].Action)
	}
}

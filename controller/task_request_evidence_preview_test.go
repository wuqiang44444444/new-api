package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceRawObjectsAreSelectableWithoutObjectKeyAndDirectoryNeverReadsBodies(t *testing.T) {
	setupEvidenceViewFixture(t)
	engine := gin.New()
	engine.Use(func(c *gin.Context) { c.Set(common.RequestIdKey, "raw-directory"); c.Next() }, service.APIRequestEvidence())
	engine.POST("/v1/images/generations", func(c *gin.Context) {
		c.Set("id", 7)
		c.Set(common.RequestIdKey, "raw-directory")
		_, err := c.GetRawData()
		require.NoError(t, err)
		c.String(200, "ok")
	})
	raw := []byte(`{ "url":"https:\/\/example.test/video?sig=fixture-secret", "api_key":"synthetic", "id":9007199254740993 }`)
	request := httptest.NewRequest("POST", "/v1/images/generations", bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(httptest.NewRecorder(), request)
	indexes, _, err := model.QueryTaskRequestEvidence(model.TaskRequestEvidenceQueryParams{RequestID: "raw-directory"})
	require.NoError(t, err)
	require.Len(t, indexes, 1)
	events, err := model.ListTaskRequestEvidenceEvents(indexes[0].Id)
	require.NoError(t, err)
	require.Len(t, events, 2)
	event := events[0]
	assert.Empty(t, event.ObjectKey)
	params := []gin.Param{{Key: "id", Value: strconv.FormatInt(indexes[0].Id, 10)}, {Key: "event_id", Value: strconv.FormatInt(event.Id, 10)}}
	for _, role := range []int{common.RoleAdminUser, common.RoleRootUser} {
		directory := callEvidenceHandler(t, GetTaskRequestEvidenceDetail, role, "/api/task_request_evidence", params...)
		assert.NotContains(t, directory.Body.String(), "fixture-secret")
		assert.NotContains(t, directory.Body.String(), `"has_body"`)
		assert.NotContains(t, directory.Body.String(), `"preview"`)
		response := callEvidenceHandler(t, GetTaskRequestEvidenceContent, role, "/api/task_request_evidence", params...)
		require.Equal(t, 200, response.Code)
		var decoded struct {
			Data struct {
				Text string `json:"text"`
			}
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &decoded))
		assert.Equal(t, string(raw), decoded.Data.Text)
		payload, status := service.ReadEvidenceEventBody(event, false)
		require.Equal(t, "available", status)
		download := callEvidenceHandler(t, GetTaskRequestEvidenceObject, role, "/api/task_request_evidence", params...)
		assert.Equal(t, payload, download.Body.Bytes())
	}
	config := system_setting.GetTaskRequestEvidenceConfig()
	config.EncryptionKeyHex = "0202020202020202020202020202020202020202020202020202020202020202"
	require.NoError(t, service.InitTaskRequestEvidenceStore(config))
	directory := callEvidenceHandler(t, GetTaskRequestEvidenceDetail, common.RoleAdminUser, "/api/task_request_evidence", params...)
	assert.Equal(t, http.StatusOK, directory.Code)
	assert.NotContains(t, directory.Body.String(), "decrypt_failed")
	selected := callEvidenceHandler(t, GetTaskRequestEvidenceContent, common.RoleAdminUser, "/api/task_request_evidence", params...)
	assert.Equal(t, http.StatusGone, selected.Code)
	assert.Contains(t, selected.Body.String(), "decrypt_failed")
}

func TestEvidenceSelectedHistoricalMalformedAndBinaryObjectsRemainReadable(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		body       []byte
	}{
		{"malformed JSON", "application/json", []byte(`{"api_key":"fixture-secret`)},
		{"form", "application/x-www-form-urlencoded", []byte("url=https%3A%2F%2Fexample.test%3Fsig%3Dfixture-secret")},
		{"multipart", "multipart/form-data; boundary=fixture", []byte("--fixture\r\ninvalid-header\r\n\r\nfixture-secret")},
		{"binary", "image/png", []byte{0xff, 0x00, 0xfe}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index := setupEvidenceViewFixture(t)
			require.NoError(t, service.GetTaskRequestEvidenceStore().Put("selected/body", tc.body))
			event := &model.TaskRequestEvidenceEvent{EvidenceId: index.Id, ObjectKey: "selected/body", ContentType: tc.kind, Sha256: service.EvidenceSha256Hex(tc.body)}
			require.NoError(t, model.CreateTaskRequestEvidenceEvent(event))
			params := []gin.Param{{Key: "id", Value: strconv.FormatInt(index.Id, 10)}, {Key: "event_id", Value: strconv.FormatInt(event.Id, 10)}}
			response := callEvidenceHandler(t, GetTaskRequestEvidenceContent, common.RoleAdminUser, "/api/task_request_evidence", params...)
			require.Equal(t, 200, response.Code)
			var decoded struct {
				Data struct {
					Text   *string `json:"text"`
					Binary []byte  `json:"base64"`
				}
			}
			require.NoError(t, common.Unmarshal(response.Body.Bytes(), &decoded))
			if tc.name == "binary" {
				assert.Nil(t, decoded.Data.Text)
				assert.Equal(t, tc.body, decoded.Data.Binary)
			} else {
				require.NotNil(t, decoded.Data.Text)
				assert.Equal(t, string(tc.body), *decoded.Data.Text)
			}
		})
	}
}

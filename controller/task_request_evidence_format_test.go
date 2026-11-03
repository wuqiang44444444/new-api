package controller

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceSelectedJSONFormattingIsExplicitAndNeverChangesStoredBytes(t *testing.T) {
	index := setupEvidenceViewFixture(t)
	body := []byte(`{"id":9007199254740993,"id":2,"amount":1.00}`)
	require.NoError(t, service.GetTaskRequestEvidenceStore().Put("format/body", body))
	event := &model.TaskRequestEvidenceEvent{EvidenceId: index.Id, ObjectKey: "format/body", ContentType: "application/json"}
	require.NoError(t, model.CreateTaskRequestEvidenceEvent(event))
	params := []gin.Param{{Key: "id", Value: strconv.FormatInt(index.Id, 10)}, {Key: "event_id", Value: strconv.FormatInt(event.Id, 10)}}
	for _, formatted := range []bool{false, true} {
		path := "/api/task_request_evidence"
		if formatted {
			path += "?format=json"
		}
		response := callEvidenceHandler(t, GetTaskRequestEvidenceContent, common.RoleAdminUser, path, params...)
		require.Equal(t, 200, response.Code)
		var result struct {
			Data struct {
				Text      string  `json:"text"`
				Formatted *string `json:"formatted_text"`
				Status    string  `json:"json_status"`
			}
		}
		require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
		assert.Equal(t, string(body), result.Data.Text)
		if formatted {
			require.NotNil(t, result.Data.Formatted)
			assert.Contains(t, *result.Data.Formatted, "9007199254740993")
			assert.Contains(t, *result.Data.Formatted, "1.00")
			assert.Equal(t, "formatted", result.Data.Status)
		} else {
			assert.Nil(t, result.Data.Formatted)
			assert.Empty(t, result.Data.Status)
		}
	}
	stored, err := service.GetTaskRequestEvidenceStore().Get("format/body")
	require.NoError(t, err)
	assert.Equal(t, body, stored)
}

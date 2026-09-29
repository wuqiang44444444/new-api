package controller

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestEvidenceListQueryFailureIsNotAnEmptySuccess(t *testing.T) {
	for _, stage := range []string{"count", "list"} {
		t.Run(stage, func(t *testing.T) {
			setupEvidenceViewFixture(t)
			db := model.DB
			injected := false
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:evidence-query-failure", func(tx *gorm.DB) {
				_, count := tx.Statement.Dest.(*int64)
				if tx.Statement.Table == "task_request_evidences" && count == (stage == "count") {
					injected = true
					tx.AddError(errors.New("fixture database unavailable"))
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Query().Remove("test:evidence-query-failure") })
			response := callEvidenceHandler(t, GetTaskRequestEvidenceList, common.RoleAdminUser, "/api/task_request_evidence?request_id=req-1")
			require.True(t, injected)
			assert.Contains(t, response.Body.String(), `"success":false`)
			assert.NotContains(t, response.Body.String(), `"items":[]`)
			assert.NotContains(t, response.Body.String(), "fixture database unavailable")

			require.NoError(t, db.Callback().Query().Remove("test:evidence-query-failure"))
			response = callEvidenceHandler(t, GetTaskRequestEvidenceList, common.RoleAdminUser, "/api/task_request_evidence?request_id=req-1")
			assert.Contains(t, response.Body.String(), `"success":true`)
			assert.Contains(t, response.Body.String(), `"total":1`)
			response = callEvidenceHandler(t, GetTaskRequestEvidenceList, common.RoleAdminUser, "/api/task_request_evidence?request_id=not-recorded")
			assert.Contains(t, response.Body.String(), `"success":true`)
			assert.Contains(t, response.Body.String(), `"items":[]`)
		})
	}
}

package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistedImageTask202RequiresInMemoryBillingTransferMarker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		transferred bool
		task        *dto.ImageTask
		wantHandled bool
	}{
		{
			name:        "durable task",
			transferred: true,
			task:        &dto.ImageTask{ID: "task_public", Object: dto.ImageTaskObject, Status: dto.ImageTaskStatusQueued},
			wantHandled: true,
		},
		{
			name:        "upstream 202 cannot spoof local task",
			transferred: false,
			task:        &dto.ImageTask{ID: "task_public"},
			wantHandled: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			handled := TryHandlePersistedImageTaskResponse(c, &relaycommon.RelayInfo{
				BillingTransferredToTask: test.transferred,
				PersistedImageTask:       test.task,
			}, &http.Response{StatusCode: http.StatusAccepted, Body: http.NoBody})
			assert.Equal(t, test.wantHandled, handled)
			if !test.wantHandled {
				return
			}
			assert.Equal(t, http.StatusAccepted, recorder.Code)
			assert.Equal(t, "/v1/images/tasks/task_public", recorder.Header().Get("Location"))
			assert.Equal(t, "task_public", recorder.Header().Get("X-Task-ID"))
			require.Contains(t, recorder.Body.String(), `"object":"image.generation.task"`)
		})
	}
}

func TestSynchronousImageResponseReleasesIdempotencyClaim(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	markSynchronousImageIdempotencyComplete(c, &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{
			ClientProtocol: model.TaskClientProtocolOpenAIImages,
		},
	})

	assert.True(t, common.GetContextKeyBool(c, constant.ContextKeyTaskIdempotencyCompletedNoReplay))
}

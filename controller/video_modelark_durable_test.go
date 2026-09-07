package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type modelArkDurablePollingAdaptor struct{ service.TaskPollingAdaptor }

func (*modelArkDurablePollingAdaptor) Init(*relaycommon.RelayInfo) {}
func (*modelArkDurablePollingAdaptor) FetchTask(string, string, *model.Task, string) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"status":"succeeded","usage":{"completion_tokens":200,"total_tokens":200}}`))}, nil
}
func (*modelArkDurablePollingAdaptor) ParseTaskResult(*model.Task, *http.Response, []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: model.TaskStatusSuccess, CompletionTokens: 200, CompletionTokensReported: true, UsageReported: true, UsageSource: "usage.completion_tokens"}, nil
}

func TestModelArkGetKeepsDurableProjectionWhenWriteAndReloadFail(t *testing.T) {
	for _, scenario := range []struct {
		name    string
		status  model.TaskStatus
		missing bool
	}{
		{"running-db-outage", model.TaskStatusInProgress, false},
		{"success-db-outage", model.TaskStatusSuccess, false},
		{"no-longer-visible", model.TaskStatusSuccess, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			status := scenario.status
			events := []string{}
			db := setupTaskSubmissionDatabase(t, true, &events)
			task := &model.Task{TaskID: "task-durable", UserId: 1, AppID: 2, ClientProtocol: model.TaskClientProtocolModelArkV3,
				Platform: constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)), Status: status, Quota: 700,
				Data: []byte(`{"usage":{"completion_tokens":100,"total_tokens":110}}`),
				PrivateData: model.TaskPrivateData{Key: "fixture-key", VideoUpstreamQueryBaseURL: "https://fixture.example",
					VideoUpstreamProfile:     dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3,
					VideoUpstreamProtocol:    dto.VideoUpstreamProtocolFunCloudModelArkV3,
					SouthboundAdapterVersion: relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3),
					AsyncBilling:             &model.TaskAsyncBillingContext{State: model.TaskBillingStatePending, ActualTokens: 100, ActualUsageReported: true}}}
			require.NoError(t, db.Create(task).Error)
			oldFactory := service.GetTaskAdaptorFunc
			service.GetTaskAdaptorFunc = func(constant.TaskPlatform) service.TaskPollingAdaptor { return &modelArkDurablePollingAdaptor{} }
			t.Cleanup(func() { service.GetTaskAdaptorFunc = oldFactory })
			writeFailed := false
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:write-outage", func(tx *gorm.DB) {
				if tx.Statement.Table == "tasks" {
					writeFailed = true
					tx.AddError(assert.AnError)
				}
			}))
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:read-outage", func(tx *gorm.DB) {
				if writeFailed && tx.Statement.Table == "tasks" {
					if scenario.missing {
						tx.AddError(gorm.ErrRecordNotFound)
						return
					}
					tx.AddError(assert.AnError)
				}
			}))
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/task-durable", nil)
			c.Set("id", 1)
			c.Set("token_id", 2)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			ModelArkVideoGet(c)
			require.True(t, writeFailed)
			if scenario.missing {
				assert.Equal(t, http.StatusNotFound, recorder.Code)
				assert.NotContains(t, recorder.Body.String(), "completion_tokens")
				return
			}
			require.Equal(t, http.StatusOK, recorder.Code)
			var got dto.ModelArkVideoTask
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &got))
			assert.Equal(t, status.ToModelArkVideoStatus(), got.Status)
			require.NotNil(t, got.Usage)
			assert.Equal(t, 100, got.Usage.CompletionTokens)
			assert.Equal(t, 110, got.Usage.TotalTokens)
		})
	}
}

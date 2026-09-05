package service

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contractViolationPollingAdaptor struct{}

type contractRecoveryPollingAdaptor struct {
	status model.TaskStatus
}

func (a *contractViolationPollingAdaptor) Init(_ *relaycommon.RelayInfo) {}

func (a *contractViolationPollingAdaptor) FetchTask(string, string, *model.Task, string) (*http.Response, error) {
	return nil, &relaycommon.UpstreamContractViolation{Reason: "unsupported task status"}
}

func (a *contractViolationPollingAdaptor) ParseTaskResult(*model.Task, *http.Response, []byte) (*relaycommon.TaskInfo, error) {
	return nil, nil
}

func (a *contractViolationPollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func (a *contractRecoveryPollingAdaptor) Init(_ *relaycommon.RelayInfo) {}

func (a *contractRecoveryPollingAdaptor) FetchTask(string, string, *model.Task, string) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
}

func (a *contractRecoveryPollingAdaptor) ParseTaskResult(*model.Task, *http.Response, []byte) (*relaycommon.TaskInfo, error) {
	return &relaycommon.TaskInfo{Status: string(a.status)}, nil
}

func (a *contractRecoveryPollingAdaptor) AdjustBillingOnComplete(*model.Task, *relaycommon.TaskInfo) int {
	return 0
}

func TestTaskContractViolationReconcilesWithoutPrematureFailure(t *testing.T) {
	for _, recoveredStatus := range []model.TaskStatus{
		model.TaskStatusQueued,
		model.TaskStatusInProgress,
		model.TaskStatusSuccess,
	} {
		t.Run(string(recoveredStatus), func(t *testing.T) {
			truncate(t)
			now := time.Now().Unix()
			channel := &model.Channel{
				Id:      991,
				Type:    constant.ChannelTypeKling,
				Name:    "contract-reconciliation",
				Key:     "provider-key",
				BaseURL: common.GetPointer("https://video.example.com"),
				Status:  common.ChannelStatusEnabled,
			}
			require.NoError(t, model.DB.Create(channel).Error)
			task := &model.Task{
				TaskID:    "task-contract-reconciliation-" + string(recoveredStatus),
				Platform:  constant.TaskPlatform("kling"),
				UserId:    991,
				ChannelId: channel.Id,
				Quota:     100,
				Status:    model.TaskStatusInProgress,
				Progress:  "50%",
				CreatedAt: now,
				UpdatedAt: now,
				PrivateData: model.TaskPrivateData{
					UpstreamTaskID: "provider-task-991",
				},
			}
			require.NoError(t, model.DB.Create(task).Error)
			tasks := map[string]*model.Task{task.PrivateData.UpstreamTaskID: task}

			require.NoError(t, updateVideoSingleTask(
				context.Background(),
				&contractViolationPollingAdaptor{},
				channel,
				task.PrivateData.UpstreamTaskID,
				tasks,
			))
			require.NoError(t, model.DB.First(task, task.ID).Error)
			assert.Equal(t, model.TaskStatusReconciliationRequired, task.Status)
			assert.Equal(t, "upstream_contract_violation: unsupported task status", task.FailReason)
			assert.NotZero(t, task.Quota)

			require.NoError(t, updateVideoSingleTask(
				context.Background(),
				&contractRecoveryPollingAdaptor{status: recoveredStatus},
				channel,
				task.PrivateData.UpstreamTaskID,
				tasks,
			))
			require.NoError(t, model.DB.First(task, task.ID).Error)
			assert.Equal(t, recoveredStatus, task.Status)
			assert.Empty(t, task.FailReason)
		})
	}
}

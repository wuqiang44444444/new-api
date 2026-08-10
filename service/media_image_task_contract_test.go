package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	mediaimageprotocol "github.com/QuantumNous/new-api/relay/mediaimage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaImageContractViolationCanRecoverBeforeDeadline(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:         "task_image_contract_recovery",
		CreatedAt:      now,
		UpdatedAt:      now,
		SubmitTime:     now,
		UserId:         804,
		ChannelId:      94,
		Status:         model.TaskStatusQueued,
		Progress:       "0%",
		Platform:       constant.TaskPlatformMediaImage,
		ClientProtocol: model.TaskClientProtocolOpenAIImages,
		PrivateData: model.TaskPrivateData{
			Key:            "provider-secret",
			UpstreamTaskID: "provider-task-contract",
			MediaImage: &model.TaskMediaImagePrivateData{
				Protocol:            mediaimageprotocol.ProtocolMediaImageTaskV1,
				QueryBaseURL:        "https://provider.example",
				QueryPathTemplate:   testMediaImageQueryPath,
				RequestedImageCount: 1,
			},
		},
	}
	require.NoError(t, task.Insert())

	oldClient := httpClient
	calls := 0
	httpClient = &http.Client{Transport: mediaImageRoundTripper(func(request *http.Request) (*http.Response, error) {
		calls++
		body := `{"id":"provider-task-contract","status":"unexpected"}`
		if calls == 2 {
			body = `{"id":"provider-task-contract","status":"running"}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    request,
		}, nil
	})}
	t.Cleanup(func() { httpClient = oldClient })

	reconciling, err := PollMediaImageTaskOnce(context.Background(), task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatus(model.TaskStatusReconciliationRequired), reconciling.Status)

	recovered, err := PollMediaImageTaskOnce(context.Background(), task.TaskID)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatus(model.TaskStatusInProgress), recovered.Status)
	assert.Equal(t, 2, calls)
}

func TestMediaImageTaskMissingProtocolFailsWithOriginalContractError(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:         "task_image_missing_protocol",
		CreatedAt:      now,
		UpdatedAt:      now,
		SubmitTime:     now,
		Status:         model.TaskStatusQueued,
		Platform:       constant.TaskPlatformMediaImage,
		ClientProtocol: model.TaskClientProtocolOpenAIImages,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "provider-task-missing-protocol",
			MediaImage: &model.TaskMediaImagePrivateData{
				QueryBaseURL:        "https://provider.example",
				QueryPathTemplate:   testMediaImageQueryPath,
				RequestedImageCount: 1,
			},
		},
	}
	require.NoError(t, task.Insert())

	failed, err := PollMediaImageTaskOnce(context.Background(), task.TaskID)

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), failed.Status)
	assert.Equal(t, "media image task protocol is required", failed.FailReason)
}

func TestMediaImageTaskMissingQueryPathFailsWithOriginalContractError(t *testing.T) {
	truncate(t)
	now := time.Now().Unix()
	task := &model.Task{
		TaskID:         "task_image_missing_query_path",
		CreatedAt:      now,
		UpdatedAt:      now,
		SubmitTime:     now,
		Status:         model.TaskStatusQueued,
		Platform:       constant.TaskPlatformMediaImage,
		ClientProtocol: model.TaskClientProtocolOpenAIImages,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "provider-task-missing-query-path",
			MediaImage: &model.TaskMediaImagePrivateData{
				Protocol:            mediaimageprotocol.ProtocolMediaImageTaskV1,
				QueryBaseURL:        "https://provider.example",
				RequestedImageCount: 1,
			},
		},
	}
	require.NoError(t, task.Insert())

	failed, err := PollMediaImageTaskOnce(context.Background(), task.TaskID)

	require.NoError(t, err)
	assert.Equal(t, model.TaskStatus(model.TaskStatusFailure), failed.Status)
	assert.Equal(t, "media image task query path is required", failed.FailReason)
}

package feicai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateResponseRequiresTrustedTopLevelID(t *testing.T) {
	body, err := CreateResponse([]byte(`{"id":" provider-task-1 "}`))
	require.NoError(t, err)
	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	assert.Equal(t, "provider-task-1", response["id"])

	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `{`},
		{name: "missing id", body: `{}`},
		{name: "control character", body: `{"id":"provider-task-1\nprivate"}`},
		{name: "too long", body: `{"id":"` + strings.Repeat("a", 192) + `"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CreateResponse([]byte(test.body))
			require.Error(t, err)
		})
	}
}

func TestTaskResponseMapsVerifiedStatuses(t *testing.T) {
	tests := []struct {
		providerStatus string
		expectedStatus string
	}{
		{providerStatus: "queued", expectedStatus: "queued"},
		{providerStatus: "processing", expectedStatus: "running"},
		{providerStatus: "in_progress", expectedStatus: "running"},
	}
	for _, test := range tests {
		t.Run(test.providerStatus, func(t *testing.T) {
			body, err := TaskResponse(
				[]byte(`{"id":"provider-task-1","status":"`+test.providerStatus+`"}`),
				"provider-task-1",
				TaskResponseContext{BaseURL: "https://video.example.com"},
			)
			require.NoError(t, err)
			var response map[string]any
			require.NoError(t, common.Unmarshal(body, &response))
			assert.Equal(t, test.expectedStatus, response["status"])
		})
	}

	completed, err := TaskResponse(
		[]byte(`{"id":"provider-task-1","status":"completed","video_url":"https://video.example.com/results/1.mp4"}`),
		"provider-task-1",
		TaskResponseContext{BaseURL: "https://video.example.com"},
	)
	require.NoError(t, err)
	var completedResponse struct {
		Status  string `json:"status"`
		Content struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
	}
	require.NoError(t, common.Unmarshal(completed, &completedResponse))
	assert.Equal(t, "succeeded", completedResponse.Status)
	assert.Equal(t, "https://video.example.com/results/1.mp4", completedResponse.Content.VideoURL)

	failed, err := TaskResponse(
		[]byte(`{"id":"provider-task-1","status":"failed","error":{"code":"bad_request","message":"https://secret.example/private"}}`),
		"provider-task-1",
		TaskResponseContext{BaseURL: "https://video.example.com"},
	)
	require.NoError(t, err)
	var failedResponse struct {
		Status string `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(failed, &failedResponse))
	assert.Equal(t, "failed", failedResponse.Status)
	assert.Equal(t, "bad_request", failedResponse.Error.Code)
	assert.Equal(t, "upstream task failed", failedResponse.Error.Message)
}

func TestTaskResponseRejectsContractViolations(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid JSON", body: `{`},
		{name: "id mismatch", body: `{"id":"other-task","status":"queued"}`},
		{name: "unknown status", body: `{"id":"provider-task-1","status":"unknown"}`},
		{name: "completed without URL", body: `{"id":"provider-task-1","status":"completed"}`},
		{name: "cross-origin URL", body: `{"id":"provider-task-1","status":"completed","video_url":"https://other.example.com/results/1.mp4"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := TaskResponse(
				[]byte(test.body),
				"provider-task-1",
				TaskResponseContext{BaseURL: "https://video.example.com"},
			)
			require.Error(t, err)
		})
	}
}

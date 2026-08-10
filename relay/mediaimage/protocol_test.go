package mediaimage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testQueryPath = "/v1/media/tasks/{task_id}"

func TestInspectCreateResponseClassifiesTaskOutcomes(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		disposition CreateDisposition
		taskID      string
		failure     string
	}{
		{name: "OpenAI passthrough", statusCode: http.StatusOK, body: `{"data":[{"url":"https://cdn.example/direct.png"}]}`, disposition: CreatePassthrough},
		{name: "HTTP 200 accepted", statusCode: http.StatusOK, body: `{"task_id":"task-200","status":"queued"}`, disposition: CreateAccepted, taskID: "task-200"},
		{name: "HTTP 202 accepted", statusCode: http.StatusAccepted, body: `{"data":{"task_id":"task-202"}}`, disposition: CreateAccepted, taskID: "task-202"},
		{name: "completed", statusCode: http.StatusOK, body: `{"status":"completed","result":{"primary_url":"https://cdn.example/completed.png"}}`, disposition: CreateCompleted},
		{name: "rejected", statusCode: http.StatusOK, body: `{"status":"failed","error_message":"prompt rejected"}`, disposition: CreateRejected, failure: "prompt rejected"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := &http.Response{
				StatusCode: test.statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(test.body)),
			}

			observation, err := InspectCreateResponse(ProtocolMediaImageTaskV1, response)

			require.NoError(t, err)
			assert.Equal(t, test.disposition, observation.Disposition)
			assert.Equal(t, test.taskID, observation.TaskID)
			assert.Equal(t, test.failure, observation.Failure)
			body, readErr := io.ReadAll(response.Body)
			require.NoError(t, readErr)
			assert.JSONEq(t, test.body, string(body))
		})
	}
}

func TestInspectCreateResponseRejectsUnsafeTaskIDs(t *testing.T) {
	unsafe := []string{"../../admin", "task/child", "task%2Fchild", "task id", "任务-1"}
	for _, taskID := range unsafe {
		t.Run(taskID, func(t *testing.T) {
			response := &http.Response{
				StatusCode: http.StatusAccepted,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"task_id":%q}`, taskID))),
			}

			_, err := InspectCreateResponse(ProtocolMediaImageTaskV1, response)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsafe characters")
		})
	}

	response := &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"task_id":"task_01:part-2.~"}`)),
	}
	observation, err := InspectCreateResponse(ProtocolMediaImageTaskV1, response)
	require.NoError(t, err)
	assert.Equal(t, "task_01:part-2.~", observation.TaskID)

	response = &http.Response{
		StatusCode: http.StatusAccepted,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"task_id":%q}`, strings.Repeat("a", 192)))),
	}
	_, err = InspectCreateResponse(ProtocolMediaImageTaskV1, response)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too long")
}

func TestWaitUsesFrozenQueryContractAndNormalizedStates(t *testing.T) {
	var attempts atomic.Int32
	result, err := Wait(context.Background(), func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, "/gateway/v1/media/tasks/task-1", request.URL.Path)
		assert.Equal(t, "secret", request.URL.Query().Get("token"))
		assert.Equal(t, "override", request.Header.Get("X-Route"))
		attempt := attempts.Add(1)
		body := `{"task_id":"task-1","status":"processing","request_id":"poll-1"}`
		if attempt == 2 {
			body = `{"task_id":"task-1","status":"success","request_id":"poll-2","result":{"urls":["https://cdn.example/one.png"]}}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	}, QuerySpec{
		Protocol:          ProtocolMediaImageTaskV1,
		BaseURL:           "https://provider.example/gateway",
		PathTemplate:      testQueryPath,
		TaskID:            "task-1",
		APIKey:            "secret",
		AuthType:          "query",
		AuthName:          "token",
		AuthValueTemplate: "{api_key}",
		Headers:           http.Header{"X-Route": []string{"override"}},
	}, WaitOptions{SkipSleep: true})

	require.NoError(t, err)
	assert.Equal(t, 2, result.Attempts)
	assert.Equal(t, StateCompleted, result.Observation.State)
	assert.Equal(t, "poll-2", result.Observation.RequestID)
	assert.Equal(t, []string{"https://cdn.example/one.png"}, result.Observation.Result.URLs)
}

func TestWaitStopsBeforeQueryWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var called atomic.Bool

	_, err := Wait(ctx, func(*http.Request) (*http.Response, error) {
		called.Store(true)
		return nil, nil
	}, QuerySpec{Protocol: ProtocolMediaImageTaskV1, BaseURL: "https://provider.example", PathTemplate: testQueryPath, TaskID: "task-1"}, WaitOptions{SkipSleep: true})

	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, called.Load())
}

func TestProtocolAndQueryPathMustBeExplicit(t *testing.T) {
	_, err := ValidateProtocol("")
	require.EqualError(t, err, "media image task protocol is required")

	_, err = BuildQueryURL(QuerySpec{
		Protocol: ProtocolMediaImageTaskV1,
		BaseURL:  "https://provider.example",
		TaskID:   "task-1",
	})
	require.EqualError(t, err, "media image task query path is required")
}

func TestNormalizeResultURLsDeduplicatesAndRejectsInvalidValues(t *testing.T) {
	urls, err := NormalizeResultURLs(Result{
		PrimaryURL: "https://cdn.example/one.png",
		URLs:       []string{"https://cdn.example/one.png", "https://cdn.example/two.png"},
	}, 4)
	require.NoError(t, err)
	assert.Equal(t, []string{"https://cdn.example/one.png", "https://cdn.example/two.png"}, urls)

	_, err = NormalizeResultURLs(Result{PrimaryURL: "file:///private/result.png"}, 4)
	require.Error(t, err)
}

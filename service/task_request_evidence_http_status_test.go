package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceResponseStatusWithoutBody(t *testing.T) {
	for _, status := range []int{200, 204, 400, 429, 500, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			setupEvidenceTestEnv(t)
			c := newEvidenceTestContext(t, []byte(`{}`))
			require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
			req, err := http.NewRequest(http.MethodPost, "https://provider.example/create", nil)
			require.NoError(t, err)
			response := &http.Response{StatusCode: status, Request: req, Header: http.Header{"X-Request-Id": {"create-id"}}}
			AttachTaskRequestEvidenceUpstreamResponse(c, response)
			events := evidenceEventRows(t, evidenceSessionFrom(c).evidenceID)
			require.Len(t, events, 2)
			assert.Equal(t, status, events[1].StatusCode)
			assert.Equal(t, http.MethodPost, events[1].Method)
			assert.Equal(t, "https://provider.example/create", events[1].Target)
			assert.Empty(t, events[1].Phase)
		})
	}
}

func TestEvidencePollingPreservesObservedResponseOnTransportError(t *testing.T) {
	for _, withBody := range []bool{false, true} {
		t.Run(map[bool]string{false: "empty", true: "body"}[withBody], func(t *testing.T) {
			setupEvidenceTestEnv(t)
			task := &model.Task{TaskID: "task-poll", UserId: 7, ClientProtocol: "openai_video"}
			require.NoError(t, model.DB.Create(task).Error)
			req, err := http.NewRequest(http.MethodGet, "https://provider.example/tasks/1", nil)
			require.NoError(t, err)
			response := &http.Response{StatusCode: 502, Header: http.Header{"Content-Type": {"text/plain"}}}
			if withBody {
				response.Body = io.NopCloser(bytes.NewBufferString("gateway failed"))
			}
			AttachRawTaskPollingEvidence(task, req, response, errors.New("transport failed"))
			if response.Body != nil {
				_, err = io.ReadAll(response.Body)
				require.NoError(t, err)
				require.NoError(t, response.Body.Close())
			}
			evidence, exists, err := model.FindTaskRequestEvidenceByTaskID(task.TaskID)
			require.NoError(t, err)
			require.True(t, exists)
			events := evidenceEventRows(t, evidence.Id)
			require.Len(t, events, 1)
			assert.Equal(t, 502, events[0].StatusCode)
			content, status := ReadEvidenceEventContent(events[0], false)
			require.Equal(t, "available", status)
			assert.Equal(t, "transport failed", content.Meta.Error)
			if withBody {
				body := evidenceCapturedBody(t, events[0])
				assert.Equal(t, "gateway failed", string(body))
			}
		})
	}
}

func TestEvidenceMalformedResponseKeepsStatusAndCaptureFailure(t *testing.T) {
	setupEvidenceTestEnv(t)
	c := newEvidenceTestContext(t, []byte(`{}`))
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
	response := &http.Response{StatusCode: 400, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewBufferString(`{"token":"private`))}
	AttachTaskRequestEvidenceUpstreamResponse(c, response)
	_, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	events := evidenceEventRows(t, evidenceSessionFrom(c).evidenceID)
	require.Len(t, events, 2)
	assert.Equal(t, 400, events[1].StatusCode)
	assert.Equal(t, `{"token":"private`, string(evidenceCapturedBody(t, events[1])))
	assert.Empty(t, events[1].Phase)
}

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
			assert.True(t, events[1].Complete)
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
			assert.False(t, events[0].Complete)
			assert.Contains(t, events[0].Detail, `"transport_failed":true`)
			if withBody {
				body, err := GetTaskRequestEvidenceStore().Get(events[0].ObjectKey)
				require.NoError(t, err)
				assert.Equal(t, "gateway failed", string(body))
			}
		})
	}
}

func TestEvidenceJSONPreservesExactNumbers(t *testing.T) {
	body := []byte(`{"id":9007199254740993,"duration":1.00,"nested":{"api_key":"private","n":18446744073709551615},"values":[0,false,null]}`)
	redacted, err := evidenceRedactBody(body, "application/json")
	require.NoError(t, err)
	assert.Contains(t, string(redacted), `9007199254740993`)
	assert.Contains(t, string(redacted), `18446744073709551615`)
	assert.Contains(t, string(redacted), `1.00`)
	assert.NotContains(t, string(redacted), "private")
	assert.Contains(t, string(redacted), `[0,false,null]`)
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
	assert.Equal(t, int64(len(`{"token":"private`)), events[1].ByteCount)
	assert.Equal(t, model.TaskRequestEvidencePhaseUnavailable, events[1].Phase)
	assert.Empty(t, events[1].ObjectKey)
	assert.Contains(t, events[1].Detail, `"capture_failure"`)
	assert.Contains(t, events[1].Detail, `"category":"structured_parse"`)
	assert.NotContains(t, events[1].Detail, "private")
}

func TestEvidenceQueryKeepsBusinessValuesWithoutSigningCredentials(t *testing.T) {
	query := map[string][]string{
		"model": {"customer-model"}, "sig": {"private-signature"},
		"X-Amz-Credential": {"private-account"}, "X-Amz-Signature": {"private-signature"},
		"key": {"private-key"},
	}
	redacted := EvidenceRedactQueryParams(query)
	assert.Equal(t, "customer-model", redacted["model"])
	for _, key := range []string{"sig", "X-Amz-Credential", "X-Amz-Signature", "key"} {
		assert.Equal(t, "[REDACTED]", redacted[key])
	}
}

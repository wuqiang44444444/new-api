package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/azurebatch"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchFailureDeliveryDoesNotRequireAChatCompletionBody(t *testing.T) {
	for _, body := range []string{`"upstream diagnostic"`, `null`, `{"error":{"message":"private diagnostic"}}`} {
		input := `{"custom_id":"failed","response":{"status_code":500,"body":` + body + `}}`
		lines, err := azurebatch.ParseResultLines(strings.NewReader(input))
		require.NoError(t, err)
		require.Len(t, lines, 1)
		assert.Equal(t, "failed", lines[0].Status)
		var output bytes.Buffer
		require.NoError(t, writeBatchPublicResult(&output, strings.NewReader(input), "customer-model"))
		assert.JSONEq(t, `{"custom_id":"failed","response":{"status_code":500,"body":{"error":{"code":"request_failed","message":"The batch request failed"}}}}`, output.String())
	}
}

func TestBatchResultDeliveryUsesFrozenPublicMetadata(t *testing.T) {
	c, request, _ := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	request.Metadata = map[string]string{"note": "客户元数据"}
	transport := http.DefaultTransport
	http.DefaultTransport = batchRoundTrip(func(r *http.Request) (*http.Response, error) {
		body := ""
		switch {
		case strings.Contains(r.URL.Path, "out-1/content"):
			body = `{"id":"row-a","custom_id":"a","response":{"status_code":200,"request_id":"request-a","body":{"model":"private-deployment","choices":[{"message":{"role":"assistant","content":"Azure is a cloud service"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}},"error":null}`
		case strings.Contains(r.URL.Path, "err-1/content"):
			body = `{"custom_id":"b","response":{"status_code":400,"body":{"error":{"code":"private-code","message":"private-deployment rejected the request","param":"internal-param"}}},"error":{"code":"private-code","message":"private-deployment rejected the request"}}`
		default:
			return transport.RoundTrip(r)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body + "\n"))}, nil
	})
	result, err := CreateBatchJob(c, request)
	require.NoError(t, err)
	assert.JSONEq(t, `{"note":"客户元数据"}`, result.Job.Metadata)
	// Delivery uses the accepted job, even after its current channel is gone.
	require.NoError(t, model.DB.Delete(&model.Channel{}, 1701).Error)
	require.NoError(t, progressBatchJob(context.Background(), result.Job))
	job, err := model.GetDueBatchJobById(result.Job.Id)
	require.NoError(t, err)
	for _, tc := range []struct{ id, expected string }{
		{job.OutputFileId, `{"id":"row-a","custom_id":"a","response":{"status_code":200,"request_id":"request-a","body":{"model":"batch-text","choices":[{"message":{"role":"assistant","content":"Azure is a cloud service"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}},"error":null}`},
		{job.ErrorFileId, `{"custom_id":"b","response":{"status_code":400,"body":{"error":{"code":"request_failed","message":"The batch request failed"}}},"error":{"code":"request_failed","message":"The batch request failed"}}`},
	} {
		var output bytes.Buffer
		written, file, err := BatchFileContent(context.Background(), tc.id, job.UserId, job.AppID, &output)
		require.NoError(t, err)
		assert.JSONEq(t, tc.expected, output.String())
		assert.Equal(t, file.SizeBytes, written)
		assert.EqualValues(t, output.Len(), written)
	}
	task, err := model.GetTaskById(job.TaskRowId)
	require.NoError(t, err)
	assert.Equal(t, 20, task.Quota, "public metadata projection must not alter usage settlement")
}

type failingBatchReadStore struct {
	batchMemoryStore
	headErr, readErr error
}

func (s *failingBatchReadStore) HeadObject(context.Context, string) (bool, error) {
	return true, s.headErr
}

func (s *failingBatchReadStore) BatchGetObject(context.Context, string, io.Writer) (int64, error) {
	return 0, s.readErr
}

func TestBatchInputReadPreservesStorageFailure(t *testing.T) {
	original := GetTaskArtifactStore()
	t.Cleanup(func() { taskArtifactStoreRuntime.swap(original, "") })
	for _, tc := range []struct {
		name             string
		headErr, readErr error
	}{
		{"unconfigured", ErrBatchObjectStoreUnavailable, nil},
		{"removed-after-head", nil, ErrBatchObjectNotFound},
		{"read-failed", nil, errors.New("storage read interrupted")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			taskArtifactStoreRuntime.swap(&failingBatchReadStore{headErr: tc.headErr, readErr: tc.readErr}, "")
			_, err := readBatchObjectForCreate(context.Background(), &model.BatchFile{ObjectKey: "test-object"})
			expected := tc.headErr
			if expected == nil {
				expected = tc.readErr
			}
			require.ErrorIs(t, err, expected)
		})
	}
}

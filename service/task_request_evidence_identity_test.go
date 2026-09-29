package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceUsesAcceptedResponseRequestIDIncludingEmptyRetry(t *testing.T) {
	setupEvidenceTestEnv(t)
	c := newEvidenceTestContext(t, []byte(`{}`))
	require.NoError(t, BeginTaskRequestEvidence(c, model.TaskRequestEvidenceKindVideoTask))
	for _, requestID := range []string{"accepted-request", "final-request", ""} {
		c.Set(common.UpstreamRequestIdKey, requestID)
		resp := &http.Response{StatusCode: 200, Header: http.Header{"X-Request-Id": []string{"not-the-accepted-id"}, "Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{}`))}
		AttachTaskRequestEvidenceUpstreamResponse(c, resp)
		_, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		index, exists, err := model.GetTaskRequestEvidenceById(evidenceSessionFrom(c).evidenceID)
		require.NoError(t, err)
		require.True(t, exists)
		assert.Equal(t, requestID, index.UpstreamRequestID)
	}
}

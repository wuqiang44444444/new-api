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

func TestEvidenceAcceptedResponseIdentityClearsOnFinalEmptyResponse(t *testing.T) {
	setupEvidenceTestEnv(t)
	c := newEvidenceTestContext(t, nil)
	require.NoError(t, BeginTaskRequestEvidence(c, "video_task"))
	for _, id := range []string{"accepted", "final", ""} {
		c.Set(common.UpstreamRequestIdKey, id)
		response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader("{}"))}
		AttachTaskRequestEvidenceUpstreamResponse(c, response)
		_, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		drainImageEvidence(t)
		indexes := evidenceIndexes(t, model.TaskRequestEvidenceQueryParams{})
		require.Len(t, indexes, 1)
		assert.Equal(t, id, indexes[0].UpstreamRequestID)
	}
	FinishTaskRequestEvidenceClientDelivery(c)
}

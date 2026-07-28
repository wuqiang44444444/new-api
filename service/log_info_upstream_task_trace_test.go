package service

import (
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppendUpstreamTaskTraceAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	relaycommon.SetUpstreamTaskTrace(ctx, &relaycommon.UpstreamTaskTrace{
		TaskID:                  "task-1",
		CreateRequestID:         "create-1",
		LastPollRequestID:       "poll-2",
		PollAttempts:            2,
		PollElapsedMilliseconds: 1234,
	})
	adminInfo := map[string]interface{}{"existing": true}

	AppendUpstreamTaskTraceAdminInfo(ctx, adminInfo)

	assert.Equal(t, true, adminInfo["existing"])
	trace, ok := adminInfo["upstream_task"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "task-1", trace["task_id"])
	assert.Equal(t, "create-1", trace["create_request_id"])
	assert.Equal(t, "poll-2", trace["last_poll_request_id"])
	assert.Equal(t, 2, trace["poll_attempts"])
	assert.Equal(t, int64(1234), trace["poll_elapsed_ms"])
}

func TestAppendUpstreamTaskTraceAdminInfoIgnoresMissingTrace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	adminInfo := map[string]interface{}{}

	AppendUpstreamTaskTraceAdminInfo(ctx, adminInfo)

	assert.NotContains(t, adminInfo, "upstream_task")
}

package relay

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTaskCreationPreservesStructuredBusinessMessage(t *testing.T) {
	for _, message := range []string{"The request failed because the output video may be related to copyright restrictions.", "输入内容可能包含敏感信息，请检查后重试。"} {
		body, err := common.Marshal(map[string]any{"error": map[string]any{"code": "ContentPolicyViolation", "message": message}})
		require.NoError(t, err)
		parsed := parseTaskUpstreamHTTPError(403, body, nil)
		public := service.TaskErrorWrapper(parsed, "fail_to_fetch_task", 403)
		assert.Equal(t, message, public.Message)
		assert.Equal(t, "ContentPolicyViolation", public.Code)
		assert.NotContains(t, public.Message, "HTTP 403")
	}
}

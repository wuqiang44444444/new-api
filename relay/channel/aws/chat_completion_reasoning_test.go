package aws

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatCompletionReasoningNovaWriter(t *testing.T) {
	client := newAwsTestClient(awsHTTPClientFunc(func(*http.Request) (*http.Response, error) {
		body := `{"output":{"message":{"content":[{"text":"answer"}]}},"usage":{"inputTokens":2,"outputTokens":1,"totalTokens":3}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	}))
	engine := gin.New()
	engine.POST("/v1/chat/completions", func(c *gin.Context) {
		info := newAwsTestRelayInfo()
		info.IsStream = false
		info.UpstreamModelName = "amazon.nova-pro-v1:0"
		request := newAwsInvokeModelInput()
		request.ModelId = &info.UpstreamModelName
		adaptor := &Adaptor{IsNova: true, AwsClient: client, AwsReq: request}
		usage, apiErr := adaptor.DoResponse(c, nil, info)
		require.Nil(t, apiErr)
		require.IsType(t, &dto.Usage{}, usage)
		assert.Equal(t, 3, usage.(*dto.Usage).TotalTokens)
	})
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	body := recorder.Body.String()
	assert.Equal(t, "answer", gjson.Get(body, "choices.0.message.content").Str)
	assert.Equal(t, "null", gjson.Get(body, "choices.0.message.reasoning_content").Raw)
	assert.EqualValues(t, 3, gjson.Get(body, "usage.total_tokens").Int())
}

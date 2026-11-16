package xunfei

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type xunfeiSSERecorder struct {
	*httptest.ResponseRecorder
}

func (xunfeiSSERecorder) CloseNotify() <-chan bool { return nil }

func TestChatCompletionReasoningXunfeiWriters(t *testing.T) {
	for _, test := range []struct {
		name   string
		writer func(*gin.Context, <-chan XunfeiChatResponse, <-chan bool) (*dto.Usage, *types.NewAPIError)
		stream bool
	}{
		{name: "JSON", writer: writeXunfeiChatResponse},
		{name: "SSE", writer: writeXunfeiChatStream, stream: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var upstream XunfeiChatResponse
			require.NoError(t, common.Unmarshal([]byte(`{"payload":{"choices":{"status":2,"text":[{"role":"assistant","content":"answer"}]},"usage":{"text":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}}}`), &upstream))
			engine := gin.New()
			engine.POST("/v1/chat/completions", func(c *gin.Context) {
				data := make(chan XunfeiChatResponse)
				stopped := make(chan bool)
				go func() {
					data <- upstream
					stopped <- true
				}()
				usage, apiErr := test.writer(c, data, stopped)
				require.Nil(t, apiErr)
				assert.Equal(t, 3, usage.TotalTokens)
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(xunfeiSSERecorder{recorder}, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
			require.Equal(t, http.StatusOK, recorder.Code)

			payload, key := recorder.Body.String(), "message"
			if test.stream {
				key = "delta"
				event, rest, ok := strings.Cut(payload, "\n\n")
				require.True(t, ok)
				require.True(t, strings.HasPrefix(event, "data: "))
				assert.Equal(t, "data: [DONE]\n\n", rest)
				payload = strings.TrimPrefix(event, "data: ")
			}
			assert.Equal(t, "answer", gjson.Get(payload, "choices.0."+key+".content").Str)
			assert.Equal(t, "null", gjson.Get(payload, "choices.0."+key+".reasoning_content").Raw)
			assert.Equal(t, "stop", gjson.Get(payload, "choices.0.finish_reason").Str)
		})
	}
}

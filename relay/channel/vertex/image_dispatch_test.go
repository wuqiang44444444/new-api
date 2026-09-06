package vertex

import (
	"net/http"
	"net/http/httptest"
	"testing"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

const imagineChatResponseBody = `{"candidates":[{"content":{"parts":[{"text":"an image: ![img](data:image/png;base64,AAA)"}]}}]}`

// 评审 S4：imagine 模型在聊天补全入口必须继续返回 Chat 形状，不得被
// Images 响应处理劫持；Images 入口才返回 data[]。
func TestVertexImagineModelKeepsChatShapeOutsideImagesRelayMode(t *testing.T) {
	adaptor := &Adaptor{}
	adaptor.RequestMode = RequestModeGemini

	buildInfo := func(relayMode int) *relaycommon.RelayInfo {
		info := &relaycommon.RelayInfo{RelayMode: relayMode, IsStream: false}
		info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: "gemini-3.1-flash-image"}
		return info
	}

	// Chat 补全：保持 choices 响应。
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(imagineChatResponseBody))
	}))
	defer server.Close()
	resp, err := http.Get(server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, apiErr := adaptor.DoResponse(c, resp, buildInfo(constant.RelayModeChatCompletions))
	// 修复前：该分支会把聊天请求劫持进 Images 处理并在“零最终图片”上 502。
	require.Nil(t, apiErr, "chat relay mode must keep the chat handler, not the Images handler")
	assert.Contains(t, recorder.Body.String(), "candidates", "chat relay mode keeps the chat response shape")
	assert.NotContains(t, recorder.Body.String(), `"b64_json"`, "chat relay mode must not be rewritten into an Images envelope")
}

package relay

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gin-gonic/gin"
)

// 评审 S3：异步入口必须先执行管理员模型映射——同一客户别名在同步与异步
// 两条路径下解析为同一 Provider 模型。
func TestMapImageAsyncRequestAppliesModelMapping(t *testing.T) {
	c := &gin.Context{}
	c.Request, _ = http.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	mapping := map[string]any{"nano-banana-2-gemini": "gemini-3.1-flash-image"}
	encoded, err := common.Marshal(mapping)
	require.NoError(t, err)
	c.Set("model_mapping", string(encoded))

	info := &relaycommon.RelayInfo{OriginModelName: "nano-banana-2-gemini"}
	imageReq := &dto.ImageRequest{Model: "nano-banana-2-gemini", Prompt: "p"}

	mapped, err := mapImageAsyncRequest(c, info, imageReq)
	require.NoError(t, err)
	assert.Equal(t, "gemini-3.1-flash-image", mapped.Model)
	assert.Equal(t, "gemini-3.1-flash-image", info.UpstreamModelName,
		"async acceptance must freeze the mapped provider model, not the customer alias")
	// 原请求对象不被改写（同步路径使用同一原始 DTO）。
	assert.Equal(t, "nano-banana-2-gemini", imageReq.Model)
}

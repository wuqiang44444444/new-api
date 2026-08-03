package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newProjectionTestContext(targetURL string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, targetURL, nil)
	return c
}

// 治本行为：视频 URL 前缀跟随入站请求主机名，与全局 ServerAddress 无关，
// 因此本地（127.0.0.1）与线上（公网域名 / 反代）各自自动正确，无需切换配置。
func TestProjectModelArkVideoTaskPrefixFollowsIncomingRequest(t *testing.T) {
	previous := system_setting.ServerAddress
	system_setting.ServerAddress = "https://tokenai.page"
	t.Cleanup(func() { system_setting.ServerAddress = previous })

	task := &model.Task{TaskID: "task-x", Status: model.TaskStatusSuccess}

	// 本地直连：host 直接来自请求自身
	local := projectModelArkVideoTask(newProjectionTestContext("http://127.0.0.1:8100/api/v3/x"), task)
	require.NotNil(t, local.Content)
	assert.Equal(t, "http://127.0.0.1:8100/v1/videos/task-x/content", local.Content.VideoURL)

	// 线上反代：host / scheme 优先取自 X-Forwarded-* 头
	prod := newProjectionTestContext("http://10.0.0.1:8100/api/v3/x")
	prod.Request.Header.Set("X-Forwarded-Host", "tokenai.page")
	prod.Request.Header.Set("X-Forwarded-Proto", "https")
	projected := projectModelArkVideoTask(prod, task)
	require.NotNil(t, projected.Content)
	assert.Equal(t, "https://tokenai.page/v1/videos/task-x/content", projected.Content.VideoURL)
}

// 兜底行为：无请求上下文（c == nil）时回退到 ServerAddress，保持历史行为不回退。
func TestProjectModelArkVideoTaskFallsBackToServerAddress(t *testing.T) {
	previous := system_setting.ServerAddress
	system_setting.ServerAddress = "https://platform.example/"
	t.Cleanup(func() { system_setting.ServerAddress = previous })

	task := &model.Task{
		TaskID: "task-modelark-content",
		Status: model.TaskStatusSuccess,
		Data:   []byte(`{"content":{"last_frame_url":"https://provider.example/private-frame"}}`),
	}

	projected := projectModelArkVideoTask(nil, task)

	require.NotNil(t, projected.Content)
	assert.Equal(t, "https://platform.example/v1/videos/task-modelark-content/content", projected.Content.VideoURL)
	assert.Equal(t, "https://platform.example/v1/videos/task-modelark-content/content?part=last_frame", projected.Content.LastFrameURL)
	assert.NotContains(t, projected.Content.LastFrameURL, "provider.example")
}

package relay

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/asyncimage"
	"github.com/QuantumNous/new-api/relay/channel/moxingimage"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrozenImageHeadersReachEveryTransport(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
	c.Request.Header.Set("X-Tenant", "original-{api_key}")
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ApiKey: "original-key", UpstreamModelName: "nano-banana-2"}}
	info.HeadersOverride = map[string]any{"Authorization": "Bearer {api_key}", "X-Tenant": "{client_header:X-Tenant}", "Host": "tenant.example"}
	task := &model.Task{TaskID: "frozen-headers", PrivateData: model.TaskPrivateData{ImageTask: &model.TaskImageExecutionData{}}}
	var err error
	task.PrivateData.ImageTask.HeadersCiphertext, err = freezeImageTaskHeaders(task.TaskID, c, info)
	require.NoError(t, err)
	serialized, err := common.Marshal(task.PrivateData.ImageTask)
	require.NoError(t, err)
	assert.NotContains(t, string(serialized), "original-key")
	assert.NotContains(t, string(serialized), "original-{api_key}")
	info.ApiKey = "changed-key"
	info.HeadersOverride = map[string]any{"Authorization": "replacement"}
	c.Request.Header.Set("X-Tenant", "changed")
	headers, err := restoreImageTaskHeaders(task)
	require.NoError(t, err)
	for _, transport := range []string{"google", "funcloud-create", "funcloud-poll", "moxing"} {
		t.Run(transport, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				assert.Equal(t, "Bearer original-key", r.Header.Get("Authorization"))
				assert.Equal(t, "original-{api_key}", r.Header.Get("X-Tenant"), "resolved values must not be interpreted a second time")
				assert.Equal(t, "tenant.example", r.Host)
				w.Header().Set("Content-Type", "application/json")
				if transport == "moxing" {
					_, _ = io.WriteString(w, `{"data":[{"url":"https://result.example/image"}]}`)
				} else {
					_, _ = io.WriteString(w, `{"code":0,"data":{"status":"success","result":["https://result.example/image"]}}`)
				}
			}))
			defer server.Close()
			info.ChannelBaseUrl = server.URL
			switch transport {
			case "google":
				info.ApiType = constant.APITypeGemini
				info.UpstreamModelName = "gemini-3.1-flash-image"
				_, apiErr := postFrozenImageRequest(context.Background(), c, info, []byte(`{}`), headers)
				require.Nil(t, apiErr)
			case "funcloud-create":
				_, _, apiErr := asyncimage.HeadlessCreateAndPoll(context.Background(), info, headers, bytes.NewBufferString(`{}`), nil)
				require.Nil(t, apiErr)
			case "funcloud-poll":
				_, apiErr := asyncimage.HeadlessPollOnly(context.Background(), info, headers, "existing-id")
				require.Nil(t, apiErr)
			case "moxing":
				_, apiErr := moxingimage.HeadlessGenerate(context.Background(), info, headers, bytes.NewBufferString(`{}`))
				require.Nil(t, apiErr)
			}
			assert.Equal(t, 1, calls)
		})
	}
	task.TaskID = "different-task"
	_, err = restoreImageTaskHeaders(task)
	require.Error(t, err, "snapshots are bound to the accepted task")
	task.PrivateData.ImageTask.HeadersCiphertext = ""
	_, err = restoreImageTaskHeaders(task)
	require.Error(t, err, "missing execution evidence cannot fall back to current channel headers")
}

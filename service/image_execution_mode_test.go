package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageExecutionModePreservesNativeRequestsAndTaskBilling(t *testing.T) {
	for _, tc := range []struct {
		name    string
		channel int
		prefer  string
		async   bool
	}{
		{"openai", constant.ChannelTypeOpenAI, "respond-async", true},
		{"azure", constant.ChannelTypeAzure, "respond-async", true},
		{"custom", constant.ChannelTypeCustom, "respond-async", false},
		{"advanced_custom", constant.ChannelTypeAdvancedCustom, "respond-async", false},
		{"gemini", constant.ChannelTypeGemini, "respond-async", true},
		{"vertex", constant.ChannelTypeVertexAi, "respond-async", true},
		{"image_relay", constant.ChannelTypeAsyncImage, "respond-async", true},
		{"default_sync", constant.ChannelTypeGemini, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"gpt-image-2"}`))
			c.Request.Header.Set("Prefer", tc.prefer)
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyChannelType, tc.channel)
			assert.Equal(t, tc.async, ImageAsyncExecutionRequested(c))
			// Retrying a request must not switch who owns its pre-consumed funds.
			nextChannel := constant.ChannelTypeGemini
			if tc.async {
				nextChannel = constant.ChannelTypeOpenAI
			}
			common.SetContextKey(c, constant.ContextKeyChannelType, nextChannel)
			assert.Equal(t, tc.async, ImageAsyncExecutionRequested(c))
		})
	}
}

// Task priority: with an explicit async preference, OpenAI/Azure select the
// platform task lifecycle regardless of stream. stream stays in the request
// for the background worker; every body-level error stays with the native
// request validator, which runs before admission.
func TestNativeImageTaskPrioritySelectsTaskEvenWithStream(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		prefer string
		want   bool
	}{
		{"stream_true", `{"stream":true}`, "respond-async", true},
		{"stream_false", `{"stream":false}`, "respond-async", true},
		{"stream_null", `{"stream":null}`, "respond-async", true},
		{"stream_wrong_type", `{"stream":"true"}`, "respond-async", true},
		{"stream_truncated_json", `{"stream":`, "respond-async", true},
		// 无异步头的原生流式保持原生路径，不进入 Task。
		{"no_prefer_native_stream", `{"stream":true}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, channel := range []int{constant.ChannelTypeOpenAI, constant.ChannelTypeAzure} {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(tc.body))
				c.Request.Header.Set("Prefer", tc.prefer)
				c.Request.Header.Set("Content-Type", "application/json")
				common.SetContextKey(c, constant.ContextKeyChannelType, channel)
				assert.Equal(t, tc.want, ImageAsyncExecutionRequested(c))
				storage, err := common.GetBodyStorage(c)
				require.NoError(t, err)
				data, err := storage.Bytes()
				require.NoError(t, err)
				assert.Equal(t, tc.body, string(data))
				assert.Equal(t, tc.want, ImageAsyncExecutionRequested(c))
			}
		})
	}
}

// Multipart edits follow the same task-priority contract: the mode decision
// no longer depends on the stream form value.
func TestNativeImageMultipartTaskPriorityWithStream(t *testing.T) {
	body := "--BOUNDARY\r\n" +
		"Content-Disposition: form-data; name=\"model\"\r\n\r\n" +
		"gpt-image-2\r\n" +
		"--BOUNDARY\r\n" +
		"Content-Disposition: form-data; name=\"stream\"\r\n\r\n" +
		"true\r\n" +
		"--BOUNDARY--\r\n"
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(body))
	c.Request.Header.Set("Prefer", "respond-async")
	c.Request.Header.Set("Content-Type", "multipart/form-data; boundary=BOUNDARY")
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeAzure)
	assert.True(t, ImageAsyncExecutionRequested(c))

	noPrefer, _ := gin.CreateTestContext(httptest.NewRecorder())
	noPrefer.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", strings.NewReader(body))
	noPrefer.Request.Header.Set("Content-Type", "multipart/form-data; boundary=BOUNDARY")
	common.SetContextKey(noPrefer, constant.ContextKeyChannelType, constant.ChannelTypeAzure)
	assert.False(t, ImageAsyncExecutionRequested(noPrefer))
}

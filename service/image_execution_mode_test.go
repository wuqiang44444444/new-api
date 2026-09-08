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

func TestNativeImageModeReadsStreamBeforeIdempotencyWithoutChangingBody(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"stream":true}`, false},
		{`{"stream":false}`, true},
		{`{"stream":null}`, true},
		{`{"stream":"true"}`, false},
		{`{"stream":`, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(tc.body))
			c.Request.Header.Set("Prefer", "respond-async")
			c.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
			assert.Equal(t, tc.want, ImageAsyncExecutionRequested(c))
			storage, err := common.GetBodyStorage(c)
			require.NoError(t, err)
			data, err := storage.Bytes()
			require.NoError(t, err)
			assert.Equal(t, tc.body, string(data))
			assert.Equal(t, tc.want, ImageAsyncExecutionRequested(c))
		})
	}
}

package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageFormatUnavailableStorageStopsBeforeGenerationAndBilling(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		kind        int
	}{
		{"openai", "gpt-image-2", constant.ChannelTypeOpenAI},
		{"azure", "gpt-image-2", constant.ChannelTypeAzure},
		{"gemini", "gemini-3.1-flash-image", constant.ChannelTypeGemini},
		{"vertex", "gemini-3.1-flash-image", constant.ChannelTypeVertexAi},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
			defer upstream.Close()
			channel := model.Channel{Id: 8961, Type: tc.kind, Key: "fixture", BaseURL: &upstream.URL, Status: 1}
			engine := imageFormatChannelEngine(t, channel, tc.model)
			model.NotifyObjectStorageSettingUpdate("")
			response := submitNativeImageCreate(t, engine, "/v1/images/generations", `{"model":"`+tc.model+`","prompt":"fixture","response_format":"url"}`, false)
			assert.Equal(t, http.StatusServiceUnavailable, response.Code)
			assert.Zero(t, calls)
			var user model.User
			require.NoError(t, model.DB.First(&user, 8961).Error)
			assert.Equal(t, 1000000, user.Quota)
			var token model.Token
			require.NoError(t, model.DB.First(&token, 8961).Error)
			assert.Equal(t, 1000000, token.RemainQuota)
			var count int64
			require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("type = ?", model.LogTypeConsume).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

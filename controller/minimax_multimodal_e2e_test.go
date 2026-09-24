package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func minimaxReferenceRequest(images, audios int) map[string]any {
	content := []any{map[string]any{"type": "text", "text": "Use the supplied references."}}
	for i := 0; i < images; i++ {
		content = append(content, map[string]any{"type": "image_url", "role": "reference_image", "image_url": map[string]any{"url": fmt.Sprintf("https://example.com/%d.png", i)}})
	}
	for i := 0; i < audios; i++ {
		content = append(content, map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": fmt.Sprintf("https://example.com/%d.mp3", i)}})
	}
	return map[string]any{"model": "customer-video", "content": content, "duration": 10, "resolution": "2k", "ratio": "9:16"}
}

func TestMiniMaxMultimodalStandardEntry(t *testing.T) {
	const hold = 1250000 // Explicit ten-second fixture tariff: $0.25 per second.
	fx := newMiniMaxFundsFixture(t, hold)
	request := minimaxReferenceRequest(9, 3)
	raw, err := common.Marshal(request)
	require.NoError(t, err)
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(string(raw)))
	var wire map[string]any
	require.NoError(t, common.Unmarshal(fx.createRequestBody(), &wire))
	expected, err := common.Marshal(request["content"])
	require.NoError(t, err)
	actual, err := common.Marshal(wire["content"])
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(actual), "all reference roles, URLs and ordering must reach the provider")
	params := wire["parameters"].(map[string]any)
	assert.Equal(t, float64(10), params["duration"])
	assert.Equal(t, "2K", params["resolution"])
	assert.Equal(t, "9:16", params["ratio"])
	task := fx.loadTask(id)
	assert.Equal(t, hold, task.Quota)
	assert.Contains(t, string(task.PrivateData.AsyncBilling.BillingProbe.Body), `"resolution":"2k"`)
	assert.Contains(t, string(task.PrivateData.AsyncBilling.BillingProbe.Body), `"ratio":"9:16"`)
	assert.Equal(t, seedanceFundsInitialQuota-hold, fx.userQuota())
	assert.EqualValues(t, 1, fx.createCalls.Load())
	stored, err := common.Marshal(task)
	require.NoError(t, err)
	assert.NotContains(t, string(stored), "example.com")
}

func TestMiniMaxMultimodalAdmissionBeforeFunding(t *testing.T) {
	cases := []struct {
		name  string
		patch func(map[string]any)
		code  int
	}{
		{"too many images", func(r map[string]any) { r["content"] = minimaxReferenceRequest(10, 0)["content"] }, 400},
		{"too many audios", func(r map[string]any) { r["content"] = minimaxReferenceRequest(0, 4)["content"] }, 400},
		{"duration above maximum", func(r map[string]any) { r["duration"] = 16 }, 400},
		{"duration below minimum", func(r map[string]any) { r["duration"] = 3 }, 400},
		{"long prompt", func(r map[string]any) { r["content"].([]any)[0].(map[string]any)["text"] = strings.Repeat("字", 7001) }, 400},
		{"asset reference", func(r map[string]any) {
			r["content"].([]any)[1].(map[string]any)["image_url"] = map[string]any{"url": "asset://foreign"}
		}, 400},
		{"mixed frame and reference", func(r map[string]any) {
			r["content"] = minimaxReferenceRequest(2, 0)["content"]
			r["content"].([]any)[1].(map[string]any)["role"] = "first_frame"
		}, 400},
		{"private parameter", func(r map[string]any) { r["prompt_optimizer"] = false }, 400},
		{"audio requires configured storage", func(r map[string]any) {
			r["content"] = []any{map[string]any{"type": "text", "text": "prompt"}, map[string]any{"type": "audio_url", "role": "reference_audio", "audio_url": map[string]any{"url": "data:audio/wav;base64,YXVkaW8="}}}
		}, 503},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newMiniMaxFundsFixture(t)
			request := minimaxReferenceRequest(1, 0)
			tc.patch(request)
			body, err := common.Marshal(request)
			require.NoError(t, err)
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer sk-"+strings.Repeat("f", 32))
			fx.engine.ServeHTTP(w, req)
			assert.Equal(t, tc.code, w.Code, w.Body.String())
			if tc.code == http.StatusServiceUnavailable {
				assert.Contains(t, w.Body.String(), "reference_audio_unavailable")
			}
			var count int64
			require.NoError(t, fx.db.Model(&model.TaskCreateAttempt{}).Count(&count).Error)
			assert.Zero(t, count)
			assert.Zero(t, fx.createCalls.Load())
			assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
		})
	}
}

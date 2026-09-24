package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func minimaxMedia(kind, role, url string) any {
	return map[string]any{"type": kind, "role": role, kind: map[string]any{"url": url}}
}

func TestMiniMaxFramesAndVideosStandardEntry(t *testing.T) {
	first := minimaxMedia("image_url", "first_frame", "https://example.com/first.jpg")
	last := minimaxMedia("image_url", "last_frame", "data:image/jpeg;base64,YQ==")
	video := minimaxMedia("video_url", "reference_video", "https://example.com/reference.mp4")
	inlineVideo := minimaxMedia("video_url", "reference_video", "data:video/mp4;base64,YQ==")
	cases := []struct {
		name     string
		media    []any
		ratio    string
		hasVideo bool
	}{
		{"first frame", []any{first}, "adaptive", false},
		{"last frame", []any{last}, "adaptive", false},
		{"paired frames", []any{first, last}, "adaptive", false},
		{"frame conditional ratio preserved", []any{first}, "9:16", false},
		{"three videos", []any{video, inlineVideo, video}, "adaptive", true},
		{"maximum reference mix", append(minimaxReferenceRequest(9, 3)["content"].([]any)[1:], video, inlineVideo, video), "16:9", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hold := minimaxFundsHold
			if tc.hasVideo {
				hold *= 2
			}
			fx := newMiniMaxFundsFixture(t, hold)
			require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
				"billing_setting.billing_expr": `{"customer-video":"u(\"has_video_input\") ? tier(\"video\", u(\"duration_seconds\") * 0.5) : tier(\"image\", u(\"duration_seconds\") * 0.25)"}`,
			}))
			request := minimaxReferenceRequest(0, 0)
			request["content"] = append(request["content"].([]any), tc.media...)
			request["duration"], request["resolution"], request["ratio"] = 6, "768p", tc.ratio
			raw, err := common.Marshal(request)
			require.NoError(t, err)
			id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(string(raw)))
			var wire map[string]any
			require.NoError(t, common.Unmarshal(fx.createRequestBody(), &wire))
			assert.Equal(t, request["content"], wire["content"], "roles, URL values and order must be preserved")
			assert.Equal(t, tc.ratio, wire["parameters"].(map[string]any)["ratio"])
			task := fx.loadTask(id)
			var probe map[string]any
			require.NoError(t, common.Unmarshal(task.PrivateData.AsyncBilling.BillingProbe.Body, &probe))
			assert.Equal(t, tc.hasVideo, probe["_task"].(map[string]any)["has_video_input"])
			assert.Equal(t, hold, task.Quota)
			assert.EqualValues(t, 1, fx.createCalls.Load())
			stored, err := common.Marshal(task.PrivateData)
			require.NoError(t, err)
			require.NotNil(t, task.PrivateData.Execution)
			require.NotNil(t, task.PrivateData.Execution.TaskPlugin)
			assert.Equal(t, plugins.MinimaxVersion(), task.PrivateData.Execution.TaskPlugin.Version)
			assert.NotContains(t, string(stored), "example.com")
			assert.NotContains(t, string(stored), "base64")
			fx.queryBodyRes.Store(minimaxSuccess)
			require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
			settled := fx.loadTask(id)
			assert.Equal(t, model.TaskBillingStateSettled, settled.BillingState)
			assert.Equal(t, task.PrivateData.AsyncBilling.BillingProbe.Body, settled.PrivateData.AsyncBilling.BillingProbe.Body)
			assert.Equal(t, seedanceFundsInitialQuota-hold, fx.userQuota())
		})
	}
}

func TestMiniMaxUnsupportedMediaDoesNotSubmitOrHold(t *testing.T) {
	first := minimaxMedia("image_url", "first_frame", "https://example.com/first.jpg")
	last := minimaxMedia("image_url", "last_frame", "https://example.com/last.jpg")
	video := minimaxMedia("video_url", "reference_video", "https://example.com/video.mp4")
	cases := []struct {
		name  string
		media []any
	}{
		{"duplicate first", []any{first, first}},
		{"duplicate last", []any{last, last}},
		{"frame with video", []any{first, video}},
		{"frame with audio", []any{last, minimaxMedia("audio_url", "reference_audio", "https://example.com/audio.mp3")}},
		{"four videos", []any{video, video, video, video}},
		{"asset video", []any{minimaxMedia("video_url", "reference_video", "asset://unavailable")}},
		{"file id", []any{minimaxMedia("image_url", "first_frame", "mm_file://unavailable")}},
		{"wrong data media", []any{minimaxMedia("video_url", "reference_video", "data:image/png;base64,YQ==")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newMiniMaxFundsFixture(t)
			request := minimaxReferenceRequest(0, 0)
			request["content"] = append(request["content"].([]any), tc.media...)
			raw, err := common.Marshal(request)
			require.NoError(t, err)
			r := httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(string(raw)))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", "Bearer sk-"+strings.Repeat("f", 32))
			w := httptest.NewRecorder()
			fx.engine.ServeHTTP(w, r)
			assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), "invalid_request")
			assert.Zero(t, fx.createCalls.Load())
			assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
			var count int64
			require.NoError(t, fx.db.Model(&model.TaskCreateAttempt{}).Count(&count).Error)
			assert.Zero(t, count)
		})
	}
}

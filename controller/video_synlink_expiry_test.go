package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSynlinkVideoSignatureExpiryRequiresExplicitEvidence(t *testing.T) {
	task := &model.Task{PrivateData: model.TaskPrivateData{VideoUpstreamProtocol: dto.VideoUpstreamProtocolSynlinkVideoV1}}
	now := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	base := "https://result.example/video?X-Tos-Algorithm=TOS4-HMAC-SHA256&X-Tos-Date=20260914T000000Z&X-Tos-Expires=86400&X-Tos-Signature=fixture"
	for _, tc := range []struct {
		name, url string
		now       time.Time
		expired   bool
	}{
		{"at expiry", base, now, true},
		{"before expiry", base, now.Add(-time.Second), false},
		{"after expiry", base, now.Add(time.Second), true},
		{"duration alone is insufficient", strings.Replace(base, "X-Tos-Date=20260914T000000Z&", "", 1), now, false},
		{"invalid date", strings.Replace(base, "20260914T000000Z", "20260230T000000Z", 1), now, false},
		{"unsupported fractional date", strings.Replace(base, "20260914T000000Z", "20260914T000000.1Z", 1), now, false},
		{"duplicate date", base + "&X-Tos-Date=20260915T000000Z", now, false},
		{"duplicate ttl", base + "&X-Tos-Expires=604800", now, false},
		{"invalid query", base + "&invalid=%XX", now, false},
		{"unknown algorithm", strings.Replace(base, "TOS4-HMAC-SHA256", "unknown", 1), now, false},
		{"unsigned url", strings.Replace(base, "&X-Tos-Signature=fixture", "", 1), now, false},
		{"zero ttl", strings.Replace(base, "86400", "0", 1), now, false},
		{"negative ttl", strings.Replace(base, "86400", "-1", 1), now, false},
		{"oversized ttl", strings.Replace(base, "86400", "18446744073709551615", 1), now, false},
		{"outside supported ttl", strings.Replace(base, "86400", "604801", 1), now, false},
	} {
		t.Run(tc.name, func(t *testing.T) { assert.Equal(t, tc.expired, synlinkVideoSignatureExpired(task, tc.url, tc.now)) })
	}
	task.PrivateData.VideoUpstreamProtocol = dto.VideoUpstreamProtocolFunCloudModelArkV3
	assert.False(t, synlinkVideoSignatureExpired(task, base, now), "other protocols keep their content contract")
}

func TestSynlinkContentExpiryDoesNotChangeSuccessfulTaskOrBilling(t *testing.T) {
	task := setupGenericTaskTest(t)
	allowPrivateTaskMediaTest(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = w.Write([]byte("video"))
		}
	}))
	t.Cleanup(provider.Close)
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink))
	task.AppID = 11
	task.Quota = 100
	task.BillingState = model.TaskBillingStateSettled
	task.PrivateData = model.TaskPrivateData{
		VideoUpstreamProtocol:     dto.VideoUpstreamProtocolSynlinkVideoV1,
		VideoUpstreamQueryBaseURL: provider.URL, Key: "frozen-key", UpstreamTaskID: "provider-task",
	}
	for _, tc := range []struct {
		name, url string
		data      string
		status    int
		code      string
	}{
		{"expired signature", "https://127.0.0.1:1/video?X-Tos-Algorithm=TOS4-HMAC-SHA256&X-Tos-Date=20200101T000000Z&X-Tos-Expires=86400&X-Tos-Signature=fixture", "{}", http.StatusGone, "video_content_expired"},
		{"explicit expiry", provider.URL + "/video", `{"expires_at":1}`, http.StatusGone, "video_content_expired"},
		{"403 alone is not expiry", provider.URL + "/forbidden", "{}", http.StatusBadGateway, "upstream_unavailable"},
		{"404 content alone is not expiry", provider.URL + "/missing", "{}", http.StatusBadGateway, "upstream_unavailable"},
		{"transport failure is not expiry", "http://127.0.0.1:1/video", "{}", http.StatusBadGateway, "upstream_unavailable"},
		{"no expiry evidence still downloads", provider.URL + "/video", "{}", http.StatusOK, ""},
	} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(tc.name+"/"+method, func(t *testing.T) {
				task.PrivateData.ResultURL = tc.url
				task.Data = []byte(tc.data)
				require.NoError(t, model.DB.Save(task).Error)
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(method, "/v1/videos/"+task.TaskID+"/content", nil)
				c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
				c.Set("id", task.UserId)
				c.Set("token_id", task.AppID)
				require.True(t, proxyLinkVideoContent(c))
				assert.Equal(t, tc.status, recorder.Code)
				if tc.code != "" {
					assert.Contains(t, recorder.Body.String(), tc.code)
				}
				assert.NotContains(t, recorder.Body.String(), "X-Tos-")
				assert.NotContains(t, recorder.Body.String(), "frozen-key")
				var saved model.Task
				require.NoError(t, model.DB.First(&saved, task.ID).Error)
				assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), saved.Status)
				assert.Equal(t, 100, saved.Quota)
				assert.Equal(t, model.TaskBillingStateSettled, saved.BillingState)
				assert.Equal(t, task.FinishTime, saved.FinishTime)
				assert.JSONEq(t, string(task.Data), string(saved.Data))
			})
		}
	}
	// Access checks precede expiry disclosure.
	task.Data = []byte(`{"expires_at":1}`)
	require.NoError(t, model.DB.Save(task).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/"+task.TaskID+"/content", nil)
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	c.Set("id", task.UserId)
	c.Set("token_id", task.AppID+1)
	require.True(t, proxyLinkVideoContent(c))
	assert.Equal(t, http.StatusNotFound, recorder.Code)
}

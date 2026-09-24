package controller

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxDashboardArtifactsDeliverFrozenVideo(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	previousSecret := common.CryptoSecret
	common.CryptoSecret = "minimax-artifact-fixture"
	t.Cleanup(func() { common.CryptoSecret = previousSecret })
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":6,"resolution":"768p","ratio":"16:9"}`))
	task := fx.loadTask(id)
	fx.queryBodyRes.Store(minimaxSuccess)
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	require.EqualValues(t, model.TaskStatusSuccess, fx.loadTask(id).Status)
	quota := fx.userQuota()
	// The stored result is a gateway placeholder, not a persisted CDN URL.
	require.NoError(t, fx.db.Delete(&model.Channel{}, fx.channelID).Error)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", fx.userID)
	c.Params = gin.Params{{Key: "task_id", Value: id}}
	c.Request = httptest.NewRequest(http.MethodGet, "/api/task/"+id+"/artifacts", nil)
	GetDashboardTaskArtifacts(c)
	require.Equal(t, http.StatusOK, w.Code)
	var response struct {
		Data struct {
			Artifacts []taskArtifactResponse `json:"artifacts"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &response))
	require.Len(t, response.Data.Artifacts, 1)
	artifact := response.Data.Artifacts[0]
	assert.Equal(t, "video", artifact.Key)
	assert.Equal(t, "video/mp4", artifact.MimeType)
	assert.NotContains(t, w.Body.String(), "8.8.8.8")
	assert.NotContains(t, w.Body.String(), "provider-fixture-key")
	contentURL, err := url.Parse(artifact.ContentURL)
	require.NoError(t, err)
	require.True(t, service.VerifyTaskArtifactAccess(contentURL.Query().Get("access"), id, "video"))
	mediaClient := service.GetSSRFProtectedHTTPClient()
	oldTransport := mediaClient.Transport
	t.Cleanup(func() { mediaClient.Transport = oldTransport })
	mediaCalls := 0
	mediaClient.Transport = fetchModelsRoundTripper(func(r *http.Request) (*http.Response, error) {
		mediaCalls++
		assert.Equal(t, "https://8.8.8.8/video.mp4", r.URL.String())
		assert.Empty(t, r.Header.Get("Authorization"))
		header := http.Header{"Content-Type": {"video/mp4"}, "Content-Length": {"10"}}
		if r.Method == http.MethodHead {
			return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		assert.Equal(t, "bytes=0-3", r.Header.Get("Range"))
		header.Set("Content-Range", "bytes 0-3/10")
		return &http.Response{StatusCode: 206, Header: header, Body: io.NopCloser(strings.NewReader("vide"))}, nil
	})
	router := gin.New()
	router.Use(middleware.TokenOrTaskArtifactAccessAuth("key", "artifact_key"))
	router.GET("/v1/tasks/:key/artifacts/:artifact_key/content", TaskArtifactContent)
	router.HEAD("/v1/tasks/:key/artifacts/:artifact_key/content", TaskArtifactContent)
	before := fx.queryCalls.Load()
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(method, artifact.ContentURL, nil)
		if method == http.MethodGet {
			r.Header.Set("Range", "bytes=0-3")
		}
		router.ServeHTTP(w, r)
		assert.Equal(t, "video/mp4", w.Header().Get("Content-Type"))
		assert.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
		if method == http.MethodGet {
			assert.Equal(t, http.StatusPartialContent, w.Code)
			assert.Equal(t, "vide", w.Body.String())
			assert.Equal(t, "bytes 0-3/10", w.Header().Get("Content-Range"))
		} else {
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Empty(t, w.Body.String())
			assert.Equal(t, "10", w.Header().Get("Content-Length"))
		}
	}
	assert.Equal(t, 2, mediaCalls)
	assert.Equal(t, before+1, fx.queryCalls.Load(), "cold lookup queries once; HEAD reuses the temporary URL")
	for _, upstreamStatus := range []int{http.StatusFound, http.StatusNotFound} {
		t.Run(http.StatusText(upstreamStatus), func(t *testing.T) {
			calls := 0
			mediaClient.Transport = fetchModelsRoundTripper(func(r *http.Request) (*http.Response, error) {
				calls++
				assert.Equal(t, "https://8.8.8.8/video.mp4", r.URL.String())
				return &http.Response{StatusCode: upstreamStatus, Header: http.Header{"Location": {"https://8.8.4.4/other.mp4"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
			})
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, artifact.ContentURL, nil))
			assert.Equal(t, http.StatusBadGateway, w.Code)
			assert.Empty(t, w.Header().Get("Location"))
			assert.Equal(t, 1, calls, "media redirects must not be followed")
			assert.EqualValues(t, model.TaskStatusSuccess, fx.loadTask(id).Status)
			assert.Equal(t, model.TaskBillingStateSettled, fx.loadTask(id).BillingState)
		})
	}
	assert.EqualValues(t, 1, fx.createCalls.Load())
	assert.Equal(t, quota, fx.userQuota(), "artifact delivery never charges again")
}

func TestMiniMaxArtifactContentKeepsSSRFProtection(t *testing.T) {
	fx := newMiniMaxFundsFixture(t)
	id := decodeSeedanceFundsCreateID(t, fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"A landscape"}],"duration":6,"resolution":"768p","ratio":"16:9"}`))
	task := fx.loadTask(id)
	fx.queryBodyRes.Store(strings.Replace(minimaxSuccess, "8.8.8.8", "127.0.0.1", 1))
	require.NoError(t, service.RefreshVideoTask(context.Background(), &task))
	previous := *system_setting.GetFetchSetting()
	system_setting.GetFetchSetting().EnableSSRFProtection = true
	system_setting.GetFetchSetting().AllowPrivateIp = false
	t.Cleanup(func() { *system_setting.GetFetchSetting() = previous })
	client := service.GetSSRFProtectedHTTPClient()
	transport := client.Transport
	t.Cleanup(func() { client.Transport = transport })
	client.Transport = fetchModelsRoundTripper(func(r *http.Request) (*http.Response, error) {
		t.Error("blocked media must not reach the transport")
		return nil, errors.New("unexpected media fetch")
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("id", task.UserId)
	c.Set("token_id", task.AppID)
	c.Params = gin.Params{{Key: "key", Value: id}, {Key: "artifact_key", Value: "video"}}
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+id+"/artifacts/video/content", nil)
	TaskArtifactContent(c)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "content_url_not_allowed")
}

func TestMiniMaxArtifactsRejectUnavailableAndForeignResources(t *testing.T) {
	for _, scenario := range []string{"pending", "failed", "deleted", "foreign_user", "foreign_app", "missing_upstream_id", "wrong_artifact", "different_part", "missing_frozen_key", "missing_plugin"} {
		t.Run(scenario, func(t *testing.T) {
			task := setupSeedanceArtifactTest(t, "https://example.invalid")
			task.Platform = "64"
			task.PrivateData.VideoUpstreamProtocol = "jdcloud_video_task_v1"
			task.PrivateData.ResultURL = "/v1/videos/" + task.TaskID + "/content"
			task.PrivateData.Execution.TaskPlugin.Key = "minimax-link"
			userID, appID, artifact, query := task.UserId, task.AppID, "video", ""
			status := http.StatusNotFound
			switch scenario {
			case "pending":
				task.Status = model.TaskStatusInProgress
				status = http.StatusConflict
			case "failed":
				task.Status = model.TaskStatusFailure
				status = http.StatusConflict
			case "deleted":
				task.ClientDeletedAt = 1
			case "foreign_user":
				userID++
			case "foreign_app":
				appID++
			case "missing_upstream_id":
				task.PrivateData.UpstreamTaskID = ""
			case "wrong_artifact":
				artifact = "other"
			case "different_part":
				query = "?part=last_frame"
			case "missing_frozen_key":
				task.PrivateData.Key = ""
				status = http.StatusBadGateway
			case "missing_plugin":
				task.PrivateData.Execution = nil
				status = http.StatusBadGateway
			}
			require.NoError(t, model.DB.Save(task).Error)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("id", userID)
			c.Set("token_id", appID)
			c.Params = gin.Params{{Key: "key", Value: task.TaskID}, {Key: "artifact_key", Value: artifact}}
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID+"/artifacts/"+artifact+"/content"+query, nil)
			TaskArtifactContent(c)
			assert.Equal(t, status, w.Code, w.Body.String())
			if scenario == "wrong_artifact" || scenario == "different_part" || scenario == "missing_frozen_key" || scenario == "missing_plugin" {
				return
			}
			w = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(w)
			c.Set("id", userID)
			c.Set("token_id", appID)
			c.Params = gin.Params{{Key: "key", Value: task.TaskID}}
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/tasks/"+task.TaskID+"/artifacts", nil)
			GetTaskArtifacts(c)
			if scenario == "deleted" || scenario == "foreign_user" || scenario == "foreign_app" {
				assert.Equal(t, http.StatusNotFound, w.Code)
			} else {
				require.Equal(t, http.StatusOK, w.Code)
				var body struct {
					Artifacts []taskArtifactResponse `json:"artifacts"`
				}
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
				assert.Empty(t, body.Artifacts)
			}
		})
	}
}

package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupVideoResultDeliveryTest(t *testing.T) string {
	t.Helper()
	previousMain := common.MainDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	t.Cleanup(func() { common.SetDatabaseTypes(previousMain, previousMain) })
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.Channel{}, &model.User{}, &model.Option{}, &model.TaskVideoResultObject{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	// 通过数据库配置路径激活真实 S3 存储实例：预签名是本地计算，不产生网络请求。
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		signature := query.Get("X-Amz-Signature")
		issuedAt, parseErr := time.Parse("20060102T150405Z", query.Get("X-Amz-Date"))
		ttl, ttlErr := strconv.ParseInt(query.Get("X-Amz-Expires"), 10, 64)
		query.Del("X-Amz-Signature")
		raw := "http://" + r.Host + r.URL.Path + "?" + query.Encode()
		signed, signErr := service.SigV4PresignURL(r.Method, raw, service.SigV4Credentials{AccessKey: "test-key", SecretKey: "test-secret"}, "us-east-1", time.Duration(ttl)*time.Second, issuedAt)
		expected, urlErr := url.Parse(signed)
		if parseErr != nil || ttlErr != nil || signErr != nil || urlErr != nil || signature != expected.Query().Get("X-Amz-Signature") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "42")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(storage.Close)
	config := system_setting.ObjectStorageConfig{
		Backend:     system_setting.ObjectStorageBackendS3,
		Endpoint:    storage.URL,
		Bucket:      "video-bucket",
		Region:      "us-east-1",
		AccountName: "test-key",
		Credential:  "test-secret",
		Revision:    common.GetUUID(),
	}
	raw, err := common.Marshal(config)
	require.NoError(t, err)
	require.NoError(t, model.SaveObjectStorageSetting(string(raw)))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	require.True(t, service.VideoResultDeliveryAvailable(context.Background()))
	return storage.URL
}

func newVideoResultDeliveryTask(t *testing.T, taskID string, finishTime int64) *model.Task {
	t.Helper()
	task := &model.Task{
		TaskID:         taskID,
		Platform:       constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink)),
		UserId:         7,
		AppID:          11,
		ChannelId:      1,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		Status:         model.TaskStatusSuccess,
		FinishTime:     finishTime,
		SubmitTime:     finishTime - 10,
		PrivateData: model.TaskPrivateData{
			ResultURL:                 "https://provider.example/video.mp4",
			Key:                       "frozen-key",
			UpstreamTaskID:            "provider-task",
			VideoUpstreamQueryBaseURL: "https://provider.example",
		},
	}
	require.NoError(t, model.DB.Create(task).Error)
	t.Cleanup(func() {
		_ = model.DB.Where("task_id = ?", task.TaskID).Delete(&model.TaskVideoResultObject{}).Error
		_ = model.DB.Unscoped().Delete(&model.Task{}, "task_id = ?", task.TaskID).Error
	})
	return task
}

func insertVideoResultReadyRow(t *testing.T, endpoint, taskID, part, objectKey string) {
	t.Helper()
	now := common.GetTimestamp()
	row := model.TaskVideoResultObject{
		TaskID: taskID,
		Part:   part,
		UserID: 7,
		AppID:  11,
		Status: model.TaskVideoResultStatusReady,
		Location: model.VideoResultStorageLocation{
			Backend: "s3", Endpoint: endpoint, Bucket: "video-bucket",
		},
		ObjectKey: objectKey,
		MimeType:  "video/mp4",
		SizeBytes: 42,
		CreatedAt: now,
		UpdatedAt: now,
	}
	require.NoError(t, model.DB.Create(&row).Error)
}

func newVideoResultDeliveryContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 7)
	c.Set("token_id", 11)
	c.Request = httptest.NewRequest(method, target, nil)
	return c, recorder
}

func TestApplyVideoResultDeliveryProjectionContract(t *testing.T) {
	endpoint := setupVideoResultDeliveryTest(t)
	now := common.GetTimestamp()
	task := newVideoResultDeliveryTask(t, "task_delivery_ready", now)
	insertVideoResultReadyRow(t, endpoint, task.TaskID, model.TaskVideoResultPartVideo, "videos/tasks/"+task.TaskID+"/video")
	insertVideoResultReadyRow(t, endpoint, task.TaskID, model.TaskVideoResultPartLastFrame, "videos/tasks/"+task.TaskID+"/last_frame")

	c, _ := newVideoResultDeliveryContext(http.MethodGet, "http://gateway.test/api/v3/contents/generations/tasks/"+task.TaskID)
	projected := projectModelArkVideoTask(c, task)
	require.NotNil(t, projected.Content)
	assert.Equal(t, "ready", projected.Content.DeliveryStatus)
	assert.Contains(t, projected.Content.VideoURL, "video-bucket")
	assert.Contains(t, projected.Content.VideoURL, "videos/tasks/"+task.TaskID+"/video")
	assert.Contains(t, projected.Content.VideoURL, "X-Amz-Signature")
	assert.Contains(t, projected.Content.LastFrameURL, "videos/tasks/"+task.TaskID+"/last_frame")
	assert.NotContains(t, projected.Content.VideoURL, "provider.example", "upstream direct links must never be projected")

	t.Run("pending stays preparing without urls", func(t *testing.T) {
		pending := newVideoResultDeliveryTask(t, "task_delivery_pending", now)
		row := model.TaskVideoResultObject{TaskID: pending.TaskID, Part: model.TaskVideoResultPartVideo, UserID: 7, AppID: 11, Status: model.TaskVideoResultStatusPending, CreatedAt: now, UpdatedAt: now}
		require.NoError(t, model.DB.Create(&row).Error)
		c, _ := newVideoResultDeliveryContext(http.MethodGet, "http://gateway.test/api/v3/x")
		projected := projectModelArkVideoTask(c, pending)
		require.NotNil(t, projected.Content)
		assert.Equal(t, "preparing", projected.Content.DeliveryStatus)
		assert.Empty(t, projected.Content.VideoURL)
		assert.Empty(t, projected.Content.LastFrameURL)
	})

	t.Run("unavailable is explicit", func(t *testing.T) {
		failed := newVideoResultDeliveryTask(t, "task_delivery_unavailable", now)
		row := model.TaskVideoResultObject{TaskID: failed.TaskID, Part: model.TaskVideoResultPartVideo, UserID: 7, AppID: 11, Status: model.TaskVideoResultStatusUnavailable, LastError: "upstream result expired", CreatedAt: now, UpdatedAt: now}
		require.NoError(t, model.DB.Create(&row).Error)
		c, _ := newVideoResultDeliveryContext(http.MethodGet, "http://gateway.test/api/v3/x")
		projected := projectModelArkVideoTask(c, failed)
		require.NotNil(t, projected.Content)
		assert.Equal(t, "unavailable", projected.Content.DeliveryStatus)
		assert.Empty(t, projected.Content.VideoURL)
		assert.Equal(t, "succeeded", projected.Status, "delivery state must not override confirmed generation success")
	})

	t.Run("unknown completion time cannot promise recovery", func(t *testing.T) {
		unknown := newVideoResultDeliveryTask(t, "task_delivery_unknown_completion", 0)
		c, _ := newVideoResultDeliveryContext(http.MethodGet, "http://gateway.test/api/v3/x")
		projected := projectModelArkVideoTask(c, unknown)
		require.NotNil(t, projected.Content)
		assert.Equal(t, "unavailable", projected.Content.DeliveryStatus)
		assert.Empty(t, projected.Content.VideoURL)
	})

	t.Run("stock tasks outside the sweep window are unavailable", func(t *testing.T) {
		old := newVideoResultDeliveryTask(t, "task_delivery_stock", now-25*3600)
		c, _ := newVideoResultDeliveryContext(http.MethodGet, "http://gateway.test/api/v3/x")
		projected := projectModelArkVideoTask(c, old)
		require.NotNil(t, projected.Content)
		assert.Equal(t, "unavailable", projected.Content.DeliveryStatus)
	})
}

func TestServeVideoResultDeliveryContract(t *testing.T) {
	endpoint := setupVideoResultDeliveryTest(t)
	now := common.GetTimestamp()
	ready := newVideoResultDeliveryTask(t, "task_serve_ready", now)
	insertVideoResultReadyRow(t, endpoint, ready.TaskID, model.TaskVideoResultPartVideo, "videos/tasks/"+ready.TaskID+"/video")

	t.Run("ready redirects to the signed object", func(t *testing.T) {
		c, recorder := newVideoResultDeliveryContext(http.MethodGet, "/v1/videos/"+ready.TaskID+"/content")
		assert.True(t, serveVideoResultDelivery(c, ready))
		assert.Equal(t, http.StatusFound, recorder.Code)
		location := recorder.Header().Get("Location")
		assert.Contains(t, location, "video-bucket")
		assert.Contains(t, location, "videos/tasks/"+ready.TaskID+"/video")
		assert.Contains(t, location, "X-Amz-Signature")
	})

	t.Run("head also redirects without body", func(t *testing.T) {
		c, recorder := newVideoResultDeliveryContext(http.MethodHead, "/v1/videos/"+ready.TaskID+"/content")
		assert.True(t, serveVideoResultDelivery(c, ready))
		c.Writer.WriteHeaderNow()
		assert.Equal(t, http.StatusFound, recorder.Code)
		assert.NotEmpty(t, recorder.Header().Get("Location"))
		request, err := http.NewRequest(http.MethodHead, recorder.Header().Get("Location"), nil)
		require.NoError(t, err)
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		assert.Equal(t, http.StatusOK, response.StatusCode, "HEAD must be authorized by a HEAD signature")
		assert.EqualValues(t, 42, response.ContentLength)
	})

	t.Run("part validation and last frame states", func(t *testing.T) {
		c, recorder := newVideoResultDeliveryContext(http.MethodGet, "/v1/videos/"+ready.TaskID+"/content?part=cover")
		assert.True(t, serveVideoResultDelivery(c, ready))
		assert.Equal(t, http.StatusBadRequest, recorder.Code)

		c, recorder = newVideoResultDeliveryContext(http.MethodGet, "/v1/videos/"+ready.TaskID+"/content?part=last_frame")
		assert.True(t, serveVideoResultDelivery(c, ready))
		assert.Equal(t, http.StatusNotFound, recorder.Code, "task without a last frame upstream fact stays content_not_found")

		withFrame := newVideoResultDeliveryTask(t, "task_serve_frame", now)
		insertVideoResultReadyRow(t, endpoint, withFrame.TaskID, model.TaskVideoResultPartLastFrame, "videos/tasks/"+withFrame.TaskID+"/last_frame")
		c, recorder = newVideoResultDeliveryContext(http.MethodGet, "/v1/videos/"+withFrame.TaskID+"/content?part=last_frame")
		assert.True(t, serveVideoResultDelivery(c, withFrame))
		assert.Equal(t, http.StatusFound, recorder.Code)
		assert.Contains(t, recorder.Header().Get("Location"), "/last_frame")
	})

	t.Run("preparing and unavailable are explicit errors", func(t *testing.T) {
		pending := newVideoResultDeliveryTask(t, "task_serve_pending", now)
		c, recorder := newVideoResultDeliveryContext(http.MethodGet, "/v1/videos/"+pending.TaskID+"/content")
		assert.True(t, serveVideoResultDelivery(c, pending))
		assert.Equal(t, http.StatusConflict, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "video_delivery_preparing")

		failed := newVideoResultDeliveryTask(t, "task_serve_failed", now)
		row := model.TaskVideoResultObject{TaskID: failed.TaskID, Part: model.TaskVideoResultPartVideo, UserID: 7, AppID: 11, Status: model.TaskVideoResultStatusUnavailable, CreatedAt: now, UpdatedAt: now}
		require.NoError(t, model.DB.Create(&row).Error)
		c, recorder = newVideoResultDeliveryContext(http.MethodGet, "/v1/videos/"+failed.TaskID+"/content")
		assert.True(t, serveVideoResultDelivery(c, failed))
		assert.Equal(t, http.StatusGone, recorder.Code)
		assert.Contains(t, recorder.Body.String(), "video_delivery_unavailable")
	})

	t.Run("out-of-scope tasks keep proxy delivery", func(t *testing.T) {
		native := newVideoResultDeliveryTask(t, "task_serve_native", now)
		native.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSora))
		require.NoError(t, model.DB.Save(native).Error)
		c, recorder := newVideoResultDeliveryContext(http.MethodGet, "/v1/videos/"+native.TaskID+"/content")
		assert.False(t, serveVideoResultDelivery(c, native))
		assert.Equal(t, http.StatusOK, recorder.Code, "no response may be written for out-of-scope tasks")
	})
}

func TestVideoResultMissingFrameUsesRecoveryWindow(t *testing.T) {
	endpoint := setupVideoResultDeliveryTest(t)
	now := common.GetTimestamp()
	for _, tc := range []struct {
		id     string
		finish int64
		frame  bool
		status int
	}{
		{"frame_gap_recent", now, true, http.StatusConflict},
		{"frame_gap_old", now - 25*3600, true, http.StatusGone},
		{"frame_gap_unknown", 0, true, http.StatusGone},
		{"no_frame_fact", now, false, http.StatusNotFound},
	} {
		t.Run(tc.id, func(t *testing.T) {
			task := newVideoResultDeliveryTask(t, tc.id, tc.finish)
			if tc.frame {
				task.Data = []byte(`{"content":{"last_frame_url":"https://provider.example/frame.png"}}`)
				require.NoError(t, model.DB.Save(task).Error)
			}
			insertVideoResultReadyRow(t, endpoint, task.TaskID, model.TaskVideoResultPartVideo, "videos/tasks/"+task.TaskID+"/video")
			c, recorder := newVideoResultDeliveryContext(http.MethodGet, "/v1/videos/"+task.TaskID+"/content?part=last_frame")
			require.True(t, serveVideoResultDelivery(c, task))
			assert.Equal(t, tc.status, recorder.Code)
			assert.Empty(t, recorder.Header().Get("Location"))
		})
	}
}

package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageTaskDashboardArtifactsAndAuthorizedDelivery(t *testing.T) {
	task := setupGenericTaskTest(t)
	task.AppID = 11
	task.ClientProtocol = model.TaskClientProtocolImageOpenAIV1
	task.PrivateData.ImageTask = &model.TaskImageExecutionData{ResponseFormat: "b64_json", ImageCount: 2, Artifacts: []model.TaskImageArtifact{
		{ObjectKey: "images/tasks/task_generic/result-0", MimeType: "image/png", Size: 5},
		{ObjectKey: "images/tasks/task_generic/result-1", MimeType: "image/png", Size: 5},
	}}
	require.NoError(t, model.DB.Save(task).Error)
	var calls atomic.Int32
	var storageStatus atomic.Int32
	storageStatus.Store(200)
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(int(storageStatus.Load()))
	}))
	defer store.Close()
	raw, err := common.Marshal(system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "images", Region: "us-east-1", AccountName: "fixture", Credential: "fixture", Revision: t.Name()})
	require.NoError(t, err)
	model.NotifyObjectStorageSettingUpdate(string(raw))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	request := func(user, app int, method, key string, dashboard bool) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("id", user)
		c.Set("token_id", app)
		c.Request = httptest.NewRequest(method, "/v1/tasks/task_generic/artifacts/"+key+"/content", nil)
		if dashboard {
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			GetDashboardTaskArtifacts(c)
		} else {
			c.Params = gin.Params{{Key: "key", Value: task.TaskID}, {Key: "artifact_key", Value: key}}
			TaskArtifactContent(c)
		}
		return w
	}
	for _, caller := range [][2]int{{7, 12}, {8, 11}} {
		assert.Equal(t, http.StatusNotFound, request(caller[0], caller[1], "GET", "result-0", false).Code)
		assert.Equal(t, http.StatusNotFound, request(caller[0], caller[1], "GET", "", true).Code)
	}
	assert.Zero(t, calls.Load(), "unauthorized requests cannot probe or sign storage objects")
	w := request(7, 0, "GET", "", true)
	require.Equal(t, http.StatusOK, w.Code)
	var listing struct {
		Data struct {
			Artifacts []taskArtifactResponse `json:"artifacts"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &listing))
	require.Len(t, listing.Data.Artifacts, 2)
	assert.Equal(t, "result-0", listing.Data.Artifacts[0].Key)
	assert.Equal(t, "image", listing.Data.Artifacts[0].Type)
	assert.NotContains(t, w.Body.String(), "images/tasks/", "private storage keys are not public URLs")
	w = request(7, 11, "GET", "result-0", false)
	require.Equal(t, http.StatusFound, w.Code, w.Body.String())
	u, err := url.Parse(w.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "300", u.Query().Get("X-Amz-Expires"))
	assert.True(t, strings.HasSuffix(u.Path, "/result-0"))
	w = request(7, 11, "HEAD", "result-0", false)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
	assert.Equal(t, "5", w.Header().Get("Content-Length"))
	assert.Empty(t, w.Body.String())
	before := calls.Load()
	assert.Equal(t, http.StatusNotFound, request(7, 11, "GET", "result-2", false).Code)
	assert.Equal(t, before, calls.Load(), "unregistered object keys must not be signed")
	storageStatus.Store(404)
	assert.Equal(t, http.StatusNotFound, request(7, 11, "GET", "result-0", false).Code)
	storageStatus.Store(403)
	assert.Equal(t, http.StatusServiceUnavailable, request(7, 11, "GET", "result-0", false).Code)
}

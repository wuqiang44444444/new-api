package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 托管交付后的合同：内容入口只重定向本站签名地址或表达交付状态；交付过程
// 不回源、不改成功任务的状态、计费与数据，也不泄露上游地址与冻结凭据。
func TestSynlinkHostedDeliveryKeepsTaskFactsAndHidesUpstream(t *testing.T) {
	task := setupGenericTaskTest(t)
	setupSeedanceArtifactStorage(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("video"))
	}))
	t.Cleanup(provider.Close)
	task.ClientProtocol = model.TaskClientProtocolModelArkV3
	task.Platform = constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeSeedanceLink))
	task.AppID = 11
	task.Quota = 100
	task.BillingState = model.TaskBillingStateSettled
	task.FinishTime = common.GetTimestamp()
	task.PrivateData = model.TaskPrivateData{
		ResultURL:                 provider.URL + "/video",
		VideoUpstreamQueryBaseURL: provider.URL,
		Key:                       "frozen-key",
		UpstreamTaskID:            "provider-task",
	}
	require.NoError(t, model.DB.Save(task).Error)

	location := model.VideoResultStorageLocation{Backend: "s3", Endpoint: "http://oss.example.test", Bucket: "video-bucket"}
	row := model.TaskVideoResultObject{
		TaskID: task.TaskID, Part: model.TaskVideoResultPartVideo, UserID: task.UserId, AppID: task.AppID,
		Status: model.TaskVideoResultStatusReady, Location: location,
		ObjectKey: "videos/tasks/" + task.TaskID + "/video", MimeType: "video/mp4", SizeBytes: 10,
		CreatedAt: common.GetTimestamp(), UpdatedAt: common.GetTimestamp(),
	}
	require.NoError(t, model.DB.Create(&row).Error)
	t.Cleanup(func() {
		_ = model.DB.Where("task_id = ?", task.TaskID).Delete(&model.TaskVideoResultObject{}).Error
	})

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(method, "/v1/videos/"+task.TaskID+"/content", nil)
			c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
			c.Set("id", task.UserId)
			c.Set("token_id", task.AppID)
			require.True(t, proxyLinkVideoContent(c))
			c.Writer.WriteHeaderNow()
			assert.Equal(t, http.StatusFound, recorder.Code)
			assert.Contains(t, recorder.Header().Get("Location"), "videos/tasks/"+task.TaskID+"/video")
			assert.NotContains(t, recorder.Body.String(), "X-Tos-")
			assert.NotContains(t, recorder.Body.String(), "frozen-key")
			var saved model.Task
			require.NoError(t, model.DB.First(&saved, task.ID).Error)
			assert.Equal(t, model.TaskStatus(model.TaskStatusSuccess), saved.Status)
			assert.Equal(t, 100, saved.Quota)
			assert.Equal(t, model.TaskBillingStateSettled, saved.BillingState)
			assert.Equal(t, task.FinishTime, saved.FinishTime)
		})
	}
}

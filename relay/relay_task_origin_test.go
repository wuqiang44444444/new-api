package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestResolveOriginTaskRejectsDeletedOrCrossProtocolRemixSources(t *testing.T) {
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })

	sources := []model.Task{
		{
			TaskID: "deleted-openai", UserId: 42, ClientProtocol: model.TaskClientProtocolOpenAIVideos,
			ClientDeletedAt: 1, Platform: constant.TaskPlatform("sora"),
		},
		{
			TaskID: "modelark-source", UserId: 42, ClientProtocol: model.TaskClientProtocolModelArkV3,
			Platform: constant.TaskPlatform("sora"),
		},
	}
	for i := range sources {
		require.NoError(t, db.Create(&sources[i]).Error)
	}

	for _, taskID := range []string{"deleted-openai", "modelark-source"} {
		t.Run(taskID, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/"+taskID+"/remix", nil)
			context.Params = gin.Params{{Key: "video_id", Value: taskID}}

			taskErr := ResolveOriginTask(context, &relaycommon.RelayInfo{
				UserId:        42,
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
			})

			require.NotNil(t, taskErr)
			assert.Equal(t, "task_not_exist", taskErr.Code)
		})
	}
}

func TestLegacyVideoFetchIsIsolatedByApplication(t *testing.T) {
	originalDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })

	task := model.Task{
		TaskID: "legacy-video-app-a", UserId: 42, AppID: 1001,
		ClientProtocol: model.TaskClientProtocolPlatformVideo,
		Platform:       constant.TaskPlatform("sora"),
	}
	require.NoError(t, db.Create(&task).Error)

	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodGet, "/v1/video/generations/legacy-video-app-a", nil)
	context.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	context.Set("id", 42)
	context.Set("token_id", 1002)

	_, taskErr := videoFetchByIDRespBodyBuilder(context)
	require.NotNil(t, taskErr)
	assert.Equal(t, "task_not_exist", taskErr.Code)

	context.Set("token_id", 1001)
	response, taskErr := videoFetchByIDRespBodyBuilder(context)
	require.Nil(t, taskErr)
	assert.Contains(t, string(response), task.TaskID)
}

package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetNativeOpenAIVideoTaskForAppSafelyRecognizesRC23Tasks(t *testing.T) {
	previousDB := DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Task{}))
	DB = db
	t.Cleanup(func() { DB = previousDB })

	tasks := []Task{
		{
			TaskID: "native", UserId: 41, AppID: 1001,
			ClientProtocol: TaskClientProtocolOpenAIVideos,
			Platform:       constant.TaskPlatform("55"),
			Properties:     Properties{OriginModelName: "sora-2"},
		},
		{
			TaskID: "rc23-native", UserId: 41,
			Platform:    constant.TaskPlatform("55"),
			Properties:  Properties{OriginModelName: "sora-2"},
			PrivateData: TaskPrivateData{TokenId: 1001},
		},
		{
			TaskID: "ambiguous", UserId: 41, AppID: 1001,
			Platform: constant.TaskPlatform("55"),
		},
		{
			TaskID: "blank-link", UserId: 41, AppID: 1001,
			Platform:   constant.TaskPlatform("54"),
			Properties: Properties{OriginModelName: VideoSKUSeedance20Oversea},
		},
	}
	for i := range tasks {
		require.NoError(t, db.Session(&gorm.Session{SkipHooks: true}).Create(&tasks[i]).Error)
	}

	task, exists, err := GetNativeOpenAIVideoTaskForApp(41, 1001, "rc23-native")
	require.NoError(t, err)
	require.True(t, exists)
	assert.Equal(t, "rc23-native", task.TaskID)

	for _, taskID := range []string{"native", "rc23-native"} {
		task, visible, lookupErr := GetNativeOpenAIVideoTaskForApp(41, 0, taskID)
		require.NoError(t, lookupErr)
		require.True(t, visible)
		assert.Equal(t, taskID, task.TaskID)
	}

	for _, test := range []struct {
		name   string
		userID int
		appID  int
		taskID string
	}{
		{name: "other app", userID: 41, appID: 1002, taskID: "rc23-native"},
		{name: "other user", userID: 42, appID: 1001, taskID: "rc23-native"},
		{name: "ambiguous model", userID: 41, appID: 1001, taskID: "ambiguous"},
		{name: "registered Link SKU", userID: 41, appID: 1001, taskID: "blank-link"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, visible, lookupErr := GetNativeOpenAIVideoTaskForApp(test.userID, test.appID, test.taskID)
			require.NoError(t, lookupErr)
			assert.False(t, visible)
		})
	}
}

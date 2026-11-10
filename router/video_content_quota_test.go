package router

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoContentQuotaDependsOnOwnedModelArkTask(t *testing.T) {
	previousDB, previousRedis := model.DB, common.RedisEnabled
	previousSQLitePath, previousMaster := common.SQLitePath, common.IsMasterNode
	previousMainDB, previousLogDB := common.MainDatabaseType(), common.LogDatabaseType()
	common.SQLitePath, common.IsMasterNode, common.RedisEnabled = filepath.Join(t.TempDir(), "videos.db"), true, false
	t.Setenv("SQL_DSN", "")
	t.Cleanup(func() {
		model.DB, common.RedisEnabled = previousDB, previousRedis
		common.SQLitePath, common.IsMasterNode = previousSQLitePath, previousMaster
		common.SetDatabaseTypes(previousMainDB, previousLogDB)
	})
	require.NoError(t, model.InitDB())
	db := model.DB
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.Token{}, &model.Task{}))
	require.NoError(t, db.Create(&[]model.User{
		{Id: 91, Username: "video-owner", AffCode: "video91", Status: common.UserStatusEnabled, Group: "default"},
		{Id: 92, Username: "other-video-user", AffCode: "video92", Status: common.UserStatusEnabled, Group: "default"},
	}).Error)
	require.NoError(t, db.Create(&[]model.Token{
		{Id: 11, UserId: 91, Key: "ownerzero", Status: common.TokenStatusEnabled, ExpiredTime: -1},
		{Id: 12, UserId: 91, Key: "otherapp", Status: common.TokenStatusEnabled, ExpiredTime: -1},
		{Id: 13, UserId: 91, Key: "nativequota", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100},
		{Id: 14, UserId: 92, Key: "otheruser", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: 100},
	}).Error)
	require.NoError(t, db.Create(&[]model.Task{
		{
			TaskID: "task_modelark", UserId: 91, AppID: 11,
			ClientProtocol: model.TaskClientProtocolModelArkV3, Status: model.TaskStatusQueued,
		},
		{
			TaskID: "task_native", UserId: 91, AppID: 13, Status: model.TaskStatusSuccess,
			PrivateData: model.TaskPrivateData{ResultURL: "data:video/mp4;base64,ZGF0YQ=="},
		},
	}).Error)

	engine := gin.New()
	SetTaskPluginProtocolRouter(engine)
	SetVideoRouter(engine)
	for _, tc := range []struct {
		name, method, path, key string
		status                  int
		body                    string
	}{
		{"zero balance reaches owned task", http.MethodGet, "/v1/videos/task_modelark/content", "ownerzero", http.StatusBadRequest, "video_not_ready"},
		{"zero balance HEAD reaches owned task", http.MethodHead, "/v1/videos/task_modelark/content", "ownerzero", http.StatusBadRequest, ""},
		{"another app cannot read task", http.MethodGet, "/v1/videos/task_modelark/content", "otherapp", http.StatusNotFound, "video_not_found"},
		{"another account cannot read task", http.MethodGet, "/v1/videos/task_modelark/content", "otheruser", http.StatusNotFound, ""},
		{"missing token is rejected", http.MethodGet, "/v1/videos/task_modelark/content", "", http.StatusUnauthorized, ""},
		{"zero balance cannot create", http.MethodPost, "/api/v3/contents/generations/tasks", "ownerzero", http.StatusUnauthorized, ""},
		{"exhausted token can still read owned task", http.MethodGet, "/v1/videos/task_modelark/content", "ownerzero", http.StatusBadRequest, "video_not_ready"},
		{"exhausted token cannot download native video", http.MethodGet, "/v1/videos/task_native/content", "ownerzero", http.StatusUnauthorized, ""},
		{"zero balance cannot download native video", http.MethodGet, "/v1/videos/task_native/content", "otherapp", http.StatusUnauthorized, ""},
		{"native download with quota works", http.MethodGet, "/v1/videos/task_native/content", "nativequota", http.StatusOK, "data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.key != "" {
				req.Header.Set("Authorization", "Bearer sk-"+tc.key)
			}
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, req)
			require.Equal(t, tc.status, recorder.Code, recorder.Body.String())
			if tc.body != "" {
				assert.Contains(t, recorder.Body.String(), tc.body)
			}
		})
	}
}

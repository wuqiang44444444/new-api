package model

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserContextPreservesUsernameInRequestLogs(t *testing.T) {
	previousDB, previousLogDB := DB, LOG_DB
	previousRedis, previousConsume, previousExport := common.RedisEnabled, common.LogConsumeEnabled, common.DataExportEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.LogConsumeEnabled, common.DataExportEnabled = previousRedis, previousConsume, previousExport
		require.NoError(t, sqlDB.Close())
	})
	DB, LOG_DB = db, db
	common.RedisEnabled, common.LogConsumeEnabled, common.DataExportEnabled = false, true, false
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))
	user := User{Username: "usage-log-user", Group: "default", Status: common.UserStatusEnabled}
	require.NoError(t, db.Create(&user).Error)

	for _, tc := range []struct {
		name    string
		logType int
	}{
		{name: "consume", logType: LogTypeConsume},
		{name: "error", logType: LogTypeError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// API key authentication and channel tests both load this user
			// snapshot and use WriteContext before recording request logs.
			cached, err := GetUserCache(user.Id)
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			cached.WriteContext(c)
			if tc.logType == LogTypeConsume {
				RecordConsumeLog(c, user.Id, RecordConsumeLogParams{
					ModelName: "test-model", TokenName: "test-key", Quota: 10,
					Group: user.Group, Other: NewLogOther(),
				})
			} else {
				RecordErrorLog(c, user.Id, 0, "test-model", "test-key", "test error", 0, 0, false, user.Group, NewLogOther())
			}

			var logs []Log
			require.NoError(t, db.Where("user_id = ? AND type = ?", user.Id, tc.logType).Find(&logs).Error)
			require.Len(t, logs, 1)
			assert.Equal(t, user.Username, logs[0].Username)
		})
	}
}

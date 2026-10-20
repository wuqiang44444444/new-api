package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	logtest "github.com/QuantumNous/new-api/clienterrlog/testutil"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestModelArkInvisibleTaskDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name          string
		userID, appID int
		protocol      string
		deletedAt     int64
		missing       bool
	}{
		{name: "missing", missing: true},
		{name: "other user", userID: 8, appID: 2, protocol: model.TaskClientProtocolModelArkV3},
		{name: "other application", userID: 1, appID: 9, protocol: model.TaskClientProtocolModelArkV3},
		{name: "other protocol", userID: 1, appID: 2, protocol: model.TaskClientProtocolKlingV1},
		{name: "deleted", userID: 1, appID: 2, protocol: model.TaskClientProtocolModelArkV3, deletedAt: 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			previousDB := model.DB
			model.DB = db
			t.Cleanup(func() {
				model.DB = previousDB
				require.NoError(t, sqlDB.Close())
			})
			require.NoError(t, db.AutoMigrate(&model.Task{}))
			if !tc.missing {
				require.NoError(t, db.Create(&model.Task{TaskID: "invisible-task", UserId: tc.userID, AppID: tc.appID, ClientProtocol: tc.protocol, ClientDeletedAt: tc.deletedAt, ChannelId: 999}).Error)
			}
			capture := logtest.New(t)
			engine := gin.New()
			engine.Use(capture.Middleware())
			engine.GET("/api/v3/contents/generations/tasks/:task_id", func(c *gin.Context) {
				c.Set("id", 1)
				c.Set("token_id", 2)
				c.Set(common.RequestIdKey, "lookup-test")
				ModelArkVideoGet(c)
				report, ok := clienterrlog.PeekReport(c.Request.Context())
				require.True(t, ok)
				assert.Equal(t, "task_lookup", report.Stage)
				assert.Equal(t, "task_not_found", report.Reason)
				assert.Equal(t, "task_not_found", report.PublicCode)
				assert.Equal(t, "modelark_v3", report.Protocol)
				assert.Zero(t, report.ChannelID)
				assert.Empty(t, report.Model)
			})
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v3/contents/generations/tasks/invisible-task", nil))
			assert.Equal(t, http.StatusNotFound, response.Code)
			assert.JSONEq(t, `{"error":{"code":"task_not_found","message":"task not found (request id: lookup-test)","request_id":"lookup-test"}}`, response.Body.String())
		})
	}
}

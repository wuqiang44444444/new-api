package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http/httptest"
	"testing"
)

func TestVideoProgressCannotOverrideAuthenticatedApplication(t *testing.T) {
	old := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = old
		s, e := db.DB()
		if e == nil {
			_ = s.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Task{}, &model.TaskCreateAttempt{}, &model.TaskBillingDelivery{}))
	for _, a := range []model.TaskCreateAttempt{
		{AttemptID: "own", PublicTaskID: "task-own", UserID: 51, AppID: 10, ChannelID: 77, ClientProtocol: model.TaskClientProtocolModelArkV3, BillingHoldState: model.TaskCreateAttemptBillingHeld, BillingSource: "wallet", Status: model.TaskCreateAttemptUnknown, HeldQuota: 25},
		{AttemptID: "other-app", PublicTaskID: "task-other-app", UserID: 51, AppID: 20, ClientProtocol: model.TaskClientProtocolModelArkV3, BillingHoldState: model.TaskCreateAttemptBillingHeld},
		{AttemptID: "other-user", PublicTaskID: "task-other-user", UserID: 52, AppID: 10, ClientProtocol: model.TaskClientProtocolModelArkV3, BillingHoldState: model.TaskCreateAttemptBillingHeld},
	} {
		require.NoError(t, db.Create(&a).Error)
	}
	r := gin.New()
	r.GET("/progress", func(c *gin.Context) { c.Set("id", 51); c.Set("token_id", 10); ListTokenVideoFundProgress(c) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/progress?app_id=20&user_id=52", nil))
	require.Equal(t, 200, w.Code)
	var body struct {
		Data struct {
			Items []model.VideoFundProgress `json:"items"`
			Total int64                     `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data.Items, 1)
	assert.Equal(t, "task-own", body.Data.Items[0].TaskID)
	assert.NotContains(t, w.Body.String(), "channel_id")
	assert.NotContains(t, w.Body.String(), "version")
	assert.NotContains(t, w.Body.String(), "other-app")
}

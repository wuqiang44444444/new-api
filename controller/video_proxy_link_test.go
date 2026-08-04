package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestProxyLinkVideoContentOwnsOnlyModelArkTasks(t *testing.T) {
	previousDB := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	newContext := func(appID int) (*gin.Context, *httptest.ResponseRecorder) {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-link/content", nil)
		c.Params = gin.Params{{Key: "task_id", Value: "task-link"}}
		c.Set("id", 501)
		c.Set("token_id", appID)
		return c, recorder
	}

	c, _ := newContext(1001)
	assert.False(t, proxyLinkVideoContent(c), "non-Link tasks must stay outside the Link handler")

	require.NoError(t, db.Create(&model.Task{
		TaskID: "task-link", UserId: 501, AppID: 1001,
		ClientProtocol: model.TaskClientProtocolModelArkV3,
		Status:         model.TaskStatusQueued,
	}).Error)

	c, recorder := newContext(1002)
	assert.True(t, proxyLinkVideoContent(c))
	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "video_not_found")

	c, recorder = newContext(1001)
	assert.True(t, proxyLinkVideoContent(c))
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "video_not_ready")
}

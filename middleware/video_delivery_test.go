package middleware

import (
	"errors"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http/httptest"
	"testing"
)

type closedVideoClient struct{ *httptest.ResponseRecorder }

func (w closedVideoClient) Write(b []byte) (int, error) { return 0, errors.New("client disconnected") }

func TestVideoDeliveryFailureIsRecordedAfterContractBufferWrites(t *testing.T) {
	old := model.DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	t.Cleanup(func() {
		model.DB = old
		sql, err := db.DB()
		if err == nil {
			_ = sql.Close()
		}
	})
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	task := model.Task{TaskID: "delivery-contract", ClientProtocol: model.TaskClientProtocolModelArkV3, Quota: 25, Status: model.TaskStatusSubmitted}
	require.NoError(t, db.Create(&task).Error)
	r := gin.New()
	r.Use(TaskCreateResponseContract())
	r.POST("/video", func(c *gin.Context) {
		defer service.TrackVideoCreateDelivery(c, &task)()
		c.Set(TaskCreateContractResponseKey, gin.H{"id": task.TaskID})
		c.JSON(200, gin.H{"internal": "buffered"})
	})
	r.ServeHTTP(closedVideoClient{httptest.NewRecorder()}, httptest.NewRequest("POST", "/video", nil))
	var saved model.Task
	require.NoError(t, db.First(&saved, task.ID).Error)
	assert.Equal(t, "write_failed", saved.VideoDeliveryState)
	assert.Equal(t, 25, saved.Quota)
	assert.Equal(t, model.TaskStatus(model.TaskStatusSubmitted), saved.Status)
}

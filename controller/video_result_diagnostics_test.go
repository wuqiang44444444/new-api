package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestVideoContentWaitingKeepsResponseAndSkipsOnlyErrorEvents(t *testing.T) {
	endpoint := setupVideoResultDeliveryTest(t)
	now := common.GetTimestamp()
	task := newVideoResultDeliveryTask(t, "task_delivery_diagnostics", now)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	sink := clienterrlog.NewSink(ctx, 32, func(clienterrlog.Event) error { return nil })
	router := gin.New()
	var observed clienterrlog.Report
	router.Use(sink.Middleware())
	auth := func(c *gin.Context) {
		c.Set("route_tag", "relay")
		c.Set("id", task.UserId)
		c.Set("token_id", task.AppID)
		c.Set(common.RequestIdKey, "request-id")
		clienterrlog.MarkAuthPassed(c, clienterrlog.AuthSourceAPIToken)
	}
	content := func(c *gin.Context) {
		VideoProxy(c)
		observed, _ = clienterrlog.PeekReport(c.Request.Context())
	}
	router.GET("/v1/videos/:task_id/content", auth, content)
	router.HEAD("/v1/videos/:task_id/content", auth, content)
	query := func(method, part string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/v1/videos/"+task.TaskID+"/content"+part, nil))
		return response
	}

	for _, state := range []string{"", model.TaskVideoResultStatusPending, model.TaskVideoResultStatusTransferring} {
		if state == model.TaskVideoResultStatusPending {
			require.NoError(t, model.RegisterVideoResultObjects(task, []string{model.TaskVideoResultPartVideo}))
		}
		if state == model.TaskVideoResultStatusTransferring {
			require.NoError(t, model.DB.Model(&model.TaskVideoResultObject{}).Where("task_id = ?", task.TaskID).Update("status", state).Error)
		}
		response := query(http.MethodGet, "")
		require.Equal(t, http.StatusConflict, response.Code)
		want, err := common.Marshal(gin.H{"error": gin.H{
			"code": "video_delivery_preparing", "request_id": "request-id",
			"message": common.MessageWithRequestId("Video result is being prepared for download", "request-id"),
		}})
		require.NoError(t, err)
		assert.Equal(t, string(want), response.Body.String())
		assert.Equal(t, http.StatusConflict, query(http.MethodHead, "").Code)
		assert.Zero(t, sink.Health().Accepted)
	}

	assert.Equal(t, http.StatusBadRequest, query(http.MethodGet, "?part=invalid").Code)
	assert.EqualValues(t, 1, sink.Health().Accepted)

	row := model.DB.Model(&model.TaskVideoResultObject{}).Where("task_id = ?", task.TaskID)
	require.NoError(t, row.Update("status", "unknown-state").Error)
	assert.Equal(t, http.StatusConflict, query(http.MethodGet, "").Code)
	assert.Equal(t, "video_delivery_invalid_object_state", observed.Reason)
	assert.EqualValues(t, 2, sink.Health().Accepted)

	require.NoError(t, row.Updates(map[string]any{"status": model.TaskVideoResultStatusReady,
		"object_key": "videos/tasks/test/video", "storage_endpoint": "http://different-storage.test"}).Error)
	assert.Equal(t, http.StatusServiceUnavailable, query(http.MethodGet, "").Code)
	assert.Equal(t, "video_delivery_signing_failed", observed.Reason)
	assert.EqualValues(t, 3, sink.Health().Accepted)

	require.NoError(t, row.Updates(map[string]any{"storage_backend": "s3", "storage_endpoint": endpoint, "storage_bucket": "video-bucket"}).Error)
	ready := query(http.MethodGet, "")
	assert.Equal(t, http.StatusFound, ready.Code)
	assert.NotEmpty(t, ready.Header().Get("Location"))
	assert.EqualValues(t, 3, sink.Health().Accepted)

	callback := model.DB.Callback().Query()
	require.NoError(t, callback.Before("gorm:query").Register("test:video_result_read_failure", func(tx *gorm.DB) {
		if tx.Statement.Table == (model.TaskVideoResultObject{}).TableName() {
			_ = tx.AddError(errors.New("video result lookup failed"))
		}
	}))
	t.Cleanup(func() { _ = callback.Remove("test:video_result_read_failure") })
	assert.Equal(t, http.StatusInternalServerError, query(http.MethodGet, "").Code)
	assert.Equal(t, "video_delivery_lookup_failed", observed.Reason)
	assert.EqualValues(t, 4, sink.Health().Accepted)
}

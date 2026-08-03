package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCompletedNoReplayReturnsStableConflictCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	c.Set(string(constant.ContextKeyTaskClientProtocol), model.TaskClientProtocolOpenAIImages)

	replayTaskCreateIdempotency(c, &model.TaskCreateIdempotency{
		Status: model.TaskCreateIdempotencyCompletedNoReplay,
	})

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"idempotency_result_unavailable"`)
}

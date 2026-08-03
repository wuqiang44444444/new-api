package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestModelArkUnknownCreateUsesStableContractError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.TaskCreateResponseContract())
	engine.POST("/", func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyTaskClientProtocol, model.TaskClientProtocolModelArkV3)
		common.SetContextKey(c, constant.ContextKeyTaskCreateOutcomeUnknown, true)
		respondTaskError(c, &taskdto.TaskError{
			Code:       "fail_to_fetch_task",
			Message:    "upstream failed",
			StatusCode: http.StatusInternalServerError,
		})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	engine.ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"create_outcome_unknown"`)
	assert.NotContains(t, recorder.Body.String(), "upstream failed")
}

func TestOpenAIImageUnknownCreateUsesStableContractError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{}`))

	writeOpenAITaskCreateOutcomeUnknown(context)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"code":"create_outcome_unknown"`)
}

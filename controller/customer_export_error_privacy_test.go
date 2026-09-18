package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestCustomerExportInternalErrorsDoNotReachClient(t *testing.T) {
	for _, handle := range []func(*gin.Context, error){respondCustomerExportError, respondBillingStatementVersionError} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		handle(c, errors.New("database error containing private query details"))
		assert.Equal(t, http.StatusInternalServerError, recorder.Code)
		assert.NotContains(t, recorder.Body.String(), "private query")
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	respondCustomerExportError(c, service.ErrCustomerExportInvalidRequest)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

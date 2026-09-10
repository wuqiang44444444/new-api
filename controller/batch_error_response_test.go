package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestBatchStorageErrorResponseKeepsClassificationAndHidesCause(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cause  error
		status int
	}{
		{"unconfigured", service.ErrBatchObjectStoreUnavailable, http.StatusServiceUnavailable},
		{"missing", service.ErrBatchObjectNotFound, http.StatusNotFound},
		{"read-failed", errors.New("internal-storage-cause"), http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/files/test/content", nil)
			respondBatchInvalid(c, fmt.Errorf("internal-read-stage: %w", tc.cause))
			assert.Equal(t, tc.status, recorder.Code)
			assert.NotContains(t, recorder.Body.String(), "internal-read-stage")
			assert.NotContains(t, recorder.Body.String(), "internal-storage-cause")
		})
	}
}

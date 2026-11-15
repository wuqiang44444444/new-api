package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestBatchFundingRejectionReturnsClientError(t *testing.T) {
	require.NoError(t, i18n.Init())
	for _, cause := range []error{model.ErrTaskAttemptInsufficientQuota, model.ErrTaskAttemptTokenQuota, model.ErrTaskAttemptSubscriptionUnavailable} {
		t.Run(cause.Error(), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/batches", nil)
			respondBatchInvalid(c, fmt.Errorf("private reservation failure: %w", cause))
			require.Equal(t, http.StatusForbidden, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"code":"insufficient_quota"`)
			assert.NotContains(t, recorder.Body.String(), "private reservation failure")
		})
	}
}

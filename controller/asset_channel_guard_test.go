package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAssetChannelFenceConflictUsesStableHTTPContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	handled := rejectAssetChannelFenceError(c, fmt.Errorf("concurrent mutation: %w", model.ErrChannelHasActiveAssetResources))

	assert.True(t, handled)
	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.Contains(t, recorder.Body.String(), `"error_code":"asset_resources_active"`)
	assert.NotContains(t, recorder.Body.String(), "concurrent mutation")
}

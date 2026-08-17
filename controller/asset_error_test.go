package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteAssetServiceErrorReportsUnsupportedModelAssetLibrary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set(common.RequestIdKey, "req_asset_unsupported")

	writeAssetServiceError(context, service.ErrAssetLibraryUnsupported)

	assert.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	require.NotEmpty(t, recorder.Body.String())
	assert.JSONEq(t, `{
		"error": {
			"message": "asset operation is not supported by this model",
			"type": "asset_error",
			"code": "unsupported_asset_operation",
			"request_id": "req_asset_unsupported"
		}
	}`, recorder.Body.String())
}

func TestWriteAssetServiceErrorReportsUnknownModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Set(common.RequestIdKey, "req_asset_model_missing")

	writeAssetServiceError(context, service.ErrAssetModelNotFound)

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.JSONEq(t, `{
		"error": {
			"message": "model was not found",
			"type": "asset_error",
			"code": "model_not_found",
			"request_id": "req_asset_model_missing"
		}
	}`, recorder.Body.String())
}

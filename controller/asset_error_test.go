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

func TestWriteAssetServiceErrorMapsDefaultGroupConfigurationConflict(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeAssetServiceError(ctx, service.ErrDefaultAssetGroupNotConfigured)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "default_asset_group_not_configured", body.Error.Code)
}

func TestWriteAssetServiceErrorMapsReservedGroupName(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	writeAssetServiceError(ctx, service.ErrReservedAssetGroupName)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &body))
	assert.Equal(t, "reserved_asset_group_name", body.Error.Code)
}

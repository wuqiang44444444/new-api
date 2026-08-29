package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func CreateAsset(c *gin.Context) {
	var req dto.CreateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid asset request")
		return
	}
	response, err := service.CreateRemoteAsset(c.Request.Context(), assetRequestGroup(c), req)
	if err != nil {
		if requiredTTL, ok := service.RequiredAssetURLTTL(err); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
				"message": "asset URL expires before the upstream fetch window",
				"type":    "asset_error", "code": "asset_url_ttl_insufficient",
				"request_id": c.GetString(common.RequestIdKey),
				"details":    gin.H{"required_min_ttl_seconds": requiredTTL},
			}})
			return
		}
		writeAssetServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, response)
}

func GetAsset(c *gin.Context) {
	response, err := service.GetRemoteAsset(
		c.Request.Context(), assetRequestGroup(c), c.Query("model"), c.Param("asset_id"),
	)
	if err != nil {
		writeAssetServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func UpdateAsset(c *gin.Context) {
	var req dto.UpdateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid asset request")
		return
	}
	response, err := service.UpdateRemoteAsset(c.Request.Context(), assetRequestGroup(c), c.Param("asset_id"), req)
	if err != nil {
		writeAssetServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func DeleteAsset(c *gin.Context) {
	if err := service.DeleteRemoteAsset(
		c.Request.Context(), assetRequestGroup(c), c.Query("model"), c.Param("asset_id"),
	); err != nil {
		writeAssetServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func CreateAssetGroup(c *gin.Context) {
	var req dto.CreateAssetGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid asset group request")
		return
	}
	response, err := service.CreateAssetGroup(c.Request.Context(), assetRequestGroup(c), req)
	if err != nil {
		writeAssetServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, response)
}

func GetAssetGroup(c *gin.Context) {
	response, err := service.GetRemoteAssetGroup(
		c.Request.Context(), assetRequestGroup(c), c.Query("model"), c.Param("group_id"),
		strings.EqualFold(strings.TrimSpace(c.Query("verification_session")), "true"),
	)
	if err != nil {
		writeAssetServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, response)
}

func assetRequestGroup(c *gin.Context) string {
	return common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
}

func writeAssetServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidAssetRequest), errors.Is(err, service.ErrAssetURLRequired),
		errors.Is(err, service.ErrUnsafeAssetURL), errors.Is(err, service.ErrAssetURLTTLInsufficient):
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "asset request is invalid")
	case errors.Is(err, service.ErrReservedAssetGroupName):
		assetAPIError(c, http.StatusBadRequest, "reserved_asset_group_name", "asset group name is reserved")
	case errors.Is(err, service.ErrDefaultAssetGroupNotConfigured):
		assetAPIError(c, http.StatusConflict, "default_asset_group_not_configured", "channel default asset group is not configured")
	case errors.Is(err, service.ErrAssetNotFound):
		assetAPIError(c, http.StatusNotFound, "asset_not_found", "asset was not found by the selected model")
	case errors.Is(err, service.ErrAssetModelNotFound):
		assetAPIError(c, http.StatusNotFound, "model_not_found", "model was not found")
	case errors.Is(err, service.ErrUnsupportedAssetType):
		assetAPIError(c, http.StatusUnprocessableEntity, "unsupported_asset_type", "asset type is not supported")
	case errors.Is(err, service.ErrUnsupportedAssetOperation), errors.Is(err, service.ErrAssetLibraryUnsupported):
		assetAPIError(c, http.StatusUnprocessableEntity, "unsupported_asset_operation", "asset operation is not supported by this model")
	case errors.Is(err, service.ErrAssetUpstreamError):
		assetAPIError(c, http.StatusBadGateway, "asset_upstream_error", "asset upstream operation failed")
	case errors.Is(err, service.ErrAssetUpstreamUnavailable), errors.Is(err, service.ErrAssetLibraryUnavailable):
		assetAPIError(c, http.StatusServiceUnavailable, "asset_upstream_unavailable", "asset upstream is unavailable")
	default:
		assetAPIError(c, http.StatusInternalServerError, "internal_error", "asset operation failed")
	}
}

func assetAPIError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{
		"message": message, "type": "asset_error", "code": code,
		"request_id": c.GetString(common.RequestIdKey),
	}})
}

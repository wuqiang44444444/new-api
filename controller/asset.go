package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func CreateAsset(c *gin.Context) {
	var req dto.CreateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid asset request")
		return
	}
	asset, err := service.CreateRemoteAsset(
		c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"),
		common.GetContextKeyString(c, constant.ContextKeyUsingGroup), req,
	)
	if err != nil {
		if requiredTTL, ok := service.RequiredAssetURLTTL(err); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
				"message": "asset URL expires before the Provider fetch window",
				"type":    "asset_error", "code": "asset_url_ttl_insufficient",
				"request_id": c.GetString(common.RequestIdKey),
				"details":    gin.H{"required_min_ttl_seconds": requiredTTL},
			}})
			return
		}
		writeAssetServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, service.AssetResponse(asset))
}

func ListAssets(c *gin.Context) {
	page := positiveQueryInt(c, "page", 1)
	pageSize := positiveQueryInt(c, "page_size", 20)
	if pageSize > 100 {
		pageSize = 100
	}
	assets, total, err := model.ListAssetsByApp(c.GetInt("id"), c.GetInt("token_id"), (page-1)*pageSize, pageSize, model.AssetListFilter{
		Status: strings.TrimSpace(c.Query("status")), AssetKind: strings.TrimSpace(c.Query("asset_kind")),
		MediaType: strings.TrimSpace(c.Query("media_type")), Name: strings.TrimSpace(c.Query("name")),
	})
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to list assets")
		return
	}
	items := make([]dto.AssetResponse, 0, len(assets))
	for i := range assets {
		items = append(items, service.AssetResponse(&assets[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": total, "page": page, "page_size": pageSize})
}

func GetAsset(c *gin.Context) {
	asset := requireOwnedAsset(c)
	if asset == nil {
		return
	}
	if err := service.RefreshAsset(c.Request.Context(), asset); err != nil {
		writeAssetServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, service.AssetResponse(asset))
}

func UpdateAsset(c *gin.Context) {
	var req dto.UpdateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid asset request")
		return
	}
	asset, err := service.RenameAssetForApp(c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"), c.Param("asset_id"), req.Name)
	if err != nil {
		writeAssetServiceError(c, err)
		return
	}
	if asset == nil {
		assetAPIError(c, http.StatusNotFound, "asset_not_found", "asset not found")
		return
	}
	c.JSON(http.StatusOK, service.AssetResponse(asset))
}

func DeleteAsset(c *gin.Context) {
	if requireOwnedAsset(c) == nil {
		return
	}
	if err := service.DeleteAssetForApp(c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"), c.Param("asset_id")); err != nil {
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
	group, verificationURL, err := service.CreateAssetGroup(
		c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"),
		common.GetContextKeyString(c, constant.ContextKeyUsingGroup), req,
	)
	if err != nil {
		writeAssetServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, service.AssetGroupResponse(group, verificationURL))
}

func ListAssetGroups(c *gin.Context) {
	page := positiveQueryInt(c, "page", 1)
	pageSize := positiveQueryInt(c, "page_size", 20)
	if pageSize > 100 {
		pageSize = 100
	}
	groups, total, err := model.ListAssetGroupsByApp(c.GetInt("id"), c.GetInt("token_id"), (page-1)*pageSize, pageSize)
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to list asset groups")
		return
	}
	items := make([]dto.AssetGroupResponse, 0, len(groups))
	for i := range groups {
		items = append(items, service.AssetGroupResponse(&groups[i], ""))
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": total, "page": page, "page_size": pageSize})
}

func GetAssetGroup(c *gin.Context) {
	group, err := model.GetAssetGroupByPublicIDForApp(c.GetInt("id"), c.GetInt("token_id"), c.Param("group_id"))
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to load asset group")
		return
	}
	if group == nil {
		assetAPIError(c, http.StatusNotFound, "asset_group_not_found", "asset group not found")
		return
	}
	verificationURL, err := service.RefreshAssetGroup(c.Request.Context(), group)
	if err != nil {
		writeAssetServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, service.AssetGroupResponse(group, verificationURL))
}

func DeleteAssetGroup(c *gin.Context) {
	group, err := model.GetAssetGroupByPublicIDForApp(c.GetInt("id"), c.GetInt("token_id"), c.Param("group_id"))
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to load asset group")
		return
	}
	if group == nil {
		assetAPIError(c, http.StatusNotFound, "asset_group_not_found", "asset group not found")
		return
	}
	if err := service.DeleteAssetGroupForApp(c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"), group.PublicID); err != nil {
		writeAssetServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func requireOwnedAsset(c *gin.Context) *model.Asset {
	asset, err := model.GetAssetByPublicIDForApp(c.GetInt("id"), c.GetInt("token_id"), c.Param("asset_id"))
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to load asset")
		return nil
	}
	if asset == nil {
		assetAPIError(c, http.StatusNotFound, "asset_not_found", "asset not found")
		return nil
	}
	return asset
}

func positiveQueryInt(c *gin.Context, key string, fallback int) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func writeAssetServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidAssetRequest), errors.Is(err, service.ErrAssetURLRequired),
		errors.Is(err, service.ErrUnsafeAssetURL), errors.Is(err, service.ErrAssetURLTTLInsufficient):
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "asset request is invalid")
	case errors.Is(err, service.ErrAssetNotFound):
		assetAPIError(c, http.StatusNotFound, "asset_not_found", "asset not found")
	case errors.Is(err, service.ErrAssetNotReady), errors.Is(err, service.ErrAssetReferenceUnresolvable),
		errors.Is(err, model.ErrAssetCountLimit):
		assetAPIError(c, http.StatusConflict, "asset_unavailable", "asset is unavailable for this model")
	case errors.Is(err, service.ErrAssetChannelMismatch):
		assetAPIError(c, http.StatusConflict, "asset_channel_mismatch", "asset belongs to a different Seedance model channel")
	case errors.Is(err, service.ErrAssetScopeConflict):
		assetAPIError(c, http.StatusConflict, "asset_scope_conflict", "asset belongs to a different Provider account scope")
	case errors.Is(err, service.ErrUnsupportedAssetType):
		assetAPIError(c, http.StatusUnprocessableEntity, "unsupported_asset_type", "asset type is not supported")
	case errors.Is(err, service.ErrAssetUpstreamError):
		assetAPIError(c, http.StatusBadGateway, "asset_upstream_error", "asset Provider operation failed")
	case errors.Is(err, service.ErrAssetUpstreamUnavailable), errors.Is(err, service.ErrAssetLibraryUnavailable):
		assetAPIError(c, http.StatusServiceUnavailable, "asset_upstream_unavailable", "asset Provider is unavailable")
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

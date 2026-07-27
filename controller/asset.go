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
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	asset, err := service.CreateRemoteAsset(c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"), userGroup, usingGroup, c.GetHeader("Idempotency-Key"), req)
	if err != nil {
		if requiredTTL, ok := service.RequiredAssetURLTTL(err); ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
				"message":    assetServiceMessage(err),
				"type":       "asset_error",
				"code":       "asset_url_ttl_insufficient",
				"request_id": c.GetString(common.RequestIdKey),
				"details":    gin.H{"required_min_ttl_seconds": requiredTTL},
			}})
			return
		}
		assetAPIError(c, assetServiceStatus(err), assetServiceCode(err), assetServiceMessage(err))
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"asset": service.AssetResponse(asset)})
}

func MigrateAsset(c *gin.Context) {
	source := requireOwnedAsset(c)
	if source == nil {
		return
	}
	var req dto.MigrateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid asset migration request")
		return
	}
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	asset, err := service.MigrateRemoteAsset(
		c.Request.Context(),
		c.GetInt("id"),
		c.GetInt("token_id"),
		userGroup,
		usingGroup,
		c.GetHeader("Idempotency-Key"),
		source.PublicID,
		req,
	)
	if err != nil {
		assetAPIError(c, assetServiceStatus(err), assetServiceCode(err), assetServiceMessage(err))
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"asset": service.AssetResponse(asset)})
}

func ListAssets(c *gin.Context) {
	page := positiveQueryInt(c, "page", 1)
	pageSize := positiveQueryInt(c, "page_size", 20)
	if pageSize > 100 {
		pageSize = 100
	}
	assets, total, err := model.ListAssetsByUser(c.GetInt("id"), (page-1)*pageSize, pageSize, model.AssetListFilter{Status: strings.TrimSpace(c.Query("status")), AssetKind: strings.TrimSpace(c.Query("asset_kind")), MediaType: strings.TrimSpace(c.Query("media_type")), Name: strings.TrimSpace(c.Query("name"))})
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
	c.JSON(http.StatusOK, service.AssetResponse(asset))
}

func UpdateAsset(c *gin.Context) {
	var req dto.UpdateAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid asset request")
		return
	}
	asset, err := service.RenameAsset(c.GetInt("id"), c.Param("asset_id"), req.Name)
	if err != nil {
		if errors.Is(err, service.ErrInvalidAssetRequest) {
			assetAPIError(c, http.StatusBadRequest, "invalid_request", "asset name could not be updated")
		} else {
			assetAPIError(c, http.StatusInternalServerError, "database_error", "asset name could not be updated")
		}
		return
	}
	if asset == nil {
		assetAPIError(c, http.StatusNotFound, "asset_not_found", "asset not found")
		return
	}
	c.JSON(http.StatusOK, service.AssetResponse(asset))
}

func DeleteAsset(c *gin.Context) {
	asset := requireOwnedAsset(c)
	if asset == nil {
		return
	}
	if err := service.DeleteAsset(c.Request.Context(), c.GetInt("id"), asset.PublicID); err != nil {
		assetAPIError(c, http.StatusInternalServerError, "asset_delete_failed", "failed to schedule asset deletion")
		return
	}
	c.Status(http.StatusNoContent)
}

func ListAssetBindings(c *gin.Context) {
	asset := requireOwnedAsset(c)
	if asset == nil {
		return
	}
	bindings, err := model.ListAssetBindings(c.GetInt("id"), asset.ID)
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to list asset bindings")
		return
	}
	items := make([]dto.AssetBindingResponse, 0, len(bindings))
	for i := range bindings {
		items = append(items, service.AssetBindingResponse(asset, &bindings[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func requireOwnedAsset(c *gin.Context) *model.Asset {
	asset, err := model.GetAssetByPublicID(c.GetInt("id"), c.Param("asset_id"))
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

func assetAPIError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{
		"message":    message,
		"type":       "asset_error",
		"code":       code,
		"request_id": c.GetString(common.RequestIdKey),
	}})
}

func assetServiceStatus(err error) int {
	switch {
	case errors.Is(err, service.ErrInvalidAssetRequest), errors.Is(err, service.ErrAssetBindingInvalidRequest),
		errors.Is(err, service.ErrAssetURLRequired), errors.Is(err, service.ErrUnsafeAssetURL), errors.Is(err, service.ErrAssetURLTTLInsufficient):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrRealPersonAuthorizationNotReady), errors.Is(err, service.ErrRealPersonVerificationRejected):
		return http.StatusForbidden
	case errors.Is(err, service.ErrUnsupportedAssetBindingTarget), errors.Is(err, service.ErrUnsupportedAssetType):
		return http.StatusUnprocessableEntity
	case errors.Is(err, service.ErrIdempotencyConflict), errors.Is(err, service.ErrAssetBindingRequired),
		errors.Is(err, service.ErrAssetCredentialChanged), errors.Is(err, model.ErrAssetCountLimit):
		return http.StatusConflict
	case errors.Is(err, service.ErrAssetUpstreamError):
		return http.StatusBadGateway
	case errors.Is(err, service.ErrAssetUpstreamUnavailable), errors.Is(err, service.ErrAssetLibraryUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

func assetServiceCode(err error) string {
	switch {
	case errors.Is(err, service.ErrInvalidAssetRequest):
		return "invalid_request"
	case errors.Is(err, service.ErrAssetBindingInvalidRequest):
		return "invalid_asset_binding_request"
	case errors.Is(err, service.ErrAssetURLRequired):
		return "asset_url_required"
	case errors.Is(err, service.ErrUnsafeAssetURL):
		return "unsafe_asset_url"
	case errors.Is(err, service.ErrAssetURLTTLInsufficient):
		return "asset_url_ttl_insufficient"
	case errors.Is(err, service.ErrIdempotencyConflict):
		return "idempotency_conflict"
	case errors.Is(err, service.ErrRealPersonAuthorizationNotReady):
		return "real_person_consent_required"
	case errors.Is(err, service.ErrRealPersonVerificationRejected):
		return "real_person_verification_rejected"
	case errors.Is(err, service.ErrUnsupportedAssetBindingTarget):
		return "unsupported_asset_binding_target"
	case errors.Is(err, service.ErrUnsupportedAssetType):
		return "unsupported_asset_type"
	case errors.Is(err, service.ErrAssetBindingRequired):
		return "asset_binding_required"
	case errors.Is(err, service.ErrAssetCredentialChanged):
		return "asset_credential_changed"
	case errors.Is(err, model.ErrAssetCountLimit):
		return "asset_limit_reached"
	case errors.Is(err, service.ErrAssetUpstreamError):
		return "asset_upstream_error"
	case errors.Is(err, service.ErrAssetUpstreamUnavailable), errors.Is(err, service.ErrAssetLibraryUnavailable):
		return "asset_upstream_unavailable"
	}
	return "internal_error"
}

func assetServiceMessage(err error) string {
	switch {
	case errors.Is(err, service.ErrInvalidAssetRequest), errors.Is(err, service.ErrAssetBindingInvalidRequest):
		return "asset request is invalid"
	case errors.Is(err, service.ErrAssetURLRequired):
		return "remote assets require a Provider-reachable HTTPS URL"
	case errors.Is(err, service.ErrUnsafeAssetURL):
		return "asset URL is not allowed"
	case errors.Is(err, service.ErrAssetURLTTLInsufficient):
		return "asset URL expires before the Provider fetch window"
	case errors.Is(err, service.ErrIdempotencyConflict):
		return "idempotency key was already used for a different request"
	case errors.Is(err, service.ErrRealPersonAuthorizationNotReady):
		return "real-person authorization is not active"
	case errors.Is(err, service.ErrRealPersonVerificationRejected):
		return "real-person verification was rejected"
	case errors.Is(err, service.ErrUnsupportedAssetBindingTarget), errors.Is(err, service.ErrUnsupportedAssetType):
		return "asset type or binding target is not supported"
	case errors.Is(err, service.ErrAssetBindingRequired):
		return "no compatible asset binding is available"
	case errors.Is(err, service.ErrAssetCredentialChanged):
		return "asset channel credential has changed"
	case errors.Is(err, model.ErrAssetCountLimit):
		return "asset limit has been reached"
	case errors.Is(err, service.ErrAssetUpstreamError):
		return "upstream asset creation was rejected"
	case errors.Is(err, service.ErrAssetUpstreamUnavailable), errors.Is(err, service.ErrAssetLibraryUnavailable):
		return "asset upstream is unavailable"
	}
	return "asset request could not be processed"
}

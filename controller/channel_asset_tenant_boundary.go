package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	assetTenantBoundaryImmutableCode      = "asset_tenant_boundary_immutable"
	assetTenantReplacementUnconfirmedCode = "asset_tenant_replacement_unconfirmed"
	assetTenantRotationUnconfirmedCode    = "asset_tenant_rotation_unconfirmed"
)

func respondAssetTenantMutationError(c *gin.Context, err error) bool {
	errorCode := ""
	switch {
	case errors.Is(err, model.ErrAssetTenantBoundaryImmutable):
		errorCode = assetTenantBoundaryImmutableCode
	case errors.Is(err, model.ErrAssetTenantReplacementUnconfirmed):
		errorCode = assetTenantReplacementUnconfirmedCode
	case errors.Is(err, model.ErrAssetTenantRotationUnconfirmed):
		errorCode = assetTenantRotationUnconfirmedCode
	default:
		return false
	}
	payload := gin.H{
		"success":    false,
		"message":    err.Error(),
		"error_code": errorCode,
	}
	if changedFields := model.AssetTenantReplacementChangedFields(err); len(changedFields) > 0 {
		payload["changed_fields"] = changedFields
	}
	c.JSON(http.StatusConflict, payload)
	return true
}

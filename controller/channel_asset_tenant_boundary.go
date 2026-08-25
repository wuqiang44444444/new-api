package controller

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	assetTenantBoundaryImmutableCode   = "asset_tenant_boundary_immutable"
	assetTenantRotationUnconfirmedCode = "asset_tenant_rotation_unconfirmed"
)

func respondAssetTenantMutationError(c *gin.Context, err error) bool {
	errorCode := ""
	switch {
	case errors.Is(err, model.ErrAssetTenantBoundaryImmutable):
		errorCode = assetTenantBoundaryImmutableCode
	case errors.Is(err, model.ErrAssetTenantRotationUnconfirmed):
		errorCode = assetTenantRotationUnconfirmedCode
	default:
		return false
	}
	c.JSON(http.StatusConflict, gin.H{
		"success":    false,
		"message":    err.Error(),
		"error_code": errorCode,
	})
	return true
}

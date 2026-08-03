package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func CreateRealPersonAuthorization(c *gin.Context) {
	var req dto.CreateRealPersonAuthorizationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		assetAPIError(c, http.StatusBadRequest, "invalid_request", "invalid authorization request")
		return
	}
	usingGroup := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	userGroup := common.GetContextKeyString(c, constant.ContextKeyUserGroup)
	authorization, verificationURL, err := service.CreateRealPersonAuthorization(c.Request.Context(), c.GetInt("id"), c.GetInt("token_id"), userGroup, usingGroup, req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAPIServiceRuleNotAccepted):
			assetAPIError(c, http.StatusPreconditionRequired, "api_service_rule_not_accepted", "accept the current API service rule before using real-person capabilities")
		case errors.Is(err, service.ErrAPIServiceRuleUnavailable):
			assetAPIError(c, http.StatusServiceUnavailable, "api_service_rule_unavailable", "API service rule is not configured")
		case errors.Is(err, service.ErrInvalidEndUserSubject):
			assetAPIError(c, http.StatusBadRequest, "invalid_end_user_subject", err.Error())
		case errors.Is(err, service.ErrRealPersonVerificationUpstream):
			if authorization != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{
					"message": "verification session could not be created", "type": "asset_error",
					"code": "verification_upstream_error", "request_id": c.GetString(common.RequestIdKey),
					"details": gin.H{"authorization_id": authorization.PublicID, "status": realPersonAuthorizationResponse(authorization).Status},
				}})
				return
			}
			assetAPIError(c, http.StatusBadGateway, "verification_upstream_error", "verification session could not be created")
		default:
			assetAPIError(c, http.StatusServiceUnavailable, "real_person_authorization_unavailable", "real-person authorization is unavailable")
		}
		return
	}
	response := realPersonAuthorizationResponse(authorization)
	response.VerificationURL = verificationURL
	c.JSON(http.StatusCreated, response)
}

func GetRealPersonAuthorization(c *gin.Context) {
	authorization, err := model.GetRealPersonAuthorizationForApp(c.GetInt("id"), c.GetInt("token_id"), c.Param("authorization_id"))
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to load authorization")
		return
	}
	if authorization == nil {
		assetAPIError(c, http.StatusNotFound, "authorization_not_found", "authorization not found")
		return
	}
	if err := service.RefreshRealPersonVerification(c.Request.Context(), authorization); err != nil {
		assetAPIError(c, http.StatusBadGateway, "verification_status_unavailable", "verification status is temporarily unavailable")
		return
	}
	c.JSON(http.StatusOK, realPersonAuthorizationResponse(authorization))
}

func RevokeRealPersonAuthorization(c *gin.Context) {
	authorization, err := model.GetRealPersonAuthorizationForApp(c.GetInt("id"), c.GetInt("token_id"), c.Param("authorization_id"))
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to load authorization")
		return
	}
	if authorization == nil {
		assetAPIError(c, http.StatusNotFound, "authorization_not_found", "authorization not found")
		return
	}
	if err := service.RevokeRealPersonAuthorization(c.Request.Context(), authorization); err != nil {
		assetAPIError(c, http.StatusInternalServerError, "authorization_revoke_failed", "failed to revoke authorization")
		return
	}
	c.JSON(http.StatusOK, realPersonAuthorizationResponse(authorization))
}

func RetryRealPersonAuthorization(c *gin.Context) {
	authorization, err := model.GetRealPersonAuthorizationForApp(c.GetInt("id"), c.GetInt("token_id"), c.Param("authorization_id"))
	if err != nil {
		assetAPIError(c, http.StatusInternalServerError, "database_error", "failed to load authorization")
		return
	}
	if authorization == nil {
		assetAPIError(c, http.StatusNotFound, "authorization_not_found", "authorization not found")
		return
	}
	h5URL, err := service.RetryRealPersonVerification(c.Request.Context(), authorization)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrRealPersonAuthorizationNotRetryable):
			assetAPIError(c, http.StatusConflict, "authorization_not_retryable", "authorization cannot be retried")
		case errors.Is(err, service.ErrRealPersonVerificationUpstream):
			assetAPIError(c, http.StatusBadGateway, "verification_upstream_error", "verification session could not be created")
		default:
			assetAPIError(c, http.StatusInternalServerError, "database_error", "verification session could not be created")
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"authorization": realPersonAuthorizationResponse(authorization), "verification_url": h5URL})
}

func CompleteRealPersonVerification(c *gin.Context) {
	verificationResponseHeaders(c)
	publicID := strings.TrimSpace(c.Query("authorization_id"))
	var authorization model.RealPersonAuthorization
	if publicID == "" || model.DB.Where("public_id = ?", publicID).First(&authorization).Error != nil {
		c.String(http.StatusNotFound, "authorization not found")
		return
	}
	_ = service.RefreshRealPersonVerification(c.Request.Context(), &authorization)
	c.String(http.StatusOK, "Verification status: %s. The API client can query the authorization status.", authorization.Status)
}

func OpenRealPersonVerification(c *gin.Context) {
	verificationResponseHeaders(c)
	h5URL, err := service.OpenRealPersonVerification(c.Request.Context(), c.Param("token"))
	if err != nil {
		c.String(http.StatusGone, "This verification link is invalid or expired.")
		return
	}
	c.Redirect(http.StatusSeeOther, h5URL)
}

func CheckRealPersonVerification(c *gin.Context) {
	verificationResponseHeaders(c)
	if _, err := service.OpenRealPersonVerification(c.Request.Context(), c.Param("token")); err != nil {
		c.Status(http.StatusGone)
		return
	}
	c.Status(http.StatusNoContent)
}

func realPersonAuthorizationResponse(authorization *model.RealPersonAuthorization) dto.RealPersonAuthorizationResponse {
	status := authorization.Status
	if status == model.RealPersonAuthorizationDeleting || status == model.RealPersonAuthorizationDeleted {
		status = model.RealPersonAuthorizationRevoked
	}
	response := dto.RealPersonAuthorizationResponse{ID: authorization.PublicID, Status: status, ErrorCode: authorization.ErrorCode, CleanupStatus: authorization.CleanupStatus, CreatedAt: authorization.CreatedAt, UpdatedAt: authorization.UpdatedAt}
	if authorization.ID > 0 {
		if session, err := model.GetLatestRealPersonVerificationSession(authorization.ID); err == nil && session != nil {
			response.ExpiresAt = session.ExpiresAt
		}
	}
	return response
}

func verificationResponseHeaders(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
}

package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	securityProofScopeTaskContractAttemptRecover  = "task_contract.attempt.recover"
	securityProofScopeTaskContractAttemptReject   = "task_contract.attempt.reject"
	securityProofScopeTaskContractExposureResolve = "task_contract.exposure.resolve"
)

func GetProviderExposureMetrics(c *gin.Context) {
	windowSeconds, _ := strconv.ParseInt(c.Query("window_seconds"), 10, 64)
	result, err := service.GetProviderExposureMetrics(windowSeconds)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

func ListProviderExposureIncidents(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	incidents, err := model.ListProviderExposureIncidents(c.Query("status"), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": incidents})
}

func ListTaskCreateAttemptsForRecovery(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	attempts, err := model.ListTaskCreateAttemptsForRecovery(
		model.TaskCreateAttemptStatus(strings.TrimSpace(c.Query("status"))),
		limit,
		offset,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": attempts})
}

type providerExposureResolutionRequest struct {
	Note               string `json:"note"`
	RestorePublicModel bool   `json:"restore_public_model"`
}

func ResolveProviderExposureIncident(c *gin.Context) {
	if !middleware.RequireSecurityProof(
		c,
		securityProofScopeTaskContractExposureResolve,
		[]string{"2fa", "passkey"},
	) {
		return
	}
	incidentID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || incidentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid incident id"})
		return
	}
	request := providerExposureResolutionRequest{}
	if err := c.ShouldBindJSON(&request); err != nil ||
		strings.TrimSpace(request.Note) == "" || len(request.Note) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "a resolution note is required"})
		return
	}
	result, err := service.ResolveProviderExposureIncident(
		incidentID,
		c.GetInt("id"),
		request.Note,
		request.RestorePublicModel,
	)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "task_contract.exposure_resolve", map[string]interface{}{
		"incident_id":          incidentID,
		"restore_public_model": request.RestorePublicModel,
		"restored":             result.Restored,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

type taskCreateAttemptRecoveryRequest struct {
	UpstreamTaskID    string `json:"upstream_task_id"`
	UpstreamRequestID string `json:"upstream_request_id"`
	ProviderVerified  bool   `json:"provider_verified"`
	Note              string `json:"note"`
}

func RecoverTaskCreateAttempt(c *gin.Context) {
	if !middleware.RequireSecurityProof(
		c,
		securityProofScopeTaskContractAttemptRecover,
		[]string{"2fa", "passkey"},
	) {
		return
	}
	attemptID := strings.TrimSpace(c.Param("attempt_id"))
	request := taskCreateAttemptRecoveryRequest{}
	if attemptID == "" || len(attemptID) > 64 || c.ShouldBindJSON(&request) != nil ||
		!request.ProviderVerified || strings.TrimSpace(request.Note) == "" ||
		len(request.Note) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "attempt id, provider verification, upstream task id, and audit note are required",
		})
		return
	}
	result, err := service.RecoverUnknownTaskCreateAttempt(
		attemptID,
		request.UpstreamTaskID,
		request.UpstreamRequestID,
		request.ProviderVerified,
		c.GetInt("id"),
		request.Note,
	)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "task_contract.attempt_recover", map[string]interface{}{
		"attempt_id":     attemptID,
		"public_task_id": result.PublicTaskID,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

type taskCreateAttemptRejectionRequest struct {
	ProviderVerified bool   `json:"provider_verified"`
	Note             string `json:"note"`
}

func RejectTaskCreateAttempt(c *gin.Context) {
	if !middleware.RequireSecurityProof(
		c,
		securityProofScopeTaskContractAttemptReject,
		[]string{"2fa", "passkey"},
	) {
		return
	}
	attemptID := strings.TrimSpace(c.Param("attempt_id"))
	request := taskCreateAttemptRejectionRequest{}
	if attemptID == "" || len(attemptID) > 64 || c.ShouldBindJSON(&request) != nil ||
		!request.ProviderVerified || strings.TrimSpace(request.Note) == "" ||
		len(request.Note) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "attempt id, provider verification, and audit note are required",
		})
		return
	}
	result, err := service.RejectUnknownTaskCreateAttempt(
		attemptID,
		request.ProviderVerified,
		c.GetInt("id"),
		request.Note,
	)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "task_contract.attempt_reject", map[string]interface{}{
		"attempt_id":     attemptID,
		"public_task_id": result.PublicTaskID,
		"released_quota": result.ReleasedQuota,
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

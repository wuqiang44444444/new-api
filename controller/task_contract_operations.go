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
	securityProofScopeTaskContractAttemptRecover = "task_contract.attempt.recover"
	securityProofScopeTaskContractAttemptReject  = "task_contract.attempt.reject"
)

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

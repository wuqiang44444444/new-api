package controller

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const securityProofScopeTaskUsageReview = "task_contract.usage.review"

func ListTaskUsageRecovery(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, offsetErr := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offsetErr != nil || limit < 1 || limit > 100 || offset < 0 {
		c.JSON(400, gin.H{"success": false, "message": "invalid pagination"})
		return
	}
	items, err := model.ListTaskUsageRecovery(c.Query("review_only") == "true", offset, limit)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "message": "unable to list usage recovery tasks"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

func ReviewTaskUsage(c *gin.Context) {
	if !middleware.RequireSecurityProof(c, securityProofScopeTaskUsageReview, []string{"2fa", "passkey"}) {
		return
	}
	var request struct {
		CompletionTokens *int   `json:"completion_tokens"`
		ProviderVerified bool   `json:"provider_verified"`
		Reference        string `json:"reference"`
	}
	if c.ShouldBindJSON(&request) != nil || (request.CompletionTokens != nil && !request.ProviderVerified) {
		c.JSON(400, gin.H{"success": false, "message": "verified provider usage is required"})
		return
	}
	task, err := model.ReviewTaskUsage(c.Param("task_id"), c.GetInt("id"), request.Reference, request.CompletionTokens)
	if err != nil {
		c.JSON(409, gin.H{"success": false, "message": err.Error()})
		return
	}
	recordManageAudit(c, "task_contract.usage_review", map[string]interface{}{"task_id": task.TaskID, "operator_id": c.GetInt("id"), "reference": strings.TrimSpace(request.Reference), "completion_tokens": request.CompletionTokens})
	// Known usage is durable pending work; the existing funding worker settles it.
	c.JSON(200, gin.H{"success": true, "data": gin.H{"task_id": task.TaskID, "billing_state": task.BillingState}})
}

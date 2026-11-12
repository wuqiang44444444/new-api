package controller

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetAdminBillingSourceReview(c *gin.Context) {
	user, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || user <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false})
		return
	}
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	started := time.Now()
	report, err := model.GetBillingSourceReview(c.Request.Context(), user, period.StartTimestamp, period.EndTimestamp)
	if err != nil {
		respondBillingSourceReviewError(c, err, time.Since(started))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": report})
}
func PostAdminBillingSourceReview(c *gin.Context) {
	if c.GetInt("role") < common.RoleRootUser {
		c.JSON(http.StatusForbidden, gin.H{"success": false})
		return
	}
	user, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || user <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false})
		return
	}
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	var input model.BillingSourceReviewDecision
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false})
		return
	}
	started := time.Now()
	if err := model.RecordBillingSourceReviewDecision(c.Request.Context(), user, period.StartTimestamp, period.EndTimestamp, input, c.GetInt("id")); err != nil {
		respondBillingSourceReviewError(c, err, time.Since(started))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "decision_version": model.BillingSourceDecisionVersion})
}

func respondBillingSourceReviewError(c *gin.Context, err error, elapsed time.Duration) {
	status, reason := http.StatusInternalServerError, "read_failed"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		status, reason = http.StatusGatewayTimeout, "timeout"
	case errors.Is(err, context.Canceled):
		return
	case errors.Is(err, model.ErrBillingSourceReviewBudget):
		status, reason = http.StatusUnprocessableEntity, "limit_exceeded"
	case errors.Is(err, model.ErrBillingStatementMaintenanceActive):
		status, reason = http.StatusServiceUnavailable, "maintenance"
	case errors.Is(err, model.ErrBillingStatementVersionTopology):
		status, reason = http.StatusConflict, "unsupported_topology"
	case errors.Is(err, model.ErrBillingStatementVersionConflict):
		status, reason = http.StatusConflict, "conflict"
	case errors.Is(err, model.ErrBillingStatementSourceIncomplete):
		status, reason = http.StatusConflict, "decision_rejected"
	}
	logger.LogWarn(c.Request.Context(), "billing source review failed: reason=%s elapsed_ms=%d", reason, elapsed.Milliseconds())
	c.JSON(status, gin.H{"success": false, "reason": reason, "request_id": c.GetString(common.RequestIdKey)})
}

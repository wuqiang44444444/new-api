package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
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
	report, err := model.GetBillingSourceReview(c.Request.Context(), user, period.StartTimestamp, period.EndTimestamp)
	if err != nil {
		respondBillingStatementVersionError(c, err)
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
	if err := model.RecordBillingSourceReviewDecision(c.Request.Context(), user, period.StartTimestamp, period.EndTimestamp, input, c.GetInt("id")); err != nil {
		respondBillingStatementVersionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "decision_version": model.BillingSourceDecisionVersion})
}

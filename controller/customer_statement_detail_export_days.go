package controller

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func GetSelfCustomerStatementDetailExportDays(c *gin.Context) {
	respondCustomerStatementDetailExportDays(c, c.GetInt("id"), false)
}

func GetAdminCustomerStatementDetailExportDays(c *gin.Context) {
	userID := parsePositiveQueryId(c, "user_id")
	if userID < 0 {
		return
	}
	if userID == 0 {
		common.ApiErrorMsg(c, "invalid user_id")
		return
	}
	respondCustomerStatementDetailExportDays(c, userID, true)
}

func respondCustomerStatementDetailExportDays(c *gin.Context, userID int, allowPending bool) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	days, err := service.CustomerStatementDetailExportDays(ctx, c.GetInt("id"), userID,
		period.StartTimestamp, period.EndTimestamp+1, c.Query("report_version"), c.Query("statement_version"), allowPending)
	if err != nil {
		respondCustomerExportError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"days": days})
}

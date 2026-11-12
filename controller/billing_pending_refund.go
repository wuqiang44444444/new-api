package controller

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// The status is current even on a confirmed statement. It is separate from
// published amounts and never changes a statement or its downloadable totals.
func addCustomerBillingRefundStatus(c *gin.Context, response gin.H, userID int, period billingReconciliationPeriod) {
	if c.Query("include_pending_refunds") != "true" || userID <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), time.Second)
	defer cancel()
	count, err := model.CountOverdueBillingRefunds(ctx, userID, period.StartTimestamp, period.EndTimestamp, common.GetTimestamp())
	if err == nil && count > 0 {
		response["pending_refund_count"] = count
	}
}

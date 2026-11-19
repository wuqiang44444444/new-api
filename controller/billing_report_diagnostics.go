package controller

import (
	"context"
	"strconv"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetBillingReportDiagnostics(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if day := c.Query("date"); day != "" {
		date, err := time.ParseInLocation(time.DateOnly, day, model.UsageAnalyticsZone())
		if err != nil || date.Unix() <= 0 {
			common.ApiErrorMsg(c, "Invalid billing report date")
			return
		}
		after, _ := strconv.ParseInt(c.Query("after"), 10, 64)
		rows, err := model.BillingReportDiagnosticLines(ctx, date.Unix(), max(after, 0))
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, rows)
		return
	}
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	rows, err := model.BillingReportDiagnosticDays(ctx, period.StartTimestamp, period.EndTimestamp+1)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rows)
}

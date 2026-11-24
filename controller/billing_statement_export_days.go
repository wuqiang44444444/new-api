package controller

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func GetAdminCustomerStatementExportDays(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	if !model.DailyReportSourceSupported() {
		respondCustomerExportError(c, model.ErrCustomerExportBackendUnsupported)
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	ctx, report, err := model.ResolveBillingReport(ctx, period.StartTimestamp, period.EndTimestamp, "")
	if err != nil {
		respondCustomerExportError(c, err)
		return
	}
	calendar, err := report.CustomerStatementExportCalendar(ctx, period.StartTimestamp, period.EndTimestamp+1)
	if err != nil {
		respondCustomerExportError(c, err)
		return
	}

	month := time.Unix(period.StartTimestamp, 0).In(billingSettlementLocation)
	now := time.Now().In(billingSettlementLocation)
	if calendar.DefaultDate == "" && month.Year() == now.Year() && month.Month() == now.Month() {
		previous := month.AddDate(0, -1, 0)
		previousCtx, previousReport, err := model.ResolveBillingReport(ctx, previous.Unix(), month.Unix()-1, "")
		if err != nil {
			respondCustomerExportError(c, err)
			return
		}
		previousCalendar, err := previousReport.CustomerStatementExportCalendar(previousCtx, previous.Unix(), month.Unix())
		if err != nil {
			respondCustomerExportError(c, err)
			return
		}
		calendar.Days = append(calendar.Days, previousCalendar.Days...)
		calendar.DefaultDate = previousCalendar.DefaultDate
	}
	common.ApiSuccess(c, calendar)
}

package controller

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func bindBillingReport(c *gin.Context, period billingReconciliationPeriod) (context.CancelFunc, bool) {
	if !model.DailyReportSourceSupported() {
		common.ApiErrorMsg(c, "Billing report source is unsupported")
		return func() {}, false
	}
	version := c.Query("report_version")
	if len(version) > 1024 {
		common.ApiErrorMsg(c, "Invalid billing report version")
		return func() {}, false
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	reportCtx, _, err := model.ResolveBillingReport(ctx, period.StartTimestamp, period.EndTimestamp, version)
	if err != nil {
		cancel()
		common.ApiError(c, err)
		return func() {}, false
	}
	c.Request = c.Request.WithContext(reportCtx)
	return cancel, true
}

func bindUpstreamBillingReport(c *gin.Context, period billingReconciliationPeriod) (context.CancelFunc, bool) {
	if model.DailyReportSourceSupported() {
		return bindBillingReport(c, period)
	}
	if c.Query("report_version") != "" {
		common.ApiErrorMsg(c, "Billing report source is unsupported")
		return func() {}, false
	}
	return func() {}, true
}

func validateUpstreamReportURL(c *gin.Context, urlKey string) bool {
	switch {
	case urlKey == "":
		return true
	case len(urlKey) > 2048:
		common.ApiErrorT(c, "The upstream URL filter exceeds 2048 bytes")
	case !model.DailyReportSourceSupported():
		common.ApiErrorT(c, "Use channel_id instead of url_key for this log source")
	case c.Query("report_version") == "":
		common.ApiErrorT(c, "A report version is required for upstream queries filtered by URL")
	default:
		return true
	}
	return false
}

func PostBillingReportRebuild(c *gin.Context) {
	if !model.DailyReportSourceSupported() {
		common.ApiErrorMsg(c, "Billing report source is unsupported")
		return
	}
	var body struct {
		Date string `json:"date"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		common.ApiErrorMsg(c, "Invalid billing report date")
		return
	}
	if err := model.RequestBillingReportRebuild(c.Request.Context(), body.Date); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

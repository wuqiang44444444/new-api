package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const maxBillingURLKeyFilterLength = 2048

// GetAdminUpstreamBillingURLReconciliation serves the admin-only upstream URL
// grouping projection. The grouping key is each channel's currently
// configured base URL; it reports what the channel configuration says now,
// proves nothing about the URL at request time, and must never feed routing,
// billing or settlement.
func GetAdminUpstreamBillingURLReconciliation(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	urlKey := strings.TrimSpace(c.Query("url_key"))
	if len(urlKey) > maxBillingURLKeyFilterLength {
		common.ApiErrorMsg(c, "invalid url_key")
		return
	}
	summary, err := model.GetProviderBillingURLSummary(period.StartTimestamp, period.EndTimestamp, urlKey)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondBillingReconciliation(c, period, gin.H{"url_key": urlKey}, summary, "main_database+log_database")
}

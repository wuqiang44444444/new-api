package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// GetAdminUpstreamBillingDetails serves the cross-customer evidence rows
// behind the upstream reconciliation summary. Admin-only; the upstream task
// id stays root-scoped like the rest of the platform.
func GetAdminUpstreamBillingDetails(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	page, pageSize, valid := parseBillingPage(c)
	if !valid {
		return
	}
	filter := model.UpstreamBillingDetailFilter{
		Start: period.StartTimestamp, End: period.EndTimestamp,
		EvidenceFilter:    strings.TrimSpace(c.Query("evidence_filter")),
		ProviderModel:     strings.TrimSpace(c.Query("model_name")),
		BillingMode:       strings.TrimSpace(c.Query("billing_mode")),
		RequestId:         strings.TrimSpace(c.Query("request_id")),
		UpstreamRequestId: strings.TrimSpace(c.Query("upstream_request_id")),
	}
	if err := model.ValidateUpstreamEvidenceFilter(filter.EvidenceFilter); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(filter.ProviderModel) > 255 || len(filter.RequestId) > 64 || len(filter.UpstreamRequestId) > 128 {
		common.ApiErrorMsg(c, "invalid filter")
		return
	}
	if filter.BillingMode != "" && !model.ValidBillingReconciliationMode(filter.BillingMode) {
		common.ApiErrorMsg(c, "invalid billing_mode")
		return
	}
	if value, present := c.GetQuery("provider_model_fallback"); present {
		fallback, err := strconv.ParseBool(value)
		if err != nil || filter.ProviderModel == "" {
			common.ApiErrorMsg(c, "invalid provider_model_fallback")
			return
		}
		filter.ProviderModelFallback = &fallback
	}
	channelId := parsePositiveQueryId(c, "channel_id")
	if channelId < 0 {
		return
	}
	urlKey := strings.TrimSpace(c.Query("url_key"))
	if !validateUpstreamReportURL(c, urlKey) {
		return
	}
	if urlKey != "" && channelId <= 0 {
		common.ApiErrorT(c, "A channel ID is required for upstream records filtered by URL")
		return
	}
	filter.URLKey = urlKey
	if channelId > 0 {
		filter.ChannelIds = []int{channelId}
	}
	if len(filter.ChannelIds) == 0 && filter.EvidenceFilter == "" {
		common.ApiErrorMsg(c, "upstream details require a channel or evidence filter")
		return
	}
	cancelReport, ok := bindUpstreamBillingReport(c, period)
	if !ok {
		return
	}
	defer cancelReport()
	details, err := model.GetUpstreamBillingDetails(c.Request.Context(), filter, page, pageSize, c.GetInt("role") >= common.RoleRootUser)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondBillingReconciliation(c, period, gin.H{
		"channel_id":      channelId,
		"evidence_filter": filter.EvidenceFilter, "model_name": filter.ProviderModel, "billing_mode": filter.BillingMode,
	}, details, "log_database+main_database")
}

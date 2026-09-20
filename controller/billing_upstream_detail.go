package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const maxBillingDetailsPageSize = 200

// GetAdminUpstreamBillingDetails serves the cross-customer evidence rows
// behind the upstream reconciliation summary. Admin-only; the upstream task
// id stays root-scoped like the rest of the platform.
func GetAdminUpstreamBillingDetails(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	page := common.GetPageQuery(c)
	if page.GetPage() < 1 || page.GetPageSize() < 1 || page.GetPageSize() > maxBillingDetailsPageSize {
		common.ApiErrorMsg(c, "invalid pagination")
		return
	}
	filter := model.UpstreamBillingDetailFilter{
		Start: period.StartTimestamp, End: period.EndTimestamp,
		ProviderModel:     strings.TrimSpace(c.Query("model_name")),
		BillingMode:       strings.TrimSpace(c.Query("billing_mode")),
		RequestId:         strings.TrimSpace(c.Query("request_id")),
		UpstreamRequestId: strings.TrimSpace(c.Query("upstream_request_id")),
	}
	if len(filter.ProviderModel) > 255 || len(filter.RequestId) > 64 || len(filter.UpstreamRequestId) > 128 {
		common.ApiErrorMsg(c, "invalid filter")
		return
	}
	if filter.BillingMode != "" && !validBillingDetailMode(filter.BillingMode) {
		common.ApiErrorMsg(c, "invalid billing_mode")
		return
	}
	channelId := parsePositiveQueryId(c, "channel_id")
	if channelId < 0 {
		return
	}
	urlKey := strings.TrimSpace(c.Query("url_key"))
	if len(urlKey) > maxBillingURLKeyFilterLength {
		common.ApiErrorMsg(c, "invalid url_key")
		return
	}
	switch {
	case channelId > 0:
		filter.ChannelIds = []int{channelId}
	case urlKey != "":
		channelIds, err := resolveUpstreamURLChannels(urlKey)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		filter.ChannelIds = channelIds
	}
	if len(filter.ChannelIds) == 0 {
		common.ApiErrorMsg(c, "upstream details require a URL or channel filter")
		return
	}
	details, err := model.GetUpstreamBillingDetails(c.Request.Context(), filter, page.GetPage(), page.GetPageSize(), c.GetInt("role") >= common.RoleRootUser)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondBillingReconciliation(c, period, gin.H{
		"url_key": urlKey, "channel_id": channelId,
		"model_name": filter.ProviderModel, "billing_mode": filter.BillingMode,
	}, details, "log_database+main_database")
}

// resolveUpstreamURLChannels maps a URL grouping key back to its channels
// under the same normalization the summary uses; fallback groups carry a
// single per-channel id. An unknown URL key resolves to nothing.
func resolveUpstreamURLChannels(urlKey string) ([]int, error) {
	if channelId, ok := strings.CutPrefix(urlKey, "channel:"); ok {
		id, err := strconv.Atoi(channelId)
		if err != nil || id <= 0 {
			return []int{}, nil
		}
		return []int{id}, nil
	}
	return model.GetChannelIdsByNormalizedBaseURL(urlKey)
}

func validBillingDetailMode(mode string) bool {
	return mode == model.BillingReconciliationModeToken || mode == model.BillingReconciliationModePerCall || mode == model.BillingReconciliationModePerSecond || mode == model.BillingReconciliationModeUnknown
}

package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetUserBillingStatementBreakdown(c *gin.Context) {
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return
	}
	if endTimestamp-startTimestamp > maxBillingStatementRangeSeconds {
		common.ApiErrorMsg(c, "time range cannot exceed 31 days")
		return
	}

	tokenId := 0
	if tokenIdText := c.Query("token_id"); tokenIdText != "" {
		parsedTokenId, err := strconv.Atoi(tokenIdText)
		if err != nil || parsedTokenId <= 0 {
			common.ApiErrorMsg(c, "invalid token_id")
			return
		}
		tokenId = parsedTokenId
	}
	modelName := strings.TrimSpace(c.Query("model_name"))
	if len(modelName) > 255 {
		common.ApiErrorMsg(c, "invalid model_name")
		return
	}

	items, summary, err := model.GetUserBillingStatementBreakdown(
		c.GetInt("id"),
		startTimestamp,
		endTimestamp,
		tokenId,
		modelName,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"period": billingStatementPeriod{
			StartTimestamp: startTimestamp,
			EndTimestamp:   endTimestamp,
		},
		"summary":      summary,
		"items":        items,
		"generated_at": common.GetTimestamp(),
		"data_source":  "settlement_logs",
		"classification": gin.H{
			"context_threshold_source": "current_model_config",
			"unconfigured_context":     "omitted",
			"quota_basis":              "settled_consume_log_quota",
		},
	})
}

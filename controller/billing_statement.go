package controller

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

const maxBillingStatementRangeSeconds int64 = 31 * 24 * 60 * 60

type billingStatementPeriod struct {
	StartTimestamp int64 `json:"start_timestamp"`
	EndTimestamp   int64 `json:"end_timestamp"`
}

type billingStatementFunds struct {
	CurrentBalance   int `json:"current_balance"`
	LifetimeConsumed int `json:"lifetime_consumed"`
}

func GetUserBillingStatement(c *gin.Context) {
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

	userId := c.GetInt("id")
	items, summary, err := model.GetUserBillingStatement(
		userId,
		startTimestamp,
		endTimestamp,
		tokenId,
		modelName,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	currentBalance, err := model.GetUserQuota(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	common.ApiSuccess(c, gin.H{
		"period": billingStatementPeriod{
			StartTimestamp: startTimestamp,
			EndTimestamp:   endTimestamp,
		},
		"funds": billingStatementFunds{
			CurrentBalance:   max(currentBalance, 0),
			LifetimeConsumed: max(user.UsedQuota, 0),
		},
		"summary":      summary,
		"items":        items,
		"generated_at": common.GetTimestamp(),
		"data_source":  "settlement_logs",
	})
}

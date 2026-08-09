package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var billingSettlementLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

type billingReconciliationPeriod struct {
	StartTimestamp int64  `json:"start_timestamp"`
	EndTimestamp   int64  `json:"end_timestamp"`
	PeriodStart    int64  `json:"period_start"`
	Timezone       string `json:"timezone"`
}

type providerBillingDiscountRequest struct {
	PeriodStart      int64           `json:"period_start"`
	ChannelId        int             `json:"channel_id"`
	ProviderModel    string          `json:"provider_model"`
	BillingMode      string          `json:"billing_mode"`
	Discount         decimal.Decimal `json:"discount"`
	CopiedFromPeriod int64           `json:"copied_from_period"`
	ExpectedVersion  int64           `json:"expected_version"`
	Reason           string          `json:"reason"`
}

func GetSelfBillingReconciliation(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	tokenId := parsePositiveQueryId(c, "token_id")
	if tokenId < 0 {
		return
	}
	modelName, billingMode, ok := parseBillingReconciliationModelFilters(c)
	if !ok {
		return
	}
	statement, err := model.GetBillingCustomerStatement(
		c.GetInt("id"), period.StartTimestamp, period.EndTimestamp, "api_key",
		tokenId, modelName, billingMode,
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondBillingReconciliation(c, period, gin.H{"dimension": "api_key"}, statement, "main_database+log_database")
}

func GetAdminCustomerBillingReconciliation(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	userId, err := strconv.Atoi(c.Query("user_id"))
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "invalid user_id")
		return
	}
	if _, err := model.GetBillingReconciliationUserById(userId); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "user not found"})
			return
		}
		common.ApiError(c, err)
		return
	}
	dimension := strings.TrimSpace(c.DefaultQuery("dimension", "api_key"))
	groupId := parsePositiveQueryId(c, "group_id")
	if groupId < 0 {
		return
	}
	modelName, billingMode, ok := parseBillingReconciliationModelFilters(c)
	if !ok {
		return
	}
	statement, err := model.GetBillingCustomerStatement(
		userId, period.StartTimestamp, period.EndTimestamp, dimension, groupId,
		modelName, billingMode,
	)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	respondBillingReconciliation(c, period, gin.H{"user_id": userId, "dimension": dimension, "group_id": groupId}, statement, "main_database+log_database")
}

func GetAdminUpstreamBillingReconciliation(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	channelId := parsePositiveQueryId(c, "channel_id")
	if channelId < 0 {
		return
	}
	modelName, billingMode, ok := parseBillingReconciliationModelFilters(c)
	if !ok {
		return
	}
	summary, err := model.GetProviderBillingSummary(
		period.StartTimestamp, period.EndTimestamp, period.PeriodStart, channelId,
		modelName, billingMode, c.GetInt("id"),
	)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondBillingReconciliation(c, period, gin.H{"channel_id": channelId}, summary, "main_database+log_database")
}

func PutAdminProviderBillingDiscount(c *gin.Context) {
	var request providerBillingDiscountRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validProviderBillingKey(request.PeriodStart, request.ChannelId, request.ProviderModel, request.BillingMode) || request.Discount.LessThanOrEqual(decimal.Zero) || request.Discount.GreaterThan(decimal.NewFromInt(1)) || request.ExpectedVersion < 0 || strings.TrimSpace(request.Reason) == "" || !validCopiedProviderBillingPeriod(request.PeriodStart, request.CopiedFromPeriod) {
		common.ApiErrorMsg(c, "invalid provider discount")
		return
	}
	if !providerBillingChannelExists(c, request.ChannelId) {
		return
	}
	discount := model.ProviderBillingDiscount{
		PeriodStart: request.PeriodStart, ChannelId: request.ChannelId, ProviderModel: strings.TrimSpace(request.ProviderModel), BillingMode: request.BillingMode,
		Discount: request.Discount, CopiedFromPeriod: request.CopiedFromPeriod, Reason: strings.TrimSpace(request.Reason),
	}
	if err := model.SaveProviderBillingDiscount(&discount, request.ExpectedVersion, c.GetInt("id")); err != nil {
		respondBillingReconciliationWriteError(c, err)
		return
	}
	common.ApiSuccess(c, discount)
}

func parseBillingReconciliationPeriod(c *gin.Context) (billingReconciliationPeriod, bool) {
	startTimestamp, endTimestamp, ok := parseFlowQuotaTimeRange(c)
	if !ok {
		return billingReconciliationPeriod{}, false
	}
	if endTimestamp-startTimestamp > maxBillingStatementRangeSeconds {
		common.ApiErrorMsg(c, "time range cannot exceed 31 days")
		return billingReconciliationPeriod{}, false
	}
	start := time.Unix(startTimestamp, 0).In(billingSettlementLocation)
	periodStart := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, billingSettlementLocation).Unix()
	nextPeriodStart := time.Date(start.Year(), start.Month()+1, 1, 0, 0, 0, 0, billingSettlementLocation).Unix()
	if startTimestamp != periodStart || endTimestamp != nextPeriodStart-1 {
		common.ApiErrorMsg(c, "billing period must be a natural month in Asia/Shanghai")
		return billingReconciliationPeriod{}, false
	}
	return billingReconciliationPeriod{StartTimestamp: startTimestamp, EndTimestamp: endTimestamp, PeriodStart: periodStart, Timezone: "Asia/Shanghai"}, true
}

func parsePositiveQueryId(c *gin.Context, name string) int {
	value := strings.TrimSpace(c.Query(name))
	if value == "" {
		return 0
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		common.ApiErrorMsg(c, "invalid "+name)
		return -1
	}
	return parsed
}

func parseBillingReconciliationModelFilters(c *gin.Context) (string, string, bool) {
	modelName := strings.TrimSpace(c.Query("model_name"))
	if len(modelName) > 255 {
		common.ApiErrorMsg(c, "invalid model_name")
		return "", "", false
	}
	billingMode := strings.TrimSpace(c.Query("billing_mode"))
	if billingMode != "" && billingMode != model.BillingReconciliationModeToken && billingMode != model.BillingReconciliationModePerCall && billingMode != model.BillingReconciliationModeUnknown {
		common.ApiErrorMsg(c, "invalid billing_mode")
		return "", "", false
	}
	return modelName, billingMode, true
}

func respondBillingReconciliation(c *gin.Context, period billingReconciliationPeriod, filters gin.H, result any, source string) {
	common.ApiSuccess(c, gin.H{
		"period": period, "filters": filters, "result": result,
		"generated_at": common.GetTimestamp(), "data_version": period.EndTimestamp, "data_source": source,
	})
}

func respondBillingReconciliationWriteError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrBillingReconciliationVersionConflict) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiError(c, err)
}

func validProviderBillingKey(periodStart int64, channelId int, providerModel string, billingMode string) bool {
	if periodStart <= 0 || channelId <= 0 || strings.TrimSpace(providerModel) == "" || len(strings.TrimSpace(providerModel)) > 255 {
		return false
	}
	period := time.Unix(periodStart, 0).In(billingSettlementLocation)
	if periodStart != time.Date(period.Year(), period.Month(), 1, 0, 0, 0, 0, billingSettlementLocation).Unix() {
		return false
	}
	return billingMode == model.BillingReconciliationModeToken || billingMode == model.BillingReconciliationModePerCall
}

func validCopiedProviderBillingPeriod(periodStart int64, copiedFromPeriod int64) bool {
	if copiedFromPeriod == 0 {
		return true
	}
	period := time.Unix(periodStart, 0).In(billingSettlementLocation)
	previous := time.Date(period.Year(), period.Month()-1, 1, 0, 0, 0, 0, billingSettlementLocation).Unix()
	return copiedFromPeriod == previous
}

func providerBillingChannelExists(c *gin.Context, channelId int) bool {
	_, err := model.GetChannelById(channelId, false)
	if err == nil {
		return true
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "channel not found"})
		return false
	}
	common.ApiError(c, err)
	return false
}

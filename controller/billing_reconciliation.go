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
	PeriodStart     int64           `json:"period_start"`
	ChannelId       int             `json:"channel_id"`
	Discount        decimal.Decimal `json:"discount"`
	ExpectedVersion int64           `json:"expected_version"`
	Reason          string          `json:"reason"`
}

type providerChannelDiscountInitRequest struct {
	PeriodStart int64 `json:"period_start"`
	ChannelIds  []int `json:"channel_ids"`
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
	// 版本绑定（方案 13）：明确版本或已有当前确认版时读取冻结视图，否则走实时查询。
	if tryRespondBillingStatementVersion(c, period, c.GetInt("id"), "api_key", false) {
		return
	}
	statement, err := model.GetBillingCustomerStatement(
		c.Request.Context(), c.GetInt("id"), period.StartTimestamp, period.EndTimestamp, "api_key",
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
	dimension := strings.TrimSpace(c.DefaultQuery("dimension", "api_key"))
	groupId := parsePositiveQueryId(c, "group_id")
	if groupId < 0 {
		return
	}
	modelName, billingMode, ok := parseBillingReconciliationModelFilters(c)
	if !ok {
		return
	}
	// 版本绑定（方案 13）：管理员可明确草稿/历史版本；未指定时自动落到当前确认版。
	if tryRespondBillingStatementVersion(c, period, userId, dimension, true) {
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
	statement, err := model.GetBillingCustomerStatement(
		c.Request.Context(), userId, period.StartTimestamp, period.EndTimestamp, dimension, groupId,
		modelName, billingMode,
	)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	respondBillingReconciliation(c, period, gin.H{"user_id": userId, "dimension": dimension, "group_id": groupId}, statement, "main_database+log_database")
}

// GetAdminUpstreamReconciliation serves the unified admin upstream view: URL
// groups with usage, official-price amounts, reference amounts and the
// channel-month discount editing area. Provider model and billing mode stay
// client-side filters of the loaded summary.
func GetAdminUpstreamReconciliation(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	urlKey := strings.TrimSpace(c.Query("url_key"))
	if len(urlKey) > maxBillingURLKeyFilterLength {
		common.ApiErrorMsg(c, "invalid url_key")
		return
	}
	summary, err := model.GetProviderBillingURLSummary(period.StartTimestamp, period.EndTimestamp, period.PeriodStart, urlKey)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondBillingReconciliation(c, period, gin.H{"url_key": urlKey}, summary, "main_database+log_database")
}

// PostAdminProviderChannelDiscountInit runs the explicit, idempotent month
// initialization for the given channels. It only fills channels whose current
// month has no record yet; manual values, corrected copies and migrated
// values are never overwritten.
func PostAdminProviderChannelDiscountInit(c *gin.Context) {
	var request providerChannelDiscountInitRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validProviderBillingPeriod(request.PeriodStart) || len(request.ChannelIds) == 0 {
		common.ApiErrorMsg(c, "invalid provider discount initialization")
		return
	}
	for _, channelId := range request.ChannelIds {
		if channelId <= 0 {
			common.ApiErrorMsg(c, "invalid provider discount initialization")
			return
		}
	}
	// 已删除渠道不阻断整批初始化：模型层逐渠道返回 invalid_channel 结果。
	outcomes, err := model.InitializeProviderChannelBillingDiscounts(request.PeriodStart, request.ChannelIds, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, gin.H{"outcomes": outcomes})
}

func PutAdminProviderBillingDiscount(c *gin.Context) {
	var request providerBillingDiscountRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validProviderBillingPeriod(request.PeriodStart) || request.Discount.LessThanOrEqual(decimal.Zero) || request.Discount.GreaterThan(decimal.NewFromInt(1)) || request.ExpectedVersion < 0 || strings.TrimSpace(request.Reason) == "" {
		common.ApiErrorMsg(c, "invalid provider discount")
		return
	}
	if !providerBillingChannelExists(c, request.ChannelId) {
		return
	}
	discount := model.ProviderChannelBillingDiscount{
		PeriodStart: request.PeriodStart, ChannelId: request.ChannelId,
		Discount: request.Discount, Reason: strings.TrimSpace(request.Reason),
	}
	if err := model.SaveProviderChannelBillingDiscount(&discount, request.ExpectedVersion, c.GetInt("id")); err != nil {
		respondBillingReconciliationWriteError(c, err)
		return
	}
	common.ApiSuccess(c, discount)
}

func validProviderBillingPeriod(periodStart int64) bool {
	if periodStart <= 0 {
		return false
	}
	period := time.Unix(periodStart, 0).In(billingSettlementLocation)
	return periodStart == time.Date(period.Year(), period.Month(), 1, 0, 0, 0, 0, billingSettlementLocation).Unix()
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
	if billingMode != "" && billingMode != model.BillingReconciliationModeToken && billingMode != model.BillingReconciliationModePerCall && billingMode != model.BillingReconciliationModePerSecond && billingMode != model.BillingReconciliationModeUnknown {
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

func providerBillingChannelExists(c *gin.Context, channelIds ...int) bool {
	for _, channelId := range channelIds {
		if _, err := model.GetChannelById(channelId, false); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "channel not found"})
				return false
			}
			common.ApiError(c, err)
			return false
		}
	}
	return true
}

package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	EffectiveDate   string          `json:"effective_date"`
	PeriodStart     int64           `json:"period_start"`
	ChannelId       int             `json:"channel_id"`
	Discount        decimal.Decimal `json:"discount"`
	ExpectedVersion int64           `json:"expected_version"`
}

type providerChannelDiscountInitRequest struct {
	EndTimestamp int64 `json:"end_timestamp"`
	PeriodStart  int64 `json:"period_start"`
	ChannelIds   []int `json:"channel_ids"`
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
	// 版本绑定（方案 13）：明确版本或已有当前确认版时读取冻结视图，其余读取已发布日报表。
	if tryRespondBillingStatementVersion(c, period, c.GetInt("id"), "api_key", false) {
		return
	}
	cancelReport, ok := bindBillingReport(c, period)
	if !ok {
		return
	}
	defer cancelReport()
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
	cancelReport, ok := bindBillingReport(c, period)
	if !ok {
		return
	}
	defer cancelReport()
	if _, err := model.GetBillingReconciliationUser(c.Request.Context(), userId); err != nil {
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

// GetAdminUpstreamReconciliation returns read-only channels and model leaves.
// Reporting identities never depend on the current base URL.
func GetAdminUpstreamReconciliation(c *gin.Context) {
	period, ok := parseBillingReconciliationPeriod(c)
	if !ok {
		return
	}
	urlKey := strings.TrimSpace(c.Query("url_key"))
	if !validateUpstreamReportURL(c, urlKey) {
		return
	}
	page, pageSize, valid := parseBillingPage(c)
	if !valid {
		return
	}
	channelID := parsePositiveQueryId(c, "channel_id")
	if channelID < 0 {
		return
	}
	level := c.DefaultQuery("level", "channels")
	search := strings.TrimSpace(c.Query("search"))
	if utf8.RuneCountInString(search) > 255 {
		common.ApiErrorMsg(c, "invalid search")
		return
	}
	cancelReport, ok := bindUpstreamBillingReport(c, period)
	if !ok {
		return
	}
	defer cancelReport()
	summary, err := model.GetUpstreamSummaryPage(c.Request.Context(), period.StartTimestamp, period.EndTimestamp, period.PeriodStart, model.UpstreamSummaryPageFilter{
		ReportURL: urlKey, GroupByURL: c.Query("group_by") == "url", Level: level,
		ChannelID: channelID, Search: search, Page: page, PageSize: pageSize,
		SortBy: c.Query("sort_by"), DataStatus: c.Query("data_status"),
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondBillingReconciliation(c, period, gin.H{"channel_id": channelID}, summary, "main_database+log_database")
}

// PostAdminProviderChannelDiscountInit runs the explicit, idempotent month
// initialization for the given channels. It only fills channels whose current
// month has no record yet; manual values, corrected copies and migrated
// values are never overwritten.
func PostAdminProviderChannelDiscountInit(c *gin.Context) {
	var request providerChannelDiscountInitRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validProviderBillingPeriod(request.PeriodStart) {
		common.ApiErrorMsg(c, "invalid provider discount initialization")
		return
	}
	if request.EndTimestamp != 0 {
		expectedEnd := time.Unix(request.PeriodStart, 0).In(billingSettlementLocation).AddDate(0, 1, 0).Unix()
		if request.EndTimestamp != expectedEnd || len(request.ChannelIds) > 0 {
			common.ApiErrorMsg(c, "invalid provider discount initialization")
			return
		}
		counts, err := model.InitializeUpstreamPeriodDiscounts(c.Request.Context(), request.PeriodStart, request.EndTimestamp-1, c.GetInt("id"))
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, gin.H{"counts": counts})
		return
	}
	if len(request.ChannelIds) == 0 || len(request.ChannelIds) > 100 {
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

// PutAdminProviderBillingDiscount updates a channel's monthly discount schedule. The
// operator no longer types a reason: the server records the fixed system
// operation description so the audit trail stays complete without frontend
// input. Period, channel, coefficient range, expected-version and admin
// permission checks are unchanged; conflicts still require a refresh.
func PutAdminProviderBillingDiscount(c *gin.Context) {
	var request providerBillingDiscountRequest
	if err := c.ShouldBindJSON(&request); err != nil || !validProviderBillingPeriod(request.PeriodStart) || request.Discount.LessThanOrEqual(decimal.Zero) || request.Discount.GreaterThan(decimal.NewFromInt(1)) || request.ExpectedVersion < 0 {
		common.ApiErrorMsg(c, "invalid provider discount")
		return
	}
	var effectiveFrom int64
	if request.EffectiveDate != "" {
		date, err := time.ParseInLocation(time.DateOnly, request.EffectiveDate, billingSettlementLocation)
		if err != nil {
			common.ApiErrorMsg(c, "invalid discount effective date")
			return
		}
		effectiveFrom = date.Unix()
	}
	discount := model.ProviderChannelBillingDiscount{
		PeriodStart: request.PeriodStart, ChannelId: request.ChannelId,
		Discount: request.Discount, Reason: model.ReasonChannelDiscountManualUpdate,
	}
	if err := model.SaveProviderChannelBillingDiscount(&discount, request.ExpectedVersion, c.GetInt("id"), effectiveFrom); err != nil {
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
	if billingMode != "" && !model.ValidBillingReconciliationMode(billingMode) {
		common.ApiErrorMsg(c, "invalid billing_mode")
		return "", "", false
	}
	return modelName, billingMode, true
}

func respondBillingReconciliation(c *gin.Context, period billingReconciliationPeriod, filters gin.H, result any, source string) {
	response := gin.H{
		"period": period, "filters": filters, "result": result,
		"generated_at": common.GetTimestamp(), "data_version": period.EndTimestamp, "data_source": source,
	}
	if report := model.BillingReportFromContext(c.Request.Context()); report != nil {
		response["billing_report"] = report
		response["data_source"] = "published_daily_billing"
		response["generated_at"] = report.AsOf
	}
	if statement, ok := result.(model.BillingCustomerStatement); ok {
		addCustomerBillingRefundStatus(c, response, statement.UserId, period)
	}
	common.ApiSuccess(c, response)
}

func respondBillingReconciliationWriteError(c *gin.Context, err error) {
	if errors.Is(err, model.ErrBillingReconciliationVersionConflict) {
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": err.Error()})
		return
	}
	common.ApiError(c, err)
}

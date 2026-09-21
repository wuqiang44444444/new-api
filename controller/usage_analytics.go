package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 日/周用量统计接口（docs/80-dev/2026-09-20 方案 §6）：服务端推导日期边界，
// 禁止任意起止时间；管理员客户查询必须显式提供目标客户；响应只包含公开汇总
// 字段，不返回上游内部信息给普通客户。

type usageAnalyticsQuery struct {
	Period model.UsageAnalyticsPeriod
}

func parseUsageAnalyticsPeriod(c *gin.Context) (model.UsageAnalyticsPeriod, bool) {
	period, err := model.ResolveUsageAnalyticsPeriod(
		strings.TrimSpace(c.Query("period")),
		strings.TrimSpace(c.Query("date")),
		common.GetTimestamp(),
	)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return model.UsageAnalyticsPeriod{}, false
	}
	return period, true
}

func respondUsageAnalytics(c *gin.Context, period model.UsageAnalyticsPeriod, filters gin.H, result any) {
	common.ApiSuccess(c, gin.H{
		"period":        period,
		"filters":       filters,
		"result":        result,
		"generated_at":  common.GetTimestamp(),
		"data_source":   "main_database+log_database",
		"coverage_note": usageAnalyticsCoverageNote(),
	})
}

// usageAnalyticsCoverageNote 是统一的覆盖说明：调用计数与结束日来自已持久化
// 事实，缺失用量按未记录展示；真实数据验收仍在进行中。
func usageAnalyticsCoverageNote() string {
	return "ended_calls_by_finish_time; missing_usage_reported_as_unrecorded; missing_finish_time_or_request_identity_not_reconstructed; rejected_attempts_without_terminal_timestamp_not_dated; upstream_errors_require_provider_evidence; log_retention_and_disabled_logging_may_reduce_coverage; real_data_acceptance_pending"
}

// GetUsageSelfSummary 返回当前用户自己的 API Key → 模型日/周用量。
func GetUsageSelfSummary(c *gin.Context) {
	period, ok := parseUsageAnalyticsPeriod(c)
	if !ok {
		return
	}
	view, err := model.GetUsageCustomerView(c.Request.Context(), period, c.GetInt("id"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondUsageAnalytics(c, period, gin.H{"scope": "self"}, view)
}

// GetUsageAdminCustomers 返回全客户用量列表与范围总览。
func GetUsageAdminCustomers(c *gin.Context) {
	period, ok := parseUsageAnalyticsPeriod(c)
	if !ok {
		return
	}
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 255 {
		common.ApiErrorMsg(c, "invalid search")
		return
	}
	overview, err := model.GetUsageCustomersOverview(c.Request.Context(), period, search)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondUsageAnalytics(c, period, gin.H{"scope": "customers", "search": search}, overview)
}

// GetUsageAdminCustomerSummary 返回显式客户的 API Key → 模型用量汇总。
func GetUsageAdminCustomerSummary(c *gin.Context) {
	period, ok := parseUsageAnalyticsPeriod(c)
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
	view, err := model.GetUsageCustomerView(c.Request.Context(), period, userId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondUsageAnalytics(c, period, gin.H{"scope": "customer", "user_id": userId}, view)
}

// GetUsageAdminUpstreamSummary 返回上游视角的 URL → 渠道 → 模型汇总。
func GetUsageAdminUpstreamSummary(c *gin.Context) {
	period, ok := parseUsageAnalyticsPeriod(c)
	if !ok {
		return
	}
	view, err := model.GetUsageUpstreamView(c.Request.Context(), period)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	respondUsageAnalytics(c, period, gin.H{"scope": "upstream"}, view)
}

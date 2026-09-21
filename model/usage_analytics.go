package model

import (
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// 客户与上游日/周用量统计（docs/80-dev/2026-09-20-客户与上游日周用量统计实施方案.md）。
// 只读投影：按“已结束调用”的结束时间归属，聚合成功、失败、取消与缺失用量，
// 不执行对账、关账或资金调整，不建立第二套 Task 状态或计费事实。

const (
	UsageAnalyticsPeriodDay  = "day"
	UsageAnalyticsPeriodWeek = "week"

	// UsageAnalyticsTimezone 是统计归属时区；自然周为周一至周日。
	UsageAnalyticsTimezone = "Asia/Shanghai"

	// maxUsageAnalyticsGroups 限制单次查询的分组数量，超过即明确失败，
	// 不做无界扫描或静默截断。
	maxUsageAnalyticsGroups = 5000

	// maxUsageAnalyticsCustomers 限制客户列表的聚合行数，防止全客户查询无界。
	maxUsageAnalyticsCustomers = 5000

	// CustomerExportJobTypeUsageSummary 是用量汇总导出复用的导出任务类型。
	// 视角由任务的 TargetUserId 推导：self（等于发起人）、customer（显式目标
	// 客户）、upstream（0，仅限管理员）。
	CustomerExportJobTypeUsageSummary = "usage_summary"

	// usageAnalyticsUnknownModel 是历史模型身份缺失时叶子行的稳定标记，
	// 不从模型名、当前映射或金额反推。
	usageAnalyticsUnknownModel = "unknown"
)

// usageAnalyticsLocation 是固定偏移时区：无夏令时，日界与周界按 86400 秒稳定计算。
var usageAnalyticsLocation = time.FixedZone(UsageAnalyticsTimezone, 8*60*60)

// UsageAnalyticsDay 是范围中的一个日历日；End 为排他边界。
// Future 表示该日尚未开始，只能显示“未到日期”，不能当作真实零用量。
type UsageAnalyticsDay struct {
	Date   string `json:"date"`
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	Future bool   `json:"future,omitempty"`
}

// UsageAnalyticsPeriod 是服务端推导的查询范围；客户端只提供 day|week 与一个
// 上海时区日历日期，不能提交任意起止时间。
type UsageAnalyticsPeriod struct {
	Period         string              `json:"period"`
	Date           string              `json:"date"`
	StartTimestamp int64               `json:"start_timestamp"`
	EndTimestamp   int64               `json:"end_timestamp"` // 排他
	Timezone       string              `json:"timezone"`
	Days           []UsageAnalyticsDay `json:"days"`
}

// ResolveUsageAnalyticsPeriod 把 day|week 与 YYYY-MM-DD 规范化为固定日界。
// 周模式取该日期所在自然周（周一至下周周一），允许跨月、跨年。
func ResolveUsageAnalyticsPeriod(period string, date string, now int64) (UsageAnalyticsPeriod, error) {
	if period != UsageAnalyticsPeriodDay && period != UsageAnalyticsPeriodWeek {
		return UsageAnalyticsPeriod{}, errors.New("period must be day or week")
	}
	dayStart, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), usageAnalyticsLocation)
	if err != nil {
		return UsageAnalyticsPeriod{}, errors.New("date must be YYYY-MM-DD in Asia/Shanghai")
	}
	var start, end time.Time
	if period == UsageAnalyticsPeriodDay {
		start, end = dayStart, dayStart.AddDate(0, 0, 1)
	} else {
		offset := (int(dayStart.Weekday()) + 6) % 7 // 周一为 0
		monday := dayStart.AddDate(0, 0, -offset)
		start, end = monday, monday.AddDate(0, 0, 7)
	}
	result := UsageAnalyticsPeriod{
		Period:         period,
		Date:           strings.TrimSpace(date),
		StartTimestamp: start.Unix(),
		EndTimestamp:   end.Unix(),
		Timezone:       UsageAnalyticsTimezone,
		Days:           make([]UsageAnalyticsDay, 0, 7),
	}
	for cursor := start; cursor.Before(end); cursor = cursor.AddDate(0, 0, 1) {
		dayEnd := cursor.AddDate(0, 0, 1)
		result.Days = append(result.Days, UsageAnalyticsDay{
			Date:   cursor.Format("2006-01-02"),
			Start:  cursor.Unix(),
			End:    dayEnd.Unix(),
			Future: cursor.Unix() > now,
		})
	}
	return result, nil
}

// UsageAnalyticsMetrics 是一个分组位置在一日或整个范围的用量、金额与质量。
// 明确的 0 显示 0；无证据计缺失行数；不适用留空。
type UsageAnalyticsMetrics struct {
	TokenDetails     map[string]int64 `json:"token_details,omitempty"`
	TotalCalls       int64            `json:"total_calls"`
	SuccessCalls     int64            `json:"success_calls"`
	FailureCalls     int64            `json:"failure_calls"`
	CancelledCalls   int64            `json:"cancelled_calls"`
	OtherResultCalls int64            `json:"other_result_calls"`

	InputTokens        int64            `json:"input_tokens"`
	OutputTokens       int64            `json:"output_tokens"`
	CacheReadTokens    int64            `json:"cache_read_tokens"`
	CacheWriteTokens   int64            `json:"cache_write_tokens"`
	ImageCount         int64            `json:"image_count"`
	Seconds            *decimal.Decimal `json:"seconds,omitempty"`
	SecondsMissingRows int64            `json:"seconds_missing_rows,omitempty"`

	GrossQuota  int64 `json:"gross_quota"`
	RefundQuota int64 `json:"refund_quota"`
	NetQuota    int64 `json:"net_quota"`
	// UnlinkedRefundQuota 是无法关联到已结束调用的退款金额：不冒充某日调用的
	// 净额，只作为该日的资金事实单列。
	UnlinkedRefundQuota int64 `json:"unlinked_refund_quota,omitempty"`

	// OriginalQuotaEstimate 是由已记录扣款及冻结折扣还原的估算原价（消费正计、
	// 可关联退款负计）；任一有金额的行无法还原时为空并携带 EstimateReasons。
	OriginalQuotaEstimate *int64   `json:"original_quota_estimate,omitempty"`
	EstimateReasons       []string `json:"estimate_reasons,omitempty"`
	// MultipleDiscounts 表示该范围命中的渠道月度系数不止一个值（仅上游视图）。
	MultipleDiscounts bool `json:"multiple_discounts,omitempty"`
	// ReferenceAmount 是折后参考金额（上游视图，逐笔取整后求和）。
	ReferenceAmount *int64 `json:"reference_amount,omitempty"`
	ReferenceKnown  bool   `json:"reference_known,omitempty"`

	RowsMissingTokens int64 `json:"rows_missing_tokens,omitempty"`
	RowsMissingMoney  int64 `json:"rows_missing_money,omitempty"`
	RowsMoneyPending  int64 `json:"rows_money_pending,omitempty"`
	// TestPricedRows 是按可靠计价依据计入上游参考金额的原生渠道测试行。
	TestPricedRows int64 `json:"test_priced_rows,omitempty"`
	// SecondsValueMissingRows 是已知秒计量单位但缺可信完成值的行。
	SecondsValueMissingRows int64 `json:"seconds_value_missing_rows,omitempty"`
	// UsageOnlyRows 是未能按可靠依据计入金额的测试行（不再代表全部测试）。
	UsageOnlyRows int64 `json:"usage_only_rows,omitempty"`
}

// usageMetricsAcc 是指标累加器：数量用整数，秒数与原价用十进制中间值，
// 到 finalize 才舍入/取整，避免部分合计与父级口径漂移。
type usageMetricsAcc struct {
	m                  UsageAnalyticsMetrics
	seconds            decimal.Decimal
	secondsKnown       bool
	original           decimal.Decimal
	originalKnown      bool
	originalIncomplete bool
	// reference 是上游折后参考金额的十进制中间值（逐笔取整后求和）。
	reference    decimal.Decimal
	coefficients map[string]struct{}
}

func (a *usageMetricsAcc) addCalls(result string, calls int64) {
	if calls <= 0 {
		return
	}
	a.m.TotalCalls += calls
	switch result {
	case usageResultSuccess:
		a.m.SuccessCalls += calls
	case usageResultFailure:
		a.m.FailureCalls += calls
	case usageResultCancelled:
		a.m.CancelledCalls += calls
	default:
		a.m.OtherResultCalls += calls
	}
}

func (a *usageMetricsAcc) addSeconds(value decimal.Decimal, known bool, missing bool) {
	if known {
		a.seconds = a.seconds.Add(value)
		a.secondsKnown = true
	}
	if missing {
		a.m.SecondsMissingRows++
	}
}

func (a *usageMetricsAcc) addCoefficient(value decimal.Decimal) {
	if a.coefficients == nil {
		a.coefficients = make(map[string]struct{})
	}
	a.coefficients[value.String()] = struct{}{}
}

func (a *usageMetricsAcc) mergeTokens(source UsageAnalyticsMetrics) {
	mergeUsageTokenDetails(&a.m.TokenDetails, source.TokenDetails)
	a.m.InputTokens += source.InputTokens
	a.m.OutputTokens += source.OutputTokens
	a.m.CacheReadTokens += source.CacheReadTokens
	a.m.CacheWriteTokens += source.CacheWriteTokens
	a.m.ImageCount += source.ImageCount
	a.m.SecondsMissingRows += source.SecondsMissingRows
	a.m.GrossQuota += source.GrossQuota
	a.m.RefundQuota += source.RefundQuota
	a.m.UnlinkedRefundQuota += source.UnlinkedRefundQuota
	a.m.RowsMissingTokens += source.RowsMissingTokens
	a.m.RowsMissingMoney += source.RowsMissingMoney
	a.m.RowsMoneyPending += source.RowsMoneyPending
	a.m.UsageOnlyRows += source.UsageOnlyRows
	a.m.TestPricedRows += source.TestPricedRows
	a.m.SecondsValueMissingRows += source.SecondsValueMissingRows
}

func (a *usageMetricsAcc) merge(source *usageMetricsAcc) {
	a.m.TotalCalls += source.m.TotalCalls
	a.m.SuccessCalls += source.m.SuccessCalls
	a.m.FailureCalls += source.m.FailureCalls
	a.m.CancelledCalls += source.m.CancelledCalls
	a.m.OtherResultCalls += source.m.OtherResultCalls
	a.mergeTokens(source.m)
	if source.secondsKnown {
		a.seconds = a.seconds.Add(source.seconds)
		a.secondsKnown = true
	}
	a.original = a.original.Add(source.original)
	a.originalKnown = a.originalKnown || source.originalKnown
	a.originalIncomplete = a.originalIncomplete || source.originalIncomplete
	a.reference = a.reference.Add(source.reference)
	a.m.EstimateReasons = mergeBillingEstimateReasons(a.m.EstimateReasons, source.m.EstimateReasons...)
	for marker := range source.coefficients {
		a.addCoefficientString(marker)
	}
}

func (a *usageMetricsAcc) addCoefficientString(marker string) {
	if a.coefficients == nil {
		a.coefficients = make(map[string]struct{})
	}
	a.coefficients[marker] = struct{}{}
}

// finalize 还原净额、秒数与估算原价。金额聚合使用有符号十进制，输出层才取整。
func (a *usageMetricsAcc) finalize() UsageAnalyticsMetrics {
	result := a.m
	result.NetQuota = result.GrossQuota - result.RefundQuota
	if a.secondsKnown {
		seconds := a.seconds
		result.Seconds = &seconds
	}
	// 无金额用量行（渠道测试）不构成“金额已知为零”，也不产生估算原价。
	if !a.originalIncomplete && (a.originalKnown || (result.GrossQuota == 0 && result.RefundQuota == 0 && result.UsageOnlyRows == 0)) {
		result.OriginalQuotaEstimate = billingStatementOriginalQuota(a.original)
		if result.OriginalQuotaEstimate == nil {
			result.EstimateReasons = mergeBillingEstimateReasons(result.EstimateReasons, BillingEstimateAmountOutOfRange)
		}
	}
	if !a.originalIncomplete && a.originalKnown && a.coefficients != nil && len(a.coefficients) > 0 {
		result.ReferenceAmount = billingStatementOriginalQuota(a.reference)
		result.ReferenceKnown = result.ReferenceAmount != nil
	}
	result.MultipleDiscounts = a.coefficients != nil && len(a.coefficients) > 1
	return result
}

// 使用结果分类：稳定的生命周期分类，不从错误文案推断。
const (
	usageResultSuccess   = "success"
	usageResultFailure   = "failure"
	usageResultCancelled = "cancelled"
	usageResultOther     = "other"
)

func usageResultFromTaskStatus(status TaskStatus) string {
	switch status {
	case TaskStatusSuccess:
		return usageResultSuccess
	case TaskStatusFailure, TaskStatusProviderContractFailure:
		return usageResultFailure
	case TaskStatusCancelled, TaskStatusExpired:
		return usageResultCancelled
	default:
		return usageResultOther
	}
}

// UsageAnalyticsDayBucket 是某一日全部范围的合计。
type UsageAnalyticsDayBucket struct {
	Date   string                `json:"date"`
	Future bool                  `json:"future,omitempty"`
	Total  UsageAnalyticsMetrics `json:"total"`
}

// UsageCustomerModelRow 是客户侧 API Key 下的一个客户模型叶子行。
type UsageCustomerModelRow struct {
	ModelName string                  `json:"model_name"`
	Days      []UsageAnalyticsMetrics `json:"days"`
	Total     UsageAnalyticsMetrics   `json:"total"`
}

// UsageCustomerKeyGroup 是一个 API Key（按稳定 token_id 区分，改名不串组）。
type UsageCustomerKeyGroup struct {
	TokenId      int                     `json:"token_id"`
	TokenName    string                  `json:"token_name"`
	TokenDeleted bool                    `json:"token_deleted,omitempty"`
	Days         []UsageAnalyticsMetrics `json:"days"`
	Total        UsageAnalyticsMetrics   `json:"total"`
	Models       []UsageCustomerModelRow `json:"models"`
}

// UsageCustomerView 是客户视角的用量汇总：API Key → 客户模型，逐日 + 范围合计。
type UsageCustomerView struct {
	DayTotals []UsageAnalyticsMetrics `json:"day_totals"`
	Total     UsageAnalyticsMetrics   `json:"total"`
	Keys      []UsageCustomerKeyGroup `json:"keys"`
}

// UsageChannelMonthDiscount 是渠道某月系数投影（只读，不物化配置）。
type UsageChannelMonthDiscount struct {
	PeriodStart  int64           `json:"period_start"`
	Value        decimal.Decimal `json:"value"`
	Version      int64           `json:"version"`
	Source       string          `json:"source"`
	SourcePeriod int64           `json:"source_period,omitempty"`
}

// UsageUpstreamModelRow 是 URL 下的上游模型 × 计费方式分组。
type UsageUpstreamModelRow struct {
	Channels              []UsageUpstreamChannelRow `json:"channels"`
	ProviderModel         string                    `json:"provider_model"`
	ProviderModelFallback bool                      `json:"provider_model_fallback,omitempty"`
	BillingMode           string                    `json:"billing_mode"`
	Days                  []UsageAnalyticsMetrics   `json:"days"`
	Total                 UsageAnalyticsMetrics     `json:"total"`
}

// UsageUpstreamChannelRow 是一个渠道行：携带该范围各月系数及其来源。
type UsageUpstreamChannelRow struct {
	ChannelId   int                         `json:"channel_id"`
	ChannelName string                      `json:"channel_name"`
	Discounts   []UsageChannelMonthDiscount `json:"discounts"`
	Days        []UsageAnalyticsMetrics     `json:"days"`
	Total       UsageAnalyticsMetrics       `json:"total"`
	UsageOnly   bool                        `json:"usage_only,omitempty"`
}

// UsageUpstreamUrlGroup 按渠道当前基础 URL 归组；归组只服务报表展示，
// 不建立供应商身份，不参与路由、计费或结算。
type UsageUpstreamUrlGroup struct {
	UrlKey       string                  `json:"url_key"`
	DisplayName  string                  `json:"display_name"`
	BaseURL      string                  `json:"base_url,omitempty"`
	Unidentified bool                    `json:"unidentified,omitempty"`
	Deleted      bool                    `json:"deleted,omitempty"`
	Models       []UsageUpstreamModelRow `json:"models"`
	Days         []UsageAnalyticsMetrics `json:"days"`
	Total        UsageAnalyticsMetrics   `json:"total"`
}

// UsageUpstreamView 是管理员上游视角：URL → 上游模型 × 计费方式 → 渠道。
type UsageUpstreamView struct {
	DayTotals []UsageAnalyticsMetrics `json:"day_totals"`
	Total     UsageAnalyticsMetrics   `json:"total"`
	UrlGroups []UsageUpstreamUrlGroup `json:"url_groups"`
}

// UsageCustomerOverviewRow 是客户列表中的一行：所选范围合计。
type UsageCustomerOverviewRow struct {
	Days        []UsageAnalyticsMetrics `json:"days"`
	UserId      int                     `json:"user_id"`
	Username    string                  `json:"username"`
	DisplayName string                  `json:"display_name,omitempty"`
	Deleted     bool                    `json:"deleted,omitempty"`
	Total       UsageAnalyticsMetrics   `json:"total"`
}

// UsageCustomersOverview 是管理员客户用量列表与范围总览。
type UsageCustomersOverview struct {
	DayTotals []UsageAnalyticsMetrics    `json:"day_totals"`
	Total     UsageAnalyticsMetrics      `json:"total"`
	Customers []UsageCustomerOverviewRow `json:"customers"`
	Truncated bool                       `json:"truncated,omitempty"`
}

// UsageAnalyticsZone exposes the fixed statistics timezone for readers that
// rebuild calendar boundaries from persisted timestamps.
func UsageAnalyticsZone() *time.Location {
	return usageAnalyticsLocation
}

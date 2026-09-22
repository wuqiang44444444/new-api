package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 用量统计的只读聚合实现。来源选择 → 结束时间归属 → 关联去重 → 指标提取 →
// 分组聚合全部集中在这里；页面与导出共用同一入口，保证同口径。
//
// 去重规则（方案 §2）：
//   - 同步最终消费/错误日志 = 一次客户调用，按日志写入时刻（该路径的结束事实）归属；
//   - task 标记的日志不单独计数，合并到其 Task，由 Task 的 finish_time 归属；
//     本地异步任务的交付日志按稳定 request_id 前缀 task-billing:<rowID>: 直接加载，
//     不受日志落库日期影响；
//   - azure_batch 使用作业终态与行级计数（作业不额外计一次）；
//   - 退款不增加调用次数；无法关联到已结束调用的退款单列为资金事实。

type usageViewOptions struct {
	// UserID 限定客户范围；0 表示全部客户（管理员视角）。
	UserID int
	// WantCustomer/WantUpstream/WantOverview 控制构建哪些投影。
	WantCustomer     bool
	WantUpstream     bool
	WantOverview     bool
	DiscountSnapshot *UsageAnalyticsDiscountSnapshot
}

type usageCustomerKey struct {
	day   int
	token int
	model string
}

type usageUpstreamKey struct {
	day      int
	channel  int
	model    string
	fallback bool
	mode     string
}

type usageOverviewKey struct {
	user int
	day  int
}

// usageTaskPieces 是一个已结束 Task 合并后的已知事实；金额与用量来自其关联
// 日志（范围内的原生任务日志 + 按稳定 ID 直接加载的交付日志），任务行本身
// 提供身份、终态与结束时间。
type usageTaskPieces struct {
	tokenDetails   map[string]int64
	gross          int64
	refund         int64
	input          int64
	output         int64
	cacheRead      int64
	cacheWrite     int64
	imageCount     int64
	seconds        decimal.Decimal
	secondsKnown   bool
	missingTokens  int64
	missingSeconds int64
	rowsSeen       int64
	billingMode    string
	tokenId        int
	tokenName      string
	// customerOriginal 是客户侧估算原价中间值（消费正计、可关联退款负计）。
	customerOriginal   decimal.Decimal
	originalKnown      bool
	originalIncomplete bool
	estimateReasons    []string
	// upstreamOriginal/Reference 是上游侧逐笔取整后的原价与折后参考中间值。
	upstreamOriginal   decimal.Decimal
	upstreamReference  decimal.Decimal
	upstreamMoneyKnown bool
	upstreamIncomplete bool
	coefficients       map[string]struct{}
}

func (p *usageTaskPieces) addCoefficient(value decimal.Decimal) {
	if p.coefficients == nil {
		p.coefficients = make(map[string]struct{})
	}
	p.coefficients[value.String()] = struct{}{}
}

// usageAggregation 是一次查询的聚合状态。
type usageAggregation struct {
	period UsageAnalyticsPeriod
	opts   usageViewOptions
	// bundle 预载范围覆盖的各自然月渠道系数；仅在需要上游金额时加载。
	bundle *usageDiscountBundle

	customer map[usageCustomerKey]*usageMetricsAcc
	upstream map[usageUpstreamKey]*usageMetricsAcc
	overview map[usageOverviewKey]*usageMetricsAcc

	taskLogs map[string]*usageTaskPieces
	tasks    []usageTaskRecord
	// overflow marks that a projection exceeded its group cap; the whole
	// query then fails closed instead of silently understating totals.
	overflow       bool
	errorRows      []usageLogRow
	logUpper       int64
	taskByIdentity map[string]*usageTaskRecord
}

type usageTaskRecord struct {
	VideoRefundState       string
	VideoRefundCompletedAt int64
	DeliveryIncomplete     bool             `gorm:"-"`
	RowID                  int64            `gorm:"column:id"`
	TaskID                 string           `gorm:"column:task_id"`
	UserID                 int              `gorm:"column:user_id"`
	ChannelID              int              `gorm:"column:channel_id"`
	Status                 TaskStatus       `gorm:"column:status"`
	FinishTime             int64            `gorm:"column:finish_time"`
	BillingState           TaskBillingState `gorm:"column:billing_state"`
	Properties             Properties       `gorm:"column:properties"`
	Quota                  int              `gorm:"column:quota"`
	PrivateData            TaskPrivateData  `gorm:"column:private_data"`
}

func newUsageAggregation(period UsageAnalyticsPeriod, opts usageViewOptions) *usageAggregation {
	return &usageAggregation{
		period:   period,
		opts:     opts,
		customer: make(map[usageCustomerKey]*usageMetricsAcc),
		upstream: make(map[usageUpstreamKey]*usageMetricsAcc),
		overview: make(map[usageOverviewKey]*usageMetricsAcc),
		taskLogs: make(map[string]*usageTaskPieces),
	}
}

func usageDayIndex(period UsageAnalyticsPeriod, timestamp int64) int {
	if timestamp < period.StartTimestamp {
		return -1
	}
	idx := int((timestamp - period.StartTimestamp) / 86400)
	if idx < 0 || idx >= len(period.Days) {
		return -1
	}
	return idx
}

func usageMonthStart(timestamp int64) int64 {
	local := time.Unix(timestamp, 0).In(usageAnalyticsLocation)
	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, usageAnalyticsLocation).Unix()
}

// GetUsageCustomerView 返回单个客户的 API Key → 客户模型用量汇总。
func GetUsageCustomerView(ctx context.Context, period UsageAnalyticsPeriod, userId int) (UsageCustomerView, error) {
	view := UsageCustomerView{Keys: []UsageCustomerKeyGroup{}, DayTotals: make([]UsageAnalyticsMetrics, len(period.Days))}
	agg, err := runUsageAggregation(ctx, period, usageViewOptions{UserID: userId, WantCustomer: true})
	if err != nil {
		return view, err
	}
	finalizeUsageCustomerView(&view, agg)
	return view, nil
}

// GetUsageUpstreamView 返回管理员上游视角的 URL → 模型 → 渠道用量汇总。
func GetUsageUpstreamView(ctx context.Context, period UsageAnalyticsPeriod, snapshots ...*UsageAnalyticsDiscountSnapshot) (UsageUpstreamView, error) {
	view := UsageUpstreamView{UrlGroups: []UsageUpstreamUrlGroup{}, DayTotals: make([]UsageAnalyticsMetrics, len(period.Days))}
	opts := usageViewOptions{WantUpstream: true}
	if len(snapshots) > 0 {
		if snapshots[0] == nil {
			return view, errors.New("usage export has no frozen discount snapshot; submit a new export")
		}
		opts.DiscountSnapshot = snapshots[0]
	}
	agg, err := runUsageAggregation(ctx, period, opts)
	if err != nil {
		return view, err
	}
	if err := finalizeUsageUpstreamView(ctx, &view, agg); err != nil {
		return view, err
	}
	return view, nil
}

// GetUsageCustomersOverview 返回全客户用量列表与范围总览。
func GetUsageCustomersOverview(ctx context.Context, period UsageAnalyticsPeriod, search string) (UsageCustomersOverview, error) {
	overview := UsageCustomersOverview{Customers: []UsageCustomerOverviewRow{}, DayTotals: make([]UsageAnalyticsMetrics, len(period.Days))}
	agg, err := runUsageAggregation(ctx, period, usageViewOptions{WantOverview: true})
	if err != nil {
		return overview, err
	}
	if err := finalizeUsageCustomersOverview(ctx, &overview, agg, search); err != nil {
		return overview, err
	}
	return overview, nil
}

// runUsageAggregation 执行完整管道：折扣预载 → 日志扫描与分类 → Task 加载与
// 交付日志按稳定 ID 合并 → 分组累加。三个投影按需构建。
func runUsageAggregation(ctx context.Context, period UsageAnalyticsPeriod, opts usageViewOptions) (*usageAggregation, error) {
	agg := newUsageAggregation(period, opts)
	if opts.DiscountSnapshot != nil {
		agg.bundle = &usageDiscountBundle{byMonth: opts.DiscountSnapshot.Months}
	} else if opts.WantUpstream {
		bundle, err := loadUsageDiscountBundle(ctx, period)
		if err != nil {
			return nil, err
		}
		agg.bundle = bundle
	}
	if err := agg.loadTasks(ctx); err != nil {
		return nil, err
	}
	agg.taskByIdentity = make(map[string]*usageTaskRecord, len(agg.tasks))
	for i := range agg.tasks {
		task := &agg.tasks[i]
		identity := usageTaskIdentity(task.UserID, task.TaskID)
		if _, exists := agg.taskByIdentity[identity]; exists {
			agg.taskByIdentity[identity] = nil
		} else {
			agg.taskByIdentity[identity] = task
		}
	}
	if err := agg.scanLogs(ctx); err != nil {
		return nil, err
	}
	if err := agg.applyFinalErrors(ctx); err != nil {
		return nil, err
	}
	if err := agg.applyTasks(ctx); err != nil {
		return nil, err
	}
	if err := agg.applyBatchJobs(ctx); err != nil {
		return nil, err
	}
	if agg.overflow {
		return nil, errors.New("usage analytics range exceeds the aggregation group limit; narrow the scope")
	}
	return agg, nil
}

// usageDiscountBundle 预载范围覆盖的各自然月渠道系数；按记录结束日所属月份
// 分别按共享月度规则读取，不做跨月平均。
type usageDiscountBundle struct {
	byMonth map[int64]map[int]ProviderChannelBillingDiscount
}

func loadUsageDiscountBundle(ctx context.Context, period UsageAnalyticsPeriod) (*usageDiscountBundle, error) {
	bundle := &usageDiscountBundle{byMonth: make(map[int64]map[int]ProviderChannelBillingDiscount)}
	months := make(map[int64]struct{})
	for i := range period.Days {
		months[usageMonthStart(period.Days[i].Start)] = struct{}{}
	}
	for monthStart := range months {
		records, err := loadProviderChannelBillingDiscounts(ctx, monthStart, nil)
		if err != nil {
			return nil, err
		}
		bundle.byMonth[monthStart] = records
	}
	return bundle, nil
}

// coefficientFor 返回某渠道在某条记录结束月的有效系数（缺少当月配置时继承上月有效值，否则为 1；
// 迁移冲突保持待填写且不投影为 1）。
func (b *usageDiscountBundle) coefficientFor(monthStart int64, channelId int) (decimal.Decimal, ProviderChannelBillingDiscount, bool) {
	records := b.byMonth[monthStart]
	record, valid := providerChannelBillingDiscountFor(records, monthStart, channelId)
	if !valid {
		return decimal.Decimal{}, ProviderChannelBillingDiscount{}, false
	}
	return record.Discount, record, true
}

// usageLogRow 是一条已解析的日志事实。
type usageLogRow struct {
	id     int64
	fact   billingReconciliationLog
	parsed parsedBillingReconciliationLog
	other  map[string]json.RawMessage
}

func usageScanLogTypes(view usageViewOptions) []int {
	return []int{LogTypeConsume, LogTypeRefund, LogTypeError}
}

// scanLogs 以有界 keyset 批次读取范围日志；ClickHouse 保持单游标交互路径。
// 交付日志（request_id 前缀 task-billing:）在这里跳过，改由 applyTasks 按
// 稳定 ID 直接加载，避免同一事件重复计入。
func (agg *usageAggregation) scanLogs(ctx context.Context) error {
	query := LOG_DB.WithContext(ctx).Model(&Log{}).
		Where("type IN ? AND created_at >= ? AND created_at < ?", usageScanLogTypes(agg.opts), agg.period.StartTimestamp, agg.period.EndTimestamp)
	if agg.opts.UserID > 0 {
		query = query.Where("user_id = ?", agg.opts.UserID)
	}
	bundle := &usageDiscountBundle{}
	if agg.bundle != nil {
		bundle = agg.bundle
	}
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return agg.scanLogsCursor(ctx, query, bundle)
	}
	boundCtx, stopBound := context.WithTimeout(ctx, 5*time.Second)
	upper, err := CustomerExportLogUpperBound(boundCtx)
	stopBound()
	if err != nil {
		return err
	}
	agg.logUpper = upper
	var cursor int64
	const batchSize = 500
	for cursor < upper {
		if err := ctx.Err(); err != nil {
			return err
		}
		batchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var batch []struct {
			ID   int64
			Fact billingReconciliationLog `gorm:"embedded"`
		}
		err := query.Session(&gorm.Session{}).WithContext(batchCtx).Select(
			"id, COALESCE(request_id, '') AS request_id, user_id, token_id, COALESCE(token_name, '') AS token_name, channel_id, COALESCE(model_name, '') AS model_name, type, created_at, prompt_tokens, completion_tokens, quota, COALESCE(content, '') AS content, COALESCE(other, '') AS other",
		).Where("id > ? AND id <= ?", cursor, upper).Order("id asc").Limit(batchSize).Scan(&batch).Error
		cancel()
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		for _, item := range batch {
			if err := agg.consumeLogRow(ctx, usageLogRow{id: item.ID, fact: item.Fact}, bundle); err != nil {
				return err
			}
		}
		cursor = batch[len(batch)-1].ID
	}
	return nil
}

// scanLogsCursor 是 ClickHouse 日志库的单游标路径；异步导出在提交时已拒绝该后端。
func (agg *usageAggregation) scanLogsCursor(ctx context.Context, query *gorm.DB, bundle *usageDiscountBundle) error {
	// Explicit column list matching the positional Scan below; a bare
	// SELECT * would return table column order and silently scramble fields.
	query = query.Session(&gorm.Session{}).Select(
		"id, user_id, COALESCE(request_id, '') AS request_id, token_id, COALESCE(token_name, '') AS token_name, channel_id, COALESCE(model_name, '') AS model_name, type, created_at, prompt_tokens, completion_tokens, quota, COALESCE(content, '') AS content, COALESCE(other, '') AS other",
	)
	rows, err := query.Order("created_at asc, id asc").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		var fact billingReconciliationLog
		var id int64
		if err := rows.Scan(&id, &fact.UserId, &fact.RequestId, &fact.TokenId, &fact.TokenName, &fact.ChannelId, &fact.ModelName, &fact.Type, &fact.CreatedAt, &fact.PromptTokens, &fact.CompletionTokens, &fact.Quota, &fact.Content, &fact.Other); err != nil {
			return err
		}
		if err := agg.consumeLogRow(ctx, usageLogRow{id: id, fact: fact}, bundle); err != nil {
			return err
		}
	}
	return rows.Err()
}

// consumeLogRow 分类一条日志并应用到对应投影。
func (agg *usageAggregation) consumeLogRow(ctx context.Context, row usageLogRow, bundle *usageDiscountBundle) error {
	if strings.HasPrefix(row.fact.RequestId, "task-billing:") {
		return nil
	}
	if strings.TrimSpace(row.fact.Other) != "" {
		if err := common.UnmarshalJsonStr(row.fact.Other, &row.other); err != nil {
			row.other = nil
		}
	}
	row.parsed = parseBillingReconciliationLog(row.fact)
	taskID := billingBreakdownString(row.other["task_id"])
	isTaskFlag, _ := billingReconciliationBool(row.other["is_task"])
	isBatch := billingBreakdownString(row.other["billing_mode"]) == "azure_batch"
	if isBatch {
		return nil
	}
	isTaskMarked := taskID != "" || isTaskFlag
	if isTaskMarked && !isBatch {
		if taskID == "" {
			// is_task 标记但无法取到 Task 身份的历史行：不能冒充独立同步调用
			// (会与其 Task 重复计数)，也无法合并——放入不会被匹配的孤儿键丢弃。
			taskID = usageAnalyticsUnknownModel + ":" + row.fact.RequestId
		}
		agg.stashTaskLogPiece(row, taskID, bundle)
		return nil
	}
	switch row.fact.Type {
	case LogTypeConsume:
		isTest := isNativeChannelTestLog(row.fact.Type, row.fact.TokenId, row.fact.TokenName, row.fact.Content)
		agg.applyStandalone(row, bundle, max(row.parsed.requestCount, 1), usageResultSuccess, isTest)
	case LogTypeError:
		agg.errorRows = append(agg.errorRows, row)
	case LogTypeRefund:
		agg.applyUnlinkedRefund(row)
	}
	return nil
}

// usageRowTokens 把一条日志的已归一用量写入累加器。
func usageRowTokens(row usageLogRow, m *UsageAnalyticsMetrics) {
	for _, key := range []string{"image_input", "image_output", "audio_input", "audio_output"} {
		if count, known := billingBreakdownNonNegativeInt(row.other[key]); known {
			mergeUsageTokenDetails(&m.TokenDetails, map[string]int64{key: count})
		}
	}
	m.InputTokens += row.parsed.inputTokens
	m.OutputTokens += row.parsed.outputTokens
	m.CacheReadTokens += row.parsed.cacheReadTokens
	m.CacheWriteTokens += row.parsed.cacheWrite.total
	if raw := row.other["image_count"]; len(raw) > 0 {
		if count, ok := billingBreakdownNonNegativeInt(raw); ok {
			m.ImageCount += count
		}
	}
}

// usageRowTokensMissing 判断一条日志是否缺乏用量证据：错误行基础字段恒为 0，
// 无法与“确认无消耗”区分；缓存语义不可解释的行输入字段不可用。
func usageRowTokensMissing(row usageLogRow, result string) bool {
	if result == usageResultFailure {
		return true
	}
	if row.parsed.unavailable || row.parsed.inputTokensUnavailable || row.parsed.cacheReadUnavailable || row.parsed.cacheWriteUnavailable {
		return true
	}
	return false
}

// usageRowSeconds 读取按秒计量的已记录用量；缺失/歧义保持未知，不反推。
func usageRowSeconds(row usageLogRow) (decimal.Decimal, bool, bool, bool) {
	value, missing, unitKnown := upstreamBillingSeconds(row.fact, row.parsed)
	if value != nil {
		return *value, true, false, unitKnown
	}
	return decimal.Decimal{}, false, missing, unitKnown
}

// usageCustomerOriginalRow 按客户账单同一算法还原一条日志的估算原价。
func usageCustomerOriginalRow(row usageLogRow) (decimal.Decimal, []string, bool) {
	quota := max(int64(row.fact.Quota), 0)
	if quota == 0 {
		return decimal.Zero, nil, false
	}
	if reasons := billingStatementEstimateReasons(row.parsed); len(reasons) > 0 {
		return decimal.Zero, reasons, false
	}
	contractRatio := 1.0
	if row.parsed.contractDiscountRatio != nil && *row.parsed.contractDiscountRatio > 0 {
		contractRatio = *row.parsed.contractDiscountRatio
	}
	original := decimal.NewFromInt(quota).Div(decimal.NewFromFloat(*row.parsed.discountRatio)).Div(decimal.NewFromFloat(contractRatio))
	if row.fact.Type == LogTypeRefund {
		original = original.Neg()
	}
	return original, nil, true
}

// usageUpstreamRowAmount 还原一条日志的上游原价与折后参考金额（逐笔取整）。
func usageUpstreamRowAmount(row usageLogRow, bundle *usageDiscountBundle) (decimal.Decimal, decimal.Decimal, []string, decimal.Decimal, bool) {
	quota := max(int64(row.fact.Quota), 0)
	if quota == 0 {
		return decimal.Decimal{}, decimal.Decimal{}, nil, decimal.Decimal{}, false
	}
	original, reasons := upstreamOriginalQuota(row.fact, row.parsed)
	if len(reasons) > 0 {
		return decimal.Decimal{}, decimal.Decimal{}, reasons, decimal.Decimal{}, false
	}
	coefficient, _, valid := bundle.coefficientFor(usageMonthStart(row.fact.CreatedAt), row.fact.ChannelId)
	if !valid {
		return decimal.Decimal{}, decimal.Decimal{}, []string{BillingEstimateMissingGroup}, decimal.Decimal{}, false
	}
	return original, UpstreamReferenceQuota(original, coefficient), nil, coefficient, true
}

// stashTaskLogPiece takes one task-marked log row and merges it into its task.
func (agg *usageAggregation) stashTaskLogPiece(row usageLogRow, taskID string, bundle *usageDiscountBundle) {
	identity := usageTaskIdentity(row.fact.UserId, taskID)
	task := agg.taskByIdentity[identity]
	if task == nil {
		return
	}
	row.fact.CreatedAt = task.FinishTime
	identity = usageDeliveryRequestID(task.RowID, "facts")
	pieces := agg.taskLogs[identity]
	if pieces == nil {
		pieces = &usageTaskPieces{}
		agg.taskLogs[identity] = pieces
	}
	row.fact.CreatedAt = task.FinishTime
	agg.appendTaskLogPiece(pieces, row, bundle)
}

// stashTaskLogPieceForTask 把交付日志按已解析的 Task 归并。
func (agg *usageAggregation) stashTaskLogPieceForTask(task *usageTaskRecord, row usageLogRow, bundle *usageDiscountBundle) {
	identity := usageDeliveryRequestID(task.RowID, "facts")
	pieces := agg.taskLogs[identity]
	if pieces == nil {
		pieces = &usageTaskPieces{}
		agg.taskLogs[identity] = pieces
	}
	row.fact.CreatedAt = task.FinishTime
	agg.appendTaskLogPiece(pieces, row, bundle)
}

// appendTaskLogPiece 把一条关联日志的已知事实合并到任务合并事实。
func (agg *usageAggregation) appendTaskLogPiece(pieces *usageTaskPieces, row usageLogRow, bundle *usageDiscountBundle) {
	pieces.rowsSeen++
	if row.fact.Type == LogTypeConsume {
		pieces.gross += max(int64(row.fact.Quota), 0)
	} else if row.fact.Type == LogTypeRefund {
		pieces.refund += max(int64(row.fact.Quota), 0)
	}
	var tokens UsageAnalyticsMetrics
	usageRowTokens(row, &tokens)
	mergeUsageTokenDetails(&pieces.tokenDetails, tokens.TokenDetails)
	pieces.input += tokens.InputTokens
	pieces.output += tokens.OutputTokens
	pieces.cacheRead += tokens.CacheReadTokens
	pieces.cacheWrite += tokens.CacheWriteTokens
	pieces.imageCount += tokens.ImageCount
	if value, known, missing, _ := usageRowSeconds(row); known {
		pieces.seconds = pieces.seconds.Add(value)
		pieces.secondsKnown = true
	} else if missing {
		pieces.missingSeconds++
	}
	if usageRowTokensMissing(row, usageResultSuccess) {
		pieces.missingTokens++
	}
	if row.parsed.billingMode != BillingReconciliationModeUnknown && pieces.billingMode == "" {
		pieces.billingMode = row.parsed.billingMode
	}
	if pieces.tokenId == 0 && row.fact.TokenId > 0 {
		pieces.tokenId = row.fact.TokenId
		pieces.tokenName = row.fact.TokenName
	}
	if original, reasons, known := usageCustomerOriginalRow(row); known {
		pieces.customerOriginal = pieces.customerOriginal.Add(original)
		pieces.originalKnown = true
	} else if len(reasons) > 0 {
		pieces.estimateReasons = mergeBillingEstimateReasons(pieces.estimateReasons, reasons...)
		pieces.originalIncomplete = true
	}
	if agg.opts.WantUpstream && row.parsed.taskBillingEvent != "customer_refund" && !isNativeChannelTestLog(row.fact.Type, row.fact.TokenId, row.fact.TokenName, row.fact.Content) {
		if original, reference, reasons, coefficient, known := usageUpstreamRowAmount(row, bundle); known {
			pieces.upstreamOriginal = pieces.upstreamOriginal.Add(original)
			pieces.upstreamReference = pieces.upstreamReference.Add(reference)
			pieces.upstreamMoneyKnown = true
			pieces.addCoefficient(coefficient)
		} else if len(reasons) > 0 {
			pieces.upstreamIncomplete = true
			pieces.estimateReasons = mergeBillingEstimateReasons(pieces.estimateReasons, reasons...)
		}
	}
}

// applyStandalone 把一条独立同步消费/最终错误日志应用到
// 启用的投影。原生渠道测试行只进入上游视图，按证据区分已计价与待核算。
func (agg *usageAggregation) applyStandalone(row usageLogRow, bundle *usageDiscountBundle, calls int64, result string, isTest bool) {
	day := usageDayIndex(agg.period, row.fact.CreatedAt)
	if day < 0 {
		return
	}
	if agg.opts.WantUpstream && row.fact.Type == LogTypeConsume {
		acc := agg.buildUpstreamStandalone(row, bundle, calls, result, isTest)
		if acc != nil {
			model := strings.TrimSpace(row.parsed.providerModel)
			fallback := false
			if model == "" {
				model = row.fact.ModelName
				fallback = true
			}
			if model == "" {
				model = usageAnalyticsUnknownModel
			}
			key := usageUpstreamKey{day: day, channel: row.fact.ChannelId, model: model, fallback: fallback, mode: row.parsed.billingMode}
			agg.mergeUpstream(key, acc)
		}
	}
	if isTest {
		return
	}
	acc := agg.buildCustomerStandalone(row, calls, result)
	key := usageCustomerKey{day: day, token: row.fact.TokenId, model: row.parsed.customerModel}
	agg.mergeCustomer(key, acc)
	if agg.opts.WantOverview {
		agg.mergeOverview(usageOverviewKey{user: row.fact.UserId, day: day}, acc)
	}
}

// buildUpstreamStandalone 构建上游视图的独立行累加器；无法安全归属渠道或
// 没有渠道证据的行不虚构分组。
func (agg *usageAggregation) buildUpstreamStandalone(row usageLogRow, bundle *usageDiscountBundle, calls int64, result string, isTest bool) *usageMetricsAcc {
	acc := &usageMetricsAcc{}
	acc.addCalls(result, calls)
	var tokens UsageAnalyticsMetrics
	usageRowTokens(row, &tokens)
	acc.mergeTokens(tokens)
	if usageRowTokensMissing(row, result) {
		acc.m.RowsMissingTokens += max(calls, 1)
	}
	if value, known, missing, unitKnown := usageRowSeconds(row); known {
		acc.addSeconds(value, true, false)
	} else if missing {
		acc.m.SecondsMissingRows += max(calls, 1)
		if unitKnown {
			acc.m.SecondsValueMissingRows += max(calls, 1)
		}
	}
	if isTest {
		// 原生渠道测试没有客户资金扣款：有可靠计价依据时计入上游参考金额；
		// 无法核算的测试保留用量、单列待核算，并使金额完整性标记不完整。
		amount := upstreamTestAmountFor(row.fact, row.parsed)
		if amount.known {
			acc.m.TestPricedRows += max(calls, 1)
			if coefficient, _, valid := bundle.coefficientFor(usageMonthStart(row.fact.CreatedAt), row.fact.ChannelId); valid {
				acc.original = acc.original.Add(amount.original)
				acc.originalKnown = true
				acc.reference = acc.reference.Add(UpstreamReferenceQuota(amount.original, coefficient))
				acc.addCoefficient(coefficient)
			} else {
				acc.m.EstimateReasons = mergeBillingEstimateReasons(acc.m.EstimateReasons, BillingEstimateMissingGroup)
				acc.originalIncomplete = true
			}
			return acc
		}
		acc.m.UsageOnlyRows += max(calls, 1)
		acc.m.EstimateReasons = mergeBillingEstimateReasons(acc.m.EstimateReasons, BillingEstimateTestAmountPending)
		acc.originalIncomplete = true
		return acc
	}
	acc.m.GrossQuota += max(int64(row.fact.Quota), 0)
	if original, reference, reasons, coefficient, known := usageUpstreamRowAmount(row, bundle); known {
		acc.original = acc.original.Add(original)
		acc.originalKnown = true
		acc.reference = acc.reference.Add(reference)
		acc.addCoefficient(coefficient)
	} else if len(reasons) > 0 {
		acc.m.EstimateReasons = mergeBillingEstimateReasons(acc.m.EstimateReasons, reasons...)
		acc.originalIncomplete = true
	}
	return acc
}

// buildCustomerStandalone 构建客户/总览视图的独立行累加器。
func (agg *usageAggregation) buildCustomerStandalone(row usageLogRow, calls int64, result string) *usageMetricsAcc {
	acc := &usageMetricsAcc{}
	acc.addCalls(result, calls)
	var tokens UsageAnalyticsMetrics
	usageRowTokens(row, &tokens)
	acc.mergeTokens(tokens)
	if usageRowTokensMissing(row, result) {
		acc.m.RowsMissingTokens += max(calls, 1)
	}
	if value, known, missing, unitKnown := usageRowSeconds(row); known {
		acc.addSeconds(value, true, false)
	} else if missing {
		acc.m.SecondsMissingRows += max(calls, 1)
		if unitKnown {
			acc.m.SecondsValueMissingRows += max(calls, 1)
		}
	}
	if row.fact.Type == LogTypeConsume {
		acc.m.GrossQuota += max(int64(row.fact.Quota), 0)
		if original, reasons, known := usageCustomerOriginalRow(row); known {
			acc.original = acc.original.Add(original)
			acc.originalKnown = true
		} else if len(reasons) > 0 {
			acc.m.EstimateReasons = mergeBillingEstimateReasons(acc.m.EstimateReasons, reasons...)
			acc.originalIncomplete = true
		}
	}
	return acc
}

// applyUnlinkedRefund 把无法关联到已结束调用的退款记录为该日、该 Key、该模型
// 的资金事实：不增加调用次数，也不冒充某次调用的净额。
func (agg *usageAggregation) applyUnlinkedRefund(row usageLogRow) {
	if agg.opts.WantUpstream {
		// 上游视图不把客户退款当作 Provider 冲减。
		return
	}
	day := usageDayIndex(agg.period, row.fact.CreatedAt)
	if day < 0 {
		return
	}
	quota := max(int64(row.fact.Quota), 0)
	if quota == 0 {
		return
	}
	acc := &usageMetricsAcc{}
	acc.m.UnlinkedRefundQuota += quota
	key := usageCustomerKey{day: day, token: row.fact.TokenId, model: row.parsed.customerModel}
	agg.mergeCustomer(key, acc)
	if agg.opts.WantOverview {
		agg.mergeOverview(usageOverviewKey{user: row.fact.UserId, day: day}, acc)
	}
}

func (agg *usageAggregation) mergeCustomer(key usageCustomerKey, acc *usageMetricsAcc) {
	if !agg.opts.WantCustomer {
		return
	}
	existing, ok := agg.customer[key]
	if !ok {
		if len(agg.customer) >= maxUsageAnalyticsGroups {
			agg.overflow = true
			return
		}
		agg.customer[key] = acc
		return
	}
	existing.merge(acc)
}

func (agg *usageAggregation) mergeUpstream(key usageUpstreamKey, acc *usageMetricsAcc) {
	existing, ok := agg.upstream[key]
	if !ok {
		if len(agg.upstream) >= maxUsageAnalyticsGroups {
			agg.overflow = true
			return
		}
		agg.upstream[key] = acc
		return
	}
	existing.merge(acc)
}

func (agg *usageAggregation) mergeOverview(key usageOverviewKey, acc *usageMetricsAcc) {
	existing, ok := agg.overview[key]
	if !ok {
		agg.overview[key] = acc
		return
	}
	existing.merge(acc)
}

// loadTasks 读取范围内已结束 Task（按 finish_time 归属），有界 keyset 批次。
// azure_batch 作业由 BatchJob 终态与行级计数代表，不作为独立 Task 调用。
func (agg *usageAggregation) loadTasks(ctx context.Context) error {
	query := DB.WithContext(ctx).Model(&Task{}).
		Where("status IN ? AND finish_time >= ? AND finish_time < ?", TerminalTaskStatuses(), agg.period.StartTimestamp, agg.period.EndTimestamp).
		Where("platform <> ?", constant.TaskPlatformAzureBatch)
	if agg.opts.UserID > 0 {
		query = query.Where("user_id = ?", agg.opts.UserID)
	}
	const batchSize = 500
	var lastID int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var batch []usageTaskRecord
		err := query.Session(&gorm.Session{}).
			Select("id, task_id, user_id, channel_id, status, finish_time, billing_state, properties, quota, private_data, video_refund_state, video_refund_completed_at").
			Where("id > ?", lastID).Order("id asc").Limit(batchSize).Scan(&batch).Error
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		for i := range batch {
			agg.tasks = append(agg.tasks, batch[i])
		}
		lastID = batch[len(batch)-1].RowID
	}
	return nil
}

// usageDeliveryRequestID 还原交付日志的确定性 request_id。
func usageDeliveryRequestID(rowID int64, event string) string {
	return "task-billing:" + strconv.FormatInt(rowID, 10) + ":" + event
}

// loadDeliveryLogs 按稳定 request_id 直接加载范围内任务的交付日志。创建、差额、
// 退款与图片完成事件不受日志落库日期影响，都能回到 Task 的结束期。
func (agg *usageAggregation) loadDeliveryLogs(ctx context.Context) error {
	var rowIDs []int64
	for i := range agg.tasks {
		rowIDs = append(rowIDs, agg.tasks[i].RowID)
	}
	if len(rowIDs) == 0 {
		return nil
	}
	rowToTask := make(map[int64]*usageTaskRecord, len(rowIDs))
	for i := range agg.tasks {
		rowToTask[agg.tasks[i].RowID] = &agg.tasks[i]
	}
	requestToTask := make(map[string]*usageTaskRecord, len(rowIDs)*5)
	var requestIDs []string
	for _, rowID := range rowIDs {
		for _, event := range []string{"create", "adjustment", "refund", "complete", "customer_refund"} {
			requestID := usageDeliveryRequestID(rowID, event)
			requestIDs = append(requestIDs, requestID)
			requestToTask[requestID] = rowToTask[rowID]
		}
	}
	bundle := agg.bundle
	if bundle == nil {
		bundle = &usageDiscountBundle{}
	}
	const chunk = 200
	for start := 0; start < len(rowIDs); start += chunk {
		var pending []TaskBillingDelivery
		if err := DB.WithContext(ctx).Select("task_row_id").Where("task_row_id IN ? AND (delivered_at = 0 OR suppress_log = ?)", rowIDs[start:min(start+chunk, len(rowIDs))], true).Find(&pending).Error; err != nil {
			return err
		}
		for _, event := range pending {
			rowToTask[event.TaskRowID].DeliveryIncomplete = true
		}
	}
	for start := 0; start < len(requestIDs); start += chunk {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := min(start+chunk, len(requestIDs))
		var logs []Log
		err := LOG_DB.WithContext(ctx).
			Select("id, request_id, user_id, token_id, token_name, channel_id, model_name, type, created_at, prompt_tokens, completion_tokens, quota, content, other").
			Where("request_id IN ?", requestIDs[start:end]).Find(&logs).Error
		if err != nil {
			return err
		}
		for _, row := range logs {
			task := requestToTask[row.RequestId]
			if task == nil || row.UserId != task.UserID {
				continue
			}
			fact := upstreamBillingLogFact(row, "")
			parsed := parseBillingReconciliationLog(fact)
			var other map[string]json.RawMessage
			if strings.TrimSpace(row.Other) != "" {
				_ = common.UnmarshalJsonStr(row.Other, &other)
			}
			logRow := usageLogRow{fact: fact, parsed: parsed, other: other}
			agg.stashTaskLogPieceForTask(task, logRow, bundle)
		}
	}
	return nil
}

// applyTasks merges each in-range task with its merged log facts. Task row
// provides identity, terminal status and the finish day; the merged pieces
// provide usage, money and API Key identity.
func (agg *usageAggregation) applyTasks(ctx context.Context) error {
	if err := agg.loadDeliveryLogs(ctx); err != nil {
		return err
	}
	for i := range agg.tasks {
		task := &agg.tasks[i]
		identity := usageDeliveryRequestID(task.RowID, "facts")
		pieces := agg.taskLogs[identity]
		delete(agg.taskLogs, identity)
		if pieces == nil {
			pieces = &usageTaskPieces{}
		}
		if pieces.tokenId == 0 {
			pieces.tokenId = task.PrivateData.TokenId
		}
		if async := task.PrivateData.AsyncBilling; async != nil {
			if async.ActualUsageReported {
				pieces.output = int64(async.ActualTokens)
				pieces.missingTokens = 0
			} else {
				pieces.missingTokens = 1
			}
		}
		if image := task.PrivateData.ImageTask; image != nil && image.Usage != nil {
			pieces.input = int64(image.Usage.PromptTokens)
			pieces.output = int64(image.Usage.CompletionTokens)
			pieces.cacheRead = int64(image.Usage.PromptTokensDetails.CachedTokens)
			pieces.missingTokens = 0
			// Image usage details come from the durable provider usage snapshot.
			if image.Usage.InputTokensDetails != nil {
				if pieces.tokenDetails == nil {
					pieces.tokenDetails = make(map[string]int64)
				}
				pieces.tokenDetails["image_input"] = int64(image.Usage.InputTokensDetails.ImageTokens)
			}
			if image.Usage.OutputTokensDetails != nil {
				if pieces.tokenDetails == nil {
					pieces.tokenDetails = make(map[string]int64)
				}
				pieces.tokenDetails["image_output"] = int64(image.Usage.OutputTokensDetails.ImageTokens)
			}
			if image.GenerationComplete {
				pieces.imageCount = int64(image.ImageCount)
			}
		} else if task.PrivateData.AsyncBilling == nil && pieces.input == 0 && pieces.output == 0 && pieces.imageCount == 0 && !pieces.secondsKnown {
			pieces.missingTokens = 1
		}
		if pieces.missingTokens > 0 {
			pieces.missingTokens = 1
		}
		if !pieces.secondsKnown {
			// 任务已按终态加载：冻结快照的实测报告门控保证预算值不冒充实测。
			if seconds, ok := trustedTaskSnapshotSeconds(task.PrivateData); ok {
				pieces.seconds = seconds
				pieces.secondsKnown = true
				pieces.missingSeconds = 0
			}
		}
		if pieces.missingSeconds > 0 || (pieces.billingMode == BillingReconciliationModePerSecond && !pieces.secondsKnown) {
			pieces.missingSeconds = 1
		}
		day := usageDayIndex(agg.period, task.FinishTime)
		if day < 0 {
			continue
		}
		result := usageResultFromTaskStatus(task.Status)
		settled := task.BillingState == "" || task.BillingState == TaskBillingStateSettled ||
			(task.VideoRefundState == "refunded" && task.VideoRefundCompletedAt > 0 && task.Quota == 0)
		hasRows := pieces != nil && pieces.rowsSeen > 0
		if agg.opts.WantCustomer || agg.opts.WantOverview {
			agg.applyTaskCustomer(task, pieces, hasRows, settled, result, day)
		}
		if agg.opts.WantUpstream {
			agg.applyTaskUpstream(task, pieces, hasRows, settled, result, day)
		}
	}
	// Pieces whose task finished outside this range stay unapplied on purpose:
	// they belong to the finish day, not to the log day.
	return nil
}

// applyTaskCustomer applies one task to the customer/overview projections.
func (agg *usageAggregation) applyTaskCustomer(task *usageTaskRecord, pieces *usageTaskPieces, hasRows bool, settled bool, result string, day int) {
	acc := &usageMetricsAcc{}
	acc.addCalls(result, 1)
	if pieces != nil {
		mergeUsageTokenDetails(&acc.m.TokenDetails, pieces.tokenDetails)
		acc.m.InputTokens += pieces.input
		acc.m.OutputTokens += pieces.output
		acc.m.CacheReadTokens += pieces.cacheRead
		acc.m.CacheWriteTokens += pieces.cacheWrite
		acc.m.ImageCount += pieces.imageCount
		acc.m.RowsMissingTokens += pieces.missingTokens
		acc.m.SecondsMissingRows += pieces.missingSeconds
	}

	if pieces != nil && pieces.secondsKnown {
		acc.addSeconds(pieces.seconds, true, false)
	}
	if !settled {
		acc.m.RowsMoneyPending++
	}
	if pieces != nil {
		acc.m.GrossQuota += pieces.gross
		acc.m.RefundQuota += pieces.refund
		acc.original = acc.original.Add(pieces.customerOriginal)
		acc.originalKnown = acc.originalKnown || pieces.originalKnown
		acc.originalIncomplete = acc.originalIncomplete || pieces.originalIncomplete
		acc.m.EstimateReasons = mergeBillingEstimateReasons(acc.m.EstimateReasons, pieces.estimateReasons...)
	}
	if !hasRows || !settled || task.DeliveryIncomplete || pieces.gross-pieces.refund != int64(task.Quota) {
		acc.m.RowsMissingMoney++
		acc.originalIncomplete = true
		// Task quota is a final funds fact only after settlement. Incomplete
		// log delivery must never expose the pre-hold as the final net charge.
		acc.m.GrossQuota, acc.m.RefundQuota = 0, 0
		if settled {
			acc.m.GrossQuota = max(int64(task.Quota), 0)
		}
	}
	model := task.Properties.OriginModelName
	if model == "" {
		model = usageAnalyticsUnknownModel
	}
	tokenId := 0
	if pieces != nil {
		tokenId = pieces.tokenId
	}
	key := usageCustomerKey{day: day, token: tokenId, model: model}
	agg.mergeCustomer(key, acc)
	if agg.opts.WantOverview {
		agg.mergeOverview(usageOverviewKey{user: task.UserID, day: day}, acc)
	}
}

// applyTaskUpstream applies one task to the upstream projection.
func (agg *usageAggregation) applyTaskUpstream(task *usageTaskRecord, pieces *usageTaskPieces, hasRows bool, settled bool, result string, day int) {
	acc := &usageMetricsAcc{}
	acc.addCalls(result, 1)
	if pieces != nil {
		mergeUsageTokenDetails(&acc.m.TokenDetails, pieces.tokenDetails)
		acc.m.InputTokens += pieces.input
		acc.m.OutputTokens += pieces.output
		acc.m.CacheReadTokens += pieces.cacheRead
		acc.m.CacheWriteTokens += pieces.cacheWrite
		acc.m.ImageCount += pieces.imageCount
		acc.m.RowsMissingTokens += pieces.missingTokens
		acc.m.SecondsMissingRows += pieces.missingSeconds
	}

	if pieces != nil && pieces.secondsKnown {
		acc.addSeconds(pieces.seconds, true, false)
	}
	if !settled {
		acc.m.RowsMoneyPending++
	}
	if pieces != nil {
		acc.m.GrossQuota += pieces.gross
		acc.m.RefundQuota += pieces.refund
		acc.original = acc.original.Add(pieces.upstreamOriginal)
		acc.originalKnown = acc.originalKnown || pieces.upstreamMoneyKnown
		acc.originalIncomplete = acc.originalIncomplete || pieces.upstreamIncomplete || (pieces.rowsSeen > 0 && !pieces.upstreamMoneyKnown)
		acc.reference = acc.reference.Add(pieces.upstreamReference)
		for coefficient := range pieces.coefficients {
			acc.addCoefficient(decimal.RequireFromString(coefficient))
		}
		acc.m.EstimateReasons = mergeBillingEstimateReasons(acc.m.EstimateReasons, pieces.estimateReasons...)
	}
	if !hasRows || !settled || task.DeliveryIncomplete || pieces.gross-pieces.refund != int64(task.Quota) {
		acc.m.RowsMissingMoney++
		acc.originalIncomplete = true
		// Task quota is a final funds fact only after settlement. Incomplete
		// log delivery must never expose the pre-hold as the final net charge.
		acc.m.GrossQuota, acc.m.RefundQuota = 0, 0
		if settled {
			acc.m.GrossQuota = max(int64(task.Quota), 0)
		}
	}
	// Customer funding can be closed while provider settlement is still unknown.
	if task.VideoRefundState == "refunded" && task.BillingState != "" && task.BillingState != TaskBillingStateSettled {
		acc.originalIncomplete = true
	}
	provider := strings.TrimSpace(task.Properties.UpstreamModelName)
	fallback := false
	if provider == "" {
		provider = strings.TrimSpace(task.Properties.OriginModelName)
		fallback = true
	}
	if provider == "" {
		provider = usageAnalyticsUnknownModel
	}
	mode := BillingReconciliationModeUnknown
	if pieces != nil && pieces.billingMode != "" {
		mode = pieces.billingMode
	}
	key := usageUpstreamKey{day: day, channel: task.ChannelID, model: provider, fallback: fallback, mode: mode}
	agg.mergeUpstream(key, acc)
}

func (agg *usageAggregation) foldCustomerDayTotals() ([]UsageAnalyticsMetrics, UsageAnalyticsMetrics) {
	dayAccs := make([]usageMetricsAcc, len(agg.period.Days))
	total := usageMetricsAcc{}
	for key, acc := range agg.customer {
		if key.day >= 0 && key.day < len(dayAccs) {
			dayAccs[key.day].merge(acc)
		}
		total.merge(acc)
	}
	days := make([]UsageAnalyticsMetrics, len(agg.period.Days))
	for i := range dayAccs {
		days[i] = dayAccs[i].finalize()
	}
	return days, total.finalize()
}

func (agg *usageAggregation) foldUpstreamDayTotals() ([]UsageAnalyticsMetrics, UsageAnalyticsMetrics) {
	dayAccs := make([]usageMetricsAcc, len(agg.period.Days))
	total := usageMetricsAcc{}
	for key, acc := range agg.upstream {
		if key.day >= 0 && key.day < len(dayAccs) {
			dayAccs[key.day].merge(acc)
		}
		total.merge(acc)
	}
	days := make([]UsageAnalyticsMetrics, len(agg.period.Days))
	for i := range dayAccs {
		days[i] = dayAccs[i].finalize()
	}
	return days, total.finalize()
}

// finalizeUsageCustomerView assembles the customer view: token groups with
// model leaves, per-day arrays and range totals, plus token identity lookup.
func finalizeUsageCustomerView(view *UsageCustomerView, agg *usageAggregation) {
	view.DayTotals, view.Total = agg.foldCustomerDayTotals()
	type tokenBuild struct {
		group  UsageCustomerKeyGroup
		days   []usageMetricsAcc
		total  usageMetricsAcc
		models map[string]*struct {
			row  UsageCustomerModelRow
			days []usageMetricsAcc
		}
	}
	builds := make(map[int]*tokenBuild)
	for key, acc := range agg.customer {
		build, ok := builds[key.token]
		if !ok {
			build = &tokenBuild{group: UsageCustomerKeyGroup{TokenId: key.token}, days: make([]usageMetricsAcc, len(agg.period.Days)),
				models: make(map[string]*struct {
					row  UsageCustomerModelRow
					days []usageMetricsAcc
				})}
			builds[key.token] = build
		}
		modelEntry, ok := build.models[key.model]
		if !ok {
			modelEntry = &struct {
				row  UsageCustomerModelRow
				days []usageMetricsAcc
			}{row: UsageCustomerModelRow{ModelName: key.model, Days: make([]UsageAnalyticsMetrics, len(agg.period.Days))},
				days: make([]usageMetricsAcc, len(agg.period.Days))}
			build.models[key.model] = modelEntry
		}
		modelEntry.days[key.day].merge(acc)
		build.days[key.day].merge(acc)
		build.total.merge(acc)
	}
	tokenIds := make([]int, 0, len(builds))
	for id := range builds {
		tokenIds = append(tokenIds, id)
	}
	sort.Ints(tokenIds)
	for _, id := range tokenIds {
		build := builds[id]
		for i := range build.days {
			build.group.Days = append(build.group.Days, build.days[i].finalize())
		}
		build.group.Total = build.total.finalize()
		modelNames := make([]string, 0, len(build.models))
		for name := range build.models {
			modelNames = append(modelNames, name)
		}
		sort.Strings(modelNames)
		for _, name := range modelNames {
			entry := build.models[name]
			for i := range entry.days {
				entry.row.Days[i] = entry.days[i].finalize()
			}
			dayAccs := entry.days
			entryTotal := usageMetricsAcc{}
			for i := range dayAccs {
				entryTotal.merge(&dayAccs[i])
			}
			entry.row.Total = entryTotal.finalize()
			build.group.Models = append(build.group.Models, entry.row)
		}
		sort.Slice(build.group.Models, func(i, j int) bool {
			if build.group.Models[i].Total.NetQuota != build.group.Models[j].Total.NetQuota {
				return build.group.Models[i].Total.NetQuota > build.group.Models[j].Total.NetQuota
			}
			return build.group.Models[i].ModelName < build.group.Models[j].ModelName
		})
		view.Keys = append(view.Keys, build.group)
	}
	sort.Slice(view.Keys, func(i, j int) bool {
		if view.Keys[i].Total.NetQuota != view.Keys[j].Total.NetQuota {
			return view.Keys[i].Total.NetQuota > view.Keys[j].Total.NetQuota
		}
		if view.Keys[i].TokenId != view.Keys[j].TokenId {
			return view.Keys[i].TokenId < view.Keys[j].TokenId
		}
		return view.Keys[i].TokenName < view.Keys[j].TokenName
	})
	attachUsageTokenIdentities(view.Keys)
}

// attachUsageTokenIdentities batch-loads API Key names; deleted keys keep their
// historical identity with a deleted marker, unknown keys show "no API Key".
func attachUsageTokenIdentities(keys []UsageCustomerKeyGroup) {
	ids := make([]int, 0, len(keys))
	for i := range keys {
		if keys[i].TokenId > 0 {
			ids = append(ids, keys[i].TokenId)
		}
	}
	if len(ids) == 0 {
		for i := range keys {
			if keys[i].TokenId == 0 {
				keys[i].TokenName = usageAnalyticsUnknownKeyLabel
			}
		}
		return
	}
	var rows []struct {
		Id        int
		Name      string
		DeletedAt gorm.DeletedAt
	}
	if err := DB.Unscoped().Model(&Token{}).Select("id, name, deleted_at").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		logUsageAnalyticsIdentityError(err)
		return
	}
	names := make(map[int]struct {
		name    string
		deleted bool
	}, len(rows))
	for _, row := range rows {
		names[row.Id] = struct {
			name    string
			deleted bool
		}{row.Name, row.DeletedAt.Valid}
	}
	for i := range keys {
		if keys[i].TokenId == 0 {
			keys[i].TokenName = usageAnalyticsUnknownKeyLabel
			continue
		}
		if info, ok := names[keys[i].TokenId]; ok {
			keys[i].TokenName = info.name
			keys[i].TokenDeleted = info.deleted
		} else {
			keys[i].TokenName = fmt.Sprintf("API Key #%d", keys[i].TokenId)
			keys[i].TokenDeleted = true
		}
	}
}

func logUsageAnalyticsIdentityError(err error) {
	common.SysError("usage analytics token identity lookup failed: " + err.Error())
}

const usageAnalyticsUnknownKeyLabel = "unknown"

// finalizeUsageUpstreamView assembles URL groups: current base URL identity via
// the shared grouping rule, channel rows with their month coefficients, and
// model leaves. Channel tests stay flagged as usage-only.
func finalizeUsageUpstreamView(ctx context.Context, view *UsageUpstreamView, agg *usageAggregation) error {
	view.DayTotals, view.Total = agg.foldUpstreamDayTotals()
	channelIDs := make([]int, 0)
	seen := map[int]bool{}
	for key := range agg.upstream {
		if !seen[key.channel] {
			seen[key.channel] = true
			channelIDs = append(channelIDs, key.channel)
		}
	}
	channels, err := getBillingURLChannelsById(channelIDs)
	if err != nil {
		return err
	}
	type modelKey struct {
		name, mode string
		fallback   bool
	}
	type channelBuild struct {
		row  UsageUpstreamChannelRow
		days []usageMetricsAcc
	}
	type modelBuild struct {
		row      UsageUpstreamModelRow
		channels map[int]*channelBuild
	}
	type urlBuild struct {
		row    UsageUpstreamUrlGroup
		models map[modelKey]*modelBuild
	}
	groups := map[string]*urlBuild{}
	for key, acc := range agg.upstream {
		if err := ctx.Err(); err != nil {
			return err
		}
		url, name := billingURLGroupIdentity(channels[key.channel], channels[key.channel].Id != 0 || key.channel == 0, key.channel)
		group := groups[url]
		if group == nil {
			group = &urlBuild{row: UsageUpstreamUrlGroup{UrlKey: url, DisplayName: name, Unidentified: strings.HasPrefix(url, billingURLGroupChannelFallbackPrefix)}, models: map[modelKey]*modelBuild{}}
			if !group.row.Unidentified {
				group.row.BaseURL = url
			}
			groups[url] = group
		}
		mk := modelKey{name: key.model, mode: key.mode, fallback: key.fallback}
		mb := group.models[mk]
		if mb == nil {
			mb = &modelBuild{row: UsageUpstreamModelRow{ProviderModel: key.model, BillingMode: key.mode, ProviderModelFallback: key.fallback}, channels: map[int]*channelBuild{}}
			group.models[mk] = mb
		}
		cb := mb.channels[key.channel]
		if cb == nil {
			cb = &channelBuild{row: UsageUpstreamChannelRow{ChannelId: key.channel, ChannelName: strings.TrimSpace(channels[key.channel].Name), Discounts: []UsageChannelMonthDiscount{}}, days: make([]usageMetricsAcc, len(agg.period.Days))}
			if cb.row.ChannelName == "" {
				cb.row.ChannelName = fmt.Sprintf("Channel #%d", key.channel)
			}
			for month := range agg.bundle.byMonth {
				_, record, valid := agg.bundle.coefficientFor(month, key.channel)
				if !valid {
					continue
				}
				cb.row.Discounts = append(cb.row.Discounts, UsageChannelMonthDiscount{PeriodStart: month, Value: record.Discount, Version: record.Version, Source: usageDiscountSource(record), SourcePeriod: record.CopiedFromPeriod})
			}
			sort.Slice(cb.row.Discounts, func(i, j int) bool { return cb.row.Discounts[i].PeriodStart < cb.row.Discounts[j].PeriodStart })
			mb.channels[key.channel] = cb
		}
		cb.days[key.day].merge(acc)
	}
	urls := make([]string, 0, len(groups))
	for url := range groups {
		urls = append(urls, url)
	}
	sort.Strings(urls)
	for _, url := range urls {
		group := groups[url]
		urlDays := make([]usageMetricsAcc, len(agg.period.Days))
		urlTotal := usageMetricsAcc{}
		keys := make([]modelKey, 0, len(group.models))
		for key := range group.models {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].name != keys[j].name {
				return keys[i].name < keys[j].name
			}
			if keys[i].mode != keys[j].mode {
				return keys[i].mode < keys[j].mode
			}
			return !keys[i].fallback && keys[j].fallback
		})
		for _, key := range keys {
			mb := group.models[key]
			modelDays := make([]usageMetricsAcc, len(agg.period.Days))
			modelTotal := usageMetricsAcc{}
			ids := make([]int, 0, len(mb.channels))
			for id := range mb.channels {
				ids = append(ids, id)
			}
			sort.Ints(ids)
			for _, id := range ids {
				cb := mb.channels[id]
				total := usageMetricsAcc{}
				for i := range cb.days {
					cb.row.Days = append(cb.row.Days, cb.days[i].finalize())
					total.merge(&cb.days[i])
					modelDays[i].merge(&cb.days[i])
				}
				cb.row.Total = total.finalize()
				cb.row.UsageOnly = cb.row.Total.TotalCalls > 0 && cb.row.Total.UsageOnlyRows >= cb.row.Total.TotalCalls
				modelTotal.merge(&total)
				mb.row.Channels = append(mb.row.Channels, cb.row)
			}
			for i := range modelDays {
				mb.row.Days = append(mb.row.Days, modelDays[i].finalize())
				urlDays[i].merge(&modelDays[i])
			}
			mb.row.Total = modelTotal.finalize()
			urlTotal.merge(&modelTotal)
			group.row.Models = append(group.row.Models, mb.row)
		}
		for i := range urlDays {
			group.row.Days = append(group.row.Days, urlDays[i].finalize())
		}
		group.row.Total = urlTotal.finalize()
		view.UrlGroups = append(view.UrlGroups, group.row)
	}
	return nil
}

// finalizeUsageCustomersOverview assembles the per-customer list with range
// totals and the overall day/range totals. Totals and day buckets always
// aggregate exactly the listed (search-filtered) customers, matching the
// billing list convention that the header total is the filtered full set.
func finalizeUsageCustomersOverview(ctx context.Context, overview *UsageCustomersOverview, agg *usageAggregation, search string) error {
	dayTotals := make([]usageMetricsAcc, len(agg.period.Days))
	type userBuild struct {
		row  UsageCustomerOverviewRow
		days []usageMetricsAcc
	}
	byUser := make(map[int]*userBuild)
	for key, acc := range agg.overview {
		build, ok := byUser[key.user]
		if !ok {
			build = &userBuild{row: UsageCustomerOverviewRow{UserId: key.user}, days: make([]usageMetricsAcc, len(agg.period.Days))}
			byUser[key.user] = build
		}
		build.days[key.day].merge(acc)
	}
	userIds := make([]int, 0, len(byUser))
	for id := range byUser {
		userIds = append(userIds, id)
	}
	sort.Ints(userIds)
	identities, err := loadUsageUserIdentities(ctx, userIds)
	if err != nil {
		return err
	}
	totalAcc := usageMetricsAcc{}
	for _, id := range userIds {
		build := byUser[id]
		row := build.row
		rowAcc := usageMetricsAcc{}
		for i := range build.days {
			rowAcc.merge(&build.days[i])
		}
		row.Total = rowAcc.finalize()
		row.Days = make([]UsageAnalyticsMetrics, len(build.days))
		for i := range build.days {
			row.Days[i] = build.days[i].finalize()
		}
		if identity, ok := identities[id]; ok {
			row.Username = identity.Username
			row.DisplayName = identity.DisplayName
			row.Deleted = identity.Deleted
		} else {
			row.Username = fmt.Sprintf("User #%d", id)
			row.Deleted = true
		}
		if search != "" {
			needle := strings.ToLower(search)
			if !strings.Contains(strings.ToLower(row.Username), needle) &&
				!strings.Contains(strings.ToLower(row.DisplayName), needle) &&
				!strings.Contains(fmt.Sprintf("%d", row.UserId), needle) {
				continue
			}
		}
		if len(overview.Customers) >= maxUsageAnalyticsCustomers {
			return errors.New("usage analytics customer limit exceeded; narrow the search")
		}
		overview.Customers = append(overview.Customers, row)
		totalAcc.merge(&rowAcc)
		for i := range build.days {
			dayTotals[i].merge(&build.days[i])
		}
	}
	days := make([]UsageAnalyticsMetrics, len(agg.period.Days))
	for i := range dayTotals {
		days[i] = dayTotals[i].finalize()
	}
	overview.DayTotals = days
	overview.Total = totalAcc.finalize()
	sort.Slice(overview.Customers, func(i, j int) bool {
		if overview.Customers[i].Total.NetQuota != overview.Customers[j].Total.NetQuota {
			return overview.Customers[i].Total.NetQuota > overview.Customers[j].Total.NetQuota
		}
		return overview.Customers[i].UserId < overview.Customers[j].UserId
	})
	return nil
}

// loadUsageUserIdentities batch-loads customer identities for the overview.
func loadUsageUserIdentities(ctx context.Context, userIds []int) (map[int]BillingReconciliationUserIdentity, error) {
	result := make(map[int]BillingReconciliationUserIdentity, len(userIds))
	if len(userIds) == 0 {
		return result, nil
	}
	var rows []struct {
		Id          int
		Username    string
		DisplayName string
		DeletedAt   gorm.DeletedAt
	}
	if err := DB.WithContext(ctx).Unscoped().Model(&User{}).Select("id, username, display_name, deleted_at").Where("id IN ?", userIds).Find(&rows).Error; err != nil {
		common.SysError("usage analytics user identity lookup failed: " + err.Error())
		return nil, err
	}
	for _, row := range rows {
		result[row.Id] = BillingReconciliationUserIdentity{
			Id:          row.Id,
			Username:    row.Username,
			DisplayName: row.DisplayName,
			Deleted:     row.DeletedAt.Valid,
		}
	}
	return result, nil
}

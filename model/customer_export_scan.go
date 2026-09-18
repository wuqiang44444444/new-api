package model

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/QuantumNous/new-api/common"

	"github.com/shopspring/decimal"
)

// 客户导出的有界源数据读取与客户安全投影。游标是真实内部唯一键（关系库
// 自增 id），不是对客户重新编号的展示 ID；ClickHouse 日志没有已验证的唯一
// 排序键，第一版对该后端失败关闭（8.4：无法满足无遗漏读取的后端不启用导出）。

var ErrCustomerExportBackendUnsupported = errors.New("log backend does not support bounded export scanning")

// CustomerExportLogDatabasePressure exposes log-database pool wait statistics
// for the export backoff (7.3). Zero values mean "no evidence"; callers treat
// them as healthy. The handle may be nil in tests or single-database setups.
func CustomerExportLogDatabasePressure() (waitCount int64, waitSeconds float64) {
	if LOG_DB == nil {
		return 0, 0
	}
	sqlDB, err := LOG_DB.DB()
	if err != nil {
		return 0, 0
	}
	stats := sqlDB.Stats()
	return stats.WaitCount, stats.WaitDuration.Seconds()
}

// customerExportScanColumns 只取必要字段；group 是保留字，按日志库方言加
// 引号。initCol 未运行的测试环境回退到按当前日志库类型推导。
func customerExportScanColumns() string {
	groupCol := logGroupCol
	if groupCol == "" {
		if common.UsingLogDatabase(common.DatabaseTypePostgreSQL) {
			groupCol = `"group"`
		} else {
			groupCol = "`group`"
		}
	}
	return "id, user_id, token_id, channel_id, COALESCE(token_name, '') AS token_name, model_name, type, created_at, prompt_tokens, completion_tokens, quota, " + groupCol + " AS group_name, COALESCE(request_id, '') AS request_id, COALESCE(content, '') AS content, COALESCE(other, '') AS other"
}

// customerExportScanRow 是导出扫描的最小行载荷（7.3：只取必要字段）。
type customerExportScanRow struct {
	Id               int
	UserId           int
	TokenId          int
	ChannelId        int
	TokenName        string
	ModelName        string
	Type             int
	CreatedAt        int64
	PromptTokens     int
	CompletionTokens int
	Quota            int
	GroupName        string
	RequestId        string
	Content          string
	Other            string
}

// CustomerExportRow 是每个日志事件一行的客户安全投影（9.1 字段组）。它由
// 既有结算事实构建，不重新定价；未知与估算显式标注。
type CustomerExportRow struct {
	EstimateReasons        []string `json:"estimate_reasons,omitempty"`
	RequestId              string   `json:"request_id"`
	EventType              string   `json:"event_type"`
	RecordTime             int64    `json:"record_time"`
	TokenId                int      `json:"token_id"`
	TokenName              string   `json:"token_name"`
	CustomerModel          string   `json:"customer_model"`
	BillingMode            string   `json:"billing_mode"`
	CountsTowardBill       bool     `json:"counts_toward_bill"`
	InputTokensUnavailable bool     `json:"input_tokens_unavailable"`
	InputTokens            int64    `json:"input_tokens"`
	OutputTokens           int64    `json:"output_tokens"`
	CacheReadTokens        int64    `json:"cache_read_tokens"`
	CacheWriteTokens       int64    `json:"cache_write_tokens"`
	RequestCount           int64    `json:"request_count"`
	GroupName              string   `json:"group_name"`
	GroupRatio             *float64 `json:"group_ratio"`
	GroupRatioSource       string   `json:"group_ratio_source"`
	ContractApplicable     string   `json:"contract_applicable"` // yes / no / unrecorded / unknown
	ContractName           string   `json:"contract_name"`
	ContractVersion        int64    `json:"contract_version"`
	ContractRatio          *float64 `json:"contract_ratio"`
	FinalRatio             *float64 `json:"final_ratio"`
	Quota                  int64    `json:"quota"`             // 原始 quota（可核对精度）；退款行保存原值，符号由事件类型表达
	OriginalEstimate       string   `json:"original_estimate"` // 折前金额估算（定点字符串）；不可用时为空
	OriginalExact          bool     `json:"original_exact"`
	HasAuxiliaryCharge     bool     `json:"has_auxiliary_charge"`
	BillingLineItems       string   `json:"billing_line_items"` // Safe JSON: quantity, unit, USD price basis and component subtotal
	MatchedTier            string   `json:"matched_tier"`
	ExplanationStatus      string   `json:"explanation_status"`     // available / unavailable / not_applicable
	PriceConversionBasis   string   `json:"price_conversion_basis"` // frozen_quota_conversion / recorded_usd_price
	PricingRule            string   `json:"pricing_rule"`           // standard_tiered / per_call_price / expression / unknown
	QualityStatus          string   `json:"quality_status"`         // exact / estimate / unrecorded / not_billing_event
}

type CustomerExportBatchParams struct {
	ModelName         string
	UserId            int
	StartTimestamp    int64 // 含起点
	EndTimestamp      int64 // 不含终点（左闭右开）
	LogTypes          []int
	TokenId           *int
	ChannelId         *int
	TokenName         string
	Group             string
	RequestId         string
	UpstreamRequestId string
	Username          string
	UpperId           int64
	CursorId          int64
	Limit             int
	StatementScope    bool // 账单明细：沿用客户结算口径，排除渠道测试行
}

// NextCustomerExportLogBatch 按 (时间范围, id) keyset 游标读取一批候选行。
// 返回行数小于 Limit 表示范围读取完成。ctx 提供数据库侧可验证的批次超时与
// 取消；只取必要字段，不使用 OFFSET 深分页。
func NextCustomerExportLogBatch(ctx context.Context, params CustomerExportBatchParams) ([]customerExportScanRow, error) {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, ErrCustomerExportBackendUnsupported
	}
	if params.Limit <= 0 || params.Limit > 5000 {
		params.Limit = 1000
	}
	query := LOG_DB.WithContext(ctx).Model(&Log{}).
		Select(customerExportScanColumns()).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", params.UserId, params.StartTimestamp, params.EndTimestamp)
	if len(params.LogTypes) > 0 {
		query = query.Where("type IN ?", params.LogTypes)
	}
	if params.TokenId != nil {
		query = query.Where("token_id = ?", *params.TokenId)
	}
	if params.ChannelId != nil {
		query = query.Where("channel_id = ?", *params.ChannelId)
	}
	for column, value := range map[string]string{"token_name": params.TokenName, "group": params.Group,
		"request_id": params.RequestId, "upstream_request_id": params.UpstreamRequestId, "username": params.Username} {
		if value != "" {
			query = query.Where(map[string]any{column: value})
		}
	}
	if !params.StatementScope {
		var err error
		query, err = applyExplicitLogTextFilter(query, "model_name", params.ModelName)
		if err != nil {
			return nil, err
		}
	}
	if params.StatementScope {
		query = query.Scopes(customerSettlementLogs).Where("type IN ?", []int{LogTypeConsume, LogTypeRefund})
	}
	if params.UpperId > 0 {
		query = query.Where("id <= ?", params.UpperId)
	}
	if params.CursorId > 0 {
		cursor := params.CursorId
		query = query.Where("id > ?", cursor)
	}
	var rows []customerExportScanRow
	err := query.Order("id asc").Limit(params.Limit).Scan(&rows).Error
	return rows, err
}

// BuildCustomerExportRows 把一批候选行转换为已按客户模型/计费方式过滤的
// 客户安全投影行。候选行与匹配行分开计数（7.3：计量候选行）。
func BuildCustomerExportRows(ctx context.Context, rows []customerExportScanRow, modelName string, billingMode string, explain func(*Log, *CustomerExportRow)) ([]CustomerExportRow, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	facts := make([]billingReconciliationLog, len(rows))
	parseds := make([]parsedBillingReconciliationLog, len(rows))
	var refundFacts []billingReconciliationLog
	var refundParseds []parsedBillingReconciliationLog
	for i, scanned := range rows {
		facts[i] = customerExportScanFact(scanned)
		parseds[i] = parseBillingReconciliationLog(facts[i])
		if scanned.Type == LogTypeRefund {
			refundFacts = append(refundFacts, facts[i])
			refundParseds = append(refundParseds, parseds[i])
		}
	}
	// 7.3.13: 沿用页面退款分类语义，但只在当前有界批次内收集标识、批量取
	// Task 事实并释放；退款引用唯一性可在单次导出内按来源修订复用。
	evidence, err := buildBillingStatementRefundEvidence(ctx, refundFacts, refundParseds)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if rows[i].Type == LogTypeRefund {
			evidence.apply(facts[i], &parseds[i])
		}
	}
	result := make([]CustomerExportRow, 0, len(rows))
	for i, scanned := range rows {
		if modelName != "" && parseds[i].customerModel != modelName {
			continue
		}
		if billingMode != "" && parseds[i].billingMode != billingMode {
			continue
		}
		row := buildCustomerExportRow(scanned, facts[i], parseds[i])
		if explain != nil {
			explain(&Log{Type: scanned.Type, PromptTokens: scanned.PromptTokens, CompletionTokens: scanned.CompletionTokens, Other: scanned.Other}, &row)
		}
		result = append(result, row)
	}
	return result, nil
}

// customerExportScanFact builds the shared billing-reconciliation log fact for
// one scanned row so the export uses the same parsing semantics as the page.
func customerExportScanFact(scanned customerExportScanRow) billingReconciliationLog {
	return billingReconciliationLog{
		GroupName: scanned.GroupName, UserId: scanned.UserId, TokenId: scanned.TokenId, TokenName: scanned.TokenName,
		ChannelId: scanned.ChannelId, ModelName: scanned.ModelName, Type: scanned.Type,
		CreatedAt: scanned.CreatedAt, PromptTokens: scanned.PromptTokens,
		CompletionTokens: scanned.CompletionTokens, Quota: scanned.Quota, Other: scanned.Other,
	}
}

func buildCustomerExportRow(scanned customerExportScanRow, fact billingReconciliationLog, parsed parsedBillingReconciliationLog) CustomerExportRow {
	facts := loadCustomerExportFacts(scanned.Other)
	row := CustomerExportRow{
		RequestId:              scanned.RequestId,
		EventType:              customerExportEventType(scanned.Type),
		RecordTime:             scanned.CreatedAt,
		TokenId:                scanned.TokenId,
		TokenName:              scanned.TokenName,
		CustomerModel:          parsed.customerModel,
		BillingMode:            parsed.billingMode,
		CountsTowardBill:       (scanned.Type == LogTypeConsume || scanned.Type == LogTypeRefund) && !isNativeChannelTestLog(scanned.Type, scanned.TokenId, scanned.TokenName, scanned.Content),
		InputTokens:            parsed.inputTokens,
		InputTokensUnavailable: parsed.inputTokensUnavailable,
		OutputTokens:           parsed.outputTokens,
		CacheReadTokens:        parsed.cacheReadTokens,
		CacheWriteTokens:       parsed.cacheWrite.total,
		Quota:                  int64(scanned.Quota),
		HasAuxiliaryCharge:     parsed.hasAuxiliaryCharge,
	}
	if parsed.isRequest {
		row.RequestCount = billingStatementRequestCount(parsed)
	}

	row.GroupRatio = parsed.discountRatio
	row.GroupRatioSource = parsed.groupRatioSource
	row.ContractApplicable = billingContractApplicableState(parsed)
	switch row.ContractApplicable {
	case "yes":
		row.ContractApplicable = "yes"
		row.ContractRatio = parsed.contractDiscountRatio
		row.ContractName = parsed.contractName
		row.ContractVersion = int64(parsed.contractVersion)
	case "no", "unrecorded":
		// 无合同或历史无合同记录，保持记录状态，计算因子为 1。
	default:
		row.ContractApplicable = "unknown"
	}
	if row.GroupRatio != nil && row.ContractApplicable != "unknown" {
		final := *row.GroupRatio
		if row.ContractRatio != nil {
			final *= *row.ContractRatio
		}
		row.FinalRatio = &final
	}
	row.GroupName = parsed.groupName
	row.PricingRule = facts.pricingRule(parsed)
	if row.CountsTowardBill && row.Quota > 0 {
		row.EstimateReasons = billingStatementEstimateReasons(parsed)
	}
	row.QualityStatus, row.OriginalEstimate, row.OriginalExact = customerExportMoneyQuality(fact, parsed, row.GroupRatio, row.ContractApplicable, row.ContractRatio)
	if row.CountsTowardBill && row.Quota > 0 && row.OriginalEstimate == "" && len(row.EstimateReasons) == 0 {
		row.EstimateReasons = []string{BillingEstimateAmountOutOfRange}
	}
	return row
}

// customerExportFacts 每行只解析一次 other 与 statement_snapshot。
type customerExportFacts struct {
	other    map[string]json.RawMessage
	snapshot map[string]json.RawMessage
}

func loadCustomerExportFacts(other string) customerExportFacts {
	var facts customerExportFacts
	if common.UnmarshalJsonStr(other, &facts.other) != nil {
		facts.other = nil
		return facts
	}
	snapshotRaw := facts.other["statement_snapshot"]
	if len(snapshotRaw) == 0 {
		if adminRaw := facts.other["admin_info"]; len(adminRaw) > 0 {
			var adminInfo map[string]json.RawMessage
			if common.Unmarshal(adminRaw, &adminInfo) == nil {
				snapshotRaw = adminInfo["statement_snapshot"]
			}
		}
	}
	if len(snapshotRaw) > 0 {
		_ = common.Unmarshal(snapshotRaw, &facts.snapshot)
	}
	return facts
}

// raw 优先读取结算快照，再回退公共字段（与账单聚合同口径）。
func (f customerExportFacts) raw(key string) json.RawMessage {
	return billingReconciliationSnapshotRaw(f.snapshot, f.other, key)
}

func (f customerExportFacts) float(key string) (float64, bool) {
	return billingReconciliationFloat(f.raw(key))
}

func (f customerExportFacts) string(key string) string {
	return billingBreakdownString(f.raw(key))
}

func (f customerExportFacts) number(key string) float64 {
	value, _ := f.float(key)
	return value
}

func (f customerExportFacts) pricingRule(parsed parsedBillingReconciliationLog) string {
	if parsed.billingMode == BillingReconciliationModePerCall {
		if _, ok := f.float("model_price"); ok {
			return "per_call_price"
		}
	}
	if f.string("expr_b64") != "" {
		return "expression"
	}
	if ratio, ok := f.float("model_ratio"); ok && ratio > 0 {
		return "standard_tiered"
	}
	return "unknown"
}

// customerExportMoneyQuality 区分精确、估算与未知。只有当次分组倍率与合同
// 事实可解释、且无附加收费时才给出折前金额；历史完全无合同记录时 C=1，
// 已记录但残缺/冲突的合同保持未知。该金额永远是估算（反推自
// 已入账的舍入金额）。
func customerExportMoneyQuality(fact billingReconciliationLog, parsed parsedBillingReconciliationLog, groupRatio *float64, contractApplicable string, contractRatio *float64) (string, string, bool) {
	if fact.Type != LogTypeConsume && fact.Type != LogTypeRefund {
		return "not_billing_event", "", false
	}
	quota := max(int64(fact.Quota), int64(0))
	if quota == 0 {
		// 零差额完成、免费调用：金额本身精确。
		return "exact", "0", true
	}
	if len(billingStatementEstimateReasons(parsed)) > 0 {
		return "unrecorded", "", false
	}
	original := decimal.NewFromInt(quota).Div(decimal.NewFromFloat(*groupRatio))
	if contractApplicable == "yes" {
		original = original.Div(decimal.NewFromFloat(*contractRatio))
	}
	if fact.Type == LogTypeRefund {
		// 退款冲回保留符号：退款月的原价与优惠可以为负，不截为零。
		original = original.Neg()
	}
	if billingStatementOriginalQuota(original) == nil {
		return "unrecorded", "", false
	}
	return "estimate", original.StringFixed(4), false
}

func customerExportEventType(logType int) string {
	switch logType {
	case LogTypeConsume:
		return "consume"
	case LogTypeRefund:
		return "refund"
	case LogTypeError:
		return "error"
	case LogTypeTopup:
		return "topup"
	case LogTypeSystem:
		return "system"
	case LogTypeManage:
		return "manage"
	case LogTypeLogin:
		return "login"
	default:
		return "unknown_" + strconv.Itoa(logType)
	}
}

// CustomerBillingLogRow is the same safe projection for an already authorized
// log response. It does not query current prices or inherit unproven facts.
func CustomerBillingLogRow(log *Log) CustomerExportRow {
	// A statement response already carries the authorized, recovered projection.
	// This is response assembly only; the source-log parser never consumes it.
	var response struct {
		Facts *CustomerExportRow `json:"billing_facts"`
	}
	if common.UnmarshalJsonStr(log.Other, &response) == nil && response.Facts != nil {
		return *response.Facts
	}
	scanned := customerBillingLogScanRow(log)
	fact := customerExportScanFact(scanned)
	return buildCustomerExportRow(scanned, fact, parseBillingReconciliationLog(fact))
}

func customerBillingLogScanRow(log *Log) customerExportScanRow {
	return customerExportScanRow{UserId: log.UserId, TokenId: log.TokenId, TokenName: log.TokenName, ChannelId: log.ChannelId, ModelName: log.ModelName, Type: log.Type, Quota: log.Quota, PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens, Other: log.Other, Content: log.Content, GroupName: log.Group, RequestId: log.RequestId, CreatedAt: log.CreatedAt}
}

func CustomerExportLogUpperBound(ctx context.Context) (int64, error) {
	var row struct{ ID int64 }
	err := LOG_DB.WithContext(ctx).Model(&Log{}).Select("id").Order("id desc").Limit(1).Scan(&row).Error
	return row.ID, err
}

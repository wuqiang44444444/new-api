package model

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// BillingStatementReadPolicy controls resource use, never accounting scope.
// Export supplies a pressure gate; interactive statements use the same parser
// and accumulator without the export throttle.
//
// ObserveFact 是版本固化的最小接线（docs/80-dev/2026-09-17 方案 8.1）：让账单版本
// 生成器在聚合的同时收集每条已解析事实作为版本明细，无需复制聚合/解析逻辑。
// 主体转换在 newBillingStatementVersionLine 中，保持 reader 只负责读取。
type BillingStatementReadPolicy struct {
	// UpperLogID binds upstream export batches/channels to one log boundary.
	UpperLogID   *int64
	BeforeBatch  func(context.Context) error
	AfterBatch   func(int) error // upstream export candidate counts, before display filters
	BatchTimeout time.Duration
	MaxGroups    int
	ObserveFact  func(BillingStatementVersionLine, *Log) error
}

func scanBillingStatementFacts(ctx context.Context, query *gorm.DB, policy BillingStatementReadPolicy, consume func(billingReconciliationLog, parsedBillingReconciliationLog) error) (resultErr error) {
	interactive := policy.BeforeBatch == nil && policy.BatchTimeout == 0 && policy.ObserveFact == nil
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) || interactive {
		// Interactive reads retain their single cursor and load refund evidence
		// once. Export/version policies retain bounded batches and source IDs.
		// Exports reject ClickHouse before reaching this reader.
		evidence, err := loadBillingStatementRefundEvidence(ctx, query)
		if err != nil {
			return err
		}
		// Names and the bounded discount-combination projection follow log ID
		// order, as exports do. A new index must not choose their traversal order.
		// ClickHouse has a separate log identity contract; retain its read path.
		if !common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
			query = query.Session(&gorm.Session{}).Order("id asc")
		}
		rows, err := query.Rows()
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var log billingReconciliationLog
			if err := rows.Scan(&log.UserId, &log.TokenId, &log.TokenName, &log.ChannelId, &log.ModelName, &log.Type, &log.CreatedAt, &log.PromptTokens, &log.CompletionTokens, &log.Quota, &log.Content, &log.Other, &log.GroupName); err != nil {
				return err
			}
			parsed := parseBillingReconciliationLog(log)
			evidence.apply(log, &parsed)
			if err := emitObservedFact(policy, log, parsed, 0); err != nil {
				return err
			}
			if err := consume(log, parsed); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	// A finite upper boundary stops new arrivals extending a running export.
	ctx = WithBillingStatementRefundReferenceCache(ctx)
	defer func() {
		if resultErr == nil {
			resultErr = ValidateBillingStatementRefundReferenceCache(ctx)
		}
	}()
	// This is a scan bound, not a data revision or a cache validity proof.
	boundCtx, stopBound := context.WithTimeout(ctx, 5*time.Second)
	upper, err := CustomerExportLogUpperBound(boundCtx)
	stopBound()
	if err != nil {
		return err
	}
	var cursor int64
	for cursor < upper {
		if err := ctx.Err(); err != nil {
			return err
		}
		if policy.BeforeBatch != nil {
			if err := policy.BeforeBatch(ctx); err != nil {
				return err
			}
		}
		timeout := policy.BatchTimeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		batchCtx, cancel := context.WithTimeout(ctx, timeout)
		var batch []struct {
			ID   int64
			Fact billingReconciliationLog `gorm:"embedded"`
		}
		columns := "id, COALESCE(request_id, '') AS request_id, user_id, token_id, COALESCE(token_name, '') AS token_name, channel_id, COALESCE(model_name, '') AS model_name, type, created_at, prompt_tokens, completion_tokens, quota, COALESCE(content, '') AS content, COALESCE(other, '') AS other, " + billingStatementGroupSelect()
		err := query.Session(&gorm.Session{}).WithContext(batchCtx).Select(columns).
			Where("id > ? AND id <= ?", cursor, upper).Order("id asc").Limit(500).Scan(&batch).Error
		var evidence billingStatementRefundEvidence
		parsed := make([]parsedBillingReconciliationLog, len(batch))
		if err == nil {
			refunds := make([]billingReconciliationLog, 0)
			var refundParsed []parsedBillingReconciliationLog
			for i, row := range batch {
				parsed[i] = parseBillingReconciliationLog(row.Fact)
				if row.Fact.Type == LogTypeRefund {
					refunds = append(refunds, row.Fact)
					refundParsed = append(refundParsed, parsed[i])
				}
			}
			evidence, err = buildBillingStatementRefundEvidence(batchCtx, refunds, refundParsed)
		}
		cancel()
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		for i, row := range batch {
			evidence.apply(row.Fact, &parsed[i])
			if err := emitObservedFact(policy, row.Fact, parsed[i], row.ID); err != nil {
				return err
			}
			if err := consume(row.Fact, parsed[i]); err != nil {
				return err
			}
		}
		cursor = batch[len(batch)-1].ID
	}
	return nil
}

// emitObservedFact 在版本固化开启观察时，把一条已解析账单事实转换为版本明细行并回调。
// 转换保持最小白名单：客户字段 + 精确 quota + 折扣三态，不保存原始 other / Task 私有数据。
func emitObservedFact(policy BillingStatementReadPolicy, fact billingReconciliationLog, parsed parsedBillingReconciliationLog, sourceID int64) error {
	if policy.ObserveFact == nil {
		return nil
	}
	row := buildCustomerExportRow(customerExportScanRow{RequestId: fact.RequestId, UserId: fact.UserId, TokenId: fact.TokenId, TokenName: fact.TokenName, ChannelId: fact.ChannelId, ModelName: fact.ModelName, Type: fact.Type, CreatedAt: fact.CreatedAt, PromptTokens: fact.PromptTokens, CompletionTokens: fact.CompletionTokens, Quota: fact.Quota, Other: fact.Other, Content: fact.Content}, fact, parsed)
	raw, err := common.Marshal(row)
	if err != nil {
		return err
	}
	line := BillingStatementVersionLine{SourceLogId: sourceID, RequestId: fact.RequestId, TokenId: fact.TokenId, TokenName: fact.TokenName, ChannelId: fact.ChannelId, CustomerModel: parsed.customerModel, Group: parsed.groupName, LogType: fact.Type, CreatedAt: fact.CreatedAt, BillingMode: parsed.billingMode, InputTokens: parsed.inputTokens, OutputTokens: parsed.outputTokens, CacheReadTokens: parsed.cacheReadTokens, CacheWriteTokens: parsed.cacheWrite.total, Quota: int64(fact.Quota), Facts: string(raw)}
	return policy.ObserveFact(line, &Log{Type: fact.Type, PromptTokens: fact.PromptTokens, CompletionTokens: fact.CompletionTokens, Other: fact.Other})
}

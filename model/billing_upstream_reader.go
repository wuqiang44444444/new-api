package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Bounded keyset reads share customer refund evidence recovery. New arrivals
// cannot extend the scan, and no log cursor is held during primary DB reads.
func scanUpstreamBillingFacts(ctx context.Context, filter UpstreamBillingDetailFilter, policy BillingStatementReadPolicy, consume func(Log, billingReconciliationLog, parsedBillingReconciliationLog) error) (resultErr error) {
	ctx = WithBillingStatementRefundReferenceCache(ctx)
	defer func() {
		if resultErr == nil {
			resultErr = ValidateBillingStatementRefundReferenceCache(ctx)
		}
	}()
	query := LOG_DB.Model(&Log{}).Where("type IN ? AND created_at >= ? AND created_at <= ?", []int{LogTypeConsume, LogTypeRefund}, filter.Start, filter.End)
	if len(filter.ChannelIds) > 0 {
		query = query.Where("channel_id IN ?", filter.ChannelIds)
	}
	if filter.RequestId != "" {
		query = query.Where("request_id = ?", filter.RequestId)
	}
	if filter.UpstreamRequestId != "" {
		query = query.Where("upstream_request_id = ?", filter.UpstreamRequestId)
	}
	// ClickHouse log IDs do not have the SQL export cursor guarantee. Keep
	// its interactive single-cursor behavior; submissions reject this backend.
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		evidence, err := loadBillingStatementRefundEvidence(ctx, query)
		if err != nil {
			return err
		}
		rows, err := query.WithContext(ctx).Order("created_at ASC, id ASC").Rows()
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var log Log
			if err := LOG_DB.ScanRows(rows, &log); err != nil {
				return err
			}
			fact := upstreamBillingLogFact(log, log.Group)
			parsed := parseBillingReconciliationLog(fact)
			evidence.apply(fact, &parsed)
			if err := consume(log, fact, parsed); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	boundCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	upper, err := CustomerExportLogUpperBound(boundCtx)
	cancel()
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
		batchCtx, stop := context.WithTimeout(ctx, timeout)
		var logs []struct {
			Log       `gorm:"embedded"`
			GroupName string
		}
		err := query.Session(&gorm.Session{}).WithContext(batchCtx).
			Select("id, user_id, token_id, token_name, channel_id, model_name, type, created_at, prompt_tokens, completion_tokens, quota, content, other, request_id, upstream_request_id, "+billingStatementGroupSelect()).
			Where("id > ? AND id <= ?", cursor, upper).Order("id ASC").Limit(500).Find(&logs).Error
		facts := make([]billingReconciliationLog, len(logs))
		parsed := make([]parsedBillingReconciliationLog, len(logs))
		var refunds []billingReconciliationLog
		var refundParsed []parsedBillingReconciliationLog
		for i, log := range logs {
			facts[i] = upstreamBillingLogFact(log.Log, log.GroupName)
			parsed[i] = parseBillingReconciliationLog(facts[i])
			if log.Type == LogTypeRefund {
				refunds = append(refunds, facts[i])
				refundParsed = append(refundParsed, parsed[i])
			}
		}
		var evidence billingStatementRefundEvidence
		if err == nil {
			evidence, err = buildBillingStatementRefundEvidence(batchCtx, refunds, refundParsed)
		}
		stop()
		if err != nil {
			return err
		}
		if len(logs) == 0 {
			break
		}
		for i, log := range logs {
			evidence.apply(facts[i], &parsed[i])
			if err := consume(log.Log, facts[i], parsed[i]); err != nil {
				return err
			}
		}
		if policy.AfterBatch != nil {
			if err := policy.AfterBatch(len(logs)); err != nil {
				return err
			}
		}
		cursor = int64(logs[len(logs)-1].Id)
	}
	return nil
}

// Customer settlement refunds reduce the local original-price base, without
// asserting anything about whether the provider refunded its usage or money.
func upstreamOriginalQuota(log billingReconciliationLog, parsed parsedBillingReconciliationLog) (decimal.Decimal, []string) {
	quota := max(int64(log.Quota), int64(0))
	if quota == 0 {
		return decimal.Zero, nil
	}
	if reasons := billingStatementEstimateReasons(parsed); len(reasons) > 0 {
		return decimal.Zero, reasons
	}
	contractRatio := 1.0
	if parsed.contractDiscountRatio != nil && *parsed.contractDiscountRatio > 0 {
		contractRatio = *parsed.contractDiscountRatio
	}
	original := decimal.NewFromInt(quota).Div(decimal.NewFromFloat(*parsed.discountRatio)).Div(decimal.NewFromFloat(contractRatio))
	if log.Type == LogTypeRefund {
		original = original.Neg()
	}
	return original, nil
}

func upstreamBillingLogFact(log Log, groupName string) billingReconciliationLog {
	return billingReconciliationLog{RequestId: log.RequestId, UserId: log.UserId, TokenId: log.TokenId, TokenName: log.TokenName, ChannelId: log.ChannelId, ModelName: log.ModelName, Type: log.Type, CreatedAt: log.CreatedAt, PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens, Quota: log.Quota, Content: log.Content, Other: log.Other, GroupName: groupName}
}

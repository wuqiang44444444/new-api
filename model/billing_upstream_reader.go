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
	db := LOG_DB
	if policy.ReadOnlySource {
		db = usageReportLogDB(ctx)
	} else {
		ctx = WithBillingStatementRefundReferenceCache(ctx)
		defer func() {
			if resultErr == nil {
				resultErr = ValidateBillingStatementRefundReferenceCache(ctx)
			}
		}()
	}
	query := db.Model(&Log{}).Where("type IN ? AND created_at >= ? AND created_at <= ?", []int{LogTypeConsume, LogTypeRefund}, filter.Start, filter.End)
	if len(filter.ChannelIds) > 0 {
		query = query.Where("channel_id IN ?", filter.ChannelIds)
	}
	query = upstreamRequestIDCandidates(query, filter.RequestId)
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
			projectFrozenChargeReplay(fact, &parsed)
			applyCustomerOriginalReplay(fact, &parsed)
			if err := consume(log, fact, parsed); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	var upper int64
	if policy.UpperLogID != nil {
		upper = *policy.UpperLogID
	} else {
		boundCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var err error
		upper, err = CustomerExportLogUpperBound(boundCtx)
		cancel()
		if err != nil {
			return err
		}
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
			Log           `gorm:"embedded"`
			GroupName     string
			TupleRevision int64
		}
		columns := "id, user_id, token_id, token_name, channel_id, model_name, type, created_at, prompt_tokens, completion_tokens, quota, content, other, request_id, upstream_request_id, " + billingStatementGroupSelect()
		if policy.ReadOnlySource {
			columns += ", username, use_time, is_stream"
		}
		if policy.ObserveLogRevision != nil && db.Dialector.Name() == "postgres" {
			columns += ", " + dailyReportTupleRevision + " AS tuple_revision"
		}
		err := query.Session(&gorm.Session{}).WithContext(batchCtx).Select(columns).
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
			if policy.ObserveLogRevision != nil {
				policy.ObserveLogRevision(log.TupleRevision)
			}
			evidence.apply(facts[i], &parsed[i])
			projectFrozenChargeReplay(facts[i], &parsed[i])
			if err := resolveCustomerOriginalReplay(ctx, facts[i], &parsed[i]); err != nil {
				return err
			}
			log.Log.Group = log.GroupName
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
	if original, ok := customerStatementOriginalQuota(log, parsed); ok {
		return original, nil
	}
	return decimal.Zero, statementOriginalUnavailableReasons(parsed)
}

func upstreamBillingLogFact(log Log, groupName string) billingReconciliationLog {
	return billingReconciliationLog{RequestId: log.RequestId, UserId: log.UserId, TokenId: log.TokenId, TokenName: log.TokenName, ChannelId: log.ChannelId, ModelName: log.ModelName, Type: log.Type, CreatedAt: log.CreatedAt, PromptTokens: log.PromptTokens, CompletionTokens: log.CompletionTokens, Quota: log.Quota, Content: log.Content, Other: log.Other, GroupName: groupName}
}

// UpstreamReferenceQuota quantizes one evidence row to the same quota unit as
// its restored original. Summaries sum these values without rounding again.
func UpstreamReferenceQuota(original, coefficient decimal.Decimal) decimal.Decimal {
	return original.Mul(coefficient).Round(0)
}

package model

import (
	"context"
	"strings"
)

func billingExportStream(logType int, recorded *bool, parsed parsedBillingReconciliationLog) *bool {
	if parsed.refundOriginalIsStream != nil {
		return parsed.refundOriginalIsStream
	}
	if recorded != nil && *recorded {
		return recorded
	}
	// Task funding and refund writers leave is_stream at its storage default.
	if logType != LogTypeConsume || parsed.isTask || parsed.taskID != "" || parsed.taskBillingEvent != "" {
		return nil
	}
	return recorded
}

// Delivery events can name the gateway request in frozen metadata. Only an
// exact earlier request with the same owner, key, channel and model supplies a
// streaming flag. Read at most one bounded set of candidates per source batch.
func resolveBillingExportTaskStreams(ctx context.Context, facts []billingReconciliationLog, parsed []parsedBillingReconciliationLog) error {
	var requestIDs []string
	pending := make(map[string][]int)
	for i, fact := range facts {
		if !strings.HasPrefix(fact.RequestId, taskBillingDeliveryRequestPrefix) || fact.sourceID <= 0 {
			continue
		}
		requestID := upstreamTaskCreateRequestID(upstreamRequestIDFromLog(Log{RequestId: fact.RequestId, Other: fact.Other}))
		if requestID == "" {
			continue
		}
		if len(pending[requestID]) == 0 {
			requestIDs = append(requestIDs, requestID)
		}
		pending[requestID] = append(pending[requestID], i)
	}
	if len(requestIDs) == 0 {
		return nil
	}
	var originals []struct {
		Id, UserId, TokenId, ChannelId int
		CreatedAt                      int64
		ModelName, RequestId, Other    string
		IsStream                       *bool
	}
	limit := len(requestIDs) * 4
	err := usageReportLogDB(ctx).WithContext(ctx).Model(&Log{}).
		Select("id, user_id, token_id, channel_id, created_at, model_name, request_id, other, is_stream").
		Where("type = ? AND request_id IN ?", LogTypeConsume, requestIDs).Limit(limit + 1).Find(&originals).Error
	if err != nil || len(originals) > limit {
		return err
	}
	matches := make(map[int][]*bool)
	for _, original := range originals {
		for _, i := range pending[original.RequestId] {
			fact := facts[i]
			if fact.UserId != original.UserId || fact.TokenId != original.TokenId || fact.ChannelId != original.ChannelId ||
				int64(original.Id) >= fact.sourceID || original.CreatedAt > fact.CreatedAt {
				continue
			}
			source := parseBillingReconciliationLog(billingReconciliationLog{
				ModelName: original.ModelName, Other: original.Other, Type: LogTypeConsume, TokenId: original.TokenId,
			})
			if source.customerModel == parsed[i].customerModel && !source.isTask && source.taskID == "" {
				matches[i] = append(matches[i], original.IsStream)
			}
		}
	}
	for i, flags := range matches {
		if len(flags) == 1 {
			parsed[i].refundOriginalIsStream = flags[0]
		}
	}
	return nil
}

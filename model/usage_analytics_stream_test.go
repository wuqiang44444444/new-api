package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageAnalyticsStreamOutcomePreservesConsumption(t *testing.T) {
	for _, tc := range []struct {
		name, stream                       string
		success, failure, cancelled, other int64
	}{
		{name: "legacy consume without stream evidence", success: 1},
		{name: "normal completion", stream: `{"status":"ok","end_reason":"done"}`, success: 1},
		{name: "client disconnect", stream: `{"status":"error","end_reason":"client_gone"}`, cancelled: 1},
		{name: "timeout", stream: `{"status":"error","end_reason":"timeout"}`, failure: 1},
		{name: "scanner failure", stream: `{"status":"error","end_reason":"scanner_error"}`, failure: 1},
		{name: "handler panic", stream: `{"status":"error","end_reason":"panic"}`, failure: 1},
		{name: "ping failure", stream: `{"status":"error","end_reason":"ping_fail"}`, failure: 1},
		{name: "protocol failure", stream: `{"status":"error","end_reason":"eof","errors":["response_failed"]}`, failure: 1},
		{name: "protocol incomplete", stream: `{"status":"error","end_reason":"handler_stop","errors":["response_incomplete"]}`, failure: 1},
		{name: "protocol cancellation", stream: `{"status":"error","end_reason":"eof","errors":["response_cancelled"]}`, cancelled: 1},
		{name: "soft error with completed stream", stream: `{"status":"error","end_reason":"done","errors":["recoverable parse error"]}`, success: 1},
		{name: "free text is not a protocol marker", stream: `{"status":"error","end_reason":"handler_stop","errors":["text mentioning response_failed"]}`, success: 1},
		{name: "unknown termination", stream: `{"status":"error","end_reason":""}`, other: 1},
		{name: "malformed evidence", stream: `"invalid"`, other: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupUsageAnalyticsTestDB(t)
			period, err := ResolveUsageAnalyticsPeriod("day", "2026-09-16", usageTestDayStart+3600)
			require.NoError(t, err)
			other := `{"contract_applicable":false,"usage_semantic":"openai","group_ratio":1,"model_ratio":1,"cache_tokens":0`
			if tc.stream != "" {
				other += `,"stream_status":` + tc.stream
			}
			other += `}`
			require.NoError(t, db.Create(&[]Log{
				// A previous failed attempt, even across midnight, does not become
				// an additional customer call when a final consume exists.
				{UserId: 7, RequestId: "stream", CreatedAt: period.StartTimestamp - 1, Type: LogTypeError, ChannelId: 21, ModelName: "chat"},
				{UserId: 7, RequestId: "stream", CreatedAt: period.StartTimestamp + 10, Type: LogTypeError, ChannelId: 21, ModelName: "chat"},
				{UserId: 7, RequestId: "stream", CreatedAt: period.StartTimestamp + 20, Type: LogTypeConsume, TokenId: 11, TokenName: "key", ChannelId: 21, ModelName: "chat", Quota: 1200, PromptTokens: 100, CompletionTokens: 40, Other: other},
			}).Error)
			customer, err := GetUsageCustomerView(context.Background(), period, 7)
			require.NoError(t, err)
			upstream, err := GetUsageUpstreamView(context.Background(), period)
			require.NoError(t, err)
			for _, metrics := range []UsageAnalyticsMetrics{customer.Total, upstream.Total} {
				assert.EqualValues(t, 1, metrics.TotalCalls)
				assert.Equal(t, tc.success, metrics.SuccessCalls)
				assert.Equal(t, tc.failure, metrics.FailureCalls)
				assert.Equal(t, tc.cancelled, metrics.CancelledCalls)
				assert.Equal(t, tc.other, metrics.OtherResultCalls)
				assert.EqualValues(t, 1200, metrics.NetQuota)
				assert.EqualValues(t, 100, metrics.InputTokens)
				assert.EqualValues(t, 40, metrics.OutputTokens)
				assert.Zero(t, metrics.RowsMissingTokens, "recorded usage remains known even when delivery failed")
			}
			previous, err := ResolveUsageAnalyticsPeriod("day", "2026-09-15", usageTestDayStart+3600)
			require.NoError(t, err)
			before, err := GetUsageCustomerView(context.Background(), previous, 7)
			require.NoError(t, err)
			assert.Zero(t, before.Total.TotalCalls)
		})
	}
}

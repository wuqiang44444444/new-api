package model

import (
	"encoding/base64"
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// Cache evidence is independent of settlement. Never infer availability from
// model names or current channel settings. Recorded billing zeroes are usable
// even when the provider response omitted the optional meter.
type upstreamCacheEvidence uint8

const (
	upstreamCacheRecorded upstreamCacheEvidence = iota
	upstreamCacheLegacyUnknown
	upstreamCacheNotReported
)

func upstreamCacheMeterEvidence(other map[string]json.RawMessage, marker string, known bool) upstreamCacheEvidence {
	if known {
		return upstreamCacheRecorded
	}
	if raw, exists := other[marker]; exists {
		reported, valid := billingReconciliationBool(raw)
		if valid && !reported {
			return upstreamCacheNotReported
		}
		return upstreamCacheLegacyUnknown
	}
	return upstreamCacheLegacyUnknown
}

func accumulateUpstreamCacheQuality(target **BillingReconciliationDataQuality, parsed parsedBillingReconciliationLog) {
	if parsed.billingMode != BillingReconciliationModeToken {
		return
	}
	q := ensureBillingReconciliationQuality(target)
	if parsed.cacheReadEvidence != upstreamCacheRecorded {
		q.CacheReadUnavailableRequests++
		if parsed.cacheReadEvidence == upstreamCacheNotReported {
			q.CacheReadUnreportedRequests++
		}
	}
	if parsed.cacheWriteEvidence != upstreamCacheRecorded {
		q.CacheWriteUnavailableRequests++
		if parsed.cacheWriteEvidence == upstreamCacheNotReported {
			q.CacheWriteUnreportedRequests++
		}
	}
}

// Seconds are read only from a recorded meter with a frozen second unit.
// Creation holds are estimates, and ordinary customer refunds are not usage.
// Missing or ambiguous meter evidence remains unknown; no price back-solving.
func upstreamBillingSeconds(log billingReconciliationLog, parsed parsedBillingReconciliationLog) (*decimal.Decimal, bool, bool) {
	if parsed.billingMode != BillingReconciliationModePerSecond || (log.Type == LogTypeRefund && !isProviderTaskUsageAdjustment(log)) {
		return nil, false, false
	}
	var other map[string]json.RawMessage
	if common.UnmarshalJsonStr(log.Other, &other) != nil {
		return nil, true, false
	}
	if billingBreakdownString(other["task_billing_event"]) == "create" {
		return nil, false, false
	}
	snapshotRaw := other["statement_snapshot"]
	if len(snapshotRaw) == 0 {
		var admin map[string]json.RawMessage
		_ = common.Unmarshal(other["admin_info"], &admin)
		snapshotRaw = admin["statement_snapshot"]
	}
	var snapshot map[string]json.RawMessage
	_ = common.Unmarshal(snapshotRaw, &snapshot)
	var units map[string]string
	unitRaw := billingReconciliationSnapshotRaw(snapshot, other, "usage_units")
	if len(unitRaw) == 0 {
		expression, err := base64.StdEncoding.DecodeString(billingBreakdownString(billingReconciliationSnapshotRaw(snapshot, other, "expr_b64")))
		if err == nil && BillingStatementExpressionMode(string(expression), nil) == BillingReconciliationModePerSecond {
			// The reserved historical duration probe already defines seconds. Its
			// missing value must not be misreported as a missing unit.
			return nil, true, true
		}
	}
	if common.Unmarshal(unitRaw, &units) != nil {
		return nil, true, false
	}
	var facts map[string]json.RawMessage
	factsKnown := common.Unmarshal(other["usage_facts"], &facts) == nil
	var meter string
	for key, unit := range units {
		if unit == "second" {
			if meter != "" {
				return nil, true, false
			}
			meter = key
		}
	}
	if meter == "" {
		return nil, true, false
	}
	// The frozen unit names the meter even when the measured value itself was
	// never recorded or is unreadable: callers keep the two gaps separate.
	if !factsKnown {
		return nil, true, true
	}
	value, ok := billingReconciliationFloat(facts[meter])
	if !ok || value < 0 {
		return nil, true, true
	}
	seconds := decimal.NewFromFloat(value)
	return &seconds, false, true
}

// upstreamTaskCreatePreHold reports a durable task creation hold row: an
// estimate by construction, so it never carries final metering evidence.
// Async image completion logs reuse the create event but carry image_count as
// execution evidence, so they are metering rows, not pre-holds.
func upstreamTaskCreatePreHold(parsed parsedBillingReconciliationLog) bool {
	return parsed.isTask && parsed.taskBillingEvent == "create" && !parsed.hasImageCount
}

// upstreamTaskCacheTracker deduplicates cache-missing counts per task: rows of
// one final usage merge their read/write meter evidence, and only a task whose
// merged evidence is not recorded counts once. Rows without a task identity
// keep the per-row counting.
type upstreamTaskCacheTracker struct {
	buckets map[billingTaskCacheKey]*upstreamTaskCacheBucket
}

type billingTaskCacheKey struct {
	userID int
	taskID string
	item   providerBillingSummaryKey
}

type upstreamTaskCacheBucket struct {
	item  providerBillingSummaryKey
	read  upstreamCacheEvidence
	write upstreamCacheEvidence
}

func newUpstreamTaskCacheTracker() *upstreamTaskCacheTracker {
	return &upstreamTaskCacheTracker{buckets: make(map[billingTaskCacheKey]*upstreamTaskCacheBucket)}
}

func mergeUpstreamCacheEvidence(a, b upstreamCacheEvidence) upstreamCacheEvidence {
	if b < a {
		return b
	}
	return a
}

func (t *upstreamTaskCacheTracker) observe(item providerBillingSummaryKey, userID int, parsed parsedBillingReconciliationLog) {
	if parsed.billingMode != BillingReconciliationModeToken || upstreamTaskCreatePreHold(parsed) {
		return
	}
	key := billingTaskCacheKey{userID: userID, taskID: parsed.taskID, item: item}
	bucket := t.buckets[key]
	if bucket == nil {
		// Min-merge identity: any recorded row wins, notReported only survives
		// when every row explicitly lacked the meter.
		bucket = &upstreamTaskCacheBucket{item: item, read: upstreamCacheNotReported, write: upstreamCacheNotReported}
		t.buckets[key] = bucket
	}
	bucket.read = mergeUpstreamCacheEvidence(bucket.read, parsed.cacheReadEvidence)
	bucket.write = mergeUpstreamCacheEvidence(bucket.write, parsed.cacheWriteEvidence)
}

// flushInto folds one deduplicated missing count per task into its item.
func (t *upstreamTaskCacheTracker) flushInto(items map[providerBillingSummaryKey]*ProviderBillingPlatformSummary) {
	for _, bucket := range t.buckets {
		item := items[bucket.item]
		if item == nil {
			continue
		}
		quality := ensureBillingReconciliationQuality(&item.DataQuality)
		if bucket.read != upstreamCacheRecorded {
			quality.CacheReadUnavailableRequests++
			if bucket.read == upstreamCacheNotReported {
				quality.CacheReadUnreportedRequests++
			}
		}
		if bucket.write != upstreamCacheRecorded {
			quality.CacheWriteUnavailableRequests++
			if bucket.write == upstreamCacheNotReported {
				quality.CacheWriteUnreportedRequests++
			}
		}
	}
}

package model

import (
	"context"

	"github.com/QuantumNous/new-api/common"
)

// Count the requested references over the key's complete refund history.
// Pagination bounds memory, not the amount of business history we can explain.
// Any failed/cancelled read fails the caller instead of certifying a partial scan.
func countBillingStatementRefundReferences(ctx context.Context, userID, tokenID int, ids []int64) (map[int64]int, error) {
	if cache, ok := ctx.Value(billingRefundReferenceCacheKey{}).(*billingRefundReferenceCache); ok && DB != nil && ShouldTrackBillingStatementRevision() {
		return cache.count(ctx, userID, tokenID, ids)
	}
	return scanBillingStatementRefundReferences(ctx, userID, tokenID, ids, 0)
}

// A positive capacity opportunistically retains other references too. Every
// requested ID is counted over the complete history even when the cache is full.
// Zero keeps the requested-reference-only reader; memory is not a business limit.
func scanBillingStatementRefundReferences(ctx context.Context, userID, tokenID int, ids []int64, capacity int) (map[int64]int, error) {
	counts := make(map[int64]int, len(ids))
	for _, id := range ids {
		counts[id] = 0
	}
	query := LOG_DB.WithContext(ctx).Model(&Log{}).
		Where("user_id = ? AND type = ? AND token_id = ?", userID, LogTypeRefund, tokenID)
	var upperIDs []int64
	if err := query.Order("id desc").Limit(1).Pluck("id", &upperIDs).Error; err != nil {
		return nil, err
	}
	if len(upperIDs) == 0 {
		return counts, nil
	}
	var cursor int64
	for cursor < upperIDs[0] {
		var batch []struct {
			ID    int64
			Other string
		}
		// Start a fresh query so the upper-bound ORDER/LIMIT cannot leak into
		// forward pagination (GORM chain methods share their statement).
		if err := LOG_DB.WithContext(ctx).Model(&Log{}).Select("id, other").
			Where("user_id = ? AND type = ? AND token_id = ? AND id > ? AND id <= ?", userID, LogTypeRefund, tokenID, cursor, upperIDs[0]).
			Order("id asc").Limit(500).Scan(&batch).Error; err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		for _, candidate := range batch {
			var other struct {
				Admin struct {
					Preauth int64 `json:"original_preauth_log_id"`
				} `json:"admin_info"`
			}
			if common.UnmarshalJsonStr(candidate.Other, &other) != nil {
				continue
			}
			id := other.Admin.Preauth
			if id <= 0 {
				continue
			}
			if capacity > 0 {
				if _, found := counts[id]; !found {
					if len(counts) < capacity {
						counts[id] = 0
					}
				}
			}
			if count, needed := counts[id]; needed && count < 2 {
				counts[id] = count + 1
			}
		}
		cursor = batch[len(batch)-1].ID
	}
	return counts, nil
}

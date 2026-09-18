package model

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type billingRefundReferenceCacheKey struct{}
type billingRefundReferenceStamp struct{ generation, revision int64 }
type billingRefundReferenceEntry struct {
	userID int
	stamp  billingRefundReferenceStamp
	counts map[int64]int
}
type billingRefundReferenceCache struct {
	mu       sync.Mutex
	entries  map[string]billingRefundReferenceEntry
	capacity int
	used     int
}

// WithBillingStatementRefundReferenceCache scopes reuse to one export/version
// operation. No process/global cache or new source of truth is introduced.
// Split databases use the uncached reader because they cannot certify revisions.
func WithBillingStatementRefundReferenceCache(ctx context.Context) context.Context {
	if _, ok := ctx.Value(billingRefundReferenceCacheKey{}).(*billingRefundReferenceCache); ok {
		return ctx
	}
	return context.WithValue(ctx, billingRefundReferenceCacheKey{}, &billingRefundReferenceCache{
		entries: make(map[string]billingRefundReferenceEntry), capacity: 20000,
	})
}

// ValidateBillingStatementRefundReferenceCache is the publication fence: a
// refund can change after the last batch that used its cached reference.
func ValidateBillingStatementRefundReferenceCache(ctx context.Context) error {
	cache, ok := ctx.Value(billingRefundReferenceCacheKey{}).(*billingRefundReferenceCache)
	if !ok {
		return nil
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for scope, entry := range cache.entries {
		stamp, err := billingRefundReferenceSourceStamp(ctx, entry.userID, scope)
		if err != nil {
			return err
		}
		if stamp != entry.stamp {
			return ErrBillingStatementVersionConflict
		}
	}
	return nil
}

func (cache *billingRefundReferenceCache) count(ctx context.Context, userID, tokenID int, ids []int64) (map[int64]int, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	scope := fmt.Sprintf("refund:%d:%d", userID, tokenID)
	entry, found := cache.entries[scope]
	if !found {
		// Register under the same customer evidence lock as version generation.
		// Writers then update this precise scope even while confirmation is off.
		err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			registration := BillingStatementRevision{Scope: fmt.Sprintf("ev:%d:0", userID), Generation: 1}
			if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope"}}, DoNothing: true}).Create(&registration).Error; err != nil {
				return err
			}
			if err := lockForUpdate(tx).Where("scope = ?", registration.Scope).Take(&registration).Error; err != nil {
				return err
			}
			return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "scope"}}, DoNothing: true}).Create(&BillingStatementRevision{Scope: scope, Generation: 1}).Error
		})
		if err != nil {
			return nil, err
		}
	}
	stamp, err := billingRefundReferenceSourceStamp(ctx, userID, scope)
	if err != nil {
		if !found && errors.Is(err, ErrBillingStatementSourceIncomplete) {
			// Existing partial retention permits an unfrozen export, but cannot
			// certify reuse. Continue the original full reference verification.
			return scanBillingStatementRefundReferences(ctx, userID, tokenID, ids, 0)
		}
		return nil, err
	}
	if found {
		if stamp != entry.stamp {
			return nil, ErrBillingStatementVersionConflict
		}
	}
	covered := found
	for _, id := range ids {
		if _, ok := entry.counts[id]; !ok {
			covered = false
		}
	}
	if !covered {
		remaining := max(cache.capacity-cache.used-1, 0)
		counts, err := scanBillingStatementRefundReferences(ctx, userID, tokenID, ids, remaining)
		if err != nil {
			return nil, err
		}
		after, err := billingRefundReferenceSourceStamp(ctx, userID, scope)
		if err != nil {
			return nil, err
		}
		if stamp != after {
			return nil, ErrBillingStatementVersionConflict
		}
		if !found && remaining > 0 {
			entry = billingRefundReferenceEntry{userID: userID, stamp: stamp, counts: make(map[int64]int)}
			cache.entries[scope] = entry
			cache.used++
		}
		for id, count := range counts {
			if _, exists := entry.counts[id]; !exists && cache.used < cache.capacity && entry.counts != nil {
				entry.counts[id] = count
				cache.used++
			}
		}
		// The current batch always gets its fully verified counts, even when
		// bounded cache space is exhausted. A later miss scans rather than guesses.
		result := make(map[int64]int, len(ids))
		for _, id := range ids {
			result[id] = counts[id]
		}
		return result, nil
	}
	result := make(map[int64]int, len(ids))
	for _, id := range ids {
		result[id] = entry.counts[id]
	}
	return result, nil
}

func billingRefundReferenceSourceStamp(ctx context.Context, userID int, scope string) (billingRefundReferenceStamp, error) {
	var state struct {
		Generation int64
		Enabled    bool
		Revision   int64
		Partial    int64
	}
	// One database statement observes maintenance, precise revision and failure
	// fences together; never combine a pre-maintenance revision with a later era.
	err := DB.WithContext(ctx).Table("billing_statement_maintenance AS m").
		Select("m.generation, m.enabled, r.revision, (SELECT COUNT(*) FROM billing_statement_retentions WHERE user_id = ? AND status = ?) AS partial", userID, BillingStatementRetentionPartial).
		Joins("JOIN billing_statement_revisions AS r ON r.scope = ?", scope).
		Where("m.id = ?", billingStatementMaintenanceRowID).Take(&state).Error
	if err != nil {
		return billingRefundReferenceStamp{}, err
	}
	if state.Enabled {
		return billingRefundReferenceStamp{}, ErrBillingStatementVersionConflict
	}
	if state.Partial > 0 {
		return billingRefundReferenceStamp{}, ErrBillingStatementSourceIncomplete
	}
	return billingRefundReferenceStamp{generation: state.Generation, revision: state.Revision}, nil
}

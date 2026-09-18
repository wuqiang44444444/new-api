package model

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestStatementRefundReferenceCacheReusesCompleteHistoryAcrossBatches(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	logs := make([]Log, 0, 502)
	for id := 1; id <= 501; id++ {
		logs = append(logs, Log{UserId: 11, TokenId: 4, Type: LogTypeRefund, CreatedAt: 1000,
			Other: fmt.Sprintf(`{"admin_info":{"original_preauth_log_id":%d}}`, id)})
	}
	logs = append(logs, Log{UserId: 11, TokenId: 4, Type: LogTypeRefund, CreatedAt: 4000000, Other: `{"admin_info":{"original_preauth_log_id":42}}`})
	require.NoError(t, db.CreateInBatches(logs, 100).Error)
	historyReads := 0
	require.NoError(t, db.Callback().Row().After("gorm:row").Register("test:refund_reads", func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" && strings.Contains(tx.Statement.SQL.String(), "id, other") {
			historyReads++
		}
	}))
	ctx := WithBillingStatementRefundReferenceCache(context.Background())
	first, err := countBillingStatementRefundReferences(ctx, 11, 4, []int64{1})
	require.NoError(t, err)
	assert.Equal(t, 1, first[1])
	second, err := countBillingStatementRefundReferences(ctx, 11, 4, []int64{42, 501})
	require.NoError(t, err)
	assert.Equal(t, map[int64]int{42: 2, 501: 1}, second, "later batches must include cross-period duplicate evidence")
	assert.Equal(t, 2, historyReads, "one two-page history scan, not a fresh scan for each export batch")
	// Current usage and a different key must not invalidate the historical
	// refund proof; a relevant refund mutation must invalidate it.
	require.NoError(t, db.Create(&Log{UserId: 11, TokenId: 4, Type: LogTypeConsume, Quota: 100}).Error)
	require.NoError(t, db.Create(&Log{UserId: 11, TokenId: 5, Type: LogTypeRefund, Other: logs[0].Other}).Error)
	_, err = countBillingStatementRefundReferences(ctx, 11, 4, []int64{1})
	require.NoError(t, err)
	assert.Equal(t, 2, historyReads)
	require.NoError(t, ValidateBillingStatementRefundReferenceCache(ctx))
	require.NoError(t, db.Create(&Log{UserId: 11, TokenId: 4, Type: LogTypeRefund, Other: logs[0].Other}).Error)
	require.ErrorIs(t, ValidateBillingStatementRefundReferenceCache(ctx), ErrBillingStatementVersionConflict)
	counts, err := countBillingStatementRefundReferences(ctx, 11, 4, []int64{1})
	require.ErrorIs(t, err, ErrBillingStatementVersionConflict)
	assert.Nil(t, counts, "a changed source cannot certify a cached unique reference")
}

func TestStatementRefundReferenceCacheFailsClosed(t *testing.T) {
	for _, scenario := range []string{"cancelled", "maintenance", "revision failure without retention"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			log := Log{UserId: 11, TokenId: 4, Type: LogTypeRefund, Other: `{"admin_info":{"original_preauth_log_id":1}}`}
			require.NoError(t, db.Create(&log).Error)
			ctx, cancel := context.WithCancel(WithBillingStatementRefundReferenceCache(context.Background()))
			defer cancel()
			_, err := countBillingStatementRefundReferences(ctx, 11, 4, []int64{1})
			require.NoError(t, err)
			switch scenario {
			case "cancelled":
				cancel()
			case "maintenance":
				_, err := BeginBillingStatementMaintenance(ctx, "verified repair", 7)
				require.NoError(t, err)
			case "revision failure without retention":
				require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:revision_failure", func(tx *gorm.DB) {
					if tx.Statement.Table == "billing_statement_revisions" {
						tx.AddError(fmt.Errorf("revision unavailable"))
					}
				}))
				log.Id = 0
				require.NoError(t, db.Create(&log).Error, "source remains durable while retention fences confirmation")
			}
			counts, err := countBillingStatementRefundReferences(ctx, 11, 4, []int64{1})
			require.Error(t, err)
			assert.Nil(t, counts)
		})
	}
}

func TestStatementRefundReferenceCacheCapacityDoesNotLimitEvidence(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.Create(&[]Log{
		{UserId: 11, TokenId: 4, Type: LogTypeRefund, Other: `{"admin_info":{"original_preauth_log_id":1}}`},
		{UserId: 11, TokenId: 4, Type: LogTypeRefund, Other: `{"admin_info":{"original_preauth_log_id":2}}`},
		{UserId: 11, TokenId: 4, Type: LogTypeRefund, Other: `{"admin_info":{"original_preauth_log_id":2}}`},
	}).Error)
	ctx := WithBillingStatementRefundReferenceCache(context.Background())
	ctx.Value(billingRefundReferenceCacheKey{}).(*billingRefundReferenceCache).capacity = 2
	first, err := countBillingStatementRefundReferences(ctx, 11, 4, []int64{1})
	require.NoError(t, err)
	assert.Equal(t, 1, first[1])
	second, err := countBillingStatementRefundReferences(ctx, 11, 4, []int64{2, 3})
	require.NoError(t, err)
	assert.Equal(t, map[int64]int{2: 2, 3: 0}, second)
}

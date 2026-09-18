package model

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrozenListRejectsMissingQualityWithoutReconstructingFacts(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	snapshot, err := FreezeBillingStatementProjection(BillingCustomerStatement{UserId: 11})
	require.NoError(t, err)
	_, err = frozenBillingStatementListItem(BillingStatementVersion{UserId: 11, SummaryProjection: snapshot, QuotaPerUnit: 500000})
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
}

func TestFrozenListLargeCustomerSetPreservesFrozenTotals(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))
	snapshot, err := FreezeBillingStatementProjection(BillingCustomerStatement{Username: "frozen", Summary: BillingReconciliationUsage{Requests: 1, GrossQuota: 10, NetQuota: 10}, DataQuality: &BillingReconciliationDataQuality{Status: "complete"}})
	require.NoError(t, err)
	versions := make([]BillingCustomerStatementListItem, 1001)
	for i := range versions {
		versions[i], err = frozenBillingStatementListItem(BillingStatementVersion{UserId: i + 1, SummaryProjection: snapshot, QuotaPerUnit: 500000})
		require.NoError(t, err)
	}
	require.NoError(t, db.Create(&Log{UserId: 1, Type: LogTypeConsume, Quota: 999, CreatedAt: 1500}).Error)
	result, err := GetBillingCustomerStatementList(context.Background(), 1000, 2000, "", "complete", "net_quota", "desc", 1, 10, versions...)
	require.NoError(t, err)
	assert.EqualValues(t, 1001, result.Total)
	assert.EqualValues(t, 10010, result.Summary.Usage.NetQuota)
}

func TestFrozenListReadsOnlyCurrentConfirmedVersion(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	snapshot, err := FreezeBillingStatementProjection(BillingCustomerStatement{UserId: 11, Username: "frozen", Summary: BillingReconciliationUsage{NetQuota: 10}, DataQuality: &BillingReconciliationDataQuality{Status: "complete"}})
	require.NoError(t, err)
	current := BillingStatementVersion{DraftPublicId: "current", UserId: 11, PeriodStart: 1000, PeriodEndExclusive: 2001, Status: BillingStatementVersionConfirmed, QuotaPerUnit: 500000, SummaryProjection: snapshot}
	require.NoError(t, db.Create(&current).Error)
	require.NoError(t, db.Create(&BillingStatementVersion{DraftPublicId: "historical", UserId: 11, PeriodStart: 1000, Status: BillingStatementVersionConfirmed}).Error)
	require.NoError(t, db.Create(&BillingStatementMonth{UserId: 11, PeriodStart: 1000, CurrentVersionId: &current.ID}).Error)
	items, err := CurrentBillingStatementListItems(context.Background(), 1000, 2000)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.EqualValues(t, 10, items[0].Usage.NetQuota)
	assert.Equal(t, "0.00002000", billingStatementListMoney(items[0]).Net)
	require.NoError(t, db.Model(&current).Update("status", BillingStatementVersionPending).Error)
	_, err = CurrentBillingStatementListItems(context.Background(), 1000, 2000)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
}

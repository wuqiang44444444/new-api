package model

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUsageRepairStartupHonorsRecordedManualRefunds(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	testUsageRepairStartup(t, db)
}

// Shared with the existing real-engine suite; no artificial reviewed file is
// created. This is the same entry used by application initialization.
func testUsageRepairStartup(t *testing.T, db *gorm.DB) {
	t.Helper()
	report, err := RepairRecordedUserUsage(db, db)
	require.NoError(t, err)
	assert.Empty(t, report.Blocked)
	assert.Equal(t, 1, report.Corrected)
	var user User
	require.NoError(t, db.First(&user, fixtureTargetUserID).Error)
	assert.Equal(t, fixtureExpectedAfter, int64(user.UsedQuota))
	assert.Equal(t, fixtureWalletQuota, int64(user.Quota))
	assert.Equal(t, fixtureRequestCount, int64(user.RequestCount))
	var channel Channel
	require.NoError(t, db.First(&channel, 7).Error)
	assert.Equal(t, int64(123456789012), channel.UsedQuota)
	var refunds int64
	require.NoError(t, db.Model(&Log{}).Where("user_id = ? AND type = ?", fixtureTargetUserID, LogTypeRefund).Select("SUM(quota)").Scan(&refunds).Error)
	assert.Equal(t, fixtureRefundQuota, refunds)
	var audits int64
	require.NoError(t, db.Model(&AuditLog{}).Count(&audits).Error)
	// Simulate a later correctly accounted charge, including its normal log.
	require.NoError(t, db.Model(&User{}).Where("id = ?", fixtureTargetUserID).UpdateColumn("used_quota", gorm.Expr("used_quota + ?", 500)).Error)
	require.NoError(t, db.Create(&Log{UserId: fixtureTargetUserID, Type: LogTypeConsume, Quota: 500, RequestId: "after-startup", Other: "{}"}).Error)
	report, err = RepairRecordedUserUsage(db, db)
	require.NoError(t, err)
	assert.Empty(t, report.Blocked)
	assert.Zero(t, report.Corrected)
	require.NoError(t, db.First(&user, fixtureTargetUserID).Error)
	assert.Equal(t, fixtureExpectedAfter+500, int64(user.UsedQuota))
	var afterAudits int64
	require.NoError(t, db.Model(&AuditLog{}).Count(&afterAudits).Error)
	assert.Equal(t, audits, afterAudits)
	// History retention must not make a completed migration run again.
	require.NoError(t, db.Where("user_id = ?", fixtureTargetUserID).Delete(&Log{}).Error)
	report, err = RepairRecordedUserUsage(db, db)
	require.NoError(t, err)
	assert.Empty(t, report.Blocked)
	require.NoError(t, db.First(&user, fixtureTargetUserID).Error)
	assert.Equal(t, fixtureExpectedAfter+500, int64(user.UsedQuota))
}

func TestUsageRepairStartupRejectsUnexplainedEvidence(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, *gorm.DB)
	}{
		{"unexplained residual", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&User{}).Where("id = ?", fixtureTargetUserID).UpdateColumn("used_quota", gorm.Expr("used_quota + 1")).Error)
		}},
		{"duplicate request", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Create(&Log{UserId: fixtureTargetUserID, Type: LogTypeConsume, Quota: 0, RequestId: "req-c1", Other: "{}"}).Error)
		}},
		{"pending delivery", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Model(&TaskBillingDelivery{}).Where("task_row_id = ?", 301).UpdateColumn("delivered_at", 0).Error)
		}},
		{"negative charge", func(t *testing.T, db *gorm.DB) {
			require.NoError(t, db.Create(&Log{UserId: fixtureTargetUserID, Type: LogTypeConsume, Quota: -1, RequestId: "negative", Other: "{}"}).Error)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newUsageRepairDB(t)
			seedUsageRepairFixture(t, db)
			seedUsageRepairRefunds(t, db)
			tc.mutate(t, db)
			var before User
			require.NoError(t, db.First(&before, fixtureTargetUserID).Error)
			report, err := RepairRecordedUserUsage(db, db)
			require.NoError(t, err)
			require.Contains(t, report.Blocked, fixtureTargetUserID)
			var after User
			require.NoError(t, db.First(&after, fixtureTargetUserID).Error)
			assert.Equal(t, before.UsedQuota, after.UsedQuota)
			assert.Equal(t, before.Quota, after.Quota)
			previous := usageRepairAvailability.Load()
			t.Cleanup(func() { usageRepairAvailability.Store(previous) })
			usageRepairAvailability.Store(report)
			assert.Nil(t, UserUsedQuotaForDisplay(&after))
			require.NotNil(t, UserUsedQuotaForDisplay(&User{Id: fixtureOperatorID}))
		})
	}
}

func TestUsageRepairStartupAuditFailureRollsBackAmount(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("fail_usage_audit", func(tx *gorm.DB) {
		if audit, ok := tx.Statement.Dest.(*AuditLog); ok && audit.EventId == "usage-recorded-refunds-v1:91" {
			tx.AddError(errors.New("audit unavailable"))
		}
	}))
	_, err := RepairRecordedUserUsage(db, db)
	require.ErrorContains(t, err, "audit unavailable")
	var user User
	require.NoError(t, db.First(&user, fixtureTargetUserID).Error)
	assert.Equal(t, fixtureUsedQuota, int64(user.UsedQuota))
	assert.Equal(t, fixtureWalletQuota, int64(user.Quota))
	require.NoError(t, db.Callback().Create().Remove("fail_usage_audit"))
	report, err := RepairRecordedUserUsage(db, db)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Corrected)
}

func TestUsageRepairStartupKeepsAlreadyNetAndUnloggedHistory(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	require.NoError(t, db.Model(&User{}).Where("id = ?", fixtureTargetUserID).UpdateColumn("used_quota", fixtureExpectedAfter).Error)
	// Existing usage with cleaned logs is not replaced by zero.
	require.NoError(t, db.Model(&User{}).Where("id = ?", fixtureOperatorID).UpdateColumn("used_quota", 12345).Error)
	report, err := RepairRecordedUserUsage(db, db)
	require.NoError(t, err)
	assert.Empty(t, report.Blocked)
	assert.Zero(t, report.Corrected)
	var user User
	require.NoError(t, db.First(&user, fixtureOperatorID).Error)
	assert.Equal(t, 12345, user.UsedQuota)
}

func TestUsageRepairStartupRejectsSplitDatabase(t *testing.T) {
	db := newUsageRepairDB(t)
	other := newUsageRepairDB(t)
	report, err := RepairRecordedUserUsage(db, other)
	require.NoError(t, err)
	assert.True(t, report.UnsupportedTopology)
	previous := usageRepairAvailability.Load()
	t.Cleanup(func() { usageRepairAvailability.Store(previous) })
	usageRepairAvailability.Store(report)
	assert.Nil(t, UserUsedQuotaForDisplay(&User{Id: 1, UsedQuota: 100}))
}

func TestUsageRepairStartupRollbackAndRestart(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	// A later user gets its own startup audit; that must not prevent reversal.
	require.NoError(t, db.Create(&User{Id: 92, Username: "later", AffCode: "u92"}).Error)
	_, err := RepairRecordedUserUsage(db, db)
	require.NoError(t, err)
	reverse, already, err := PrepareRecordedUsageRepairRollback(db, fixtureTargetUserID, "fixture")
	require.NoError(t, err)
	assert.False(t, already)
	applied, err := ApplyUserUsageRepair(db, reverse, fixtureOperatorID)
	require.NoError(t, err)
	assert.True(t, applied)
	_, already, err = PrepareRecordedUsageRepairRollback(db, fixtureTargetUserID, "fixture")
	require.NoError(t, err)
	assert.True(t, already)
	report, err := RepairRecordedUserUsage(db, db)
	require.NoError(t, err)
	assert.Contains(t, report.Blocked, fixtureTargetUserID)
	var user User
	require.NoError(t, db.First(&user, fixtureTargetUserID).Error)
	assert.Equal(t, fixtureUsedQuota, int64(user.UsedQuota))
	assert.Equal(t, fixtureWalletQuota, int64(user.Quota))
}

func TestUsageRepairStartupRollbackRejectsLaterMaintenance(t *testing.T) {
	db := newUsageRepairDB(t)
	seedUsageRepairFixture(t, db)
	seedUsageRepairRefunds(t, db)
	_, err := RepairRecordedUserUsage(db, db)
	require.NoError(t, err)
	require.NoError(t, db.Create(&AuditLog{EventId: "later-maintenance", Action: "admin.update", Success: true, Content: "{}"}).Error)
	_, _, err = PrepareRecordedUsageRepairRollback(db, fixtureTargetUserID, "fixture")
	require.ErrorContains(t, err, "maintenance audit changed")
}

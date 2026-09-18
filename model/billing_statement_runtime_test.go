package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
)

func TestBillingStatementNonMasterWriterInvalidatesDraftWhileDisabled(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	previousDB, previousLog := DB, LOG_DB
	previousMaster := common.IsMasterNode
	t.Cleanup(func() { DB, LOG_DB = previousDB, previousLog; common.IsMasterNode = previousMaster })
	path := filepath.Join(t.TempDir(), "writers.db")
	master, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	masterSQL, err := master.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = masterSQL.Close() })
	DB, LOG_DB = master, master
	require.NoError(t, migrateBillingStatementVersionDB())
	require.NoError(t, master.AutoMigrate(&Log{}, &Option{}))
	require.NoError(t, InitBillingStatementSourceTracking())
	enableVersionSwitch(t)
	ctx := context.Background()
	month := int64(1756656000)
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, month, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, month, draft)
	slave, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	slaveSQL, err := slave.DB()
	require.NoError(t, err)
	defer slaveSQL.Close()
	DB, LOG_DB, common.IsMasterNode = slave, slave, false
	require.NoError(t, InitBillingStatementSourceTracking())
	require.NoError(t, InitBillingStatementSourceTracking())
	common.OptionMap[BillingStatementVersionEnabledKey] = "false"
	require.NoError(t, slave.Save(&Option{Key: BillingStatementVersionEnabledKey, Value: "false"}).Error)
	require.NoError(t, slave.Create(&Log{UserId: 11, Type: LogTypeConsume, CreatedAt: month + 60, Quota: 10}).Error)
	DB, LOG_DB = master, master
	common.OptionMap[BillingStatementVersionEnabledKey] = "true"
	require.NoError(t, master.Save(&Option{Key: BillingStatementVersionEnabledKey, Value: "true"}).Error)
	_, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "check", "", "", 7)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
	assert.False(t, committed)
}

func TestConfirmedBillingStatementRetrySurvivesDisabledSwitch(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, 1756656000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, 1756656000, draft)
	first, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "same", "", "", 7)
	require.NoError(t, err)
	require.True(t, committed)
	require.NoError(t, DB.Save(&Option{Key: BillingStatementVersionEnabledKey, Value: "false"}).Error)
	second, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "same", "", "", 7)
	require.NoError(t, err)
	assert.False(t, committed)
	assert.Equal(t, first.ID, second.ID)
}

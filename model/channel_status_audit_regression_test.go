package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupChannelAuditRegressionDB(t *testing.T) *gorm.DB {
	t.Helper()
	previousDB, previousLogDB := DB, LOG_DB
	previousType, previousCache := common.MainDatabaseType(), common.MemoryCacheEnabled
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&Channel{}, &Ability{}, &User{}, &AuditLog{}, &ChannelAssetScopeIdentity{}, &ChannelAssetCredential{}, &ChannelDefaultAssetGroup{}))
	DB, LOG_DB = db, db
	common.MemoryCacheEnabled = false
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.MemoryCacheEnabled = previousCache
		common.SetMainDatabaseType(previousType)
		require.NoError(t, sqlDB.Close())
	})
	return db
}

func TestChannelStatusAuditRecoveryPrivacyAndCorrelation(t *testing.T) {
	db := setupChannelAuditRegressionDB(t)
	channel := Channel{Key: "key-a\nkey-b", Status: common.ChannelStatusEnabled, ChannelInfo: ChannelInfo{IsMultiKey: true}}
	require.NoError(t, db.Create(&channel).Error)
	privateReason := "Bearer fixture-secret https://example.invalid/?signature=private"
	observation := ChannelStatusAudit{RequestID: "test-failed", Trigger: "response_time_exceeded"}
	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusAutoDisabled, privateReason, observation))
	require.True(t, UpdateChannelStatus(channel.Id, "key-b", common.ChannelStatusAutoDisabled, privateReason, observation))
	require.True(t, UpdateChannelStatus(channel.Id, "key-a", common.ChannelStatusEnabled, "", ChannelStatusAudit{RequestID: "test-recovered", Trigger: "channel_test_recovered"}))
	var rows []AuditLog
	require.NoError(t, db.Order("id").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, "test-failed", rows[0].RequestId)
	assert.Equal(t, `"response_time_exceeded"`, auditParam(t, rows[0].Other.Op.Params, "reason"))
	assert.Equal(t, `"auto_disabled"`, auditParam(t, rows[1].Other.Op.Params, "before"))
	assert.Equal(t, `"enabled"`, auditParam(t, rows[1].Other.Op.Params, "after"))
	assert.Equal(t, "test-recovered", rows[1].RequestId)
	encoded, err := common.Marshal(rows)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "fixture-secret")
	assert.NotContains(t, string(encoded), "signature")
}

func TestChannelStatusAuditRollbackAndConcurrentRepeat(t *testing.T) {
	db := setupChannelAuditRegressionDB(t)
	channel := Channel{Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	// Fail ability persistence after the channel write: the transaction must undo
	// both the state change and any claim that it committed.
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("audit:fail_ability", func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			tx.AddError(errors.New("injected ability write failure"))
		}
	}))
	changed, err := UpdateChannelStatusWithActor(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual", 0)
	require.Error(t, err)
	assert.False(t, changed)
	require.NoError(t, db.Callback().Update().Remove("audit:fail_ability"))
	var stored Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	var count int64
	require.NoError(t, db.Model(&AuditLog{}).Count(&count).Error)
	assert.Zero(t, count)

	// Two independent callers race for the same transition; exactly one wins.
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	failures := make(chan error, 1)
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		results <- UpdateChannelStatus(channel.Id, "", common.ChannelStatusAutoDisabled, "auto")
	}()
	go func() {
		defer wg.Done()
		<-start
		changed, err := UpdateChannelStatusWithActor(channel.Id, "", common.ChannelStatusAutoDisabled, "manual", 0)
		results <- changed
		failures <- err
	}()
	close(start)
	wg.Wait()
	require.NoError(t, <-failures)
	assert.NotEqual(t, <-results, <-results)
	require.NoError(t, db.Model(&AuditLog{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestChannelStatusAuditGeneralEdit(t *testing.T) {
	db := setupChannelAuditRegressionDB(t)
	channel := Channel{Status: common.ChannelStatusEnabled, Group: "default", Models: "model-a", Key: "fixture"}
	require.NoError(t, db.Create(&channel).Error)
	channel.Status = common.ChannelStatusManuallyDisabled
	require.NoError(t, channel.UpdateWithActor(0, ChannelStatusAudit{RequestID: "edit-request"}))
	var rows []AuditLog
	require.NoError(t, db.Where("action = ?", channelStatusAuditAction).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "edit-request", rows[0].RequestId)
	assert.Equal(t, `"enabled"`, auditParam(t, rows[0].Other.Op.Params, "before"))
	assert.Equal(t, `"manually_disabled"`, auditParam(t, rows[0].Other.Op.Params, "after"))
	require.NoError(t, channel.UpdateWithActor(0))
	var count int64
	require.NoError(t, db.Model(&AuditLog{}).Where("action = ?", channelStatusAuditAction).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

// The pool rejects COMMIT after a successful transaction body and rolls the
// real test transaction back, without introducing a production test hook.
type rejectedChannelCommitPool struct{ *sql.DB }

func (p rejectedChannelCommitPool) BeginTx(ctx context.Context, options *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &rejectedChannelCommit{tx}, nil
}

type rejectedChannelCommit struct{ *sql.Tx }

func (tx rejectedChannelCommit) Commit() error {
	if err := tx.Tx.Rollback(); err != nil {
		return err
	}
	return errors.New("injected commit rejection")
}

func TestChannelStatusAuditCommitFailureAndUnavailableLog(t *testing.T) {
	db := setupChannelAuditRegressionDB(t)
	channel := Channel{Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	failingDB := db.WithContext(context.Background())
	failingDB.Statement.ConnPool = rejectedChannelCommitPool{sqlDB}
	DB = failingDB
	changed, err := UpdateChannelStatusWithActor(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual", 0)
	require.ErrorContains(t, err, "injected commit rejection")
	assert.False(t, changed)
	DB = db
	var stored Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusEnabled, stored.Status)
	var count int64
	require.NoError(t, db.Model(&AuditLog{}).Count(&count).Error)
	assert.Zero(t, count)
	// A log database outage must not undo the committed business transition.
	LOG_DB = nil
	changed, err = UpdateChannelStatusWithActor(channel.Id, "", common.ChannelStatusManuallyDisabled, "manual", 0)
	require.NoError(t, err)
	assert.True(t, changed)
	require.NoError(t, db.First(&stored, channel.Id).Error)
	assert.Equal(t, common.ChannelStatusManuallyDisabled, stored.Status)
}

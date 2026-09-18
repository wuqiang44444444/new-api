package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingStatementLogIndexPreservesStatementsAndWrites(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Channel{}, &Task{}))
	require.NoError(t, db.Create(&User{Id: 7, Username: "index-customer", Quota: 8800}).Error)
	const start int64 = 1756656000
	const end = start + 1000
	const facts = `{"contract_applicable":true,"group_ratio":0.8,"model_ratio":1,"usage_semantic":"openai","contract_id":1,"contract_version":1,"contract_discount_ratio":0.5,"contract_name":"original"}`
	// More than 300 combinations, deliberately opposite timestamp/ID order.
	// The indexed traversal must not change which rows overflow or which
	// historical name wins. ID order is also the existing export contract.
	logs := make([]Log, 0, 308)
	for i := 0; i < 301; i++ {
		logs = append(logs, Log{UserId: 7, TokenId: 11, TokenName: "old-key", ChannelId: 21,
			ModelName: fmt.Sprintf("model-%03d", i), Type: LogTypeConsume,
			CreatedAt: start + 500 - int64(i), Quota: 40, PromptTokens: 10, CompletionTokens: 2, Other: facts})
	}
	logs = append(logs,
		Log{UserId: 7, TokenId: 11, TokenName: "old-key", ChannelId: 21, ModelName: "model-000", Type: LogTypeRefund, CreatedAt: start + 900, Quota: 40, Other: facts},
		Log{UserId: 7, TokenId: 11, TokenName: "new-key", ChannelId: 21, ModelName: "model-000", Type: LogTypeConsume, CreatedAt: start, Quota: 40, Other: `{"contract_applicable":true,"group_ratio":0.8,"model_ratio":1,"contract_id":1,"contract_version":1,"contract_discount_ratio":0.5,"contract_name":"renamed"}`},
		Log{UserId: 7, TokenName: "模型测试", Content: "模型测试", Type: LogTypeConsume, CreatedAt: start + 10, Quota: 99999},
		Log{UserId: 8, TokenId: 11, Type: LogTypeConsume, CreatedAt: start + 10, Quota: 99999},
		Log{UserId: 7, TokenId: 11, Type: LogTypeConsume, CreatedAt: end + 1, Quota: 99999},
	)
	require.NoError(t, db.CreateInBatches(&logs, 50).Error)
	// A refund in this period must still recover facts from a prior-period
	// consumption. It also tests the inclusive upper time boundary.
	original := Log{UserId: 7, TokenId: 12, ModelName: "manual", Type: LogTypeConsume, CreatedAt: start - 1, Quota: 87,
		Other: `{"contract_applicable":false,"group_ratio":0.87,"model_price":1}`}
	require.NoError(t, createLog(&original))
	refund := Log{UserId: 7, TokenId: 12, ModelName: "manual", Type: LogTypeRefund, CreatedAt: end, Quota: 87,
		Other: fmt.Sprintf(`{"admin_info":{"original_preauth_log_id":%d}}`, original.Id)}
	require.NoError(t, createLog(&refund))

	filters := []struct {
		dimension string
		group     int
		model     string
		mode      string
	}{
		{"api_key", 0, "", ""}, {"channel", 21, "", ""},
		{"api_key", 11, "model-000", "token"}, {"api_key", 12, "manual", "per_call"},
	}
	before := make([]BillingCustomerStatement, len(filters))
	for i, f := range filters {
		var err error
		before[i], err = GetBillingCustomerStatement(context.Background(), 7, start, end, f.dimension, f.group, f.model, f.mode)
		require.NoError(t, err)
	}
	assert.EqualValues(t, 12080, before[0].Summary.GrossQuota)
	assert.EqualValues(t, 127, before[0].Summary.RefundQuota)
	assert.EqualValues(t, 11953, before[0].Summary.NetQuota)
	assert.Equal(t, "new-key", before[2].Groups[0].Name)
	assert.Equal(t, "original", before[2].DiscountCombinations[0].ContractName)
	require.NotNil(t, before[3].OriginalQuota)
	assert.EqualValues(t, -100, *before[3].OriginalQuota)
	require.Len(t, before[0].DiscountCombinations, 301)
	legacyItems, legacySummary, err := GetUserBillingStatement(7, start, end, 11, "")
	require.NoError(t, err)
	breakdownItems, breakdownSummary, err := GetUserBillingStatementBreakdown(7, start, end, 11, "")
	require.NoError(t, err)

	var storedBefore []Log
	require.NoError(t, db.Order("id asc").Find(&storedBefore).Error)
	require.NoError(t, migrateBillingStatementLogIndex(db))
	require.NoError(t, migrateBillingStatementLogIndex(db), "restarts must not recreate the index")
	var storedAfter []Log
	require.NoError(t, db.Order("id asc").Find(&storedAfter).Error)
	assert.Equal(t, storedBefore, storedAfter, "index migration must not rewrite log facts")
	if common.UsingLogDatabase(common.DatabaseTypeSQLite) {
		// Reverse any remaining unordered scans to catch accidental dependence
		// on a planner's current choice (including refund evidence reads).
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)
		require.NoError(t, db.Exec("PRAGMA reverse_unordered_selects = ON").Error)
		var plan []struct{ Detail string }
		require.NoError(t, db.Raw("EXPLAIN QUERY PLAN SELECT other FROM logs WHERE user_id = ? AND type IN (2,6) AND type = 6 AND created_at >= ? AND created_at <= ?", 7, start, end).Scan(&plan).Error)
		require.Len(t, plan, 1)
		assert.Contains(t, plan[0].Detail, "idx_logs_user_type_created_at")
		assert.Contains(t, plan[0].Detail, "user_id=? AND type=? AND created_at>? AND created_at<?")
		plan = nil
		require.NoError(t, db.Raw("EXPLAIN QUERY PLAN SELECT id, other FROM logs WHERE user_id = ? AND type = ? AND token_id = ? AND id > ? AND id <= ? ORDER BY id ASC LIMIT 500", 7, LogTypeRefund, 11, 0, 999999).Scan(&plan).Error)
		require.Len(t, plan, 1)
		assert.Contains(t, plan[0].Detail, "idx_logs_user_type_token_cursor")

	}
	for i, f := range filters {
		after, err := GetBillingCustomerStatement(context.Background(), 7, start, end, f.dimension, f.group, f.model, f.mode)
		require.NoError(t, err)
		assert.Equal(t, before[i], after, "complete response for filter %+v", f)
		exported, err := GetBillingCustomerStatement(context.Background(), 7, start, end, f.dimension, f.group, f.model, f.mode,
			BillingStatementReadPolicy{BeforeBatch: func(context.Context) error { return nil }})
		require.NoError(t, err)
		assert.Equal(t, after, exported, "interactive and export order must agree")
	}
	legacyAfter, legacySummaryAfter, err := GetUserBillingStatement(7, start, end, 11, "")
	require.NoError(t, err)
	assert.Equal(t, legacyItems, legacyAfter)
	assert.Equal(t, legacySummary, legacySummaryAfter)
	breakdownAfter, breakdownSummaryAfter, err := GetUserBillingStatementBreakdown(7, start, end, 11, "")
	require.NoError(t, err)
	assert.Equal(t, breakdownItems, breakdownAfter)
	assert.Equal(t, breakdownSummary, breakdownSummaryAfter)

	// Identical index keys are legal and every successful write remains visible.
	for i := 0; i < 2; i++ {
		log := Log{UserId: 7, TokenId: 11, TokenName: "new-key", Type: LogTypeConsume, CreatedAt: start + 50, ModelName: "model-000", Quota: 40, Other: facts}
		require.NoError(t, createLog(&log))
		require.NotZero(t, log.Id)
	}
	rollback := errors.New("rollback fixture")
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&Log{UserId: 7, Type: LogTypeConsume, CreatedAt: start + 50, Quota: 99999}).Error; err != nil {
			return err
		}
		return rollback
	}), rollback)
	afterWrites, err := GetBillingCustomerStatement(context.Background(), 7, start, end, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, before[0].Summary.GrossQuota+80, afterWrites.Summary.GrossQuota)
	assert.Equal(t, before[0].Summary.RefundQuota, afterWrites.Summary.RefundQuota)
	assert.EqualValues(t, 8800, *afterWrites.CurrentBalance)
	var count int64
	require.NoError(t, db.Model(&Log{}).Count(&count).Error)
	assert.EqualValues(t, len(storedBefore)+2, count)
}

func TestBillingStatementLogIndexStartupTopologies(t *testing.T) {
	for _, shared := range []bool{true, false} {
		t.Run(fmt.Sprintf("shared=%t", shared), func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			previousMaster := common.IsMasterNode
			common.IsMasterNode = true
			t.Cleanup(func() { common.IsMasterNode = previousMaster })
			if shared {
				t.Setenv("LOG_SQL_DSN", "")
				require.NoError(t, InitLogDB())
				require.NoError(t, InitLogDB())
			} else {
				// Exercise the separate log DB migrator, with no primary DB
				// available: the index must be created on LOG_DB alone.
				DB = nil
				require.NoError(t, migrateLOGDB())
				require.NoError(t, migrateLOGDB())
				DB = db
			}
			assert.True(t, db.Migrator().HasIndex(&Log{}, "idx_logs_user_type_created_at"))
			require.NoError(t, createLog(&Log{UserId: 9, Type: LogTypeError, Content: "fixture"}))
		})
	}
}

func TestBillingStatementLogIndexConcurrentReadAndWrite(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	if common.UsingLogDatabase(common.DatabaseTypeSQLite) {
		// Use the deployed WAL/timeout settings, rather than shared-memory
		// SQLite, whose locking behavior is different from the file database.
		var err error
		db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "logs.db")+"?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate"), &gorm.Config{})
		require.NoError(t, err)
		LOG_DB = db
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	require.NoError(t, db.AutoMigrate(&Log{}))
	require.NoError(t, migrateBillingStatementLogIndex(db))
	first := Log{UserId: 7, TokenId: 11, Type: LogTypeConsume, CreatedAt: 1000, Quota: 40, Other: `{"model_price":1}`}
	require.NoError(t, createLog(&first))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cursorOpen, writeDone := make(chan struct{}), make(chan struct{})
	readDone := make(chan error, 1)
	var readQuota int
	go func() {
		query := db.WithContext(ctx).Model(&Log{}).
			Select("user_id, token_id, token_name, channel_id, model_name, type, created_at, prompt_tokens, completion_tokens, quota, content, other, "+billingStatementGroupSelect()).
			Where("user_id = ? AND type IN ? AND created_at >= ? AND created_at <= ?", 7, []int{LogTypeConsume, LogTypeRefund}, 1000, 1000)
		readDone <- scanBillingStatementFacts(ctx, query, BillingStatementReadPolicy{}, func(log billingReconciliationLog, _ parsedBillingReconciliationLog) error {
			if readQuota == 0 {
				close(cursorOpen)
			}
			select {
			case <-writeDone:
			case <-ctx.Done():
				return ctx.Err()
			}
			readQuota += log.Quota
			return nil
		})
	}()
	select {
	case <-cursorOpen:
	case err := <-readDone:
		t.Fatalf("reader closed before opening cursor: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Commit a duplicate index key while the reader still holds its cursor.
	second := first
	second.Id = 0
	writeErr := createLog(&second)
	close(writeDone)
	readErr := <-readDone
	require.NoError(t, writeErr)
	require.NoError(t, readErr)
	assert.Equal(t, 40, readQuota, "open cursor retains its original statement snapshot")
	var rows []Log
	require.NoError(t, db.Where("user_id = ?", 7).Order("id asc").Find(&rows).Error)
	require.Len(t, rows, 2)
	assert.Equal(t, 80, rows[0].Quota+rows[1].Quota, "next read sees both committed writes exactly once")
}

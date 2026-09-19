package model

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBillingStatementTaskIndexPreservesRefundStatements(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Channel{}, &Task{}))
	require.NoError(t, db.Create(&User{Id: 7, Username: "refund-index", Quota: 8800}).Error)
	// Cross the 200-ID evidence batch boundary. Historical tasks precede the
	// refund period and must remain usable without querying current pricing.
	tasks := make([]Task, 201)
	logs := make([]Log, len(tasks))
	for i := range tasks {
		id := fmt.Sprintf("refund-%03d", i)
		tasks[i] = Task{TaskID: id, CreatedAt: 900, UserId: 7, AppID: 11, ChannelId: 21,
			Properties: Properties{OriginModelName: "video"}, Quota: 100,
			PrivateData: TaskPrivateData{TokenId: 11, AsyncBilling: &TaskAsyncBillingContext{
				TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: `tier("seconds", u("duration"))`, UsageUnits: map[string]string{"duration": "second"}},
			}},
		}
		logs[i] = Log{UserId: 7, TokenId: 11, TokenName: "key", ChannelId: 21, ModelName: "video",
			CreatedAt: 1100, Type: LogTypeRefund, Quota: 100,
			Other: fmt.Sprintf(`{"contract_applicable":false,"task_id":%q,"model_price":0,"group_ratio":1}`, id)}
	}
	require.NoError(t, db.CreateInBatches(&tasks, 20).Error)
	require.NoError(t, db.CreateInBatches(&logs, 50).Error)
	// Same provider ID in another user's scope is not ambiguous evidence.
	foreign := tasks[0]
	foreign.ID, foreign.UserId = 0, 8
	require.NoError(t, db.Create(&foreign).Error)

	before, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 11, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 20100, before.Summary.RefundQuota)
	assert.EqualValues(t, -20100, before.Summary.NetQuota)
	require.Len(t, before.Groups, 1)
	require.Len(t, before.Groups[0].Models, 1)
	assert.Equal(t, BillingReconciliationModePerSecond, before.Groups[0].Models[0].BillingMode)
	token := 11
	filter := BillingStatementLogFilter{UserId: 7, Start: 1000, End: 1200, TokenId: &token, BillingMode: "per_second"}
	detailBefore, err := GetBillingStatementLogs(context.Background(), filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	var storedTasksBefore []Task
	var storedLogsBefore []Log
	require.NoError(t, db.Order("id asc").Find(&storedTasksBefore).Error)
	require.NoError(t, db.Order("id asc").Find(&storedLogsBefore).Error)

	// Use the actual startup entry; LOG_DB must not be needed to index tasks.
	LOG_DB = nil
	require.NoError(t, migrateBillingReconciliationDB())
	require.NoError(t, migrateBillingReconciliationDB())
	LOG_DB = db
	require.True(t, db.Migrator().HasIndex(&Task{}, "idx_tasks_task_user_app"))
	var storedTasksAfter []Task
	var storedLogsAfter []Log
	require.NoError(t, db.Order("id asc").Find(&storedTasksAfter).Error)
	require.NoError(t, db.Order("id asc").Find(&storedLogsAfter).Error)
	assert.Equal(t, storedTasksBefore, storedTasksAfter, "migration preserves complete task facts")
	assert.Equal(t, storedLogsBefore, storedLogsAfter, "migration preserves original logs")

	// Capture the production evidence SQL, so the plan check follows future
	// query changes rather than testing an independently handwritten query.
	type evidenceQuery struct {
		sql  string
		vars []any
	}
	var queries []evidenceQuery
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:refund_index_plan", func(tx *gorm.DB) {
		if tx.Statement.Table == "tasks" {
			queries = append(queries, evidenceQuery{tx.Statement.SQL.String(), append([]any(nil), tx.Statement.Vars...)})
		}
	}))
	after, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 11, "", "")
	require.NoError(t, db.Callback().Query().Remove("test:refund_index_plan"))
	require.NoError(t, err)
	assert.Equal(t, before, after)
	require.Len(t, queries, 2, "refund IDs remain bounded to 200 per query")
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		for _, q := range queries {
			var plan []struct{ Detail string }
			require.NoError(t, db.Raw("EXPLAIN QUERY PLAN "+q.sql, q.vars...).Scan(&plan).Error)
			require.Len(t, plan, 1)
			assert.Contains(t, plan[0].Detail, "idx_tasks_task_user_app")
			assert.Contains(t, plan[0].Detail, "task_id=? AND user_id=? AND app_id=?")
		}
	}
	detailAfter, err := GetBillingStatementLogs(context.Background(), filter, 1, 20, common.RoleCommonUser)
	require.NoError(t, err)
	assert.Equal(t, detailBefore, detailAfter)
	assert.EqualValues(t, 201, detailAfter.Total)
	exported, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 11, "", "",
		BillingStatementReadPolicy{BeforeBatch: func(context.Context) error { return nil }})
	require.NoError(t, err)
	assert.Equal(t, after, exported, "interactive and export statements agree")

	// The index must allow duplicate identities; evidence ambiguity continues
	// to fail closed instead of changing writes or picking an arbitrary task.
	duplicate := tasks[0]
	duplicate.ID = 0
	require.NoError(t, db.Create(&duplicate).Error)
	ambiguous, err := GetBillingCustomerStatement(context.Background(), 7, 1000, 1200, "api_key", 11, "", "unknown")
	require.NoError(t, err)
	assert.EqualValues(t, 1, ambiguous.DataQuality.UnknownBillingModeRequests)
	assert.EqualValues(t, -100, ambiguous.Summary.NetQuota)

	rollback := errors.New("rollback fixture")
	require.ErrorIs(t, db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Task{}).Where("id = ?", tasks[1].ID).Update("quota", 999).Error; err != nil {
			return err
		}
		return rollback
	}), rollback)
	var persisted Task
	require.NoError(t, db.First(&persisted, tasks[1].ID).Error)
	assert.Equal(t, 100, persisted.Quota)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = GetBillingCustomerStatement(ctx, 7, 1000, 1200, "api_key", 11, "", "")
	require.ErrorIs(t, err, context.Canceled)
}

func TestBillingStatementTaskIndexConcurrentReadAndWrite(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	if common.UsingMainDatabase(common.DatabaseTypeSQLite) {
		var err error
		db, err = gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tasks.db")+"?_pragma=busy_timeout(30000)&_pragma=journal_mode(WAL)&_txlock=immediate"), &gorm.Config{})
		require.NoError(t, err)
		conn, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, conn.Close()) })
	}
	require.NoError(t, db.AutoMigrate(&Task{}))
	require.NoError(t, migrateBillingStatementTaskIndex(db))
	first := Task{UserId: 7, AppID: 11, TaskID: "same-id", Quota: 100}
	require.NoError(t, db.Create(&first).Error)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := db.WithContext(ctx).Model(&Task{}).Select("quota").
		Where("user_id = ? AND app_id = ? AND task_id IN ?", 7, 11, []string{"same-id"}).Rows()
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next())
	var quota int
	require.NoError(t, rows.Scan(&quota))
	assert.Equal(t, 100, quota)
	// Keep the reader open while another connection commits the same index
	// key. Context bounds a locking regression without timing assertions.
	second := first
	second.ID, second.Quota = 0, 200
	require.NoError(t, db.WithContext(ctx).Create(&second).Error)
	assert.False(t, rows.Next(), "open cursor retains its original snapshot")
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	var next []int
	require.NoError(t, db.Model(&Task{}).Where("user_id = ? AND app_id = ? AND task_id = ?", 7, 11, "same-id").Order("id asc").Pluck("quota", &next).Error)
	assert.Equal(t, []int{100, 200}, next)
}

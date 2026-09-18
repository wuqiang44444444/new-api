package model

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var bsvTestCounter int64

// setupBillingStatementVersionTestDB 建立独立内存库并接线版本固化迁移；
// 每个测试一个独立库，避免共享缓存命名冲突。
func setupBillingStatementVersionTestDB(t *testing.T) *gorm.DB {
	previousOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap = previousOptions })
	previousDB, previousLogDB := DB, LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	n := atomic.AddInt64(&bsvTestCounter, 1)
	dsn := fmt.Sprintf("file:bsv_%d_%s?mode=memory&cache=shared", n, strings.ReplaceAll(t.Name(), "/", "_"))
	var db *gorm.DB
	var err error
	switch dialect := os.Getenv("TEST_BILLING_STATEMENT_DIALECT"); dialect {
	case "":
		db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{})
		require.NoError(t, err)
	case "mysql", "postgres":
		db = setupBillingStatementServerDB(t, dialect)
		databaseType := common.DatabaseTypeMySQL
		if dialect == "postgres" {
			databaseType = common.DatabaseTypePostgreSQL
		}
		common.SetDatabaseTypes(databaseType, databaseType)
	default:
		t.Fatalf("unsupported test database: %s", dialect)
	}
	initCol()
	DB, LOG_DB = db, db
	require.NoError(t, migrateBillingStatementVersionDB())
	require.NoError(t, InitBillingStatementSourceTracking())
	require.NoError(t, db.AutoMigrate(&Option{}, &Log{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		DB, LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		initCol()
		_ = sqlDB.Close()
	})
	return db
}

func enableVersionSwitch(t *testing.T) {
	t.Helper()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMap[BillingStatementVersionEnabledKey] = "true"
	// 确认发布以主库 option 行为最终权威，测试库需同步写入。
	require.NoError(t, DB.Save(&Option{Key: BillingStatementVersionEnabledKey, Value: "true"}).Error)
	t.Cleanup(func() {
		delete(common.OptionMap, BillingStatementVersionEnabledKey)
		DB.Where(&Option{Key: BillingStatementVersionEnabledKey}).Delete(&Option{})
	})
}

// markDraftPendingWithVector 将草稿置为待确认并冻结来源向量，模拟生成完成后的状态。
func markDraftPendingWithVector(t *testing.T, ctx context.Context, userId int, periodStart int64, draft *BillingStatementVersion) {
	t.Helper()
	projection, err := FreezeBillingStatementProjection(BillingCustomerStatement{UserId: userId, Dimension: "api_key", DataQuality: &BillingReconciliationDataQuality{Status: "complete"}})
	require.NoError(t, err)
	require.NoError(t, DB.Model(draft).Updates(map[string]interface{}{"summary_projection": projection, "integrity": `{"rows":0,"gross":0,"refund":0}`}).Error)
	for _, role := range []string{"summary_csv", "detail_csv"} {
		require.NoError(t, DB.Create(&BillingStatementArtifact{VersionId: draft.ID, Role: role, ObjectKey: fmt.Sprintf("fixture/%d/%s", draft.ID, role), Sha256: "fixture"}).Error)
	}
	vec, err := snapshotBillingStatementDependencyVector(ctx, userId, periodStart)
	require.NoError(t, err)
	stamp, err := billingSourceReviewStamp(ctx, DB, userId)
	require.NoError(t, err)
	var revisions map[string]int64
	require.NoError(t, common.UnmarshalJsonStr(string(vec.Dependencies), &revisions))
	revisions[vec.ScopeCustomerMonth] = vec.RevCustomerMonth
	detail, err := common.Marshal(billingSourceAttestation{Version: BillingSourceDecisionVersion, UserID: userId, Start: periodStart, Stamp: stamp, Revisions: revisions, BillingSourceVerification: BillingSourceVerification{Fingerprint: "fixture"}})
	require.NoError(t, err)
	require.NoError(t, upsertBillingStatementRetentionTx(DB, userId, periodStart, BillingStatementRetentionIntact, string(detail), nowSeconds()))
	require.NoError(t, DB.Model(&BillingStatementVersion{}).Where("id = ?", draft.ID).Updates(map[string]any{
		"status":                   BillingStatementVersionPending,
		"period_end_exclusive":     periodStart + 31*24*60*60,
		"rev_scope_customer_month": vec.ScopeCustomerMonth,
		"rev_customer_month":       vec.RevCustomerMonth,
		"dependencies":             vec.Dependencies,
		"rev_scope_maintenance":    vec.ScopeMaintenance,
		"rev_maintenance":          vec.RevMaintenance,
	}).Error)
}

func TestAcquireDraftEnforcesSingleActiveDraft(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	ctx := context.Background()

	_, d1, err := AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	require.NotNil(t, d1)
	assert.Equal(t, BillingStatementVersionQueued, d1.Status)

	// 同客户月份第二个活动草稿必须被拒绝。
	_, _, err = AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)

	// 不同客户或不同月份不受影响。
	_, d2, err := AcquireBillingStatementDraft(ctx, 1002, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	assert.NotEqual(t, d1.ID, d2.ID)
}

func TestConfirmAssignsVersionAndSwitchesCurrentPointer(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()

	_, draft, err := AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 1001, 1756608000, draft)

	v, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "idem-1", "qa", "", 7)
	require.NoError(t, err)
	assert.True(t, committed)
	require.NotNil(t, v.VersionNumber)
	assert.Equal(t, 1, *v.VersionNumber)

	m, err := GetBillingStatementMonthByUserPeriod(ctx, 1001, 1756608000)
	require.NoError(t, err)
	require.NotNil(t, m.CurrentVersionId)
	assert.Equal(t, v.ID, *m.CurrentVersionId)
	assert.Nil(t, m.ActiveDraftId)

	// 幂等重试：同一草稿再确认返回同一结果，不重复发布、版本号不变。
	v2, committed2, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "idem-1", "qa", "", 7)
	require.NoError(t, err)
	assert.False(t, committed2)
	assert.Equal(t, v.ID, v2.ID)
	assert.Equal(t, *v.VersionNumber, *v2.VersionNumber)
}

func TestConfirmBlockedWhenSourceChanged(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()

	_, draft, err := AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 1001, 1756608000, draft)

	// 来源在待确认后变化：递增客户月修订号。
	var scope string
	require.NoError(t, DB.Model(&BillingStatementVersion{}).Select("rev_scope_customer_month").Where("id = ?", draft.ID).Scan(&scope).Error)
	require.NoError(t, DB.Transaction(func(tx *gorm.DB) error {
		return IncrementBillingStatementRevisionTx(tx, scope)
	}))

	_, _, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "idem-1", "qa", "", 7)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
}

func TestConfirmBlockedWhenMaintenanceGenerationChanged(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()

	_, draft, err := AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 1001, 1756608000, draft)

	// 进入维护再退出：代际变化，旧草稿必须失效。
	maintenance, err := BeginBillingStatementMaintenance(ctx, "repair", 7)
	require.NoError(t, err)
	_, err = EndBillingStatementMaintenance(ctx, maintenance.Generation, "verified fixture", 7)
	require.NoError(t, err)
	require.NoError(t, SetBillingStatementVersionEnabled(ctx, true, 7))

	_, _, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "idem-1", "qa", "", 7)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
}

func TestConfirmBlockedWhenSwitchOff(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	ctx := context.Background()
	// 开关默认关（未设置 OptionMap）。
	_, draft, err := AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 1001, 1756608000, draft)

	_, _, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "idem-1", "qa", "", 7)
	assert.ErrorIs(t, err, ErrBillingStatementVersionDisabled)
}

func TestAbandonDraftReleasesActivePointer(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	ctx := context.Background()

	_, draft, err := AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	require.NoError(t, AbandonBillingStatementDraft(ctx, draft.DraftPublicId, 7, "test"))

	m, err := GetBillingStatementMonthByUserPeriod(ctx, 1001, 1756608000)
	require.NoError(t, err)
	assert.Nil(t, m.ActiveDraftId)

	// 放弃后可重新占用。
	_, d2, err := AcquireBillingStatementDraft(ctx, 1001, 1756608000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	assert.NotEqual(t, draft.ID, d2.ID)
}

func TestTopologyFlag(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	assert.True(t, BillingStatementVersionTopologyOK()) // 同库测试内 LOG_DB == DB
}

func TestCreateLogBumpsRevisionWhenEnabled(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	require.NoError(t, DB.AutoMigrate(&Log{}))

	// 同库拓扑下写消费日志：客户月修订号应递增。
	log := &Log{UserId: 2001, Type: LogTypeConsume, Quota: 100, CreatedAt: 1756608000 + 1000, TokenId: 5}
	require.NoError(t, createLog(log))
	scope := fmt.Sprintf("cm:%d:%d", 2001, naturalMonthStartAt(log.CreatedAt))
	var rev BillingStatementRevision
	require.NoError(t, DB.Where("scope = ?", scope).First(&rev).Error)
	assert.Equal(t, int64(1), rev.Revision)

	// 预扣关联退款额外递增证据作用域。
	preauth := &Log{UserId: 2001, Type: LogTypeRefund, Quota: -100, CreatedAt: 1756608000 + 2000, TokenId: 5,
		Other: `{"admin_info":{"original_preauth_log_id":123}}`}
	require.NoError(t, createLog(preauth))
	var evRev BillingStatementRevision
	require.NoError(t, DB.Where("scope = ?", fmt.Sprintf("ev:%d:0", 2001)).First(&evRev).Error)
	assert.Equal(t, int64(2), evRev.Revision)
}

func TestCreateLogTracksRevisionWhenDisabled(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	// 开关默认关。
	require.NoError(t, DB.AutoMigrate(&Log{}))
	log := &Log{UserId: 2002, Type: LogTypeConsume, Quota: 50, CreatedAt: 1756608000 + 1000}
	require.NoError(t, createLog(log))
	var count int64
	require.NoError(t, DB.Model(&BillingStatementRevision{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestDeleteOldLogMarksRetentionAndBumpsRevision(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()
	require.NoError(t, DB.AutoMigrate(&Log{}))

	// 造一条上月结算日志。
	old := &Log{UserId: 3001, Type: LogTypeConsume, Quota: 100, CreatedAt: 1756608000 + 1000, TokenId: 9}
	require.NoError(t, createLog(old))
	monthStart := naturalMonthStartAt(old.CreatedAt)

	// 删除该日志（target 覆盖其 created_at）。
	deleted, err := DeleteOldLogBatch(ctx, old.CreatedAt+5000, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// 保留状态应为 partial，客户月修订号已递增。
	ret, err := GetBillingStatementRetention(ctx, 3001, monthStart)
	require.NoError(t, err)
	require.NotNil(t, ret)
	assert.Equal(t, BillingStatementRetentionPartial, ret.Status)

	var rev BillingStatementRevision
	require.NoError(t, DB.Where("scope = ?", fmt.Sprintf("cm:%d:%d", 3001, monthStart)).First(&rev).Error)
	assert.GreaterOrEqual(t, rev.Revision, int64(1))
}

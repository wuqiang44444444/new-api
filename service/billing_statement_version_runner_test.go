package service

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

var bsvSvcCounter int64

func setupVersionServiceTestDB(t *testing.T) *gorm.DB {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	n := atomic.AddInt64(&bsvSvcCounter, 1)
	dsn := fmt.Sprintf("file:bsvs_%d_%s?mode=memory&cache=shared", n, strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB, model.LOG_DB = db, db
	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}, &model.Channel{}, &model.Task{}))
	require.NoError(t, db.AutoMigrate(
		&model.BillingStatementMonth{}, &model.BillingStatementVersion{}, &model.BillingStatementVersionLine{},
		&model.BillingStatementArtifact{}, &model.BillingStatementAudit{}, &model.BillingStatementRevision{},
		&model.BillingStatementRetention{}, &model.BillingStatementMaintenance{},
	))
	require.NoError(t, db.Create(&model.BillingStatementMaintenance{ID: 1, Enabled: false, Generation: 1, UpdatedAt: time.Now().Unix()}).Error)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		_ = sqlDB.Close()
	})
	return db
}

// TestRunGenerationPersistsDetailAndPending 验证生成主流程：读聚合、收明细、完整性校验、
// 产物 staged 上传、进待确认。
func TestRunGenerationPersistsDetailAndPending(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	t.Setenv("TMPDIR", t.TempDir())
	store := &stubExportStore{uploaded: map[string][]byte{}}
	customerExportStoreOverride = store
	t.Cleanup(func() { customerExportStoreOverride = nil })
	ctx := context.Background()
	if common.OptionMap == nil {
		common.OptionMap = map[string]string{}
	}
	common.OptionMap[model.BillingStatementVersionEnabledKey] = "true"
	t.Cleanup(func() { delete(common.OptionMap, model.BillingStatementVersionEnabledKey) })

	// 造一个已结束自然月的两条结算日志。
	periodStart := time.Date(2025, 9, 1, 0, 0, 0, 0, time.FixedZone("Asia/Shanghai", 8*3600)).Unix() // 2025-09-01 00:00:00 +0800
	require.NoError(t, db.Create(&model.User{Id: 4001, Username: "version-fixture", Status: common.UserStatusEnabled, Role: common.RoleCommonUser}).Error)
	consume := &model.Log{UserId: 4001, Username: "version-fixture", Type: model.LogTypeConsume, Quota: 500, CreatedAt: periodStart + 1000, TokenId: 7, TokenName: "key-7", ModelName: "m1"}
	refund := &model.Log{UserId: 4001, Username: "version-fixture", Type: model.LogTypeRefund, Quota: 100, CreatedAt: periodStart + 2000, TokenId: 7, TokenName: "key-7", ModelName: "m1"}
	require.NoError(t, createLogForTest(db, consume))
	require.NoError(t, createLogForTest(db, refund))

	require.NoError(t, db.AutoMigrate(&model.CustomerExportJob{}, &model.CustomerExportSlot{}, &model.SystemTask{}, &model.Option{}))
	for id := int64(1); id <= model.CustomerExportSlotCount; id++ {
		require.NoError(t, db.Create(&model.CustomerExportSlot{ID: id}).Error)
	}
	require.NoError(t, db.Create(&model.User{Id: 9, Username: "version-admin", AffCode: "admin9", Status: common.UserStatusEnabled, Role: common.RoleAdminUser}).Error)
	verifySourceForGenerationTest(t, 4001, periodStart)
	draft, err := SubmitBillingStatementVersionJob(ctx, 9, 4001, periodStart, periodStart+30*86400)
	require.NoError(t, err)
	// 第二个客户在同一个调度唤醒期间入队，必须保留自己的可执行任务。
	require.NoError(t, db.Create(&model.User{Id: 4002, Username: "version-second", AffCode: "second", Status: common.UserStatusEnabled}).Error)
	verifySourceForGenerationTest(t, 4002, periodStart)
	second, err := SubmitBillingStatementVersionJob(ctx, 9, 4002, periodStart, periodStart+30*86400)
	require.NoError(t, err)
	job, err := model.ClaimNextQueuedCustomerExportJob("version-test", time.Now().Unix()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, job)
	runCustomerExportJob(ctx, job, "version-test", &customerExportPressureTracker{})
	// 版本应进入待确认，明细净额 = 汇总净额。
	v, err := model.GetBillingStatementVersionByDraftPublicId(ctx, draft.DraftPublicId)
	require.NoError(t, err)
	assert.Equal(t, model.BillingStatementVersionPending, v.Status)

	var lineCount int64
	require.NoError(t, db.Model(&model.BillingStatementVersionLine{}).Where("version_id = ?", v.ID).Count(&lineCount).Error)
	assert.EqualValues(t, 2, lineCount)

	var net int64
	require.NoError(t, db.Model(&model.BillingStatementVersionLine{}).Where("version_id = ?", v.ID).Select("COALESCE(SUM(CASE WHEN log_type = ? THEN quota WHEN log_type = ? THEN -quota ELSE 0 END),0)", model.LogTypeConsume, model.LogTypeRefund).Scan(&net).Error)
	assert.EqualValues(t, 400, net) // 500 consume - 100 refund

	// 产物：汇总 + 明细 CSV 已登记并上传；对象键位于 billing/statements 命名空间。
	artifacts, err := model.ListBillingStatementArtifacts(ctx, v.ID)
	require.NoError(t, err)
	require.Len(t, artifacts, 2)
	for _, artifact := range artifacts {
		assert.False(t, artifact.Staged)
		assert.Equal(t, "draft", artifact.RetentionClass)
		assert.Contains(t, artifact.ObjectKey, model.BillingStatementNamespace+"/"+v.DraftPublicId+"/")
		_, ok := store.uploaded[artifact.ObjectKey]
		assert.True(t, ok, "artifact uploaded: %s", artifact.ObjectKey)
	}
	job, err = model.ClaimNextQueuedCustomerExportJob("version-second", time.Now().Unix()+120, 1800)
	require.NoError(t, err)
	require.NotNil(t, job)
	runCustomerExportJob(ctx, job, "version-second", &customerExportPressureTracker{})
	second, err = model.GetBillingStatementVersion(ctx, second.ID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingStatementVersionPending, second.Status)

}

// createLogForTest 直接写日志（绕过开关接线，专注生成链路）。
func createLogForTest(db *gorm.DB, log *model.Log) error {
	return db.Create(log).Error
}

func verifySourceForGenerationTest(t *testing.T, user int, start int64) {
	t.Helper()
	ctx := context.Background()
	end := start + 30*86400 - 1
	report, err := model.GetBillingSourceReview(ctx, user, start, end)
	require.NoError(t, err)
	for _, issue := range report.Issues {
		require.NoError(t, model.RecordBillingSourceReviewDecision(ctx, user, start, end, model.BillingSourceReviewDecision{Fingerprint: report.Fingerprint, IssueID: issue.ID, Note: "fixture explanation", Decision: model.BillingSourceKeep, Reason: "accept_missing_details", RequestID: "fixture-request-" + strings.ReplaceAll(issue.ID, ":", "-")}, 1))
	}
	require.NoError(t, model.RecordBillingSourceVerification(ctx, user, start, end, model.BillingSourceVerification{Fingerprint: report.Fingerprint, BackupEvidence: "fixture backup", RetentionEvidence: "fixture retention"}, 1))
}

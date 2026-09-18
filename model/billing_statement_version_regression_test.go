package model

import (
	"context"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillingStatementMonthConcurrentAcquire(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			_, _, err := AcquireBillingStatementDraft(context.Background(), 11, 1756656000, "Asia/Shanghai", 7)
			results <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	require.Equal(t, 1, success)
	var count int64
	require.NoError(t, db.Model(&BillingStatementMonth{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, db.Model(&BillingStatementVersion{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	assert.Error(t, db.Create(&BillingStatementMonth{UserId: 11, PeriodStart: 1756656000}).Error)
}

func TestBillingStatementCleanupKeepsLimitAndAllCustomers(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Log{}))
	timestamp := int64(1756656100)
	month := naturalMonthStartAt(timestamp)
	for _, user := range []int{11, 12} {
		require.NoError(t, db.Create(&Log{UserId: user, Type: LogTypeConsume, Quota: 10, CreatedAt: timestamp}).Error)
	}
	ctx := context.Background()
	deleted, err := DeleteOldLogBatch(ctx, timestamp+1, 1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	var count int64
	require.NoError(t, db.Model(&Log{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	deleted, err = DeleteOldLogBatch(ctx, timestamp+1, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, deleted)
	for _, user := range []int{11, 12} {
		r, err := GetBillingStatementRetention(ctx, user, month)
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.Equal(t, BillingStatementRetentionPartial, r.Status)
	}
	// 同一批命中多个客户不能按月份覆盖。
	for _, user := range []int{13, 14} {
		require.NoError(t, db.Create(&Log{UserId: user, Type: LogTypeConsume, Quota: 10, CreatedAt: timestamp}).Error)
	}
	_, err = DeleteOldLogBatch(ctx, timestamp+1, 10)
	require.NoError(t, err)
	for _, user := range []int{13, 14} {
		r, err := GetBillingStatementRetention(ctx, user, month)
		require.NoError(t, err)
		require.NotNil(t, r)
		assert.Equal(t, BillingStatementRetentionPartial, r.Status)
	}
}

func TestBillingStatementSourceChangesBlockOldDraft(t *testing.T) {
	for _, change := range []string{"off_create", "direct_batch", "log_update", "task_update", "task_delete"} {
		t.Run(change, func(t *testing.T) {
			db := setupBillingStatementVersionTestDB(t)
			enableVersionSwitch(t)
			require.NoError(t, db.AutoMigrate(&Log{}, &Task{}))
			ctx := context.Background()
			month := int64(1756656000)
			log := Log{UserId: 11, Type: LogTypeConsume, Quota: 10, CreatedAt: month + 100}
			require.NoError(t, db.Create(&log).Error)
			task := Task{UserId: 11, AppID: 1, TaskID: "source-task"}
			require.NoError(t, db.Create(&task).Error)
			require.NoError(t, db.Create(&Log{UserId: 11, TokenId: 1, Type: LogTypeRefund, CreatedAt: month + 101, Quota: 10, Other: `{"task_id":"source-task","model_price":0}`}).Error)
			_, draft, err := AcquireBillingStatementDraft(ctx, 11, month, "Asia/Shanghai", 7)
			require.NoError(t, err)
			markDraftPendingWithVector(t, ctx, 11, month, draft)
			switch change {
			case "off_create":
				require.NoError(t, SetBillingStatementVersionEnabled(ctx, false, 7))
				require.NoError(t, db.Create(&Log{UserId: 11, Type: LogTypeConsume, Quota: 5, CreatedAt: month + 200}).Error)
				require.NoError(t, SetBillingStatementVersionEnabled(ctx, true, 7))
			case "direct_batch":
				require.NoError(t, db.Create(&[]Log{{UserId: 11, Type: LogTypeRefund, Quota: 1, CreatedAt: month + 300}}).Error)
			case "log_update":
				require.NoError(t, db.Model(&log).Update("quota", 20).Error)
			case "task_update":
				require.NoError(t, db.Model(&task).Update("properties", Properties{OriginModelName: "corrected"}).Error)
			case "task_delete":
				require.NoError(t, db.Delete(&task).Error)
			}
			_, _, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "test", "", "", 7)
			assert.ErrorIs(t, err, ErrBillingStatementVersionConflict)
		})
	}
}

func TestBillingStatementConfirmationRequiresDraftQualityAndArtifacts(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()
	month := int64(1756656000)
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, month, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, month, draft)
	projection, err := FreezeBillingStatementProjection(BillingCustomerStatement{DataQuality: &BillingReconciliationDataQuality{Status: "partial", UnavailableRequests: 1}})
	require.NoError(t, err)
	require.NoError(t, db.Model(draft).Update("summary_projection", projection).Error)
	for _, ack := range []string{"", `{"acknowledged":true,"draft_public_id":"another"}`} {
		_, _, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "test", ack, "", 7)
		require.Error(t, err)
	}
	ack := fmt.Sprintf(`{"acknowledged":true,"draft_public_id":%q}`, draft.DraftPublicId)
	require.NoError(t, db.Model(&BillingStatementArtifact{}).Where("version_id = ?", draft.ID).Update("staged", true).Error)
	_, _, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "test", ack, "", 7)
	require.Error(t, err)
	require.NoError(t, db.Model(&BillingStatementArtifact{}).Where("version_id = ?", draft.ID).Update("staged", false).Error)
	_, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "test", ack, "", 7)
	require.NoError(t, err)
	assert.True(t, committed)
	var artifacts []BillingStatementArtifact
	require.NoError(t, db.Where("version_id = ?", draft.ID).Find(&artifacts).Error)
	for _, a := range artifacts {
		assert.Equal(t, "confirmed", a.RetentionClass)
	}
}

func TestBillingStatementInvalidDraftCannotConfirm(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()
	month := int64(1756656000)
	_, d, err := AcquireBillingStatementDraft(ctx, 11, month, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, month, d)
	require.NoError(t, MarkBillingStatementDraftInvalid(ctx, d.DraftPublicId, "source changed"))
	_, _, err = ConfirmBillingStatementVersion(ctx, d.DraftPublicId, nil, "test", "", "", 7)
	require.Error(t, err)
	v, err := GetBillingStatementVersion(ctx, d.ID)
	require.NoError(t, err)
	assert.Equal(t, BillingStatementVersionInvalid, v.Status)
}

func TestBillingStatementUnknownRetentionDoesNotImplyComplete(t *testing.T) {
	setupBillingStatementVersionTestDB(t)
	assert.ErrorIs(t, VerifyBillingStatementRetention(context.Background(), 11, 1756656000), ErrBillingStatementSourceIncomplete)
	// 核验依据不可为空，不把页面勾选质量提示当作来源完整性证明。
	assert.Error(t, RecordBillingSourceVerification(context.Background(), 11, 1756656000, 1759247999, BillingSourceVerification{}, 7))
}

func TestBillingStatementFrozenListUsesVersionsBeforeSortAndTotals(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Log{}))
	ctx := context.Background()
	start := int64(1756656000)
	end := start + 30*86400 - 1
	// 历史来源已经清理：确认客户仍出现在列表，新的实时日志也不得替换其账单。
	require.NoError(t, db.Create(&Log{UserId: 11, Type: LogTypeConsume, Quota: 999999, CreatedAt: start + 10}).Error)
	require.NoError(t, db.Create(&Log{UserId: 12, Type: LogTypeConsume, Quota: 500000, CreatedAt: start + 10}).Error)
	original := int64(600000)
	discount := int64(200000)
	snapshot, err := FreezeBillingStatementProjection(BillingCustomerStatement{UserId: 11, Username: "confirmed", Summary: BillingReconciliationUsage{Requests: 2, GrossQuota: 450000, RefundQuota: 50000, NetQuota: 400000}, OriginalQuota: &original, DiscountQuota: &discount, DataQuality: &BillingReconciliationDataQuality{Status: "complete"}})
	require.NoError(t, err)
	v := BillingStatementVersion{UserId: 11, PeriodStart: start, PeriodEndExclusive: end + 1, Status: BillingStatementVersionConfirmed, SummaryProjection: snapshot, QuotaPerUnit: 100000, Currency: "USD", CurrencyRate: 1}
	item, err := frozenBillingStatementListItem(v)
	require.NoError(t, err)
	result, err := GetBillingCustomerStatementList(ctx, start, end, "", "", "net_quota", "desc", 1, 1, item)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, 11, result.Items[0].UserId)
	assert.EqualValues(t, 400000, result.Items[0].Usage.NetQuota)
	assert.EqualValues(t, 900000, result.Summary.Usage.NetQuota)
	require.NotNil(t, result.Summary.MoneyUSD)
	assert.Equal(t, "5.00000000", result.Summary.MoneyUSD.Net)
	assert.EqualValues(t, 2, result.Total)
	require.NoError(t, db.Where("user_id = ?", 11).Delete(&Log{}).Error)
	result, err = GetBillingCustomerStatementList(ctx, start, end, "confirmed", "", "net_quota", "desc", 1, 10, item)
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	assert.Equal(t, "4.00000000", result.Summary.MoneyUSD.Net)
}

func TestBillingStatementTaskPollingDoesNotInvalidateDraft(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	require.NoError(t, db.AutoMigrate(&Task{}))
	task := Task{UserId: 11, TaskID: "polling"}
	require.NoError(t, db.Create(&task).Error)
	ctx := context.Background()
	month := int64(1756656000)
	_, d, err := AcquireBillingStatementDraft(ctx, 11, month, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, month, d)
	require.NoError(t, db.Model(&task).Updates(map[string]interface{}{"progress": "50%", "updated_at": 123, "status": "IN_PROGRESS"}).Error)
	// Native CAS writes the entire struct, including unchanged evidence fields.
	require.NoError(t, db.First(&task, task.ID).Error)
	from := task.Status
	task.Progress = "75%"
	won, err := task.UpdateWithStatus(from)
	require.NoError(t, err)
	require.True(t, won)
	_, committed, err := ConfirmBillingStatementVersion(ctx, d.DraftPublicId, nil, "polling", "", "", 7)
	require.NoError(t, err)
	assert.True(t, committed)
}

func TestBillingStatementRevisionFailurePreservesSourceAndBlocksConfirmation(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&Log{}))
	enableVersionSwitch(t)
	ctx := context.Background()
	month := naturalMonthStartAt(1756656100)
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, month, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, month, draft)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:reject_revision", func(tx *gorm.DB) {
		if tx.Statement.Table == "billing_statement_revisions" {
			tx.AddError(errors.New("revision rejected"))
		}
	}))
	err = db.Create(&Log{UserId: 11, Type: LogTypeConsume, Quota: 17, CreatedAt: 1756656100}).Error
	require.NoError(t, err)
	_, _, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "test", "", "", 7)
	assert.ErrorIs(t, err, ErrBillingStatementSourceIncomplete)
	var count int64
	require.NoError(t, db.Model(&Log{}).Count(&count).Error)
	assert.EqualValues(t, 1, count)
}

func TestBillingStatementCurrentMonthConsumptionDoesNotBlockConfirmation(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	require.NoError(t, db.AutoMigrate(&Log{}, &Task{}))
	ctx := context.Background()
	month := int64(1756656000)
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, month, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, month, draft)
	require.NoError(t, db.Create(&Log{UserId: 11, TokenId: 7, Type: LogTypeConsume, Quota: 10, CreatedAt: month + 32*86400}).Error)
	require.NoError(t, db.Create(&Task{UserId: 11, AppID: 7, TaskID: "new-unrelated-task"}).Error)
	_, committed, err := ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "current-month", "", "", 7)
	require.NoError(t, err)
	assert.True(t, committed)
}

func TestBillingStatementCleanupAfterExecutionRecordExpires(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	require.NoError(t, db.AutoMigrate(&CustomerExportJob{}))
	ctx := context.Background()
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, 1756656000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	require.NoError(t, db.Model(draft).Updates(map[string]any{"status": BillingStatementVersionFailed, "source_job_id": "expired-execution"}).Error)
	marked, err := MarkBillingStatementDraftCleaning(ctx, draft.DraftPublicId, 7)
	require.NoError(t, err)
	require.True(t, marked)
	// 重试中断的清理仍可到达删除事实步骤。
	marked, err = MarkBillingStatementDraftCleaning(ctx, draft.DraftPublicId, 7)
	require.NoError(t, err)
	require.True(t, marked)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := DeleteBillingStatementDraftFactsTx(ctx, tx, draft.DraftPublicId)
		return err
	}))
	_, next, err := AcquireBillingStatementDraft(ctx, 11, 1756656000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	assert.NotEqual(t, draft.DraftPublicId, next.DraftPublicId)
}

func TestBillingStatementKnownLogWriteFailureBlocksConfirmation(t *testing.T) {
	db := setupBillingStatementVersionTestDB(t)
	enableVersionSwitch(t)
	ctx := context.Background()
	month := naturalMonthStartAt(1756656100)
	_, draft, err := AcquireBillingStatementDraft(ctx, 11, month, "Asia/Shanghai", 7)
	require.NoError(t, err)
	markDraftPendingWithVector(t, ctx, 11, month, draft)
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("test:reject_source", func(tx *gorm.DB) {
		if tx.Statement.Table == "logs" {
			tx.AddError(errors.New("source write rejected"))
		}
	}))
	require.ErrorContains(t, createLog(&Log{UserId: 11, Type: LogTypeConsume, Quota: 17, CreatedAt: month + 100}), "source write rejected")
	_, _, err = ConfirmBillingStatementVersion(ctx, draft.DraftPublicId, nil, "confirm", "", "", 7)
	assert.ErrorIs(t, err, ErrBillingStatementVersionConflict, "failure generation invalidates the pending draft")
	retention, err := GetBillingStatementRetention(ctx, 11, month)
	require.NoError(t, err)
	require.NotNil(t, retention)
	assert.Equal(t, BillingStatementRetentionPartial, retention.Status)
	var count int64
	require.NoError(t, db.Model(&Log{}).Count(&count).Error)
	assert.Zero(t, count, "failure fence must not replay a settled consumption")
}

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type interruptedStatementCleanupStore struct {
	*stubExportStore
	failDelete bool
}

func (s *interruptedStatementCleanupStore) ExportDeleteObject(ctx context.Context, key string) error {
	if s.failDelete {
		return errors.New("temporary storage failure")
	}
	return s.stubExportStore.ExportDeleteObject(ctx, key)
}

func TestBillingStatementCleanupResumesAfterStorageFailureAndExpiredExecution(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.CustomerExportJob{}))
	ctx := context.Background()
	_, draft, err := model.AcquireBillingStatementDraft(ctx, 11, 1756656000, "Asia/Shanghai", 7)
	require.NoError(t, err)
	require.NoError(t, db.Model(draft).Updates(map[string]any{"status": model.BillingStatementVersionFailed, "source_job_id": "old-execution"}).Error)
	require.NoError(t, db.Create(&model.CustomerExportJob{JobID: "old-execution", JobType: model.CustomerExportJobTypeStatementVersion,
		Status: model.CustomerExportJobStatusFailed, FinishedAt: time.Now().Add(-8 * 24 * time.Hour).Unix()}).Error)
	_, err = model.CleanupCustomerExportJobRecords(time.Now().Unix(), int64(customerExportRecordRetention.Seconds()))
	require.NoError(t, err)
	_, err = model.GetCustomerExportJob("old-execution")
	require.ErrorIs(t, err, model.ErrCustomerExportNotFound)
	key := "billing/statements/failed/detail.csv"
	require.NoError(t, db.Create(&model.BillingStatementArtifact{VersionId: draft.ID, ObjectKey: key, StoreIdentity: "stub", Staged: true}).Error)
	require.NoError(t, db.Create(&model.BillingStatementVersionLine{VersionId: draft.ID, Quota: 17}).Error)
	store := &interruptedStatementCleanupStore{stubExportStore: &stubExportStore{uploaded: map[string][]byte{key: []byte("partial")}}, failDelete: true}
	previous := customerExportStoreOverride
	customerExportStoreOverride = store
	t.Cleanup(func() { customerExportStoreOverride = previous })

	require.ErrorContains(t, CleanupBillingStatementDraft(ctx, draft.DraftPublicId, 7), "temporary storage failure")
	saved, err := model.GetBillingStatementVersion(ctx, draft.ID)
	require.NoError(t, err)
	assert.Equal(t, model.BillingStatementVersionCleaning, saved.Status)
	artifacts, err := model.ListBillingStatementArtifacts(ctx, draft.ID)
	require.NoError(t, err)
	require.Len(t, artifacts, 1, "object reference must survive failed deletion")
	var count int64
	require.NoError(t, db.Model(&model.BillingStatementVersionLine{}).Where("version_id = ?", draft.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)

	store.failDelete = false
	require.NoError(t, CleanupBillingStatementDraft(ctx, draft.DraftPublicId, 7))
	assert.NotContains(t, store.uploaded, key)
	require.NoError(t, db.Model(&model.BillingStatementVersionLine{}).Where("version_id = ?", draft.ID).Count(&count).Error)
	assert.Zero(t, count)
	artifacts, err = model.ListBillingStatementArtifacts(ctx, draft.ID)
	require.NoError(t, err)
	assert.Empty(t, artifacts)
	_, _, err = model.AcquireBillingStatementDraft(ctx, 11, 1756656000, "Asia/Shanghai", 7)
	require.NoError(t, err, "cleanup releases the month for a new draft")
}

package service

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type failingConfirmationStore struct {
	*stubExportStore
	fail bool
}

func (s *failingConfirmationStore) ExportPutFile(ctx context.Context, key, mime, path string, size int64) error {
	if s.fail {
		return errors.New("temporary storage failure")
	}
	return s.stubExportStore.ExportPutFile(ctx, key, mime, path, size)
}
func TestBillingStatementConfirmationNoteRecoversOnDownload(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	t.Setenv("TMPDIR", t.TempDir())
	store := &failingConfirmationStore{stubExportStore: &stubExportStore{uploaded: map[string][]byte{}}, fail: true}
	customerExportStoreOverride = store
	t.Cleanup(func() { customerExportStoreOverride = nil })
	n := 2
	v := &model.BillingStatementVersion{DraftPublicId: "note-retry", Status: model.BillingStatementVersionConfirmed, VersionNumber: &n, ConfirmedAt: time.Now().Unix(), PublicReason: "Verified correction"}
	require.NoError(t, db.Create(v).Error)
	require.Error(t, FinalizeBillingStatementVersionArtifacts(context.Background(), v))
	var artifact model.BillingStatementArtifact
	require.NoError(t, db.Where("version_id = ?", v.ID).First(&artifact).Error)
	assert.True(t, artifact.Staged)
	store.fail = false
	_, _, _, err := PresignBillingStatementVersionArtifact(context.Background(), v, "confirmation_note")
	require.NoError(t, err)
	require.NoError(t, FinalizeBillingStatementVersionArtifacts(context.Background(), v))
	var count int64
	require.NoError(t, db.Model(&model.BillingStatementArtifact{}).Where("version_id = ?", v.ID).Count(&count).Error)
	assert.EqualValues(t, 1, count)
	require.NoError(t, db.First(&artifact, artifact.ID).Error)
	assert.False(t, artifact.Staged)
	assert.Contains(t, string(store.uploaded[artifact.ObjectKey]), "Verified correction")
	assert.Equal(t, model.BillingStatementVersionConfirmed, v.Status)
}

package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"testing"
)

func TestStatementArtifactChecksMissingCorruptAndTruncatedObjects(t *testing.T) {
	data := []byte("frozen,statement\n")
	manifest := model.BillingStatementArtifact{ObjectKey: "frozen", StoreIdentity: "stub", SizeBytes: int64(len(data)), Sha256: fmt.Sprintf("%x", sha256.Sum256(data))}
	for _, tc := range []struct {
		name  string
		data  []byte
		valid bool
	}{
		{"intact", data, true}, {"missing", nil, false}, {"same size changed", []byte("broken,statement\n"), false}, {"truncated", data[:4], false}, {"extended", append(append([]byte{}, data...), 'x'), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &stubExportStore{uploaded: map[string][]byte{}}
			if tc.data != nil {
				store.uploaded["frozen"] = tc.data
			}
			err := verifyBillingStatementArtifact(context.Background(), store, manifest)
			if tc.valid {
				require.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrBillingStatementArtifactUnavailable)
			}
		})
	}
}

func TestConfirmedStatementMissingObjectCannotProduceDownloadURL(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	customerExportStoreOverride = &stubExportStore{uploaded: map[string][]byte{}}
	t.Cleanup(func() { customerExportStoreOverride = nil })
	n := 1
	v := model.BillingStatementVersion{Status: model.BillingStatementVersionConfirmed, VersionNumber: &n, DraftPublicId: "missing-file"}
	require.NoError(t, db.Create(&v).Error)
	require.NoError(t, db.Create(&model.BillingStatementArtifact{VersionId: v.ID, Role: "summary_csv", ObjectKey: "missing", StoreIdentity: "stub", Sha256: "known"}).Error)
	url, _, _, err := PresignBillingStatementVersionArtifact(context.Background(), &v, "")
	assert.ErrorIs(t, err, ErrBillingStatementArtifactUnavailable)
	assert.Empty(t, url)
	var current model.BillingStatementVersion
	require.NoError(t, db.First(&current, v.ID).Error)
	assert.Equal(t, model.BillingStatementVersionConfirmed, current.Status)
}

type revokingStatementStore struct {
	*stubExportStore
	revoke func() error
}

func (s *revokingStatementStore) ExportOpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	if err := s.revoke(); err != nil {
		return nil, err
	}
	return s.stubExportStore.ExportOpenObject(ctx, key)
}

func TestStatementConfirmationRechecksRoleAfterReadingArtifacts(t *testing.T) {
	db := setupVersionServiceTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 9, Username: "admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}).Error)
	v := model.BillingStatementVersion{Status: model.BillingStatementVersionPending, DraftPublicId: "role-revoked"}
	require.NoError(t, db.Create(&v).Error)
	content := []byte("frozen")
	require.NoError(t, db.Create(&model.BillingStatementArtifact{VersionId: v.ID, ObjectKey: "summary", StoreIdentity: "stub", Sha256: fmt.Sprintf("%x", sha256.Sum256(content)), SizeBytes: int64(len(content))}).Error)
	previous := customerExportStoreOverride
	customerExportStoreOverride = &revokingStatementStore{stubExportStore: &stubExportStore{uploaded: map[string][]byte{"summary": content}}, revoke: func() error {
		return db.Model(&model.User{}).Where("id = ?", 9).Update("role", common.RoleCommonUser).Error
	}}
	t.Cleanup(func() { customerExportStoreOverride = previous })
	_, committed, err := ConfirmBillingStatementVersion(context.Background(), v.DraftPublicId, nil, "key", "", "", 9)
	assert.ErrorIs(t, err, ErrBillingStatementVersionForbidden)
	assert.False(t, committed)
	require.NoError(t, db.First(&v, v.ID).Error)
	assert.Equal(t, model.BillingStatementVersionPending, v.Status)
}

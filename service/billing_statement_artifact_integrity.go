package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var ErrBillingStatementVersionForbidden = errors.New("billing statement confirmation requires an enabled administrator")

var ErrBillingStatementArtifactUnavailable = errors.New("billing statement file is unavailable; contact the administrator")

// Verify bytes outside the publication transaction. Publication still rechecks
// the draft status, source revisions and manifest under its existing locks.
func ConfirmBillingStatementVersion(ctx context.Context, draftID string, base *int64, key, acknowledged, reason string, actor int) (*model.BillingStatementVersion, bool, error) {
	v, err := model.GetBillingStatementVersionByDraftPublicId(ctx, draftID)
	if err != nil {
		return nil, false, err
	}
	if v.Status == model.BillingStatementVersionPending {
		store, err := currentExportObjectStore()
		if err != nil {
			return nil, false, err
		}
		artifacts, err := model.ListBillingStatementArtifacts(ctx, v.ID)
		if err != nil {
			return nil, false, err
		}
		verifyCtx, cancel := context.WithTimeout(ctx, customerExportJobBudget)
		defer cancel()
		for _, artifact := range artifacts {
			if err := verifyBillingStatementArtifact(verifyCtx, store, artifact); err != nil {
				return nil, false, err
			}
		}
	}
	// Object verification may take time. Re-read authority at publication rather
	// than retaining the role observed by HTTP middleware before storage I/O.
	var authorized int64
	if err := model.DB.WithContext(ctx).Model(&model.User{}).
		Where("id = ? AND role >= ? AND status = ?", actor, common.RoleAdminUser, common.UserStatusEnabled).
		Count(&authorized).Error; err != nil {
		return nil, false, err
	}
	if authorized != 1 {
		return nil, false, ErrBillingStatementVersionForbidden
	}
	return model.ConfirmBillingStatementVersion(ctx, draftID, base, key, acknowledged, reason, actor)
}

func verifyBillingStatementArtifact(ctx context.Context, store exportObjectStore, artifact model.BillingStatementArtifact) error {
	if artifact.Staged || artifact.StoreIdentity != store.ExportIdentity() || artifact.SizeBytes < 0 || artifact.SizeBytes == math.MaxInt64 || len(artifact.Sha256) != sha256.Size*2 {
		return ErrBillingStatementArtifactUnavailable
	}
	body, err := store.ExportOpenObject(ctx, artifact.ObjectKey)
	if err != nil {
		common.SysError("billing statement artifact cannot be read")
		return ErrBillingStatementArtifactUnavailable
	}
	defer body.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, io.LimitReader(body, artifact.SizeBytes+1))
	if err != nil || size != artifact.SizeBytes || fmt.Sprintf("%x", digest.Sum(nil)) != artifact.Sha256 {
		common.SysError("billing statement artifact integrity check failed")
		return ErrBillingStatementArtifactUnavailable
	}
	return nil
}

func (s *s3ArtifactStore) ExportOpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.objectURL(key), nil)
	if err != nil {
		return nil, ErrBillingStatementArtifactUnavailable
	}
	req.Host = req.URL.Host
	SigV4SignRequest(req, s.credentials, s.region(), hexSHA256(nil), time.Now())
	client := &http.Client{Timeout: customerExportJobBudget, CheckRedirect: rejectObjectStorageRedirect}
	response, err := client.Do(req)
	if err != nil {
		return nil, ErrBillingStatementArtifactUnavailable
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, ErrBillingStatementArtifactUnavailable
	}
	return response.Body, nil
}

func (s *azureBlobArtifactStore) ExportOpenObject(ctx context.Context, key string) (io.ReadCloser, error) {
	response, err := s.blobClient(key).DownloadStream(ctx, nil)
	if err != nil {
		return nil, ErrBillingStatementArtifactUnavailable
	}
	return response.Body, nil
}

func (s *s3ArtifactStore) ExportObjectExists(ctx context.Context, key string) (bool, error) {
	return s.HeadObject(ctx, key)
}
func (s *azureBlobArtifactStore) ExportObjectExists(ctx context.Context, key string) (bool, error) {
	return s.headImageObject(ctx, key)
}

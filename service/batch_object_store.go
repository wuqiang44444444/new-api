package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Batch file bytes live only in the private object store that the task
// artifact store already configures. The store keeps its task-facing
// interface; batch files additionally require raw put/get of bounded JSONL
// objects, which both shipped backends provide.

var ErrBatchObjectStoreUnavailable = errors.New("object storage is not configured; batch file upload is unavailable")

var ErrBatchObjectNotFound = errors.New("batch file content is no longer available")

// batchObjectPut is satisfied by the shipped stores' internal put path.
type batchObjectPut interface {
	BatchPutObject(ctx context.Context, objectKey, mimeType string, content io.Reader) (int64, error)
}

// BatchObjectGetter streams one stored object back. Methods are attached to
// the shipped stores in this package so the task artifact store files stay
// untouched.
type BatchObjectGetter interface {
	BatchGetObject(ctx context.Context, objectKey string, w io.Writer) (int64, error)
}

// batchObjectKey returns the private object reference for one batch file.
// The object name is random and independent of the database row id so bytes
// are stored before any row exists; a failed upload leaves an orphan object
// for infra lifecycle handling, never a dangling database reference.
func batchObjectKey(userId int, objectName string) string {
	return fmt.Sprintf("batch/%d/%s.jsonl", userId, objectName)
}

// PutBatchObjectFinal stores complete batch bytes.
func PutBatchObjectFinal(ctx context.Context, objectKey string, data []byte) error {
	_, err := PutBatchObject(ctx, objectKey, bytes.NewReader(data))
	return err
}

// PutBatchObject stores batch file bytes and returns the written size.
func PutBatchObject(ctx context.Context, objectKey string, content io.Reader) (int64, error) {
	store := GetTaskArtifactStore()
	if !store.Enabled() {
		return 0, ErrBatchObjectStoreUnavailable
	}
	putter, ok := store.(batchObjectPut)
	if !ok {
		return 0, ErrBatchObjectStoreUnavailable
	}
	return putter.BatchPutObject(ctx, objectKey, "application/jsonl", content)
}

// GetBatchObject streams one stored batch object.
func GetBatchObject(ctx context.Context, objectKey string, w io.Writer) (int64, error) {
	store := GetTaskArtifactStore()
	if !store.Enabled() {
		return 0, ErrBatchObjectStoreUnavailable
	}
	getter, ok := store.(BatchObjectGetter)
	if !ok {
		return 0, ErrBatchObjectStoreUnavailable
	}
	return getter.BatchGetObject(ctx, objectKey, w)
}

// BatchObjectExists verifies a stored object is still readable; infra cleanup
// must surface as an explicit file-unavailable error, never as a silent loss.
func BatchObjectExists(ctx context.Context, objectKey string) (bool, error) {
	store := GetTaskArtifactStore()
	if !store.Enabled() {
		return false, ErrBatchObjectStoreUnavailable
	}
	head, ok := store.(interface {
		HeadObject(ctx context.Context, objectKey string) (bool, error)
	})
	if !ok {
		return false, ErrBatchObjectStoreUnavailable
	}
	return head.HeadObject(ctx, objectKey)
}

// BatchGetObject for the S3 backend streams the object body through a
// short-lived presigned request that is never exposed to the caller.
func (s *s3ArtifactStore) BatchGetObject(ctx context.Context, objectKey string, w io.Writer) (int64, error) {
	requestURL, err := SigV4PresignURL(http.MethodGet, s.objectURL(objectKey), s.credentials, s.region(), time.Minute, time.Now())
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{Timeout: 120 * time.Second, CheckRedirect: rejectObjectStorageRedirect}
	resp, err := client.Do(req)
	if err != nil {
		return 0, errors.New("object storage GET request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return 0, ErrBatchObjectNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("object store GET returned HTTP %d", resp.StatusCode)
	}
	return io.Copy(w, resp.Body)
}

// BatchGetObject for the Azure Blob backend streams the object body.
func (s *azureBlobArtifactStore) BatchGetObject(ctx context.Context, objectKey string, w io.Writer) (int64, error) {
	download, err := s.blockBlobClient(s.prefixedKey(objectKey)).DownloadStream(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer download.Body.Close()
	return io.Copy(w, download.Body)
}

func (s *s3ArtifactStore) BatchPutObject(ctx context.Context, key, mime string, r io.Reader) (int64, error) {
	return s.putObject(ctx, key, mime, r)
}
func (s *azureBlobArtifactStore) BatchPutObject(ctx context.Context, key, mime string, r io.Reader) (int64, error) {
	counter := &countingReader{r: r}
	_, err := s.blockBlobClient(s.prefixedKey(key)).UploadStream(ctx, counter, nil)
	if err != nil {
		return 0, errors.New("batch object upload failed")
	}
	return counter.n, nil
}

package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
)

// Export artifacts live in a dedicated private namespace. The capability
// interface reuses the existing object-storage runtime and backends; it adds
// only the bounded file upload, short-lived signed download and deletion that
// exports need. No fake Task, no second credential/config/store client.

var ErrCustomerExportStorageUnavailable = errors.New("customer export object storage is unavailable")

// Object keys are recorded in the database; cleanup only touches recorded
// keys and never other Task artifacts.
const CustomerExportObjectNamespace = "exports/jobs"

type exportObjectStore interface {
	// ExportPutFile is a bounded file-backed upload: the body streams from the
	// file handle instead of the existing io.ReadAll whole-payload path.
	ExportIdentity() string
	ExportOpenObject(ctx context.Context, objectKey string) (io.ReadCloser, error)
	ExportObjectExists(ctx context.Context, objectKey string) (bool, error)
	ExportPutFile(ctx context.Context, objectKey string, mimeType string, path string, size int64) error
	ExportPresignURL(objectKey string, ttl time.Duration, filename ...string) (url string, expiresAt int64, err error)
	ExportDeleteObject(ctx context.Context, objectKey string) error
}

// customerExportStoreOverride 只用于测试注入内存存储实现。
var customerExportStoreOverride exportObjectStore

func currentExportObjectStore() (exportObjectStore, error) {
	if customerExportStoreOverride != nil {
		return customerExportStoreOverride, nil
	}
	store := GetTaskArtifactStore()
	switch impl := store.(type) {
	case *s3ArtifactStore:
		return impl, nil
	case *azureBlobArtifactStore:
		return impl, nil
	default:
		return nil, ErrCustomerExportStorageUnavailable
	}
}

// PresignCustomerExportURL 为已通过鉴权的下载签发短时对象存储 GET 地址。
func PresignCustomerExportURL(objectKey string, ttl time.Duration, identity string) (string, int64, error) {
	store, err := currentExportObjectStore()
	if err != nil {
		return "", 0, err
	}
	if identity == "" || store.ExportIdentity() != identity {
		return "", 0, ErrCustomerExportStorageUnavailable
	}
	return store.ExportPresignURL(objectKey, ttl)
}

// ─── S3 兼容后端 ────────────────────────────────────────────────────────────

func (s *s3ArtifactStore) ExportPutFile(ctx context.Context, objectKey string, mimeType string, path string, size int64) error {
	return s.putObjectFile(ctx, objectKey, mimeType, path, size, "customer_export_store")
}

func (s *s3ArtifactStore) ExportPresignURL(objectKey string, ttl time.Duration, filename ...string) (string, int64, error) {
	if len(filename) == 0 {
		return s.PresignObjectURLWithExpiry(objectKey, ttl)
	}
	target, err := url.Parse(s.objectURL(objectKey))
	if err != nil {
		return "", 0, err
	}
	query := target.Query()
	query.Set("response-content-disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename[0]}))
	target.RawQuery = query.Encode()
	issuedAt := time.Now()
	signed, err := SigV4PresignURL(http.MethodGet, target.String(), s.credentials, s.region(), ttl, issuedAt)
	return signed, issuedAt.Add(ttl).Unix(), err
}

func (s *s3ArtifactStore) ExportDeleteObject(ctx context.Context, objectKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.objectURL(objectKey), nil)
	if err != nil {
		return err
	}
	req.Host = req.URL.Host
	SigV4SignRequest(req, s.credentials, s.region(), hexSHA256(nil), time.Now())
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: rejectObjectStorageRedirect}
	resp, err := client.Do(req)
	ObserveImageHTTPExchange(ctx, req, resp, err, "customer_export_delete")
	if err != nil {
		return errors.New("delete export artifact failed")
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("object store rejected export delete with HTTP %d", resp.StatusCode)
	}
	return nil
}

// ─── Azure Blob 原生后端 ───────────────────────────────────────────────────

func (s *azureBlobArtifactStore) ExportPutFile(ctx context.Context, objectKey string, mimeType string, path string, size int64) error {
	return s.putObjectFile(ctx, objectKey, mimeType, path, size, "customer_export_store")
}

func (s *azureBlobArtifactStore) ExportPresignURL(objectKey string, ttl time.Duration, filename ...string) (string, int64, error) {
	if len(filename) > 0 {
		now := time.Now().UTC()
		protocol := sas.ProtocolHTTPS
		if strings.HasPrefix(strings.ToLower(s.config.Endpoint), "http://") {
			protocol = sas.ProtocolHTTPSandHTTP
		}
		query, err := sas.BlobSignatureValues{Protocol: protocol, StartTime: now.Add(-azureSASClockSkewBackoff), ExpiryTime: now.Add(ttl), Permissions: (&sas.BlobPermissions{Read: true}).String(), ContainerName: s.container, BlobName: s.prefixedKey(objectKey), ContentDisposition: mime.FormatMediaType("attachment", map[string]string{"filename": filename[0]})}.SignWithSharedKey(s.credential)
		if err != nil {
			return "", 0, err
		}
		return s.blobClient(objectKey).URL() + "?" + query.Encode(), now.Add(ttl).Unix(), nil
	}
	issuedAt := time.Now()
	url, err := s.presignObjectURL(objectKey, ttl)
	if err != nil {
		return "", 0, err
	}
	return url, issuedAt.Add(ttl).Unix(), nil
}

func (s *azureBlobArtifactStore) ExportDeleteObject(ctx context.Context, objectKey string) error {
	deleteCtx, cancel := context.WithTimeout(ctx, azureHeadTimeout)
	defer cancel()
	if _, err := s.blockBlobClient(objectKey).Delete(deleteCtx, nil); err != nil {
		if bloberror.HasCode(err, bloberror.BlobNotFound) {
			return nil
		}
		RecordImageDeliveryError(ctx, "customer_export_delete", err)
		return azureStoreError("delete export artifact failed", err)
	}
	return nil
}

var (
	_                   = common.GetTimestamp
	_ exportObjectStore = (*s3ArtifactStore)(nil)
	_ exportObjectStore = (*azureBlobArtifactStore)(nil)
)

// Identity excludes credentials: rotation preserves access; changing the
// endpoint/bucket/prefix must never delete or sign an unrelated object.
func (s *s3ArtifactStore) ExportIdentity() string {
	return fmt.Sprintf("s3:%x", sha256.Sum256([]byte(s.config.S3Endpoint+"\x00"+s.config.S3Bucket+"\x00"+s.config.S3Prefix)))
}
func (s *azureBlobArtifactStore) ExportIdentity() string {
	return fmt.Sprintf("azure:%x", sha256.Sum256([]byte(s.config.Endpoint+"\x00"+s.config.AccountName+"\x00"+s.container)))
}

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/sas"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// Azure Blob 原生存储实现（本地扩展）。Shared Key 请求签名与 service SAS
// 由官方 azblob SDK 承担，业务层不感知签名差异；对象命名、前缀拼接与
// HEAD 404 → deleted、其它错误 → unavailable 的语义与 S3 实现保持一致。

const (
	azureBlobBackendLabel    = "azure_blob"
	azurePutTimeout          = 2 * time.Minute
	azureHeadTimeout         = 15 * time.Second
	azureSASClockSkewBackoff = 2 * time.Minute
)

type azureBlobArtifactStore struct {
	config     system_setting.ObjectStorageConfig
	container  string
	credential *azblob.SharedKeyCredential
	client     *azblob.Client
}

// NewAzureBlobArtifactStore builds the Azure Blob store from a validated
// configuration snapshot and plaintext account key. It performs no network access.
func NewAzureBlobArtifactStore(config system_setting.ObjectStorageConfig, accountKey string) (TaskArtifactStore, error) {
	if config.Backend != system_setting.ObjectStorageBackendAzureBlob {
		return nil, errors.New("azure blob artifact store requires azure_blob backend")
	}
	if err := system_setting.ValidateObjectStorageConfig(config, accountKey); err != nil {
		return nil, err
	}
	credential, err := azblob.NewSharedKeyCredential(config.AccountName, accountKey)
	if err != nil {
		return nil, err
	}
	client, err := azblob.NewClientWithSharedKeyCredential(config.Endpoint, credential, &azblob.ClientOptions{
		ClientOptions: azcore.ClientOptions{Transport: &http.Client{CheckRedirect: rejectObjectStorageRedirect}},
	})
	if err != nil {
		return nil, err
	}
	return &azureBlobArtifactStore{
		config:     config,
		container:  config.Bucket,
		credential: credential,
		client:     client,
	}, nil
}

func (s *azureBlobArtifactStore) Enabled() bool { return true }

func (s *azureBlobArtifactStore) prefixedKey(objectKey string) string {
	prefix := strings.Trim(s.config.Prefix, "/")
	if prefix == "" {
		return objectKey
	}
	return prefix + "/" + objectKey
}

func (s *azureBlobArtifactStore) blobClient(objectKey string) *blob.Client {
	container := s.client.ServiceClient().NewContainerClient(s.container)
	return container.NewBlobClient(s.prefixedKey(objectKey))
}

func (s *azureBlobArtifactStore) blockBlobClient(objectKey string) *blockblob.Client {
	container := s.client.ServiceClient().NewContainerClient(s.container)
	return container.NewBlockBlobClient(s.prefixedKey(objectKey))
}

func (s *azureBlobArtifactStore) Resolve(task *model.Task, artifactKey string) (*StoredArtifactRef, error) {
	if task == nil || strings.TrimSpace(artifactKey) == "" {
		return nil, errors.New("task and artifact key are required")
	}
	// 只有存在持久化登记事实的对象才返回引用：无事实时返回 nil，让既有
	// 产物消费者继续走原 Provider 下载路径。
	artifact := model.FindImageTaskArtifact(task, strings.TrimSpace(artifactKey))
	if artifact == nil {
		return nil, nil
	}
	return &StoredArtifactRef{
		Backend:   azureBlobBackendLabel,
		Bucket:    s.container,
		ObjectKey: artifact.ObjectKey,
		MimeType:  artifact.MimeType,
		Size:      artifact.Size,
	}, nil
}

func (s *azureBlobArtifactStore) Persist(ctx context.Context, task *model.Task, artifact hosttypes.TaskArtifact, content io.Reader) (*StoredArtifactRef, error) {
	if task == nil || strings.TrimSpace(artifact.Key) == "" || content == nil {
		return nil, errors.New("task, artifact key, and content are required")
	}
	data, err := io.ReadAll(content)
	if err != nil {
		return nil, fmt.Errorf("read artifact content failed: %w", err)
	}
	objectKey := model.ImageTaskArtifactObjectKey(task.TaskID, artifact.Key)
	size, err := s.putObject(ctx, objectKey, artifact.MimeType, data)
	if err != nil {
		return nil, err
	}
	return &StoredArtifactRef{
		Backend:   azureBlobBackendLabel,
		Bucket:    s.container,
		ObjectKey: objectKey,
		MimeType:  artifact.MimeType,
		Size:      size,
	}, nil
}

func (s *azureBlobArtifactStore) Serve(c *gin.Context, task *model.Task, ref *StoredArtifactRef) error {
	if c == nil || ref == nil {
		return errors.New("request context and artifact reference are required")
	}
	url, err := s.presignObjectURL(ref.ObjectKey, s.presignTTLForRead())
	if err != nil {
		return err
	}
	// 客户端直连对象存储（§5 投递选择）；不落任何签名 URL 到日志。
	c.Redirect(http.StatusFound, url)
	return nil
}

func (s *azureBlobArtifactStore) presignTTLForRead() time.Duration {
	return time.Duration(system_setting.DefaultTaskArtifactStorePresignTTLSeconds) * time.Second
}

// presignObjectURL issues a short-lived read-only service SAS for one object.
func (s *azureBlobArtifactStore) presignObjectURL(objectKey string, ttl time.Duration) (string, error) {
	blobName := s.prefixedKey(objectKey)
	// The SDK formats SAS timestamps with a literal Z; supply UTC explicitly.
	now := time.Now().UTC()
	permissions := sas.BlobPermissions{Read: true}
	protocol := sas.ProtocolHTTPS
	if strings.HasPrefix(strings.ToLower(s.config.Endpoint), "http://") {
		protocol = sas.ProtocolHTTPSandHTTP
	}
	query, err := sas.BlobSignatureValues{
		Protocol:      protocol,
		StartTime:     now.Add(-azureSASClockSkewBackoff),
		ExpiryTime:    now.Add(ttl),
		Permissions:   permissions.String(),
		ContainerName: s.container,
		BlobName:      blobName,
	}.SignWithSharedKey(s.credential)
	if err != nil {
		return "", err
	}
	return s.blobClient(objectKey).URL() + "?" + query.Encode(), nil
}

// putObject uploads the whole payload as one block blob.
func (s *azureBlobArtifactStore) putObject(ctx context.Context, objectKey, mimeType string, data []byte) (int64, error) {
	httpHeaders := &blob.HTTPHeaders{}
	if mimeType != "" {
		httpHeaders.BlobContentType = &mimeType
	}
	putCtx, cancel := context.WithTimeout(ctx, azurePutTimeout)
	defer cancel()
	_, err := s.blockBlobClient(objectKey).UploadBuffer(putCtx, data, &blockblob.UploadBufferOptions{
		HTTPHeaders: httpHeaders,
	})
	if err != nil {
		return 0, azureStoreError("upload artifact to object store failed", err)
	}
	return int64(len(data)), nil
}

// headObject 轻量探测对象存在性；BlobNotFound 与其它错误被区分（§5 已删除/
// 暂不可用）。容器缺失等错误保持为错误，不误判删除。
func (s *azureBlobArtifactStore) headObject(ctx context.Context, objectKey string) (bool, error) {
	headCtx, cancel := context.WithTimeout(ctx, azureHeadTimeout)
	defer cancel()
	_, err := s.blobClient(objectKey).GetProperties(headCtx, nil)
	if err == nil {
		return true, nil
	}
	if bloberror.HasCode(err, bloberror.BlobNotFound) {
		return false, nil
	}
	return false, azureStoreError("object store HEAD failed", err)
}

func (s *azureBlobArtifactStore) fetchObjectBytes(ctx context.Context, objectKey string) ([]byte, error) {
	download, err := s.blobClient(objectKey).DownloadStream(ctx, nil)
	if err != nil {
		return nil, azureStoreError("download image object failed", err)
	}
	defer func() { _ = download.Body.Close() }()
	return readImageObjectBytes(download.Body)
}

// ─── imageObjectStore 能力实现 ────────────────────────────────────────────

func (s *azureBlobArtifactStore) putImageObject(ctx context.Context, objectKey, mimeType string, data []byte) (*ImageObjectRef, error) {
	size, err := s.putObject(ctx, objectKey, mimeType, data)
	if err != nil {
		return nil, err
	}
	return &ImageObjectRef{ObjectKey: objectKey, MimeType: mimeType, Size: size}, nil
}

func (s *azureBlobArtifactStore) presignImageObjectURL(objectKey string) (string, int64, error) {
	url, err := s.presignObjectURL(objectKey, imageResultURLTTLSeconds*time.Second)
	if err != nil {
		return "", 0, err
	}
	return url, time.Now().Add(imageResultURLTTLSeconds * time.Second).Unix(), nil
}

func (s *azureBlobArtifactStore) headImageObject(ctx context.Context, objectKey string) (bool, error) {
	return s.headObject(ctx, objectKey)
}

func (s *azureBlobArtifactStore) fetchImageObjectBytes(ctx context.Context, objectKey string) ([]byte, error) {
	return s.fetchObjectBytes(ctx, objectKey)
}

// azureStoreError 把 SDK 错误收敛为不含上游响应正文的脱敏描述；只有 HTTP
// 状态与已知错误码参与表达，原始正文不进入日志或响应。
func azureStoreError(action string, err error) error {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		detail := respErr.ErrorCode
		if detail == "" {
			detail = fmt.Sprintf("HTTP %d", respErr.StatusCode)
		}
		return fmt.Errorf("%s: %s", action, detail)
	}
	return fmt.Errorf("%s", action)
}

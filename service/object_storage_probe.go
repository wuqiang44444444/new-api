package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// 对象存储连通性测试（本地扩展）。在目标容器下创建随机测试对象，验证
// PUT（仅对象不存在时创建）、HEAD、鉴权 GET 内容一致、短期签名 GET、删除
// 与删除后 404 确认；只清理本次创建的对象，不操作客户文件。清理失败单独
// 提示，不伪装全部成功；不回显签名 URL、Authorization 或上游响应正文。

const (
	objectStorageProbeNamespace   = "object-storage-connectivity-check"
	objectStorageProbeTimeout     = 45 * time.Second
	objectStorageProbeMaxAttempts = 3
)

var errProbeObjectExists = errors.New("probe object already exists")

// ObjectStorageTestStep 是一步测试的脱敏结果。
type ObjectStorageTestStep struct {
	Name    string `json:"name"`
	Success bool   `json:"success"`
	Detail  string `json:"detail,omitempty"`
}

// ObjectStorageTestResult 汇总一次连通性测试。
type ObjectStorageTestResult struct {
	Success       bool                    `json:"success"`
	Message       string                  `json:"message,omitempty"`
	Steps         []ObjectStorageTestStep `json:"steps,omitempty"`
	CleanupFailed bool                    `json:"cleanup_failed"`
}

func (r *ObjectStorageTestResult) addStep(name string, err error) bool {
	step := ObjectStorageTestStep{Name: name, Success: err == nil}
	if err != nil {
		step.Detail = err.Error()
	}
	r.Steps = append(r.Steps, step)
	return err == nil
}

// objectStorageProbeAdapter 由各 backend 的存储实例提供测试所需的原子操作。
type objectStorageProbeAdapter interface {
	probePutIfAbsent(ctx context.Context, objectKey, mimeType string, data []byte) error
	probeHead(ctx context.Context, objectKey string, size int64) error
	probeGet(ctx context.Context, objectKey string, expected []byte) error
	probePresignedGet(ctx context.Context, objectKey string, ttl time.Duration, expected []byte) error
	probeDelete(ctx context.Context, objectKey string) error
	probeHeadMissing(ctx context.Context, objectKey string) error
}

// RunObjectStorageConnectionTest 以给定表单配置（含明文凭据）执行连通性
// 测试；不读取也不替换在线配置。结果只包含脱敏步骤描述。
func RunObjectStorageConnectionTest(config system_setting.ObjectStorageConfig, credential string) *ObjectStorageTestResult {
	result := &ObjectStorageTestResult{}

	if err := system_setting.ValidateObjectStorageConfig(config, credential); err != nil {
		result.addStep("validate", err)
		result.Message = "configuration is invalid"
		return result
	}
	result.addStep("validate", nil)

	var store TaskArtifactStore
	var err error
	switch config.Backend {
	case system_setting.ObjectStorageBackendAzureBlob:
		store, err = NewAzureBlobArtifactStore(config, credential)
	case system_setting.ObjectStorageBackendS3:
		store, err = NewS3ArtifactStore(legacyS3Config(config, credential))
	default:
		err = fmt.Errorf("unsupported backend %q", config.Backend)
	}
	if err != nil {
		result.addStep("build", err)
		result.Message = "failed to build storage client"
		return result
	}
	result.addStep("build", nil)

	adapter, ok := store.(objectStorageProbeAdapter)
	if !ok {
		result.addStep("build", errors.New("storage implementation does not support connectivity probe"))
		result.Message = "storage implementation does not support connectivity probe"
		return result
	}

	return runObjectStorageProbe(adapter, result)
}

func runObjectStorageProbe(adapter objectStorageProbeAdapter, result *ObjectStorageTestResult) *ObjectStorageTestResult {

	ctx, cancel := context.WithTimeout(context.Background(), objectStorageProbeTimeout)
	defer cancel()

	payload := []byte("new-api object storage connectivity check")
	objectKey := ""
	uploaded := false
	defer func() {
		if !uploaded {
			return
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if delErr := adapter.probeDelete(cleanupCtx, objectKey); delErr != nil {
			result.CleanupFailed = true
			result.Success = false
			result.Message = "test object cleanup failed"
			result.Steps = append(result.Steps, ObjectStorageTestStep{
				Name: "cleanup", Success: false, Detail: "delete test object failed",
			})
			return
		}
		if missingErr := adapter.probeHeadMissing(cleanupCtx, objectKey); missingErr != nil {
			result.CleanupFailed = true
			result.Success = false
			result.Message = "test object cleanup failed"
			result.Steps = append(result.Steps, ObjectStorageTestStep{
				Name: "cleanup", Success: false, Detail: "test object still readable after delete",
			})
		}
	}()

	var putErr error
	for attempt := 0; attempt < objectStorageProbeMaxAttempts; attempt++ {
		objectKey, putErr = buildObjectStorageProbeKey()
		if putErr != nil {
			result.addStep("upload", putErr)
			result.Message = "failed to allocate test object key"
			return result
		}
		putErr = adapter.probePutIfAbsent(ctx, objectKey, "text/plain", payload)
		if putErr == nil {
			break
		}
		if !errors.Is(putErr, errProbeObjectExists) {
			break
		}
	}
	if !result.addStep("upload", putErr) {
		result.Message = "upload test object failed"
		return result
	}
	uploaded = true

	if !result.addStep("head", adapter.probeHead(ctx, objectKey, int64(len(payload)))) {
		result.Message = "HEAD check failed"
		return result
	}
	if !result.addStep("get", adapter.probeGet(ctx, objectKey, payload)) {
		result.Message = "authenticated GET content mismatch"
		return result
	}
	if !result.addStep("signed_get", adapter.probePresignedGet(ctx, objectKey, imageResultURLTTLSeconds*time.Second, payload)) {
		result.Message = "signed GET check failed"
		return result
	}
	result.Success = true
	result.Message = "connectivity test passed"
	return result
}

func buildObjectStorageProbeKey() (string, error) {
	random, err := common.GenerateRandomCharsKey(16)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s.txt", objectStorageProbeNamespace, random), nil
}

// probeGetViaPresignedURL 以无 Authorization 头的普通 GET 校验签名 URL 可读
// 且内容一致；不跟随带凭据的重定向。
func probeGetViaPresignedURL(ctx context.Context, url string, expected []byte) error {
	content, err := fetchImageObjectViaPresignedURL(ctx, url)
	if err != nil {
		return err
	}
	if !bytes.Equal(content, expected) {
		return errors.New("content mismatch via signed URL")
	}
	return nil
}

// ─── Azure Blob probe 实现 ────────────────────────────────────────────────

func (s *azureBlobArtifactStore) probePutIfAbsent(ctx context.Context, objectKey, mimeType string, data []byte) error {
	httpHeaders := &blob.HTTPHeaders{}
	if mimeType != "" {
		httpHeaders.BlobContentType = &mimeType
	}
	_, err := s.blockBlobClient(objectKey).UploadBuffer(ctx, data, azblobUploadIfAbsentOptions(httpHeaders))
	if err != nil {
		if bloberror.HasCode(err, bloberror.BlobAlreadyExists) {
			return errProbeObjectExists
		}
		return azureStoreError("upload test object failed", err)
	}
	return nil
}

func (s *azureBlobArtifactStore) probeHead(ctx context.Context, objectKey string, size int64) error {
	props, err := s.blobClient(objectKey).GetProperties(ctx, nil)
	if err != nil {
		return azureStoreError("HEAD test object failed", err)
	}
	if props.ContentLength == nil || *props.ContentLength != size {
		return errors.New("HEAD content length mismatch")
	}
	return nil
}

func (s *azureBlobArtifactStore) probeGet(ctx context.Context, objectKey string, expected []byte) error {
	content, err := s.fetchObjectBytes(ctx, objectKey)
	if err != nil {
		return err
	}
	if !bytes.Equal(content, expected) {
		return errors.New("content mismatch")
	}
	return nil
}

func (s *azureBlobArtifactStore) probePresignedGet(ctx context.Context, objectKey string, ttl time.Duration, expected []byte) error {
	url, err := s.presignObjectURL(objectKey, ttl)
	if err != nil {
		return err
	}
	return probeGetViaPresignedURL(ctx, url, expected)
}

func (s *azureBlobArtifactStore) probeDelete(ctx context.Context, objectKey string) error {
	_, err := s.blobClient(objectKey).Delete(ctx, nil)
	if err != nil {
		return azureStoreError("delete test object failed", err)
	}
	return nil
}

func (s *azureBlobArtifactStore) probeHeadMissing(ctx context.Context, objectKey string) error {
	_, err := s.blobClient(objectKey).GetProperties(ctx, nil)
	if err == nil {
		return errors.New("test object still exists after delete")
	}
	if bloberror.HasCode(err, bloberror.BlobNotFound) {
		return nil
	}
	return azureStoreError("HEAD after delete failed", err)
}

// azblobUploadIfAbsentOptions 构造带 If-None-Match:"*" 的上传选项，确保只在
// 对象不存在时创建，避免覆盖任何既有对象。
func azblobUploadIfAbsentOptions(httpHeaders *blob.HTTPHeaders) *blockblob.UploadBufferOptions {
	etagAny := azcore.ETagAny
	return &blockblob.UploadBufferOptions{
		HTTPHeaders: httpHeaders,
		AccessConditions: &blob.AccessConditions{
			ModifiedAccessConditions: &blob.ModifiedAccessConditions{
				IfNoneMatch: &etagAny,
			},
		},
	}
}

// ─── S3 兼容 probe 实现 ───────────────────────────────────────────────────

func (s *s3ArtifactStore) probePutIfAbsent(ctx context.Context, objectKey, mimeType string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.objectURL(objectKey), nil)
	if err != nil {
		return err
	}
	req.Host = req.URL.Host
	if mimeType != "" {
		req.Header.Set("Content-Type", mimeType)
	}
	req.Header.Set("If-None-Match", "*")
	req.ContentLength = int64(len(data))
	req.Body = newBytesReader(data)
	req.GetBody = func() (io.ReadCloser, error) { return newBytesReader(data), nil }
	SigV4SignRequest(req, s.credentials, s.region(), hexSHA256(data), time.Now())
	return doObjectStoreProbeRequest(ctx, req, "upload test object failed")
}

func (s *s3ArtifactStore) probeHead(ctx context.Context, objectKey string, size int64) error {
	req, err := s.signedProbeRequest(ctx, http.MethodHead, objectKey)
	if err != nil {
		return err
	}
	resp, err := probeHTTPClient().Do(req)
	if err != nil {
		return errors.New("object storage request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode == http.StatusNotFound {
		return errors.New("test object not found after upload")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HEAD test object failed: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength != size {
		return errors.New("HEAD content length mismatch")
	}
	return nil
}

func (s *s3ArtifactStore) probeGet(ctx context.Context, objectKey string, expected []byte) error {
	req, err := s.signedProbeRequest(ctx, http.MethodGet, objectKey)
	if err != nil {
		return err
	}
	resp, err := probeHTTPClient().Do(req)
	if err != nil {
		return errors.New("object storage request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	content, err := io.ReadAll(io.LimitReader(resp.Body, fetchImageObjectMaxBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET test object failed: HTTP %d", resp.StatusCode)
	}
	if !bytes.Equal(content, expected) {
		return errors.New("content mismatch")
	}
	return nil
}

func (s *s3ArtifactStore) probePresignedGet(ctx context.Context, objectKey string, ttl time.Duration, expected []byte) error {
	url, err := SigV4PresignURL(http.MethodGet, s.objectURL(objectKey), s.credentials, s.region(), ttl, time.Now())
	if err != nil {
		return err
	}
	return probeGetViaPresignedURL(ctx, url, expected)
}

func (s *s3ArtifactStore) probeDelete(ctx context.Context, objectKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, s.objectURL(objectKey), nil)
	if err != nil {
		return err
	}
	req.Host = req.URL.Host
	SigV4SignRequest(req, s.credentials, s.region(), hexSHA256(nil), time.Now())
	return doObjectStoreProbeRequest(ctx, req, "delete test object failed")
}

func (s *s3ArtifactStore) probeHeadMissing(ctx context.Context, objectKey string) error {
	req, err := s.signedProbeRequest(ctx, http.MethodHead, objectKey)
	if err != nil {
		return err
	}
	resp, err := probeHTTPClient().Do(req)
	if err != nil {
		return errors.New("object storage request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("test object still exists after delete: HTTP %d", resp.StatusCode)
	}
	return errors.New("test object still exists after delete")
}

func (s *s3ArtifactStore) signedProbeRequest(ctx context.Context, method, objectKey string) (*http.Request, error) {
	url, err := SigV4PresignURL(method, s.objectURL(objectKey), s.credentials, s.region(), time.Minute, time.Now())
	if err != nil {
		return nil, err
	}
	return http.NewRequestWithContext(ctx, method, url, nil)
}

func probeHTTPClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second, CheckRedirect: rejectObjectStorageRedirect}
}

func doObjectStoreProbeRequest(ctx context.Context, req *http.Request, action string) error {
	client := probeHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: request failed", action)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode == http.StatusPreconditionFailed {
		return errProbeObjectExists
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: HTTP %d", action, resp.StatusCode)
	}
	return nil
}

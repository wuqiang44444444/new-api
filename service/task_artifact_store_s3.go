package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// S3 兼容对象存储实现（G9）。使用部署方提供的端点（当前为 Azure Blob 的
// S3 兼容网关），path-style 寻址，代码内 SigV4 签名；不引入新依赖。
// 该实现同时满足既有 TaskArtifactStore 消费者与图片输入/结果对象读写。

const (
	s3DefaultRegion       = "us-east-1"
	s3PutTimeout          = 2 * time.Minute
	s3PresignClockSkewLe  = 30 * time.Second
	s3ObjectKeyMaxSegment = 128
)

type s3ArtifactStore struct {
	config      system_setting.TaskArtifactStoreConfig
	credentials SigV4Credentials
	httpClient  *http.Client
}

// NewS3ArtifactStore builds the S3-compatible store from validated startup
// configuration. It performs no network access.
func NewS3ArtifactStore(config system_setting.TaskArtifactStoreConfig) (TaskArtifactStore, error) {
	if config.Mode != system_setting.TaskArtifactStoreModeS3 {
		return nil, errors.New("s3 artifact store requires s3 mode")
	}
	if err := system_setting.ValidateTaskArtifactStoreConfig(config); err != nil {
		return nil, err
	}
	return &s3ArtifactStore{
		config: config,
		credentials: SigV4Credentials{
			AccessKey: config.S3AccessKey,
			SecretKey: config.S3SecretKey,
		},
		httpClient: &http.Client{CheckRedirect: rejectObjectStorageRedirect},
	}, nil
}

func (s *s3ArtifactStore) Enabled() bool { return true }

func (s *s3ArtifactStore) region() string {
	if strings.TrimSpace(s.config.S3Region) != "" {
		return strings.TrimSpace(s.config.S3Region)
	}
	return s3DefaultRegion
}

// objectURL 组装 path-style 绝对对象地址。
func (s *s3ArtifactStore) objectURL(objectKey string) string {
	base := strings.TrimRight(s.config.S3Endpoint, "/")
	return fmt.Sprintf("%s/%s/%s", base, s.config.S3Bucket, s.prefixedKey(objectKey))
}

func (s *s3ArtifactStore) prefixedKey(objectKey string) string {
	prefix := strings.Trim(s.config.S3Prefix, "/")
	if prefix == "" {
		return objectKey
	}
	return prefix + "/" + objectKey
}

func (s *s3ArtifactStore) Resolve(task *model.Task, artifactKey string) (*StoredArtifactRef, error) {
	if task == nil || strings.TrimSpace(artifactKey) == "" {
		return nil, errors.New("task and artifact key are required")
	}
	// 只有存在持久化登记事实的对象才返回引用（评审 S5）：无事实时返回
	// nil，让既有产物消费者继续走原 Provider 下载路径，不因启用 S3 而
	// 被重定向到不存在的对象。
	artifact := model.FindImageTaskArtifact(task, strings.TrimSpace(artifactKey))
	if artifact == nil {
		return nil, nil
	}
	return &StoredArtifactRef{
		Backend:   "s3",
		Bucket:    s.config.S3Bucket,
		ObjectKey: artifact.ObjectKey,
		MimeType:  artifact.MimeType,
		Size:      artifact.Size,
	}, nil
}

func (s *s3ArtifactStore) Persist(ctx context.Context, task *model.Task, artifact hosttypes.TaskArtifact, content io.Reader) (*StoredArtifactRef, error) {
	if task == nil || strings.TrimSpace(artifact.Key) == "" || content == nil {
		return nil, errors.New("task, artifact key, and content are required")
	}
	// Persist 只在显式持久化时建立对象；引用登记由调用方事实承载。
	objectKey := model.ImageTaskArtifactObjectKey(task.TaskID, artifact.Key)
	size, err := s.putObject(ctx, objectKey, artifact.MimeType, content)
	if err != nil {
		return nil, err
	}
	return &StoredArtifactRef{
		Backend:   "s3",
		Bucket:    s.config.S3Bucket,
		ObjectKey: objectKey,
		MimeType:  artifact.MimeType,
		Size:      size,
	}, nil
}

func (s *s3ArtifactStore) Serve(c *gin.Context, task *model.Task, ref *StoredArtifactRef) error {
	if c == nil || ref == nil {
		return errors.New("request context and artifact reference are required")
	}
	url, err := s.PresignObjectURL(ref.ObjectKey, s.presignTTLForRead())
	if err != nil {
		return err
	}
	// 客户端直连对象存储（§5 投递选择）；不落任何签名 URL 到日志。
	c.Redirect(http.StatusFound, url)
	return nil
}

func (s *s3ArtifactStore) presignTTLForRead() time.Duration {
	ttl := time.Duration(s.config.S3PresignTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = time.Duration(system_setting.DefaultTaskArtifactStorePresignTTLSeconds) * time.Second
	}
	return ttl
}

// PresignObjectURL issues a short-lived presigned GET URL for one object key.
func (s *s3ArtifactStore) PresignObjectURL(objectKey string, ttl time.Duration) (string, error) {
	return SigV4PresignURL(http.MethodGet, s.objectURL(objectKey), s.credentials, s.region(), ttl, time.Now())
}

// PresignObjectURLWithExpiry returns the URL and its absolute expiry time.
func (s *s3ArtifactStore) PresignObjectURLWithExpiry(objectKey string, ttl time.Duration) (string, int64, error) {
	issuedAt := time.Now()
	url, err := SigV4PresignURL(http.MethodGet, s.objectURL(objectKey), s.credentials, s.region(), ttl, issuedAt)
	if err != nil {
		return "", 0, err
	}
	return url, issuedAt.Add(ttl).Unix(), nil
}

// putObject uploads bytes with header-style SigV4. size<0 表示流式长度未知
// （此时以 UNSIGNED-PAYLOAD 签名并以 chunked 发送）。
func (s *s3ArtifactStore) putObject(ctx context.Context, objectKey, mimeType string, content io.Reader) (int64, error) {
	requestURL := s.objectURL(objectKey)
	data, err := io.ReadAll(content)
	if err != nil {
		return 0, fmt.Errorf("read artifact content failed: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, requestURL, nil)
	if err != nil {
		return 0, err
	}
	req.Host = req.URL.Host
	if mimeType != "" {
		req.Header.Set("Content-Type", mimeType)
	}
	req.ContentLength = int64(len(data))
	body := newBytesReader(data)
	req.Body = body
	req.GetBody = func() (io.ReadCloser, error) { return newBytesReader(data), nil }
	SigV4SignRequest(req, s.credentials, s.region(), hexSHA256(data), time.Now())

	putCtx, cancel := context.WithTimeout(ctx, s3PutTimeout)
	defer cancel()
	req = req.WithContext(putCtx)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, errors.New("upload artifact to object store failed")
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("object store rejected upload with HTTP %d", resp.StatusCode)
	}
	return int64(len(data)), nil
}

// HeadObject 轻量探测对象存在性；404 与其它错误被区分（§5 已删除/暂不可用）。
func (s *s3ArtifactStore) HeadObject(ctx context.Context, objectKey string) (bool, error) {
	url, err := SigV4PresignURL(http.MethodHead, s.objectURL(objectKey), s.credentials, s.region(), time.Minute, time.Now())
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, err
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: rejectObjectStorageRedirect}
	resp, err := client.Do(req)
	if err != nil {
		return false, errors.New("object storage HEAD request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("object store HEAD returned HTTP %d", resp.StatusCode)
	}
	return true, nil
}

// ─── imageObjectStore 能力实现 ────────────────────────────────────────────

func (s *s3ArtifactStore) putImageObject(ctx context.Context, objectKey, mimeType string, data []byte) (*ImageObjectRef, error) {
	size, err := s.putObject(ctx, objectKey, mimeType, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &ImageObjectRef{ObjectKey: objectKey, MimeType: mimeType, Size: size}, nil
}

func (s *s3ArtifactStore) presignImageObjectURL(objectKey string) (string, int64, error) {
	return s.PresignObjectURLWithExpiry(objectKey, imageResultURLTTLSeconds*time.Second)
}

func (s *s3ArtifactStore) headImageObject(ctx context.Context, objectKey string) (bool, error) {
	return s.HeadObject(ctx, objectKey)
}

func (s *s3ArtifactStore) fetchImageObjectBytes(ctx context.Context, objectKey string) ([]byte, error) {
	url, _, err := s.presignImageObjectURL(objectKey)
	if err != nil {
		return nil, err
	}
	return fetchImageObjectViaPresignedURL(ctx, url)
}

func newBytesReader(data []byte) io.ReadCloser {
	return io.NopCloser(bytes.NewReader(data))
}

// BuildImageTaskObjectKey 生成图片任务对象的确定性键（评审 S6：无随机段，
// 崩溃后可按 taskID+序号补登记/续传）。taskID 形如 task_xxxx。
func BuildImageTaskObjectKey(taskID, leaf string) (string, error) {
	taskID = strings.TrimSpace(taskID)
	leaf = strings.TrimSpace(leaf)
	if taskID == "" || leaf == "" || len(taskID) > s3ObjectKeyMaxSegment || len(leaf) > s3ObjectKeyMaxSegment {
		return "", errors.New("object namespace or leaf is empty or too long")
	}
	if strings.ContainsAny(taskID, "/\\") || strings.ContainsAny(leaf, "/\\") ||
		strings.Contains(taskID, "..") || strings.Contains(leaf, "..") {
		return "", errors.New("object key contains unsafe segments")
	}
	return fmt.Sprintf("images/tasks/%s/%s", taskID, leaf), nil
}

// BuildImageObjectKey derives a safe object key under the image namespace.
func BuildImageObjectKey(namespace, leaf string) (string, error) {
	namespace = strings.TrimSpace(namespace)
	leaf = strings.TrimSpace(leaf)
	if namespace == "" || leaf == "" || len(namespace) > s3ObjectKeyMaxSegment || len(leaf) > s3ObjectKeyMaxSegment {
		return "", errors.New("object namespace or leaf is empty or too long")
	}
	if strings.Contains(namespace, "..") || strings.Contains(leaf, "..") ||
		strings.Contains(namespace, "\\") || strings.Contains(leaf, "\\") ||
		strings.HasPrefix(namespace, "/") || strings.HasPrefix(leaf, "/") {
		return "", errors.New("object key contains unsafe segments")
	}
	random, err := common.GenerateRandomCharsKey(16)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("images/%s/%s-%s", namespace, leaf, random), nil
}

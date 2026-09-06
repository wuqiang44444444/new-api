package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// 图片业务所需的统一存储能力接口（本地扩展）。图片功能只依赖该最小接口，
// 不再断言具体 s3ArtifactStore；S3 与 Azure Blob 实现各自履约，签名差异由
// 实现吸收。图片结果签名有效期固定 300 秒；旧 TaskArtifactStore 其他消费者的
// TTL 语义保持各自实现不变。

// imageResultURLTTLSeconds 是图片结果签名 URL 的固定有效期（§5：逐张有效
// 300 秒，到期后重新授权查询续签），不提供可调 TTL。
const imageResultURLTTLSeconds = 300

// fetchImageObjectMaxBytes 与既有 b64_json 读取上限保持一致。
const fetchImageObjectMaxBytes = 64 << 20

// ImageObjectRef 描述一个已保存的图片对象（输入或结果）。
type ImageObjectRef struct {
	ObjectKey string `json:"object_key"`
	MimeType  string `json:"mime_type,omitempty"`
	Size      int64  `json:"size,omitempty"`
}

// imageObjectStore 是图片业务所需的最小读写、HEAD 与签名能力。
type imageObjectStore interface {
	putImageObject(ctx context.Context, objectKey, mimeType string, data []byte) (*ImageObjectRef, error)
	presignImageObjectURL(objectKey string) (string, int64, error)
	headImageObject(ctx context.Context, objectKey string) (bool, error)
	fetchImageObjectBytes(ctx context.Context, objectKey string) ([]byte, error)
}

// currentImageObjectStore 返回当前生效且具备图片能力的存储实例；存储禁用
// 或实现不具备该能力时失败关闭。
func currentImageObjectStore(ctx context.Context) (imageObjectStore, error) {
	session, err := imageObjectSessionForContext(ctx)
	if err != nil {
		return nil, ErrTaskArtifactStoreDisabled
	}
	return session.store, nil
}

// PutImageObject uploads image bytes for one image task and returns the ref.
// objectKey 由调用方按任务身份派生（见 BuildImageTaskObjectKey）。
func PutImageObject(ctx context.Context, objectKey, mimeType string, data []byte) (*ImageObjectRef, error) {
	session, err := imageObjectSessionForContext(ctx)
	if err != nil {
		return nil, err
	}
	ref, err := session.store.putImageObject(ctx, objectKey, mimeType, data)
	if err != nil {
		// A real write failure invalidates an earlier successful readiness probe.
		session.mu.Lock()
		session.expires = time.Time{}
		session.mu.Unlock()
	}
	return ref, err
}

// PresignImageObjectURL issues the fixed 300-second result URL plus expiry.
func PresignImageObjectURL(ctx context.Context, objectKey string) (string, int64, error) {
	store, err := currentImageObjectStore(ctx)
	if err != nil {
		return "", 0, err
	}
	return store.presignImageObjectURL(objectKey)
}

// HeadImageObject reports whether a stored image object still exists.
func HeadImageObject(ctx context.Context, objectKey string) (bool, error) {
	store, err := currentImageObjectStore(ctx)
	if err != nil {
		return false, err
	}
	return store.headImageObject(ctx, objectKey)
}

// FetchImageObjectBytes downloads one stored image object（显式 b64_json 的
// 逐张读取，§5）。下载走短期签名 URL，签名不进入日志或响应。
func FetchImageObjectBytes(ctx context.Context, objectKey string) ([]byte, error) {
	store, err := currentImageObjectStore(ctx)
	if err != nil {
		return nil, err
	}
	readCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	return store.fetchImageObjectBytes(readCtx, objectKey)
}

// PutEphemeralImageResult stores a synchronous-mode image result (explicit
// response_format=url) and returns its 300-second presigned URL. The object
// name carries a random id so results are unguessable; lifecycle stays with
// the deployment's bucket policy (§5 清理).
func PutEphemeralImageResult(ctx context.Context, mimeType string, data []byte) (string, *types.NewAPIError) {
	ctx, err := WithImageObjectStore(ctx)
	if err != nil {
		return "", types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	objectKey, err := BuildImageObjectKey("ephemeral", "result")
	if err != nil {
		return "", types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	if _, err := PutImageObject(ctx, objectKey, mimeType, data); err != nil {
		common.SysError("put ephemeral image result failed: " + err.Error())
		return "", types.NewError(errors.New("failed to store image result"), types.ErrorCodeBadResponseBody)
	}
	url, _, err := PresignImageObjectURL(ctx, objectKey)
	if err != nil {
		return "", types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	return url, nil
}

// fetchImageObjectViaPresignedURL 以短期签名 URL 下载对象内容；签名 URL
// 只在本次调用内使用，不落日志。供各实现复用。
func fetchImageObjectViaPresignedURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.New("object storage download failed")
	}
	client := &http.Client{Timeout: 60 * time.Second, CheckRedirect: rejectObjectStorageRedirect}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("object storage download failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("object storage download failed")
	}
	return readImageObjectBytes(resp.Body)
}

// Never return a truncated image as a successful download.
func readImageObjectBytes(reader io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, fetchImageObjectMaxBytes+1))
	if err != nil {
		return nil, errors.New("image object could not be read")
	}
	if len(data) > fetchImageObjectMaxBytes {
		return nil, errors.New("image object exceeds the read size limit")
	}
	return data, nil
}

// Signed storage requests must never be forwarded to a redirect target.
func rejectObjectStorageRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}

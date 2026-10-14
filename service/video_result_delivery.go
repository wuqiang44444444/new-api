package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// Link 视频结果托管交付（本地扩展）。复用既有对象存储配置与运行时实例，
// 只新增视频结果所需的有界上传与 GET/HEAD 预签名能力。

// videoResultUploadPartSize 是视频分片上传的单片缓冲上限；进程内任一时刻
// 只缓冲单片，不保存完整视频正文。
const videoResultUploadPartSize = int64(16) << 20

// videoResultObjectStore 是视频结果托管所需的最小存储能力。objectKey 由
// 平台按任务身份生成；上传内容必须是流式读取并受 maxBytes 约束。
type videoResultObjectStore interface {
	putVideoResultObject(ctx context.Context, objectKey, mimeType string, content io.Reader, maxBytes int64) (int64, error)
	presignVideoResultObjectURL(objectKey, method string, ttl time.Duration) (string, int64, error)
	videoResultLocation() model.VideoResultStorageLocation
}

// currentVideoResultObjectStore 返回当前生效且具备视频结果能力的存储实例；
// 存储禁用或实现不具备该能力时失败关闭。
func currentVideoResultObjectStore(ctx context.Context) (videoResultObjectStore, error) {
	session, err := imageObjectSessionForContext(ctx)
	if err != nil {
		return nil, ErrTaskArtifactStoreDisabled
	}
	store, ok := session.store.(videoResultObjectStore)
	if !ok {
		return nil, ErrTaskArtifactStoreDisabled
	}
	return store, nil
}

// VideoResultDeliveryAvailable 报告当前存储是否可用于结果托管（配置生效且
// 具备视频能力）。转存 worker 与交付路径用它失败关闭，不回退旧代理。
func VideoResultDeliveryAvailable(ctx context.Context) bool {
	_, err := currentVideoResultObjectStore(ctx)
	return err == nil
}

// PresignVideoResultDownload 为一个 ready 对象签发固定 24 小时的 GET/HEAD
// URL。登记位置与当前配置不一致或存储禁用时失败关闭并返回错误，
// 不换桶解释、不下发上游直链。
func PresignVideoResultDownload(ctx context.Context, obj *model.TaskVideoResultObject, method string) (string, int64, error) {
	if method != http.MethodGet && method != http.MethodHead {
		return "", 0, errors.New("video result download method is unsupported")
	}
	if obj == nil || obj.Status != model.TaskVideoResultStatusReady || obj.ObjectKey == "" {
		return "", 0, errors.New("video result object is not ready for delivery")
	}
	store, err := currentVideoResultObjectStore(ctx)
	if err != nil {
		return "", 0, err
	}
	if store.videoResultLocation() != obj.Location {
		return "", 0, ErrTaskArtifactStoreDisabled
	}
	ttl := time.Duration(model.VideoResultPresignTTLSeconds) * time.Second
	url, expiresAt, err := store.presignVideoResultObjectURL(obj.ObjectKey, method, ttl)
	if err != nil {
		return "", 0, err
	}
	return url, expiresAt, nil
}

// boundVideoResultReader 限制一次转存读取的最大字节数；超出上限即报错，
// 保证任何前缀都不会被当成完整对象上传成功。
type boundVideoResultReader struct {
	reader    io.Reader
	remaining int64
}

func newBoundVideoResultReader(reader io.Reader, maxBytes int64) *boundVideoResultReader {
	return &boundVideoResultReader{reader: reader, remaining: maxBytes}
}

func (r *boundVideoResultReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		var extra [1]byte
		n, err := r.reader.Read(extra[:])
		if n > 0 {
			return 0, errVideoResultTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}

var errVideoResultTooLarge = errors.New("video result exceeds the transfer size limit")

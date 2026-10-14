package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"

	"github.com/QuantumNous/new-api/model"
)

// azureBlobArtifactStore 的视频结果托管能力（本地扩展，独立文件零改动既有
// 实现）。上传走 SDK 的 UploadStream 缓冲分片流水线：BlockSize 与并发都有
// 界，配合调用方的 maxBytes 读取上限，任一时刻不保存完整视频正文。

const (
	azureVideoResultBlockSize = int64(16) << 20
	azureVideoConcurrency     = 2
)

func (s *azureBlobArtifactStore) putVideoResultObject(ctx context.Context, objectKey, mimeType string, content io.Reader, maxBytes int64) (int64, error) {
	bounded := newBoundVideoResultReader(content, maxBytes)
	counter := &countingVideoResultReader{reader: bounded}
	httpHeaders := &blob.HTTPHeaders{}
	if mimeType != "" {
		httpHeaders.BlobContentType = &mimeType
	}
	putCtx, cancel := context.WithTimeout(ctx, videoResultTransferTimeout)
	defer cancel()
	blockSize := azureVideoResultBlockSize
	concurrency := azureVideoConcurrency
	_, err := s.blockBlobClient(objectKey).UploadStream(putCtx, counter, &blockblob.UploadStreamOptions{
		BlockSize:   blockSize,
		Concurrency: concurrency,
		HTTPHeaders: httpHeaders,
	})
	if err != nil {
		if errors.Is(err, errVideoResultTooLarge) {
			return 0, err
		}
		RecordImageDeliveryError(ctx, "video_result_store", err)
		return 0, azureStoreError("upload video result to object store failed", err)
	}
	if counter.count == 0 {
		return 0, errors.New("upstream video result is empty")
	}
	return counter.count, nil
}

// countingVideoResultReader 统计实际读取字节，作为对象大小事实来源。
type countingVideoResultReader struct {
	reader io.Reader
	count  int64
}

func (r *countingVideoResultReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.count += int64(n)
	return n, err
}

func (s *azureBlobArtifactStore) presignVideoResultObjectURL(objectKey, _ string, ttl time.Duration) (string, int64, error) {
	presigned, err := s.presignObjectURL(objectKey, ttl)
	if err != nil {
		return "", 0, err
	}
	return presigned, time.Now().Add(ttl).Unix(), nil
}

func (s *azureBlobArtifactStore) videoResultLocation() model.VideoResultStorageLocation {
	return model.VideoResultStorageLocation{
		Backend:  azureBlobBackendLabel,
		Endpoint: strings.TrimRight(s.config.Endpoint, "/"),
		Bucket:   s.container,
		Prefix:   strings.Trim(s.config.Prefix, "/"),
	}
}

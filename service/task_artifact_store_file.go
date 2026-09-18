package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/blockblob"
)

// File-backed upload is shared by exports and image delivery. The caller pins
// the storage instance; each operation retains its own diagnostic stage.
func (s *s3ArtifactStore) putObjectFile(ctx context.Context, objectKey string, mimeType string, path string, size int64, stage string) error {
	if size <= 0 {
		return errors.New("artifact file is empty")
	}
	digest, err := streamFileSha256(path)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	url := s.objectURL(objectKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	if err != nil {
		return err
	}
	req.Host = req.URL.Host
	if mimeType != "" {
		req.Header.Set("Content-Type", mimeType)
	}
	req.Body = io.NopCloser(file)
	req.ContentLength = size
	SigV4SignRequest(req, s.credentials, s.region(), digest, time.Now())
	client := &http.Client{Timeout: 15 * time.Minute, CheckRedirect: rejectObjectStorageRedirect}
	resp, err := client.Do(req)
	ObserveImageHTTPExchange(ctx, req, resp, err, stage)
	if err != nil {
		return errors.New("upload artifact failed")
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("object store rejected artifact upload with HTTP %d", resp.StatusCode)
	}
	return nil
}

func (s *azureBlobArtifactStore) putObjectFile(ctx context.Context, objectKey string, mimeType string, path string, size int64, stage string) error {
	if size <= 0 {
		return errors.New("artifact file is empty")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	httpHeaders := &blob.HTTPHeaders{}
	if mimeType != "" {
		httpHeaders.BlobContentType = &mimeType
	}
	putCtx, cancel := context.WithTimeout(ctx, azurePutTimeout)
	defer cancel()
	if _, err := s.blockBlobClient(objectKey).UploadFile(putCtx, file, &blockblob.UploadFileOptions{
		HTTPHeaders: httpHeaders,
	}); err != nil {
		RecordImageDeliveryError(ctx, stage, err)
		return azureStoreError("upload artifact failed", err)
	}
	return nil
}

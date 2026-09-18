package service

import (
	"context"
	"errors"
	"os"
	"time"
)

// Image uploads reuse the existing file-backed S3/Azure uploader and the bound
// image storage session. No image-size policy or second storage client is added.
func PutImageObjectFile(ctx context.Context, key, mimeType string, file *os.File) error {
	session, err := imageObjectSessionForContext(ctx)
	if err != nil {
		return err
	}
	uploader, ok := session.store.(interface {
		putObjectFile(context.Context, string, string, string, int64, string) error
	})
	if !ok {
		return errors.New("image storage does not support file uploads")
	}
	stat, err := file.Stat()
	if err != nil {
		return err
	}
	err = uploader.putObjectFile(ctx, key, mimeType, file.Name(), stat.Size(), "result_store")
	RecordImageDeliveryError(ctx, "result_store", err)
	if err != nil {
		session.mu.Lock()
		session.expires = time.Time{}
		session.mu.Unlock()
	}
	return err
}

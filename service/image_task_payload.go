package service

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/QuantumNous/new-api/model"
)

// StoreImageTaskPayload keeps native request bodies out of Task JSON and below
// the object store's per-object read limit, without limiting the native input.
func StoreImageTaskPayload(ctx context.Context, taskID, name string, reader io.Reader) ([]model.TaskImageArtifact, error) {
	var refs []model.TaskImageArtifact
	buffer := make([]byte, 8<<20)
	for index := 0; ; index++ {
		n, err := io.ReadFull(reader, buffer)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return nil, err
		}
		if n > 0 {
			key, keyErr := BuildImageTaskObjectKey(taskID, fmt.Sprintf("%s-%d", name, index))
			if keyErr != nil {
				return nil, keyErr
			}
			if _, putErr := PutImageObject(ctx, key, "application/octet-stream", buffer[:n]); putErr != nil {
				return nil, putErr
			}
			refs = append(refs, model.TaskImageArtifact{ObjectKey: key, Size: int64(n)})
		}
		if err != nil {
			return refs, nil
		}
	}
}

// RestoreImageTaskPayload spools privately before any provider send. The caller
// owns closing/removing the file; one chunk at a time bounds heap consumption.
func RestoreImageTaskPayload(ctx context.Context, refs []model.TaskImageArtifact) (*os.File, error) {
	file, err := os.CreateTemp("", "image-task-payload-*")
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			file.Close()
			os.Remove(file.Name())
		}
	}()
	for _, ref := range refs {
		data, err := FetchImageObjectBytes(ctx, ref.ObjectKey)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) != ref.Size {
			return nil, fmt.Errorf("image payload object size mismatch")
		}
		if _, err := file.Write(data); err != nil {
			return nil, err
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	ok = true
	return file, nil
}

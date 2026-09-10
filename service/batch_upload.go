package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// Batch file upload keeps the whole content private: bytes go to the object
// store, the database row only records the reference and the validated shape.
// Validation is a full streaming pass; the batch is rejected as a whole with
// the offending line number before anything is stored. The 20 MiB upload cap
// doubles as the in-memory buffering budget.

type BatchUploadResult struct {
	File *model.BatchFile
}

func UploadBatchFile(ctx context.Context, userId int, tokenId int, filename string, purpose string, content io.Reader) (*BatchUploadResult, error) {
	if userId <= 0 {
		return nil, fmt.Errorf("batch upload requires an authenticated caller")
	}
	if !strings.EqualFold(purpose, "batch") {
		return nil, &dto.BatchValidateError{Message: "purpose must be \"batch\""}
	}
	if strings.TrimSpace(filename) == "" {
		filename = "upload.jsonl"
	}
	if len(filename) > 255 {
		filename = filename[:255]
	}

	data, err := io.ReadAll(io.LimitReader(content, dto.MaxBatchFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read upload: %w", err)
	}
	if int64(len(data)) > dto.MaxBatchFileBytes {
		return nil, &dto.BatchValidateError{Message: fmt.Sprintf("batch exceeds the maximum file size of %d bytes", dto.MaxBatchFileBytes)}
	}

	parse, err := ValidateBatchJSONL(bytes.NewReader(data), "")
	if err != nil {
		return nil, err
	}

	objectName, err := model.NewBatchObjectName()
	if err != nil {
		return nil, err
	}
	objectKey := batchObjectKey(userId, objectName)
	if err := PutBatchObjectFinal(ctx, objectKey, data); err != nil {
		return nil, err
	}

	file, err := model.CreateBatchFile(model.CreateBatchFileParams{
		UserId: userId, AppID: tokenId, TokenId: tokenId,
		FileName: filename, Purpose: "batch",
		SizeBytes: parse.SizeBytes, LineCount: parse.LineCount, ModelName: parse.Model,
		Checksum: parse.Checksum, ObjectKey: objectKey, UploadSource: "api",
	})
	if err != nil {
		return nil, err
	}
	return &BatchUploadResult{File: file}, nil
}

// BatchFileContent streams one owned file's stored bytes.
func BatchFileContent(ctx context.Context, fileId string, userId int, appID int, w io.Writer) (int64, *model.BatchFile, error) {
	file, err := model.GetBatchFileOwned(fileId, userId, appID)
	if err != nil {
		return 0, nil, err
	}
	if file.ObjectKey == "" {
		return 0, file, ErrBatchObjectNotFound
	}
	exists, err := BatchObjectExists(ctx, file.ObjectKey)
	if err != nil {
		return 0, file, fmt.Errorf("check batch file content: %w", err)
	}
	if !exists {
		return 0, file, ErrBatchObjectNotFound
	}
	written, err := GetBatchObject(ctx, file.ObjectKey, w)
	if err != nil {
		return 0, file, err
	}
	return written, file, nil
}

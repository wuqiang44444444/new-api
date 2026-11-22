package service

import (
	"archive/zip"
	"compress/flate"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

var (
	ErrCustomerExportArchiveUnavailable = errors.New("export archive is unavailable")
	ErrCustomerExportArchiveBusy        = errors.New("export archive service is busy")
)

// Compress at most one archive per process. Source objects and the response
// stream keep the working set independent of the size of the CSV files.
var customerExportArchiveSlot = make(chan struct{}, 1)

type CustomerExportArchive struct {
	store exportObjectStore
	files []model.CustomerExportArtifactFile
}

func PrepareCustomerExportArchive(job *model.CustomerExportJob) (*CustomerExportArchive, error) {
	artifact := job.DecodeArtifact()
	if artifact == nil || len(artifact.Files) < 2 {
		return nil, ErrCustomerExportArchiveUnavailable
	}
	store, err := currentExportObjectStore()
	if err != nil || artifact.StoreIdentity == "" || artifact.StoreIdentity != store.ExportIdentity() {
		return nil, ErrCustomerExportArchiveUnavailable
	}
	keys := make(map[string]bool, len(artifact.Files))
	for _, file := range artifact.Files {
		name := file.FileName
		if len(name) <= len(".csv") || !strings.HasSuffix(name, ".csv") || strings.ContainsAny(name, "/\\:\r\n\x00") ||
			file.ObjectKey != CustomerExportObjectNamespace+"/"+job.JobID+"/"+name ||
			file.SizeBytes <= 0 || file.SizeBytes == math.MaxInt64 || len(file.Sha256) != sha256.Size*2 || keys[strings.ToLower(name)] {
			return nil, ErrCustomerExportArchiveUnavailable
		}
		keys[strings.ToLower(name)] = true
	}
	return &CustomerExportArchive{store: store, files: artifact.Files}, nil
}

func (archive *CustomerExportArchive) Write(ctx context.Context, destination io.Writer) error {
	select {
	case customerExportArchiveSlot <- struct{}{}:
		defer func() { <-customerExportArchiveSlot }()
	default:
		return ErrCustomerExportArchiveBusy
	}
	return archive.writeZIP(ctx, destination, nil)
}

func (archive *CustomerExportArchive) writeZIP(ctx context.Context, destination io.Writer, onFilePackaged func(int)) error {
	writer := zip.NewWriter(destination)
	writer.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(w, flate.BestSpeed)
	})
	for i, file := range archive.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		body, err := archive.store.ExportOpenObject(ctx, file.ObjectKey)
		if err != nil {
			return ErrCustomerExportArchiveUnavailable
		}
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: file.FileName, Method: zip.Deflate})
		if err != nil {
			body.Close()
			return err
		}
		digest := sha256.New()
		size, err := io.Copy(io.MultiWriter(entry, digest), io.LimitReader(body, file.SizeBytes+1))
		body.Close()
		if err != nil {
			return err
		}
		if size != file.SizeBytes || hex.EncodeToString(digest.Sum(nil)) != file.Sha256 {
			return ErrCustomerExportArchiveUnavailable
		}
		if onFilePackaged != nil {
			onFilePackaged(i + 1)
		}
	}
	// Omit the central directory on error so a partial archive cannot be
	// mistaken for a complete export.
	return writer.Close()
}

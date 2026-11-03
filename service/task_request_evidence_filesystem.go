package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

type EvidenceStoredFile struct {
	Key        string
	ModifiedAt int64
}

func (s *localEncryptedEvidenceStore) WalkEvidenceNamespaces(ctx context.Context, visit func([]int64) error) error {
	dir, err := os.Open(s.baseDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := visit([]int64{0}); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(256)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		ids := make([]int64, 0, len(entries))
		for _, entry := range entries {
			id, err := strconv.ParseInt(entry.Name(), 10, 64)
			if err == nil && id > 0 && strconv.FormatInt(id, 10) == entry.Name() && entry.IsDir() {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			if err := visit(ids); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
	}
}

// Snapshot one bounded batch before deleting entries. Never follow a namespace
// symlink or recurse into resource, input, or delivery storage.
func (s *localEncryptedEvidenceStore) WalkEvidenceFiles(ctx context.Context, evidenceID int64, visit func(EvidenceStoredFile) error) error {
	path := s.baseDir
	if evidenceID > 0 {
		path = filepath.Join(path, strconv.FormatInt(evidenceID, 10))
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("evidence namespace is not a directory")
	}
	dir, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, readErr := dir.ReadDir(256)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return err
			}
			if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			key := entry.Name()
			if evidenceID > 0 {
				key = strconv.FormatInt(evidenceID, 10) + "/" + key
			}
			if err := visit(EvidenceStoredFile{Key: key, ModifiedAt: info.ModTime().Unix()}); err != nil {
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			return nil
		}
	}
}

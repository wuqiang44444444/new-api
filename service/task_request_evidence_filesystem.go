package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

type EvidenceStoredFile struct {
	Key        string
	ModifiedAt int64
}

func (s *localEncryptedEvidenceStore) EvidenceNamespaces() ([]int64, error) {
	entries, err := os.ReadDir(s.baseDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ids := []int64{0}
	for _, entry := range entries {
		id, err := strconv.ParseInt(entry.Name(), 10, 64)
		if err == nil && id > 0 && strconv.FormatInt(id, 10) == entry.Name() && entry.IsDir() {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// Snapshot one directory before deleting entries. Never follow a namespace
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
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
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
	return nil
}

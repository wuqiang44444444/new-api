package service

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// Legacy GCM objects require whole-object authentication. Bound that old read
// path independently of the streaming format; never migrate or rewrite files.
const evidenceLegacyMaxBytes = (32 << 20) + (128 << 10)

var (
	evidenceLegacyReadSlot = make(chan struct{}, 1)
	errEvidenceReadBudget  = errors.New("evidence read resource limit")
)

func (s *localEncryptedEvidenceStore) openLegacy(file *os.File) (io.ReadCloser, error) {
	defer file.Close()
	select {
	case evidenceLegacyReadSlot <- struct{}{}:
	default:
		return nil, errEvidenceReadBudget
	}
	retained := false
	defer func() {
		if !retained {
			<-evidenceLegacyReadSlot
		}
	}()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > evidenceLegacyMaxBytes {
		return nil, errEvidenceReadBudget
	}
	nonceSize := s.aead.NonceSize()
	if info.Size() < int64(nonceSize) {
		return nil, ErrEvidenceIntegrityFailed
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	sealed := make([]byte, int(info.Size()))
	if _, err := io.ReadFull(file, sealed); err != nil {
		return nil, err
	}
	var extra [1]byte
	if n, err := file.Read(extra[:]); n != 0 || err != io.EOF {
		return nil, ErrEvidenceIntegrityFailed
	}
	// Reuse the ciphertext allocation, with the exact overlap required by AEAD.
	plaintext, err := s.aead.Open(sealed[nonceSize:nonceSize], sealed[:nonceSize], sealed[nonceSize:], nil)
	if err != nil {
		return nil, ErrEvidenceDecryptFailed
	}
	retained = true
	return &evidenceReadCloser{Reader: bytes.NewReader(plaintext), size: int64(len(plaintext)), close: func() error {
		<-evidenceLegacyReadSlot
		return nil
	}}, nil
}

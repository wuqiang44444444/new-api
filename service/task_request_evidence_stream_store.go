package service

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/minio/sio"
)

// DARE v2 supplies authenticated sequencing and final-package verification.
// Derive a different key for each immutable event location, binding ciphertext
// to its identity; moving an object to another event cannot authenticate.
const evidenceStreamMagic = "NEVSIO3\n"

func (s *localEncryptedEvidenceStore) streamConfig(key string) sio.Config {
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write([]byte("new-api/evidence-v3/" + key))
	return sio.Config{Key: mac.Sum(nil), MinVersion: sio.Version20, MaxVersion: sio.Version20}
}

type evidenceStreamObject struct {
	store      *localEncryptedEvidenceStore
	file       *os.File
	writer     io.WriteCloser
	target     string
	evidenceID int64
	closed     bool
}

func (s *localEncryptedEvidenceStore) beginStream(key string, evidenceID int64) (*evidenceStreamObject, error) {
	return s.beginStreamObject(key, evidenceID, 0)
}

func (s *localEncryptedEvidenceStore) beginStreamObject(key string, evidenceID, sourceEventID int64) (*evidenceStreamObject, error) {
	if len(s.key) != 32 {
		return nil, errors.New("evidence stream key unavailable")
	}
	target, err := s.resolvePath(key)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(filepath.Dir(target), ".evidence-*")
	if err != nil {
		return nil, err
	}
	object := &evidenceStreamObject{store: s, file: file, target: target, evidenceID: evidenceID}
	// Reservation may precede cleanup. Check after creating staging so cleanup
	// cannot finish first and leave a late worker accumulating a new file.
	if evidenceID > 0 {
		index, exists, err := model.GetTaskRequestEvidenceById(evidenceID)
		if err != nil || !exists || IsEvidenceBodyExpired(index, common.GetTimestamp()) {
			object.Abort()
			_ = os.Remove(filepath.Dir(target)) // Only remove our now-empty namespace.
			if err != nil {
				return nil, err
			}
			return nil, model.ErrTaskRequestEvidenceBodyClosed
		}
	}
	if _, err = io.WriteString(file, evidenceStreamMagic); err == nil {
		object.writer, err = sio.EncryptWriter(struct{ io.Writer }{file}, s.streamConfig(key))
		if err == nil {
			var header [9]byte
			prefix := header[:1]
			if sourceEventID > 0 {
				header[0] = evidenceStreamBodyReference
				binary.BigEndian.PutUint64(header[1:], uint64(sourceEventID))
				prefix = header[:]
			}
			_, err = object.writer.Write(prefix)
		}
	}
	if err != nil {
		object.Abort()
		return nil, err
	}
	return object, nil
}

func (o *evidenceStreamObject) Write(p []byte) (int, error) {
	if o.closed {
		return 0, os.ErrClosed
	}
	return o.writer.Write(p)
}

func (o *evidenceStreamObject) Abort() {
	if o.closed {
		return
	}
	o.closed = true
	_ = o.file.Close()
	_ = os.Remove(o.file.Name())
}

func (o *evidenceStreamObject) Close() (err error) {
	if o.closed {
		return os.ErrClosed
	}
	defer o.Abort()
	if err = o.writer.Close(); err != nil {
		return err
	}
	if err = o.file.Sync(); err != nil {
		return err
	}
	if err = o.file.Close(); err != nil {
		return err
	}
	publish := func() error { return os.Link(o.file.Name(), o.target) }
	if o.evidenceID > 0 {
		err = model.CommitTaskRequestEvidenceObject(o.evidenceID, EvidenceRetentionSeconds(), publish)
	} else {
		err = publish()
	}
	if err != nil {
		return err
	}
	// Persist the final directory entry without syncing every ancestor per block.
	dir, err := os.Open(filepath.Dir(o.target))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

type evidenceStreamReader struct {
	io.Reader
	file *os.File
	size int64
}

func (r *evidenceStreamReader) Close() error { return r.file.Close() }
func (r *evidenceStreamReader) Size() int64  { return r.size }

func (s *localEncryptedEvidenceStore) openStream(key string) (io.ReadCloser, error) {
	return s.openStreamObject(key, true)
}

func (s *localEncryptedEvidenceStore) openStreamObject(key string, allowBodyReference bool) (io.ReadCloser, error) {
	path, err := s.resolvePath(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrTaskRequestEvidenceUnavailable
	}
	if err != nil {
		return nil, err
	}
	reader := bufio.NewReader(file)
	magic, _ := reader.Peek(len(evidenceStreamMagic))
	if string(magic) != evidenceStreamMagic {
		if !allowBodyReference {
			_ = file.Close()
			return nil, ErrEvidenceDecryptFailed
		}
		return s.openLegacy(file)
	}
	_, _ = reader.Discard(len(evidenceStreamMagic))
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	size, err := sio.DecryptedSize(uint64(info.Size() - int64(len(evidenceStreamMagic))))
	if err != nil || size == 0 {
		_ = file.Close()
		return nil, ErrEvidenceDecryptFailed
	}
	decrypted, err := sio.DecryptReader(reader, s.streamConfig(key))
	if err != nil {
		_ = file.Close()
		return nil, ErrEvidenceDecryptFailed
	}
	var marker [1]byte
	if _, err = io.ReadFull(decrypted, marker[:]); err != nil {
		_ = file.Close()
		return nil, ErrEvidenceDecryptFailed
	}
	stream := &evidenceStreamReader{Reader: decrypted, file: file, size: int64(size) - 1}
	if marker[0] == 0 {
		return stream, nil
	}
	defer stream.Close()
	if marker[0] != evidenceStreamBodyReference || !allowBodyReference {
		return nil, ErrEvidenceDecryptFailed
	}
	return s.openSharedEvidenceBody(key, stream)
}

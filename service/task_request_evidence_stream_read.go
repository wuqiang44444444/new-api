package service

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/minio/sio"
)

const evidenceViewBytes = 64 << 10

var evidenceReadSlots = make(chan struct{}, 8)

type evidenceReadCloser struct {
	io.Reader
	close func() error
	once  sync.Once
	err   error
	size  int64
}

func (r *evidenceReadCloser) Close() error {
	r.once.Do(func() { r.err = r.close() })
	return r.err
}
func (r *evidenceReadCloser) Size() int64 { return r.size }

// Download keeps the authenticated storage envelope and all captured bytes.
// It never loads a new-format object in full or interprets its business body.
func OpenEvidenceEventBody(event *model.TaskRequestEvidenceEvent, expired bool) (io.ReadCloser, string) {
	if expired {
		return nil, "expired"
	}
	if event.Id == 0 && event.ObjectKey == "" {
		return nil, "not_recorded"
	}
	select {
	case evidenceReadSlots <- struct{}{}:
	default:
		return nil, "storage_unavailable"
	}
	store := GetTaskRequestEvidenceStore()
	if store == nil {
		<-evidenceReadSlots
		return nil, "storage_unavailable"
	}
	key := event.ObjectKey
	if key == "" {
		key = EvidenceObjectKey(event.EvidenceId, event.Id)
	}
	var reader io.ReadCloser
	var err error
	if local, ok := store.(*localEncryptedEvidenceStore); ok {
		reader, err = local.openStream(key)
	} else {
		// Object stores implementing the pre-existing interface contain bounded
		// legacy objects; the production stream store uses openStream above.
		var body []byte
		body, err = store.Get(key)
		if err == nil {
			reader = io.NopCloser(bytes.NewReader(body))
		}
	}
	if err != nil {
		<-evidenceReadSlots
		return nil, evidenceReadStatus(err)
	}
	buffered := bufio.NewReader(reader)
	if _, err = buffered.Peek(1); err != nil && err != io.EOF {
		_ = reader.Close()
		<-evidenceReadSlots
		return nil, evidenceReadStatus(err)
	}
	size := int64(-1)
	if sized, ok := reader.(interface{ Size() int64 }); ok {
		size = sized.Size()
	}
	return &evidenceReadCloser{Reader: buffered, size: size, close: func() error { defer func() { <-evidenceReadSlots }(); return reader.Close() }}, "available"
}

func evidenceReadStatus(err error) string {
	var authError sio.Error
	switch {
	case errors.Is(err, errEvidenceReadBudget):
		return "storage_unavailable"
	case errors.Is(err, ErrTaskRequestEvidenceUnavailable):
		return "missing"
	case errors.Is(err, ErrEvidenceDecryptFailed), errors.As(err, &authError):
		return "decrypt_failed"
	case errors.Is(err, ErrEvidenceIntegrityFailed):
		return "integrity_failed"
	default:
		return "read_failed"
	}
}

// Only the small storage envelope is parsed; the opaque business body is read
// through a fixed display window. UTF-8/JSON validity does not gate capture.
func readEvidenceContentWindow(event *model.TaskRequestEvidenceEvent, expired bool) (*EvidenceEventContent, string) {
	reader, status := OpenEvidenceEventBody(event, expired)
	if status != "available" {
		return nil, status
	}
	defer reader.Close()
	var source io.Reader = reader
	var checksum hash.Hash
	digest := evidenceLegacySHA256(event)
	if digest != "" {
		checksum = sha256.New()
		source = io.TeeReader(reader, checksum)
	}
	buffered := bufio.NewReaderSize(source, 128<<10)
	var meta *EvidenceObjectMeta
	prefix, _ := buffered.Peek(len(evidenceObjectMagic) + 1)
	if string(prefix) == evidenceObjectMagic+"\n" {
		_, _ = buffered.Discard(len(prefix))
		header, err := buffered.ReadSlice('\n')
		if err != nil {
			return nil, "read_failed"
		}
		var decoded EvidenceObjectMeta
		if common.Unmarshal(bytes.TrimSuffix(header, []byte{'\n'}), &decoded) != nil {
			return nil, "read_failed"
		}
		meta = &decoded
	}
	body, err := io.ReadAll(io.LimitReader(buffered, evidenceViewBytes+1))
	if err != nil {
		return nil, evidenceReadStatus(err)
	}
	if checksum != nil {
		if _, err := io.CopyBuffer(io.Discard, buffered, make([]byte, 64<<10)); err != nil {
			return nil, evidenceReadStatus(err)
		}
		if hex.EncodeToString(checksum.Sum(nil)) != digest {
			return nil, "integrity_failed"
		}
	}
	truncated := len(body) > evidenceViewBytes
	if truncated {
		body = body[:evidenceViewBytes]
	}
	return &EvidenceEventContent{Meta: meta, Body: body, BodyStatus: "available", DisplayLimited: truncated}, "available"
}

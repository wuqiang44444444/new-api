package service

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"io"
	"strconv"
	"strings"
)

const evidenceStreamBodyReference = 1
const maxEvidenceEnvelopeBytes = len(evidenceObjectMagic) + 1 + (128 << 10)

// The authenticated reference contains a source event ID and the delivery
// envelope. A download still returns the delivery envelope and original body.
func (s *localEncryptedEvidenceStore) openSharedEvidenceBody(key string, reference *evidenceStreamReader) (io.ReadCloser, error) {
	namespace, _, _ := strings.Cut(key, "/")
	evidenceID, err := strconv.ParseInt(namespace, 10, 64)
	if err != nil || evidenceID <= 0 {
		return nil, ErrEvidenceIntegrityFailed
	}
	var encodedID [8]byte
	if _, err = io.ReadFull(reference, encodedID[:]); err != nil {
		return nil, ErrEvidenceDecryptFailed
	}
	sourceID := int64(binary.BigEndian.Uint64(encodedID[:]))
	if sourceID <= 0 || reference.size > int64(len(encodedID)+maxEvidenceEnvelopeBytes) {
		return nil, ErrEvidenceIntegrityFailed
	}
	// Read to EOF to authenticate the entire small reference before following it.
	envelope, err := io.ReadAll(io.LimitReader(reference, int64(maxEvidenceEnvelopeBytes)+1))
	if err != nil {
		return nil, ErrEvidenceDecryptFailed
	}
	if len(envelope) > maxEvidenceEnvelopeBytes || !bytes.HasPrefix(envelope, []byte(evidenceObjectMagic+"\n")) || envelope[len(envelope)-1] != '\n' {
		return nil, ErrEvidenceIntegrityFailed
	}
	source, err := s.openStreamObject(EvidenceObjectKey(evidenceID, sourceID), false)
	if err != nil {
		return nil, err
	}
	stream := source.(*evidenceStreamReader)
	buffered := bufio.NewReaderSize(stream, maxEvidenceEnvelopeBytes)
	magic, err := buffered.ReadSlice('\n')
	if err != nil || string(magic) != evidenceObjectMagic+"\n" {
		_ = stream.Close()
		return nil, ErrEvidenceIntegrityFailed
	}
	magicSize := len(magic)
	metadata, err := buffered.ReadSlice('\n')
	bodySize := stream.Size() - int64(magicSize+len(metadata))
	if err != nil || bodySize <= 0 {
		_ = stream.Close()
		return nil, ErrEvidenceIntegrityFailed
	}
	return &evidenceReadCloser{Reader: io.MultiReader(bytes.NewReader(envelope), buffered), close: stream.Close, size: int64(len(envelope)) + bodySize}, nil
}

package service

import (
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"

	"github.com/QuantumNous/new-api/model"
)

// CopyEvidenceDownload holds the final byte until the source authenticates EOF
// and any historical digest matches. A failed transfer stays below Content-Length
// even if the advertised size was derived from a truncated ciphertext.
func CopyEvidenceDownload(dst io.Writer, payload evidenceBodyReader, event *model.TaskRequestEvidenceEvent) error {
	var source io.Reader = payload
	var checksum hash.Hash
	digest := evidenceLegacySHA256(event)
	if digest != "" {
		checksum = sha256.New()
		source = io.TeeReader(payload, checksum)
	}

	var tail [1]byte
	size := payload.Size()
	if size > 0 {
		prefixSize := size - 1
		n, err := io.CopyBuffer(dst, io.LimitReader(source, prefixSize), make([]byte, 64<<10))
		if err != nil {
			return err
		}
		if n != prefixSize {
			return io.ErrUnexpectedEOF
		}
		if _, err = io.ReadFull(source, tail[:]); err != nil {
			return err
		}
	}

	var extra [1]byte
	if _, err := io.ReadFull(source, extra[:]); err != io.EOF {
		if err == nil {
			return ErrEvidenceIntegrityFailed
		}
		return err
	}
	if checksum != nil && hex.EncodeToString(checksum.Sum(nil)) != digest {
		return ErrEvidenceIntegrityFailed
	}
	if size > 0 {
		_, err := dst.Write(tail[:])
		return err
	}
	return nil
}

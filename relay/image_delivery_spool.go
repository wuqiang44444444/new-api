package relay

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/shirou/gopsutil/disk"
)

var errImageDeliveryDiskReserve = errors.New("image delivery disk reserve reached")
var imageDeliveryDiskMu sync.Mutex

// This resource guard has no per-image size limit. Serialize check+write in
// bounded chunks so concurrent deliveries leave a reserve on the spool volume.
// Other processes can still consume space; deployments should isolate TMPDIR.
type imageDeliverySpoolWriter struct{ file *os.File }

func (w imageDeliverySpoolWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		chunk := p[:min(len(p), 1<<20)]
		imageDeliveryDiskMu.Lock()
		info, err := disk.Usage(filepath.Dir(w.file.Name()))
		reserve := uint64(128 << 20)
		if raw := os.Getenv("IMAGE_DELIVERY_MIN_FREE_MB"); raw != "" {
			mb, parseErr := strconv.ParseUint(raw, 10, 40)
			if parseErr != nil || mb == 0 {
				imageDeliveryDiskMu.Unlock()
				return written, errImageDeliveryDiskReserve
			}
			reserve = mb << 20
		}
		if err == nil && (info.Free < reserve || info.Free-reserve < uint64(len(chunk))) {
			err = errImageDeliveryDiskReserve
		}
		n := 0
		if err == nil {
			n, err = w.file.Write(chunk)
		}
		imageDeliveryDiskMu.Unlock()
		written += n
		if err != nil {
			return written, err
		}
		p = p[n:]
	}
	return written, nil
}

func (w imageDeliverySpoolWriter) WriteString(s string) (int, error) {
	written := 0
	for len(s) > 0 {
		chunk := s[:min(len(s), 1<<20)]
		n, err := w.Write([]byte(chunk))
		written += n
		if err != nil {
			return written, err
		}
		s = s[n:]
	}
	return written, nil
}

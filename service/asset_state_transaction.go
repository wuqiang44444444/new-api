package service

import (
	"strings"
	"time"
)

// runAssetStateTransaction retries only the local persistence of an already
// known upstream result. Callers must keep every upstream request outside fn.
func runAssetStateTransaction(fn func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = fn()
		if err == nil || !isRetryableAssetStateDBError(err) {
			return err
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
		}
	}
	return err
}

func isRetryableAssetStateDBError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{
		"database is locked",
		"database table is locked",
		"sqlite_busy",
		"deadlock detected",
		"deadlock found",
		"serialization failure",
		"could not serialize access",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

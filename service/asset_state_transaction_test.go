package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRetryableAssetStateDBErrorClassification(t *testing.T) {
	assert.True(t, isRetryableAssetStateDBError(errors.New("database is locked (5) (SQLITE_BUSY)")))
	assert.True(t, isRetryableAssetStateDBError(errors.New("deadlock detected")))
	assert.False(t, isRetryableAssetStateDBError(errors.New("unique constraint failed")))
}

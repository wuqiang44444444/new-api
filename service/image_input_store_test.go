package service

import (
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type inputSigningStore struct {
	sessionTestImageStore
	ttl time.Duration
}

func (s *inputSigningStore) presignImageInputURL(key string, ttl time.Duration) (string, error) {
	s.ttl = ttl
	return "https://storage.example/" + key, nil
}
func TestImageInputSigningUsesDispatchBudgetAndKeepsResultTTL(t *testing.T) {
	restoreRuntime(t)
	store := &inputSigningStore{}
	taskArtifactStoreRuntime.swap(store, "input-test")
	ctx, err := WithImageObjectStore(t.Context())
	require.NoError(t, err)
	_, err = PutImageObject(ctx, "input-0", "image/png", []byte("bytes"))
	require.NoError(t, err)
	_, err = PresignImageInputURL(ctx, "input-0")
	require.NoError(t, err)
	assert.Equal(t, 2*time.Hour, store.ttl)
	longer, cancel := context.WithTimeout(ctx, 3*time.Hour)
	defer cancel()
	_, err = PresignImageInputURL(longer, "input-0")
	require.NoError(t, err)
	assert.InDelta(t, float64(4*time.Hour), float64(store.ttl), float64(time.Second))
	_, expiry, err := PresignImageObjectURL(ctx, "input-0")
	require.NoError(t, err)
	assert.InDelta(t, 300, expiry-time.Now().Unix(), 1)
	assert.Equal(t, 1, store.puts, "signing existing task inputs must not upload again")
}

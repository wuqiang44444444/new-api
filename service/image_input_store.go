package service

import (
	"context"
	"errors"
	"time"
)

// Input URLs are issued at dispatch, covering the execution deadline plus one hour
// for provider fetch delays. Queue time does not consume this lifetime.
func PresignImageInputURL(ctx context.Context, key string) (string, error) {
	store, err := currentImageObjectStore(ctx)
	if err != nil {
		return "", err
	}
	signer, ok := store.(interface {
		presignImageInputURL(string, time.Duration) (string, error)
	})
	if !ok {
		return "", errors.New("image input signing is unavailable")
	}
	ttl := 2 * time.Hour
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline)+time.Hour > ttl {
		ttl = time.Until(deadline) + time.Hour
	}
	if ttl > 7*24*time.Hour {
		return "", errors.New("image execution exceeds input signing lifetime")
	}
	return signer.presignImageInputURL(key, ttl)
}

func (s *s3ArtifactStore) presignImageInputURL(key string, ttl time.Duration) (string, error) {
	return s.PresignObjectURL(key, ttl)
}
func (s *azureBlobArtifactStore) presignImageInputURL(key string, ttl time.Duration) (string, error) {
	return s.presignObjectURL(key, ttl)
}

package service

import (
	"context"
	"strings"

	"github.com/QuantumNous/new-api/model"
)

// The current client may rotate credentials, but must still address the saved
// location. No old credential registry, bucket fallback, or object migration.
func funCloudHostedStorageLocation(ctx context.Context) (model.FunCloudHostedStorageLocation, error) {
	session, err := imageObjectSessionForContext(ctx)
	if err != nil {
		return model.FunCloudHostedStorageLocation{}, err
	}
	store, ok := session.store.(interface {
		hostedAssetLocation() model.FunCloudHostedStorageLocation
	})
	if !ok {
		return model.FunCloudHostedStorageLocation{}, ErrTaskArtifactStoreDisabled
	}
	return store.hostedAssetLocation(), nil
}

func (s *s3ArtifactStore) hostedAssetLocation() model.FunCloudHostedStorageLocation {
	return model.FunCloudHostedStorageLocation{Backend: "s3", Endpoint: strings.TrimRight(s.config.S3Endpoint, "/"), Bucket: s.config.S3Bucket, Prefix: strings.Trim(s.config.S3Prefix, "/")}
}

func (s *azureBlobArtifactStore) hostedAssetLocation() model.FunCloudHostedStorageLocation {
	return model.FunCloudHostedStorageLocation{Backend: "azure_blob", Endpoint: strings.TrimRight(s.config.Endpoint, "/"), Bucket: s.container, Prefix: strings.Trim(s.config.Prefix, "/")}
}

func validateFunCloudHostedObject(ctx context.Context, saved model.FunCloudHostedStorageLocation, objectKey string) error {
	ctx, err := WithImageObjectStore(ctx)
	if err != nil {
		return ErrTaskArtifactStoreDisabled
	}
	current, err := funCloudHostedStorageLocation(ctx)
	if err != nil || saved.Backend == "" || current != saved {
		return ErrTaskArtifactStoreDisabled
	}
	exists, err := HeadImageObject(ctx, objectKey)
	if err != nil || !exists {
		return ErrTaskArtifactStoreDisabled
	}
	return nil
}

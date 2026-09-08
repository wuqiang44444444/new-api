package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostedStorageCredentialsRotateWithoutChangingObjectLocation(t *testing.T) {
	for _, backend := range []string{"s3", "azure_blob"} {
		t.Run(backend, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodHead, r.Method)
				assert.Equal(t, "/images/prod/assets/source.png", r.URL.Path)
				w.Header().Set("Content-Length", "12")
			}))
			t.Cleanup(server.Close)
			config := system_setting.ObjectStorageConfig{Backend: backend, Endpoint: server.URL, Bucket: "images", Prefix: "prod/", Region: "us-east-1", AccountName: "account"}
			var location model.FunCloudHostedStorageLocation
			for _, credential := range []string{"Zmlyc3Qta2V5", "c2Vjb25kLWtleQ=="} {
				var store TaskArtifactStore
				var err error
				if backend == "s3" {
					store, err = NewS3ArtifactStore(legacyS3Config(config, credential))
				} else {
					store, err = NewAzureBlobArtifactStore(config, credential)
				}
				require.NoError(t, err)
				ctx := context.WithValue(t.Context(), imageObjectSessionKey{}, &imageObjectSession{store: store.(imageObjectStore)})
				if location.Backend == "" {
					location, err = funCloudHostedStorageLocation(ctx)
					require.NoError(t, err)
				}
				require.NoError(t, validateFunCloudHostedObject(ctx, location, "assets/source.png"))
				signed, err := SignFunCloudHostedAssetURL(ctx, model.TaskHostedMediaFact{ObjectKey: "assets/source.png", StorageLocation: location})
				require.NoError(t, err)
				parsed, err := url.Parse(signed)
				require.NoError(t, err)
				assert.Equal(t, "/images/prod/assets/source.png", parsed.Path)
				assert.Equal(t, server.Listener.Addr().String(), parsed.Host)
			}
		})
	}
}

package service

import (
	"net/url"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureSASValidityUsesUTCOnNonUTCHost(t *testing.T) {
	previous := time.Local
	time.Local = time.FixedZone("acceptance-UTC+8", 8*60*60)
	t.Cleanup(func() { time.Local = previous })
	store, err := NewAzureBlobArtifactStore(system_setting.ObjectStorageConfig{
		Backend: system_setting.ObjectStorageBackendAzureBlob, Endpoint: "https://account.blob.core.windows.net", AccountName: "account", Bucket: "images", Prefix: "tests",
	}, "dGVzdC1rZXk=")
	require.NoError(t, err)
	signed, err := store.(*azureBlobArtifactStore).presignObjectURL("result.png", 5*time.Minute)
	require.NoError(t, err)
	parsed, err := url.Parse(signed)
	require.NoError(t, err)
	start, err := time.Parse(time.RFC3339, parsed.Query().Get("st"))
	require.NoError(t, err)
	end, err := time.Parse(time.RFC3339, parsed.Query().Get("se"))
	require.NoError(t, err)
	now := time.Now()
	assert.True(t, start.Before(now), "SAS must already be valid on hosts outside UTC")
	assert.True(t, end.After(now), "SAS must not already be expired")
	assert.Equal(t, 7*time.Minute, end.Sub(start))
	assert.WithinDuration(t, now.Add(5*time.Minute), end, time.Second)
}

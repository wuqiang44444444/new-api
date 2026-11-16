package service

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoResultStoresPreserveInterruptedSourceClassification(t *testing.T) {
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(storage.Close)
	client, err := azblob.NewClientWithNoCredential(storage.URL, nil)
	require.NoError(t, err)
	azure := &azureBlobArtifactStore{client: client, container: "videos"}
	s3 := newS3VideoResultTestStore(t, newFakeS3VideoStore())

	for name, store := range map[string]videoResultObjectStore{"azure": azure, "s3": s3} {
		t.Run(name, func(t *testing.T) {
			source := videoResultSourceReader{io.MultiReader(strings.NewReader("partial"), iotest.ErrReader(io.ErrUnexpectedEOF))}
			size, err := store.putVideoResultObject(t.Context(), "videos/tasks/test/video", "video/mp4", source, 1024)
			require.ErrorIs(t, err, errVideoResultSourceRead)
			assert.Zero(t, size, "a partial file cannot be published")
		})
	}
}

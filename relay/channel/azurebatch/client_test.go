package azurebatch

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestVerifyConnectionPreservesPurposeAndVersion(t *testing.T) {
	c := NewClient("https://example.invalid", "test-key", "2025-01-01-preview")
	c.HTTPClient.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "/openai/files", r.URL.Path)
		assert.Equal(t, "batch", r.URL.Query().Get("purpose"))
		assert.Equal(t, "2025-01-01-preview", r.URL.Query().Get("api-version"))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	require.NoError(t, c.VerifyConnection(context.Background()))
}
func TestCreateClassificationKeepsUncertainResponses(t *testing.T) {
	for _, code := range []int{400, 429, 500, 503, 504} {
		c := NewClient("https://example.invalid", "test-key", "v1")
		c.HTTPClient.Transport = testTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		})
		_, err := c.CreateBatch(context.Background(), "file", "24h")
		require.Error(t, err)
		if code < 500 {
			assert.ErrorIs(t, err, ErrUpstreamRejected)
		} else {
			assert.NotErrorIs(t, err, ErrUpstreamRejected)
		}
	}
}

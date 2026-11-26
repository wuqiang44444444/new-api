package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type synchronousTextTestTransport func(*http.Request) (*http.Response, error)

func (f synchronousTextTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSynchronousTextResponseAcceptancePrecedesClientHandoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		downstream, cancel := context.WithCancel(t.Context())
		defer cancel()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://upstream.test", nil)
		require.NoError(t, err)

		upstream := WithAPIRequestEvidenceTransport(synchronousTextTestTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("usage"))}, nil
		}))
		var outbound context.Context
		client := &http.Client{Transport: synchronousTextTestTransport(func(req *http.Request) (*http.Response, error) {
			outbound = req.Context()
			resp, err := upstream.RoundTrip(req)
			cancel()
			synctest.Wait()
			return resp, err
		})}

		resp, err := DoSynchronousTextRequest(client, req, downstream)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.NoError(t, outbound.Err(), "cancellation ran before Client.Do returned")
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, "usage", string(body))
		require.NoError(t, resp.Body.Close())
		assert.ErrorIs(t, outbound.Err(), context.Canceled)
	})
}

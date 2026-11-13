package relay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncImageResultDownloadRetriesTransientFailure(t *testing.T) {
	setting := system_setting.GetFetchSetting()
	previous := *setting
	setting.EnableSSRFProtection = false
	t.Cleanup(func() { *setting = previous })
	service.InitHttpClient()

	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Empty(t, r.Header.Get("Authorization"))
		if gets.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = io.WriteString(w, "result bytes")
	}))
	defer server.Close()
	result, mime, err := downloadImageURL(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, "result bytes", string(result))
	assert.Equal(t, "image/png", mime)
	assert.EqualValues(t, 2, gets.Load())
}

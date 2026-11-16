package service

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionResponseBodyValidators(t *testing.T) {
	const original = `{"choices":[{"message":{"content":"answer"}}]}`
	const extended = `{"choices":[{"message":{"content":"answer","reasoning_content":null}}]}`
	for _, test := range []struct {
		name   string
		path   string
		status int
		input  string
		want   string
	}{
		{"patched", "/v1/chat/completions", http.StatusOK, original, extended},
		{"explicit null already present", "/v1/chat/completions", http.StatusOK, extended, extended},
		{"different endpoint", "/v1/completions", http.StatusOK, original, original},
		{"non-success", "/v1/chat/completions", http.StatusBadRequest, original, original},
	} {
		t.Run(test.name, func(t *testing.T) {
			checksum := sha256.Sum256([]byte(test.input))
			digest := "sha-256=:" + base64.StdEncoding.EncodeToString(checksum[:]) + ":"
			validators := map[string]string{
				"Content-Digest": digest,
				"Repr-Digest":    digest,
				"Digest":         "sha-256=fixture",
				"Content-MD5":    "fixture",
				"ETag":           `"upstream"`,
			}
			upstream := &http.Response{StatusCode: test.status, Header: make(http.Header)}
			for name, value := range validators {
				upstream.Header.Set(name, value)
			}
			upstream.Header.Set("Content-Length", strconv.Itoa(len(test.input)))
			upstream.Header.Set("X-Unrelated", "keep")
			upstream.Header.Set(common.RequestIdKey, "upstream-request")

			engine := gin.New()
			engine.POST(test.path, func(c *gin.Context) {
				c.Header(common.RequestIdKey, "gateway-request")
				IOCopyBytesGracefully(c, upstream, []byte(test.input))
				assert.Equal(t, "upstream-request", c.GetString(common.UpstreamRequestIdKey))
			})
			recorder := httptest.NewRecorder()
			engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, test.path, nil))
			require.Equal(t, test.status, recorder.Code)
			require.Equal(t, test.want, recorder.Body.String())
			assert.Equal(t, strconv.Itoa(len(test.want)), recorder.Header().Get("Content-Length"))
			assert.Equal(t, "keep", recorder.Header().Get("X-Unrelated"))
			assert.Equal(t, "gateway-request", recorder.Header().Get(common.RequestIdKey))
			for name, value := range validators {
				if test.input != test.want {
					assert.Empty(t, recorder.Header().Values(name), name)
				} else {
					assert.Equal(t, []string{value}, recorder.Header().Values(name), name)
				}
				assert.Equal(t, value, upstream.Header.Get(name))
			}
		})
	}
}

package moxingimage

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImageResultContractMatchesWorker(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		valid      bool
		tokens     int
		count      *int
	}{
		{"absent usage", `{"data":[{"url":"https://example.com/a.png"}]}`, true, 0, nil},
		{"numeric usage", `{"data":[{"url":"https://example.com/a.png"}],"usage":{"output_tokens":123,"input_images":2}}`, true, 123, common.GetPointer(2)},
		{"string zero", `{"data":[{"url":"https://example.com/a.png"}],"usage":{"output_tokens":"0","input_images":"0"}}`, true, 0, common.GetPointer(0)},
		{"multiple", `{"data":[{"url":"https://example.com/a.png"},{"url":"https://example.com/b.png"}]}`, false, 0, nil},
		{"mixed", `{"data":[{"url":"https://example.com/a.png"},{"url":"bad"}]}`, false, 0, nil},
		{"empty", `{"data":[]}`, false, 0, nil},
		{"negative usage", `{"data":[{"url":"https://example.com/a.png"}],"usage":{"output_tokens":-1}}`, false, 0, nil},
		{"overflow usage", `{"data":[{"url":"https://example.com/a.png"}],"usage":{"output_tokens":9223372036854775808}}`, false, 0, nil},
		{"fraction count", `{"data":[{"url":"https://example.com/a.png"}],"usage":{"input_images":1.5}}`, false, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, tc.body) }))
			defer server.Close()
			info := moxingImageInfo(server.URL)
			c, _ := moxingImageContext(context.Background())
			syncUsage, syncErr := (&Adaptor{}).DoResponse(c, testHTTPResponse(200, tc.body), info)
			urls, workerUsage, workerErr := HeadlessGenerate(context.Background(), info, nil, strings.NewReader(`{}`))
			if !tc.valid {
				require.NotNil(t, syncErr)
				require.NotNil(t, workerErr)
				return
			}
			require.Nil(t, syncErr)
			require.Nil(t, workerErr)
			require.Len(t, urls, 1)
			got := syncUsage.(*dto.Usage)
			assert.Equal(t, tc.tokens, got.CompletionTokens)
			assert.Equal(t, tc.count, got.InputImages)
			if workerUsage != nil {
				assert.Equal(t, got, workerUsage)
			}
		})
	}
}

package channel

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoRequestCapturesProviderRequestIdentityAndClearsPreviousAttempt(t *testing.T) {
	service.InitHttpClient()
	for _, tc := range []struct {
		name        string
		channelType int
		headers     map[string]string
		want        string
	}{
		{"Azure", constant.ChannelTypeAzure, map[string]string{"apim-request-id": "azure-request"}, "azure-request"},
		{"OpenAI", constant.ChannelTypeOpenAI, map[string]string{"x-request-id": "openai-request"}, "openai-request"},
		{"gateway identity", constant.ChannelTypeAzure, map[string]string{common.RequestIdKey: "gateway-request", "apim-request-id": "azure-request"}, "gateway-request"},
		{"Azure header priority", constant.ChannelTypeAzure, map[string]string{"apim-request-id": "azure-request", "x-request-id": "other-request"}, "azure-request"},
		{"missing", constant.ChannelTypeAzure, nil, ""},
		{"oversized", constant.ChannelTypeAzure, map[string]string{"apim-request-id": strings.Repeat("x", 129)}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for key, value := range tc.headers {
					w.Header().Set(key, value)
				}
				_, _ = io.WriteString(w, `{}`)
			}))
			defer server.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
			c.Set(common.UpstreamRequestIdKey, "previous-attempt")
			req, err := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(`{}`))
			require.NoError(t, err)
			resp, err := doRequest(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: tc.channelType}})
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, tc.want, c.GetString(common.UpstreamRequestIdKey))
		})
	}
}

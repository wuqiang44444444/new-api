package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatementSnapshotPreservesPublicModelSeparatelyFromPricing(t *testing.T) {
	other := model.NewLogOther()
	appendBillingStatementSnapshot(&relaycommon.RelayInfo{OriginModelName: "public-model", BillingModelName: "price-model", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "provider-model"}}, other)
	admin := other.Snapshot()["admin_info"].(map[string]interface{})
	snapshot := admin["statement_snapshot"].(map[string]interface{})
	assert.Equal(t, "public-model", snapshot["customer_model"])
	assert.Equal(t, "provider-model", snapshot["provider_model"])
}

func TestBatchCompletionCarriesCachedInputIntoCustomerStatement(t *testing.T) {
	c, request, _ := batchLifecycleFixture(t, 200, `{"id":"provider-1","status":"validating"}`)
	transport := http.DefaultTransport
	http.DefaultTransport = batchRoundTrip(func(r *http.Request) (*http.Response, error) {
		response, err := transport.RoundTrip(r)
		if err == nil && strings.Contains(r.URL.Path, "out-1/content") {
			require.NoError(t, response.Body.Close())
			response.Body = io.NopCloser(strings.NewReader(`{"custom_id":"a","response":{"status_code":200,"body":{"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":8}}}}}` + "\n"))
		}
		return response, err
	})
	result, err := CreateBatchJob(c, request)
	require.NoError(t, err)
	require.NoError(t, progressBatchJob(context.Background(), result.Job))
	s, err := model.GetBillingCustomerStatement(1701, 1, common.GetTimestamp()+10, "api_key", 0, "", "")
	require.NoError(t, err)
	assert.EqualValues(t, 10, s.Summary.InputTokens)
	assert.EqualValues(t, 8, s.Summary.CacheReadTokens)
	assert.EqualValues(t, 20, s.Summary.NetQuota)
}

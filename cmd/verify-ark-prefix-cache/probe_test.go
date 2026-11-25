package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExperimentRecordsExplicitCacheLifecycle(t *testing.T) {
	now := time.Now()
	expiry := now.Add(time.Hour)
	plan := experimentPlan{
		Run: "reuse-comparison", Endpoint: "ep-isolated-test",
		Steps: []experimentStep{
			{Name: "warm", Operation: "create", At: now, InputRepeats: 32, ExpireAt: expiry},
			{Name: "hit", Operation: "reuse", At: now, Prefix: "warm"},
			{Name: "delete-original", Operation: "delete", At: now, Prefix: "warm"},
			{Name: "delete-child", Operation: "delete", At: now, Prefix: "hit"},
		},
	}
	require.NoError(t, plan.validate(now.Add(-time.Second)))
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer private-probe-key", r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Idempotency-Key"))
		switch calls.Add(1) {
		case 1, 2:
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "/responses", r.URL.Path)
			var request struct {
				Model            string `json:"model"`
				PreviousResponse string `json:"previous_response_id"`
				Store            bool   `json:"store"`
				ExpireAt         int64  `json:"expire_at"`
				Caching          struct {
					Type   string `json:"type"`
					Prefix bool   `json:"prefix"`
				} `json:"caching"`
			}
			if !assert.NoError(t, common.DecodeJsonBody(r.Body, &request)) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			assert.Equal(t, plan.Endpoint, request.Model)
			assert.True(t, request.Store)
			assert.Equal(t, "enabled", request.Caching.Type)
			if calls.Load() == 1 {
				assert.True(t, request.Caching.Prefix)
				assert.Equal(t, expiry.Unix(), request.ExpireAt)
				assert.Empty(t, request.PreviousResponse)
				fmt.Fprintf(w, `{"id":"resp_private_prefix","object":"response","status":"completed","model":%q,"created_at":%d,"expire_at":%d,"usage":{"input_tokens":768,"output_tokens":0},"metadata":{"private":"never exported"}}`, seedLiteModel, now.Unix(), expiry.Unix())
				return
			}
			assert.False(t, request.Caching.Prefix)
			assert.Zero(t, request.ExpireAt)
			assert.Equal(t, "resp_private_prefix", request.PreviousResponse)
			fmt.Fprintf(w, `{"id":"resp_private_child","object":"response","status":"completed","model":%q,"usage":{"input_tokens":780,"output_tokens":1,"input_tokens_details":{"cached_tokens":700}}}`, seedLiteModel)
		case 3, 4:
			assert.Equal(t, http.MethodDelete, r.Method)
			expected := "resp_private_prefix"
			if calls.Load() == 4 {
				expected = "resp_private_child"
			}
			assert.Equal(t, "/responses/"+expected, r.URL.Path)
			fmt.Fprintf(w, `{"id":%q,"object":"response","deleted":true}`, expected)
		default:
			t.Errorf("unexpected outbound request")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)

	var output bytes.Buffer
	require.NoError(t, runExperiment(t.Context(), server.Client(), server.URL+"/responses", "private-probe-key", plan, &output))
	assert.Equal(t, int32(4), calls.Load())
	for _, secret := range []string{"private-probe-key", plan.Endpoint, "resp_private_prefix", "resp_private_child", "never exported", syntheticLine} {
		assert.NotContains(t, output.String(), secret)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	require.Len(t, lines, 4)
	var created, reused, deleted observation
	require.NoError(t, common.UnmarshalJsonStr(lines[0], &created))
	require.NoError(t, common.UnmarshalJsonStr(lines[1], &reused))
	require.NoError(t, common.UnmarshalJsonStr(lines[2], &deleted))
	assert.Equal(t, "confirmed", created.Outcome)
	assert.Equal(t, expiry.Unix(), created.ExpireAt)
	assert.True(t, created.ModelConfirmed)
	require.NotNil(t, created.Usage)
	require.NotNil(t, created.Usage.OutputTokens)
	assert.Zero(t, *created.Usage.OutputTokens)
	require.NotNil(t, reused.Usage)
	require.NotNil(t, reused.Usage.InputDetails)
	require.NotNil(t, reused.Usage.InputDetails.CachedTokens)
	assert.Equal(t, 700, *reused.Usage.InputDetails.CachedTokens)
	assert.Len(t, created.ResponseIDDigest, 64)
	assert.Equal(t, created.ResponseIDDigest, deleted.ResponseIDDigest)
}

func TestExperimentStopsOnUnconfirmedResponse(t *testing.T) {
	now := time.Now()
	plan := experimentPlan{Run: "unknown-create", Endpoint: "ep-isolated-test", Steps: []experimentStep{
		{Name: "first", Operation: "create", At: now, InputRepeats: 32, ExpireAt: now.Add(time.Hour)},
		{Name: "second", Operation: "create", At: now, InputRepeats: 32, ExpireAt: now.Add(time.Hour)},
	}}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		fmt.Fprint(w, `{"id":"resp_private_unconfirmed","object":"response","status":"incomplete","model":"doubao-seed-2-1-lite-260915"}`)
	}))
	t.Cleanup(server.Close)

	var output bytes.Buffer
	err := runExperiment(t.Context(), server.Client(), server.URL, "private-probe-key", plan, &output)
	require.ErrorContains(t, err, "unconfirmed_response")
	assert.Equal(t, int32(1), calls.Load())
	assert.NotContains(t, output.String(), "resp_private_unconfirmed")
	assert.NotContains(t, output.String(), "second")
}

package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync/atomic"
)

func TestChannelTestPersistsNativePricingWithoutCustomerCharge(t *testing.T) {
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousRedis, previousMemory := common.RedisEnabled, common.MemoryCacheEnabled
	previousLog, previousExport := common.LogConsumeEnabled, common.DataExportEnabled
	mainType, logType := common.MainDatabaseType(), common.LogDatabaseType()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.RedisEnabled, common.MemoryCacheEnabled = previousRedis, previousMemory
		common.LogConsumeEnabled, common.DataExportEnabled = previousLog, previousExport
		common.SetDatabaseTypes(mainType, logType)
	})
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Log{}))
	common.MemoryCacheEnabled, common.LogConsumeEnabled, common.DataExportEnabled = false, true, false
	group := ratio_setting.GroupRatio2JSONString()
	models, err := common.Marshal(ratio_setting.GetModelRatioCopy())
	require.NoError(t, err)
	completion, err := common.Marshal(ratio_setting.GetCompletionRatioCopy())
	require.NoError(t, err)
	cache := ratio_setting.CacheRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(group))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(string(models)))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(string(completion)))
		require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(cache))
	})
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":3}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"billing-test-evidence":1}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"billing-test-evidence":2}`))
	require.NoError(t, ratio_setting.UpdateCacheRatioByJSONString(`{"billing-test-evidence":0.1}`))
	user := &model.User{Username: "billing-operator", Group: "default", Status: common.UserStatusEnabled, Quota: 1000000}
	require.NoError(t, db.Create(user).Error)
	for _, tc := range []struct {
		name, usage string
		original    int
	}{
		{"cache hit", `,"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":{"cached_tokens":80}}`, 48},
		{"explicit no cache", `,"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110,"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":0}}`, 120},
		{"optional cache details omitted", `,"usage":{"prompt_tokens":100,"completion_tokens":10,"total_tokens":110}`, 120},
		{"missing complete usage", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/v1/chat/completions", r.URL.Path)
				var request map[string]any
				if !assert.NoError(t, common.DecodeJson(r.Body, &request)) {
					return
				}
				assert.Equal(t, []any{map[string]any{"role": "user", "content": "hi"}}, request["messages"])
				assert.NotContains(t, request, "cache_control")
				assert.NotContains(t, request, "prompt_cache_key")
				w.Header().Set("Content-Type", "application/json")
				body := `{"id":"fixture","model":"billing-test-evidence","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]`
				body += tc.usage
				_, _ = w.Write([]byte(body + `}`))
			}))
			t.Cleanup(server.Close)
			channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "billing fixture", Key: "fixture", Models: "billing-test-evidence", Group: "default", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(server.URL)}
			require.NoError(t, db.Create(channel).Error)
			result := testChannel(context.Background(), channel, user.Id, "billing-test-evidence", string(constant.EndpointTypeOpenAI), false)
			require.NoError(t, result.localErr)
			require.Nil(t, result.newAPIError)
			var log model.Log
			require.NoError(t, db.Where("channel_id = ? AND type = ?", channel.Id, model.LogTypeConsume).First(&log).Error)
			var facts struct {
				CacheWrite *int `json:"cache_write_tokens"`
				Pricing    struct {
					Version  int    `json:"version"`
					Status   string `json:"status"`
					Original *int   `json:"original_quota"`
				} `json:"test_pricing"`
			}
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &facts))
			assert.Equal(t, 1, facts.Pricing.Version)
			if tc.usage != "" {
				assert.Equal(t, "settled", facts.Pricing.Status)
				assert.Equal(t, "priced", service.ChannelTestUpstreamCostStatus(result.context))
				require.NotNil(t, facts.Pricing.Original)
				assert.Equal(t, tc.original, *facts.Pricing.Original, "native price excludes the operator group factor")
				assert.Equal(t, tc.original, log.Quota)
				require.NotNil(t, facts.CacheWrite)
				assert.Zero(t, *facts.CacheWrite)
			} else {
				assert.Equal(t, "estimated", facts.Pricing.Status)
				assert.Equal(t, "pending", service.ChannelTestUpstreamCostStatus(result.context))
				assert.Nil(t, facts.Pricing.Original)
			}
			var saved model.User
			require.NoError(t, db.First(&saved, user.Id).Error)
			assert.Equal(t, user.Quota, saved.Quota)
			assert.Zero(t, saved.UsedQuota)
			assert.Zero(t, saved.RequestCount)
			var savedChannel model.Channel
			require.NoError(t, db.First(&savedChannel, channel.Id).Error)
			assert.Zero(t, savedChannel.UsedQuota)
		})
	}

	for _, tc := range []struct {
		name, testModel, body, costStatus string
		status, calls                     int
	}{
		{"local price rejection", "unconfigured-price-fixture", "", "not_sent", 200, 0},
		{"upstream rejects request", "billing-test-evidence", `{"error":{"message":"unsupported operation","type":"invalid_request_error"}}`, "pending", 400, 1},
		{"invalid response after send", "billing-test-evidence", `invalid-json`, "pending", 200, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Name: "cost evidence fixture", Key: "fixture", Models: tc.testModel, Group: "default", Status: common.ChannelStatusEnabled, BaseURL: common.GetPointer(server.URL)}
			require.NoError(t, db.Create(channel).Error)
			result := testChannel(context.Background(), channel, user.Id, tc.testModel, string(constant.EndpointTypeOpenAI), false)
			require.Error(t, result.localErr)
			assert.EqualValues(t, tc.calls, calls.Load())
			assert.Equal(t, tc.costStatus, service.ChannelTestUpstreamCostStatus(result.context))
			check := channelAutoCheckResult(channel, result, "generation_probe")
			assert.Equal(t, tc.costStatus, check.Detail[service.ChannelTestUpstreamCostKey])
			var count int64
			require.NoError(t, db.Model(&model.Log{}).Where("channel_id = ? AND type = ?", channel.Id, model.LogTypeConsume).Count(&count).Error)
			assert.Zero(t, count, "failed tests retain audit evidence without fabricating a zero-cost consume record")
		})
	}
}

package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/perf_metrics_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultChannelAffinityResponsesRequiresCompletedStream(t *testing.T) {
	relay := defaultAffinityStreamFixture(t, "/v1/responses", types.RelayFormatOpenAIResponses, "", func(w http.ResponseWriter, body string) {
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n")
		if strings.Contains(body, "complete-fixture") {
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"fixture\",\"status\":\"completed\",\"usage\":{\"input_tokens\":5,\"output_tokens\":1,\"total_tokens\":6}}}\n\n")
		}
	})
	partial := `{"model":"gpt-4o","input":"partial-fixture","stream":true}`
	complete := `{"model":"gpt-4o","input":"complete-fixture","stream":true}`
	relay(partial, "response.output_text.delta", "binding_missing")
	relay(complete, "response.completed", "binding_missing")
	relay(complete, "response.completed", "binding_used")
}

func TestDefaultChannelAffinityChatRequiresAllChoices(t *testing.T) {
	for _, tc := range []struct {
		name, override  string
		requested, sent int
	}{
		{"unchanged", "", 2, 2},
		{"overridden_to_two", `{"n":2}`, 1, 2},
		{"overridden_to_one", `{"n":1}`, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			relay := defaultAffinityStreamFixture(t, "/v1/chat/completions", types.RelayFormatOpenAI, tc.override, func(w http.ResponseWriter, body string) {
				var upstream struct {
					N int `json:"n"`
				}
				if !assert.NoError(t, common.UnmarshalJsonStr(body, &upstream)) || !assert.Equal(t, tc.sent, upstream.N) {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				_, _ = fmt.Fprint(w, `data: {"id":"fixture","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{"content":"one"},"finish_reason":"stop"}]}`+"\n\n")
				if tc.sent == 2 {
					// Repeated finish_reason for index 0 must not count as a second choice.
					_, _ = fmt.Fprint(w, `data: {"id":"fixture","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"},{"index":1,"delta":{"content":"par"},"finish_reason":null}]}`+"\n\n")
					if strings.Contains(body, "complete-fixture") {
						_, _ = fmt.Fprint(w, `data: {"id":"fixture","object":"chat.completion.chunk","model":"gpt-4o","choices":[{"index":1,"delta":{"content":"tial"},"finish_reason":"stop"}]}`+"\n\n")
					}
				}
			})
			const body = `{"model":"gpt-4o","messages":[{"role":"user","content":%q}],"n":%d,"stream":true}`
			if tc.sent == 2 {
				relay(fmt.Sprintf(body, "partial-fixture", tc.requested), "chat.completion.chunk", "binding_missing")
			}
			complete := fmt.Sprintf(body, "complete-fixture", tc.requested)
			relay(complete, "chat.completion.chunk", "binding_missing")
			relay(complete, "chat.completion.chunk", "binding_used")
		})
	}
}

func defaultAffinityStreamFixture(t *testing.T, path string, format types.RelayFormat, override string, stream func(http.ResponseWriter, string)) func(string, string, string) {
	t.Helper()
	user, token := setupResponsesWSRequestTest(t)
	// Each fixture has its own database, but shares the process affinity cache.
	require.NoError(t, model.DB.Model(token).Update("id", user.Id).Error)
	token.Id = user.Id
	setting := operation_setting.GetChannelAffinitySetting()
	previousSetting := *setting
	previousTimeout, previousCount := constant.StreamingTimeout, constant.CountToken
	previousBatch, previousQuota := common.BatchUpdateEnabled, common.QuotaPerUnit
	previousModelRatios := ratio_setting.ModelRatio2JSONString()
	previousPerfEnabled := perf_metrics_setting.GetSetting().Enabled
	require.NoError(t, i18n.Init())
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o":1}`))
	setting.Enabled = true
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"perf_metrics_setting.enabled": "false"}))
	constant.StreamingTimeout, constant.CountToken = 30, false
	common.BatchUpdateEnabled, common.QuotaPerUnit = false, 500_000
	t.Cleanup(func() {
		*setting = previousSetting
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"perf_metrics_setting.enabled": fmt.Sprint(previousPerfEnabled)}))
		constant.StreamingTimeout, constant.CountToken = previousTimeout, previousCount
		common.BatchUpdateEnabled, common.QuotaPerUnit = previousBatch, previousQuota
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(previousModelRatios))
	})
	require.NoError(t, model.DB.Model(user).Updates(map[string]any{"quota": 1_000_000, "setting": `{"billing_preference":"wallet_only"}`}).Error)
	require.NoError(t, model.DB.Model(token).Update("remain_quota", 1_000_000).Error)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		stream(w, string(body))
	}))
	t.Cleanup(upstream.Close)
	channel := model.Channel{Name: "default-affinity-fixture", Type: constant.ChannelTypeOpenAI, Key: "fixture", Status: common.ChannelStatusEnabled, Group: "default", Models: "gpt-4o", BaseURL: &upstream.URL}
	if override != "" {
		channel.ParamOverride = &override
	}
	require.NoError(t, model.DB.Create(&channel).Error)
	require.NoError(t, model.DB.Create(&model.Ability{ChannelId: channel.Id, Model: "gpt-4o", Group: "default", Enabled: true}).Error)

	var events []service.PolicyEvent
	engine := gin.New()
	engine.POST(path, middleware.TokenAuth(), func(c *gin.Context) {
		defer common.CleanupBodyStorage(c)
		c.Next()
		events = service.RequestPolicy(c).Events()
	}, middleware.Distribute(), func(c *gin.Context) { Relay(c, format) })
	return func(body, chunk, decision string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer sk-"+token.Key)
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.Contains(t, response.Body.String(), chunk)
		assert.True(t, slices.ContainsFunc(events, func(event service.PolicyEvent) bool {
			return event.Decision.Source == "default_channel_affinity" && event.Decision.Reason == decision
		}), "expected %s in routing events: %+v", decision, events)
	}
}

package controller

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesStreamProtocolOutcomeAndBilling(t *testing.T) {
	for _, tc := range []struct {
		name, modelName, finish, status  string
		wantInput, wantOutput, wantQuota int
	}{
		{"Claude normal", "claude-sonnet-4-6", "end_turn", "completed", 100, 50, 300},
		{"Claude budget", "claude-sonnet-4-6", "max_tokens", "incomplete", 100, 50, 300},
		{"Claude pause", "claude-sonnet-4-6", "pause_turn", "incomplete", 100, 50, 300},
		{"Claude missing stop", "claude-sonnet-4-6", "", "failed", 100, 50, 300},
		{"Gemini normal", "gemini-3.5-flash", "STOP", "completed", 73, 124, 613},
		{"Gemini budget", "gemini-3.5-flash", "MAX_TOKENS", "incomplete", 73, 124, 613},
		{"Gemini missing finish reason", "gemini-3.5-flash", "", "failed", 73, 124, 613},
	} {
		t.Run(tc.name, func(t *testing.T) {
			channelType := constant.ChannelTypeAnthropic
			frames := []string{
				`{"type":"message_start","message":{"id":"msg_fixture","model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":0}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"summary"}}`,
				`{"type":"content_block_stop","index":0}`,
				fmt.Sprintf(`{"type":"message_delta","delta":{"stop_reason":%q},"usage":{"output_tokens":50,"output_tokens_details":{"thinking_tokens":30}}}`, tc.finish),
			}
			if tc.finish != "" {
				frames = append(frames, `{"type":"message_stop"}`)
			}
			if tc.modelName == "gemini-3.5-flash" {
				channelType = constant.ChannelTypeGemini
				frames = []string{fmt.Sprintf(`{"candidates":[{"finishReason":%q,"content":{"role":"model","parts":[{"text":"answer"}]}}],"usageMetadata":{"promptTokenCount":73,"candidatesTokenCount":6,"thoughtsTokenCount":118,"totalTokenCount":197}}`, tc.finish)}
			}
			fx, recorder, calls := runResponsesContractRequest(t, channelType, tc.modelName, `"input":"hi","stream":true`, frames)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			require.EqualValues(t, 1, calls)
			var terminal *dto.OpenAIResponsesResponse
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				data, ok := strings.CutPrefix(line, "data: ")
				if !ok || data == "[DONE]" {
					continue
				}
				var event dto.ResponsesStreamResponse
				require.NoError(t, common.UnmarshalJsonStr(data, &event))
				switch event.Type {
				case "response.completed", "response.incomplete", "response.failed":
					require.Nil(t, terminal, "emit a terminal event once")
					assert.Equal(t, "response."+tc.status, event.Type)
					terminal = event.Response
				}
			}
			require.NotNil(t, terminal, recorder.Body.String())
			assert.JSONEq(t, fmt.Sprintf("%q", tc.status), string(terminal.Status))
			require.NotNil(t, terminal.Usage)
			assert.Equal(t, tc.wantInput, terminal.Usage.InputTokens)
			assert.Equal(t, tc.wantOutput, terminal.Usage.OutputTokens)
			if tc.status == "incomplete" {
				require.NotNil(t, terminal.IncompleteDetails)
				assert.Equal(t, "max_output_tokens", terminal.IncompleteDetails.Reason)
			}
			if channelType == constant.ChannelTypeAnthropic {
				assert.Contains(t, recorder.Body.String(), "response.reasoning_summary_text.delta")
				require.NotNil(t, terminal.Usage.OutputTokensDetails)
				assert.Equal(t, 30, terminal.Usage.OutputTokensDetails.ReasoningTokens)
				assert.Equal(t, 30, terminal.Usage.CompletionTokenDetails.ReasoningTokens)
			}

			var logs []model.Log
			require.NoError(t, fx.db.Where("request_id = ? AND type = ?", "responses-compatibility", model.LogTypeConsume).Find(&logs).Error)
			require.Len(t, logs, 1)
			var other struct {
				Stream struct {
					Status         string   `json:"status"`
					EndReason      string   `json:"end_reason"`
					ResponseStatus string   `json:"response_status"`
					Errors         []string `json:"errors"`
				} `json:"stream_status"`
				Admin struct {
					Policy []struct {
						Decision struct {
							Action string `json:"action"`
						} `json:"decision"`
					} `json:"request_policy"`
				} `json:"admin_info"`
			}
			require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
			assert.Equal(t, tc.status, other.Stream.ResponseStatus)
			assert.Equal(t, "eof", other.Stream.EndReason)
			wantHealth, wantPolicy := "ok", "success"
			if tc.status == "failed" {
				wantHealth, wantPolicy = "error", "stop"
				assert.Contains(t, other.Stream.Errors, "response_failed")
			} else {
				assert.Empty(t, other.Stream.Errors)
			}
			assert.Equal(t, wantHealth, other.Stream.Status)
			require.NotEmpty(t, other.Admin.Policy)
			assert.Equal(t, wantPolicy, other.Admin.Policy[len(other.Admin.Policy)-1].Decision.Action)
			assert.Equal(t, tc.wantInput, logs[0].PromptTokens)
			assert.Equal(t, tc.wantOutput, logs[0].CompletionTokens)
			assert.Equal(t, tc.wantQuota, logs[0].Quota)
			var user model.User
			var token model.Token
			require.NoError(t, fx.db.First(&user, fx.user.Id).Error)
			require.NoError(t, fx.db.First(&token, fx.token.Id).Error)
			assert.Equal(t, fx.user.Quota-tc.wantQuota, user.Quota)
			assert.Equal(t, fx.token.RemainQuota-tc.wantQuota, token.RemainQuota)
		})
	}
}

func TestResponsesRejectUnsupportedStateFields(t *testing.T) {
	for _, provider := range []struct {
		modelName   string
		channelType int
	}{
		{"claude-sonnet-4-6", constant.ChannelTypeAnthropic},
		{"gemini-3.5-flash", constant.ChannelTypeGemini},
	} {
		for _, field := range []struct{ name, value string }{
			{"previous_response_id", `"resp_fixture"`},
			{"conversation", `{"id":"conv_fixture"}`},
		} {
			t.Run(provider.modelName+"/"+field.name, func(t *testing.T) {
				part := fmt.Sprintf(`"input":"hi",%q:%s`, field.name, field.value)
				fx, recorder, calls := runResponsesContractRequest(t, provider.channelType, provider.modelName, part, nil)
				require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
				assert.Contains(t, recorder.Body.String(), field.name)
				assert.Contains(t, recorder.Body.String(), "convert_request_failed")
				assert.Zero(t, calls)
				var consumes int64
				require.NoError(t, fx.db.Model(&model.Log{}).Where("request_id = ? AND type = ?", "responses-compatibility", model.LogTypeConsume).Count(&consumes).Error)
				assert.Zero(t, consumes)
			})
		}
	}
}

func runResponsesContractRequest(t *testing.T, channelType int, modelName, requestPart string, frames []string, wrap ...func(*httptest.ResponseRecorder) http.ResponseWriter) (*nativeDiagFixture, *httptest.ResponseRecorder, int32) {
	t.Helper()
	fx := newNativeRelayErrorFixture(t)
	oldLog, oldTimeout, oldQuota := common.LogConsumeEnabled, constant.StreamingTimeout, common.QuotaPerUnit
	common.LogConsumeEnabled, constant.StreamingTimeout, common.QuotaPerUnit = true, 10, 500000
	t.Cleanup(func() {
		common.LogConsumeEnabled, constant.StreamingTimeout, common.QuotaPerUnit = oldLog, oldTimeout, oldQuota
	})
	withTieredBillingConfig(t, map[string]string{modelName: "tiered_expr"}, map[string]string{modelName: `tier("standard",p*1.5+c*9)`})
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.Contains(requestPart, `"stream":true`) && len(frames) == 1 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, frames[0])
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range frames {
			_, _ = io.WriteString(w, "data: "+frame+"\n\n")
		}
	}))
	t.Cleanup(upstream.Close)
	selected := model.Channel{Id: 8913, Type: channelType, Key: "fixture", BaseURL: &upstream.URL, Status: common.ChannelStatusEnabled, Group: "default", Models: modelName}
	require.NoError(t, fx.db.Create(&selected).Error)
	router := gin.New()
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set(common.RequestIdKey, "responses-compatibility")
		common.SetContextKey(c, constant.ContextKeyUserId, fx.user.Id)
		common.SetContextKey(c, constant.ContextKeyUserQuota, fx.user.Quota)
		common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
		common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
		common.SetContextKey(c, constant.ContextKeyTokenId, fx.token.Id)
		common.SetContextKey(c, constant.ContextKeyTokenKey, fx.token.Key)
		require.Nil(t, middleware.SetupContextForSelectedChannel(c, &selected, modelName))
		Relay(c, types.RelayFormatOpenAIResponses)
	})
	body := fmt.Sprintf(`{"model":%q,"max_output_tokens":128,%s}`, modelName, requestPart)
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	var writer http.ResponseWriter = recorder
	if len(wrap) != 0 {
		writer = wrap[0](recorder)
	}
	router.ServeHTTP(writer, request)
	return fx, recorder, calls.Load()
}

func TestResponsesTerminalWriteDisconnectPreservesBilling(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		channelType          int
		model                string
		frames               []string
		input, output, quota int
	}{
		{"Claude", constant.ChannelTypeAnthropic, "claude-sonnet-4-6", []string{
			`{"type":"message_start","message":{"id":"msg_fixture","model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":0}}}`,
			`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":50}}`,
			`{"type":"message_stop"}`,
		}, 100, 50, 300},
		{"Gemini", constant.ChannelTypeGemini, "gemini-3.5-flash", []string{
			`{"candidates":[{"finishReason":"STOP","content":{"role":"model","parts":[{"text":"answer"}]}}]}`,
			`{"usageMetadata":{"promptTokenCount":73,"candidatesTokenCount":6,"thoughtsTokenCount":118,"totalTokenCount":197}}`,
		}, 73, 124, 613},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var broken *brokenResponsesTerminalWriter
			fx, _, calls := runResponsesContractRequest(t, tc.channelType, tc.model, `"input":"hi","stream":true`, tc.frames, func(rec *httptest.ResponseRecorder) http.ResponseWriter {
				broken = &brokenResponsesTerminalWriter{ResponseRecorder: rec}
				return broken
			})
			require.True(t, broken.failed)
			require.EqualValues(t, 1, calls)
			require.NotContains(t, broken.Body.String(), "event: response.completed")
			var log model.Log
			require.NoError(t, fx.db.Where("request_id = ? AND type = ?", "responses-compatibility", model.LogTypeConsume).Take(&log).Error)
			assert.Equal(t, tc.input, log.PromptTokens)
			assert.Equal(t, tc.output, log.CompletionTokens)
			assert.Equal(t, tc.quota, log.Quota)

			var other struct {
				Stream map[string]any `json:"stream_status"`
			}
			require.NoError(t, common.UnmarshalJsonStr(log.Other, &other))
			assert.Equal(t, "client_gone", other.Stream["end_reason"])
			assert.Equal(t, "completed", other.Stream["response_status"])
			assert.Contains(t, log.Other, "stream_not_successful")
		})
	}
}

type brokenResponsesTerminalWriter struct {
	*httptest.ResponseRecorder
	failed bool
}

func (w *brokenResponsesTerminalWriter) Write(p []byte) (int, error) {
	return w.WriteString(string(p))
}

func (w *brokenResponsesTerminalWriter) WriteString(s string) (int, error) {
	if strings.Contains(s, "event: response.completed") {
		w.failed = true
		return 0, syscall.EPIPE
	}
	return w.ResponseRecorder.WriteString(s)
}

func TestClaudeResponsesReasoningDetailsPresence(t *testing.T) {
	for _, mode := range []string{"JSON", "SSE"} {
		for _, tc := range []struct {
			name    string
			details string
			tokens  *int
		}{
			{name: "omitted"},
			{name: "explicit zero", details: `,"output_tokens_details":{"thinking_tokens":0}`, tokens: common.GetPointer(0)},
			{name: "reported", details: `,"output_tokens_details":{"thinking_tokens":30}`, tokens: common.GetPointer(30)},
		} {
			t.Run(mode+"/"+tc.name, func(t *testing.T) {
				jsonUsage := `"usage":{"output_tokens":50` + tc.details + `}`
				request := `"input":"hi"`
				frames := []string{`{"id":"msg_fixture","type":"message","model":"claude-sonnet-4-6","role":"assistant","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":50` + tc.details + `}}`}
				if mode == "SSE" {
					request += `,"stream":true`
					frames = []string{
						`{"type":"message_start","message":{"id":"msg_fixture","model":"claude-sonnet-4-6","usage":{"input_tokens":100,"output_tokens":0}}}`,
						`{"type":"message_delta","delta":{"stop_reason":"end_turn"},` + jsonUsage + `}`,
						`{"type":"message_stop"}`,
					}
				}
				fx, recorder, calls := runResponsesContractRequest(t, constant.ChannelTypeAnthropic, "claude-sonnet-4-6", request, frames)
				require.EqualValues(t, 1, calls)
				require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

				var response *dto.OpenAIResponsesResponse
				if mode == "JSON" {
					require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
				} else {
					for _, line := range strings.Split(recorder.Body.String(), "\n") {
						if data, ok := strings.CutPrefix(line, "data: "); ok {
							var event dto.ResponsesStreamResponse
							require.NoError(t, common.UnmarshalJsonStr(data, &event))
							if event.Type == "response.completed" {
								response = event.Response
							}
						}
					}
				}
				require.NotNil(t, response)
				require.NotNil(t, response.Usage)
				if tc.tokens == nil {
					assert.Nil(t, response.Usage.OutputTokensDetails)
				} else {
					require.NotNil(t, response.Usage.OutputTokensDetails)
					assert.Equal(t, *tc.tokens, response.Usage.OutputTokensDetails.ReasoningTokens)
					assert.Equal(t, *tc.tokens, response.Usage.CompletionTokenDetails.ReasoningTokens)
				}
				assert.Equal(t, 50, response.Usage.OutputTokens)
				var log model.Log
				require.NoError(t, fx.db.Where("request_id = ? AND type = ?", "responses-compatibility", model.LogTypeConsume).Take(&log).Error)
				assert.Equal(t, 300, log.Quota)
			})
		}
	}
}

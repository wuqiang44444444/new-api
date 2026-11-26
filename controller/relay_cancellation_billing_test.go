package controller

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeRetryClientCancellationBilling(t *testing.T) {
	testClaudeRetryClientCancellationBilling(t, "/v1/chat/completions", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"stream":true,"max_tokens":100}`, types.RelayFormatOpenAI)
}

func TestClaudeNativeRetryClientCancellationBilling(t *testing.T) {
	testClaudeRetryClientCancellationBilling(t, "/v1/messages", `{"model":"claude-sonnet-4-6","messages":[{"role":"user","content":"Hi"}],"stream":true,"max_tokens":100}`, types.RelayFormatClaude)
}

func TestClaudeResponsesRetryClientCancellationBilling(t *testing.T) {
	testClaudeRetryClientCancellationBilling(t, "/v1/responses", `{"model":"claude-sonnet-4-6","input":"Hi","stream":true,"max_output_tokens":100}`, types.RelayFormatOpenAIResponses)
}

func TestClaudeResponsesJSONClientCancellationBilling(t *testing.T) {
	testClaudeRetryClientCancellationBilling(t, "/v1/responses", `{"model":"claude-sonnet-4-6","input":"Hi","max_output_tokens":100}`, types.RelayFormatOpenAIResponses)
}

type cancelAtResponseHeaders struct {
	http.RoundTripper
	upstream string
	cancel   func()
}

func (t cancelAtResponseHeaders) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.RoundTripper.RoundTrip(req)
	if err == nil && resp.StatusCode == http.StatusOK && req.URL.Scheme+"://"+req.URL.Host == t.upstream {
		t.cancel()
	}
	return resp, err
}

func testClaudeRetryClientCancellationBilling(t *testing.T, path, body string, format types.RelayFormat) {
	t.Helper()
	const modelName = "claude-sonnet-4-6"
	const requestID = "chat-retry-client-cancellation"
	streaming := strings.Contains(body, `"stream":true`)

	for _, tc := range []struct {
		name             string
		acceptBeforeDrop bool
		wantQuota        int
	}{
		{name: "drop while waiting for retry headers"},
		{name: "drop after 200 before first write", acceptBeforeDrop: true, wantQuota: 40},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newNativeRelayErrorFixture(t)
			oldCache, oldRetry, oldConsume := common.MemoryCacheEnabled, common.RetryTimes, common.LogConsumeEnabled
			oldCount, oldStreamTimeout := constant.CountToken, constant.StreamingTimeout
			oldRanges := operation_setting.AutomaticRetryStatusCodeRanges
			setting := operation_setting.GetGeneralSetting()
			oldPing := setting.PingIntervalEnabled
			common.MemoryCacheEnabled, common.RetryTimes, common.LogConsumeEnabled = false, 2, true
			constant.CountToken, constant.StreamingTimeout = true, 10
			setting.PingIntervalEnabled = false
			operation_setting.AutomaticRetryStatusCodeRanges = []operation_setting.StatusCodeRange{{Start: 401, End: 407}, {Start: 500, End: 503}}
			t.Cleanup(func() {
				common.MemoryCacheEnabled, common.RetryTimes, common.LogConsumeEnabled = oldCache, oldRetry, oldConsume
				constant.CountToken, constant.StreamingTimeout = oldCount, oldStreamTimeout
				setting.PingIntervalEnabled = oldPing
				operation_setting.AutomaticRetryStatusCodeRanges = oldRanges
			})
			withTieredBillingConfig(t, map[string]string{modelName: "tiered_expr"}, map[string]string{modelName: `tier("standard", p * 2 + c * 4)`})
			downstream, cancel := context.WithCancel(context.Background())
			defer cancel()
			waiting := make(chan struct{})
			abandoned := make(chan struct{})
			release := make(chan struct{})
			sendJSON := make(chan struct{})
			var firstCalls, retryCalls, extraCalls atomic.Int32
			first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				firstCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"error":{"type":"not_found_error","message":"synthetic 404"}}`)
			}))
			t.Cleanup(first.Close)
			second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				retryCalls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				if tc.acceptBeforeDrop && !streaming {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.(http.Flusher).Flush()
					close(waiting)
					select {
					case <-sendJSON:
						_, _ = io.WriteString(w, `{"id":"msg_fixture","type":"message","model":"claude-sonnet-4-6","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":40,"output_tokens":0}}`)
					case <-r.Context().Done():
					case <-release:
					}
					return
				}
				if tc.acceptBeforeDrop {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_fixture\",\"model\":\"claude-sonnet-4-6\",\"usage\":{\"input_tokens\":40,\"output_tokens\":0}}}\n\n")
					w.(http.Flusher).Flush()
				}
				close(waiting)
				select {
				case <-r.Context().Done():
					close(abandoned)
				case <-release:
				}
			}))
			t.Cleanup(second.Close)
			if tc.acceptBeforeDrop && !streaming {
				client := service.GetHttpClient()
				base := client.Transport
				client.Transport = cancelAtResponseHeaders{RoundTripper: base, upstream: second.URL, cancel: func() {
					cancel()
					close(sendJSON)
				}}
				t.Cleanup(func() { client.Transport = base })
			}
			third := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				extraCalls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(third.Close)

			channels := make([]model.Channel, 0, 3)
			for i, upstream := range []*httptest.Server{first, second, third} {
				ch := model.Channel{
					Id: 9000 + i, Type: constant.ChannelTypeAnthropic, Key: "test-only", BaseURL: &upstream.URL,
					Status: common.ChannelStatusEnabled, Group: "default", Models: modelName, Priority: common.GetPointer(int64(3 - i)),
				}
				require.NoError(t, fx.db.Create(&ch).Error)
				require.NoError(t, ch.AddAbilities(fx.db))
				channels = append(channels, ch)
			}

			engine := gin.New()
			var usedChannels []string
			engine.POST(path, func(c *gin.Context) {
				c.Set(common.RequestIdKey, requestID)
				common.SetContextKey(c, constant.ContextKeyUserId, fx.user.Id)
				common.SetContextKey(c, constant.ContextKeyUserQuota, fx.user.Quota)
				common.SetContextKey(c, constant.ContextKeyUserGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUsingGroup, "default")
				common.SetContextKey(c, constant.ContextKeyUserSetting, dto.UserSetting{BillingPreference: "wallet_only"})
				common.SetContextKey(c, constant.ContextKeyTokenId, fx.token.Id)
				common.SetContextKey(c, constant.ContextKeyTokenKey, fx.token.Key)
				if !assert.Nil(t, middleware.SetupContextForSelectedChannel(c, &channels[0], modelName)) {
					return
				}
				Relay(c, format)
				usedChannels = c.GetStringSlice("use_channel")
			})
			recorder := &cancelBeforeSSEWrite{ResponseRecorder: httptest.NewRecorder()}
			if tc.acceptBeforeDrop && streaming {
				recorder.cancel = cancel
			}
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body)).WithContext(downstream)
			request.Header.Set("Content-Type", "application/json")
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				engine.ServeHTTP(recorder, request)
			}()
			defer func() {
				close(release)
				<-finished
			}()

			select {
			case <-waiting:
			case <-time.After(3 * time.Second):
				require.FailNow(t, "404 did not reach retry")
			}
			if !tc.acceptBeforeDrop {
				var reserved model.User
				var reservedToken model.Token
				require.NoError(t, fx.db.First(&reserved, fx.user.Id).Error)
				require.NoError(t, fx.db.First(&reservedToken, fx.token.Id).Error)
				require.Less(t, reserved.Quota, fx.user.Quota, "request must actually pre-consume before canceling")
				require.Less(t, reservedToken.RemainQuota, fx.token.RemainQuota)
				cancel()
			}
			if !tc.acceptBeforeDrop || streaming {
				select {
				case <-abandoned:
				case <-time.After(3 * time.Second):
					require.FailNow(t, "upstream connection stayed open after the client canceled")
				}
			}
			select {
			case <-finished:
			case <-time.After(3 * time.Second):
				require.FailNow(t, "relay did not finish after cancellation")
			}
			assert.EqualValues(t, 1, firstCalls.Load())
			assert.EqualValues(t, 1, retryCalls.Load())
			assert.Zero(t, extraCalls.Load())
			assert.Equal(t, []string{"9000", "9001"}, usedChannels)
			if streaming {
				assert.NotContains(t, recorder.Body.String(), "data:", "the client never received an SSE frame")
			}
			var consume []model.Log
			require.NoError(t, fx.db.Where("request_id = ? AND type = ?", requestID, model.LogTypeConsume).Find(&consume).Error)
			if tc.acceptBeforeDrop {
				if streaming {
					require.True(t, recorder.dropped, "client must cancel at the first attempted SSE write")
				} else {
					assert.ErrorIs(t, downstream.Err(), context.Canceled)
				}
				require.Len(t, consume, 1)
				assert.Equal(t, tc.wantQuota, consume[0].Quota)
				assert.Equal(t, 40, consume[0].PromptTokens)
				assert.Zero(t, consume[0].CompletionTokens)
				if streaming && format == types.RelayFormatOpenAIResponses {
					var other map[string]any
					require.NoError(t, common.UnmarshalJsonStr(consume[0].Other, &other))
					streamStatus, ok := other["stream_status"].(map[string]any)
					require.True(t, ok)
					assert.Equal(t, "client_gone", streamStatus["end_reason"])
					assert.NotEqual(t, "failed", streamStatus["response_status"])
					assert.NotContains(t, streamStatus, "errors")
				}
			} else {
				assert.Empty(t, consume)
			}
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				var user model.User
				var token model.Token
				assert.NoError(c, fx.db.First(&user, fx.user.Id).Error)
				assert.NoError(c, fx.db.First(&token, fx.token.Id).Error)
				assert.Equal(c, fx.user.Quota-tc.wantQuota, user.Quota)
				assert.Equal(c, fx.token.RemainQuota-tc.wantQuota, token.RemainQuota)
			}, 3*time.Second, 10*time.Millisecond)
		})
	}
}

type cancelBeforeSSEWrite struct {
	*httptest.ResponseRecorder
	cancel  context.CancelFunc
	dropped bool
}

func (w *cancelBeforeSSEWrite) Write(data []byte) (int, error) {
	if w.drop(string(data)) {
		return len(data), nil
	}
	return w.ResponseRecorder.Write(data)
}

func (w *cancelBeforeSSEWrite) WriteString(data string) (int, error) {
	if w.drop(data) {
		return len(data), nil
	}
	return w.ResponseRecorder.WriteString(data)
}

func (w *cancelBeforeSSEWrite) drop(data string) bool {
	if w.cancel == nil {
		return false
	}
	if !w.dropped && strings.HasPrefix(data, "data:") {
		w.dropped = true
		w.cancel()
	}
	return w.dropped
}

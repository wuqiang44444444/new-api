package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionReasoningRoundTripPreservesRequestModes(t *testing.T) {
	setupRelayRouterTestDB(t)
	service.InitHttpClient()
	require.NoError(t, i18n.Init())
	originalRatio := ratio_setting.ModelRatio2JSONString()
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-4o-mini":1}`))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(originalRatio))
	})
	originalMemoryCache := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = false
	settings := model_setting.GetGlobalSettings()
	originalSettings := *settings
	settings.ChatCompletionsToResponsesPolicy.Enabled = false
	t.Cleanup(func() {
		common.MemoryCacheEnabled = originalMemoryCache
		*settings = originalSettings
	})

	const modelName = "gpt-4o-mini"
	const upstreamResponse = `{"id":"chatcmpl-history","object":"chat.completion","model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`
	upstreamRequests := make(chan string, 30)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		upstreamRequests <- string(body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), "strict history") {
			var request struct {
				Messages []map[string]common.RawMessage
			}
			if !assert.NoError(t, common.Unmarshal(body, &request)) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			for _, message := range request.Messages {
				if _, present := message["reasoning_content"]; present {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"error":{"message":"fixture rejects reasoning_content in input messages","type":"invalid_request_error","code":"unknown_parameter"}}`)
					return
				}
			}
		}
		response := upstreamResponse
		if strings.Contains(string(body), "show reasoning") {
			response = strings.Replace(response, `"content":"answer"`, `"content":"answer","reasoning":"step"`, 1)
		}
		_, _ = io.WriteString(w, response)
	}))
	defer upstream.Close()

	user := model.User{Username: "reasoning-roundtrip", Status: common.UserStatusEnabled, Group: "default", Quota: 1000000}
	require.NoError(t, model.DB.Create(&user).Error)
	token := model.Token{UserId: user.Id, Key: strings.Repeat("d", 32), Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, Group: "default"}
	require.NoError(t, model.DB.Create(&token).Error)
	channel := model.Channel{Type: constant.ChannelTypeOpenAI, Name: "reasoning-fixture", Key: "fixture", Status: common.ChannelStatusEnabled, Models: modelName, Group: "default", BaseURL: &upstream.URL}
	require.NoError(t, channel.Insert())

	engine := gin.New()
	SetRelayRouter(engine)
	send := func(t *testing.T, body string, wantStatus int) (common.RawMessage, string) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer sk-"+token.Key)
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, req)
		require.Equal(t, wantStatus, recorder.Code, recorder.Body.String())
		var outbound string
		select {
		case outbound = <-upstreamRequests:
		default:
			t.Fatal("fixture did not receive the outbound request")
		}
		if wantStatus != http.StatusOK {
			assert.Contains(t, recorder.Body.String(), "fixture rejects reasoning_content in input messages")
			return nil, outbound
		}
		var response struct {
			Choices []struct{ Message common.RawMessage }
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.Len(t, response.Choices, 1)
		return response.Choices[0].Message, outbound
	}

	for _, mode := range []struct {
		name            string
		global, channel bool
	}{
		{name: "converted"},
		{name: "global passthrough", global: true},
		{name: "channel passthrough", channel: true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			settings.PassThroughRequestEnabled = mode.global
			rawSetting, err := common.Marshal(map[string]bool{"pass_through_body_enabled": mode.channel})
			require.NoError(t, err)
			require.NoError(t, model.DB.Model(&channel).UpdateColumn("setting", string(rawSetting)).Error)

			for _, test := range []struct {
				prompt          string
				clientMessage   string
				upstreamMessage string
			}{
				{
					prompt:          "hello",
					clientMessage:   `{"role":"assistant","content":"answer","reasoning_content":null}`,
					upstreamMessage: `{"role":"assistant","content":"answer"}`,
				},
				{
					prompt:          "show reasoning",
					clientMessage:   `{"role":"assistant","content":"answer","reasoning":"step","reasoning_content":"step"}`,
					upstreamMessage: `{"role":"assistant","content":"answer","reasoning":"step","reasoning_content":"step"}`,
				},
			} {
				userMessage := `{"role":"user","content":` + strconv.Quote(test.prompt) + `}`
				const prefix = `{ "model":"gpt-4o-mini", "messages":[`
				const suffix = `,{"role":"user","content":"continue"}] }`
				first := prefix + userMessage + `] }`
				message, firstOutbound := send(t, first, http.StatusOK)
				assert.JSONEq(t, test.clientMessage, string(message))
				second := prefix + userMessage + "," + string(message) + suffix
				_, secondOutbound := send(t, second, http.StatusOK)
				if mode.global || mode.channel {
					assert.Equal(t, first, firstOutbound)
					assert.Equal(t, second, secondOutbound)
				} else {
					assert.JSONEq(t, prefix+userMessage+","+test.upstreamMessage+suffix, secondOutbound)
				}
			}

			for _, test := range []struct {
				prompt          string
				legacyAssistant string
				addedField      string
				expectRejection bool
			}{
				{
					prompt:          "strict history",
					legacyAssistant: `{"role":"assistant","content":"answer"}`,
					addedField:      `"reasoning_content":null`,
				},
				{
					prompt:          "strict history show reasoning",
					legacyAssistant: `{"role":"assistant","content":"answer","reasoning":"step"}`,
					addedField:      `"reasoning_content":"step"`,
					expectRejection: true,
				},
			} {
				t.Run(test.prompt, func(t *testing.T) {
					history := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":` + strconv.Quote(test.prompt) + `}`
					message, _ := send(t, history+`]}`, http.StatusOK)
					const continuation = `,{"role":"user","content":"continue"}]}`
					assert.JSONEq(t, strings.TrimSuffix(test.legacyAssistant, "}")+","+test.addedField+"}", string(message))
					replay := history + "," + string(message) + continuation
					status := http.StatusOK
					if mode.global || mode.channel || test.expectRejection {
						status = http.StatusBadRequest
					}
					_, outbound := send(t, replay, status)
					if mode.global || mode.channel {
						assert.Equal(t, replay, outbound)
					}

					_, compatibleOutbound := send(t, history+","+test.legacyAssistant+continuation, http.StatusOK)
					assert.NotContains(t, compatibleOutbound, "reasoning_content")
				})
			}
		})
	}
}

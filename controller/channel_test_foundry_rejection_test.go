package controller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFoundryClaudeChannelProbeRejection(t *testing.T) {
	const foundry = "https://fixture.services.ai.azure.com/anthropic"
	const rejection = `{"error":{"type":"invalid_request_error","message":"555420: Deployment request rejected"}}`
	for _, tc := range []struct {
		name, base, body string
		channelStatus    int
		channelType      int
		httpStatus       int
		manual, batch    bool
		recovery         bool
		pass             bool
	}{
		{name: "scheduled check survives stop rules", base: foundry, body: rejection, pass: true},
		{name: "cannot restore automatic disable", base: foundry, body: rejection, channelStatus: common.ChannelStatusAutoDisabled, pass: true},
		{name: "manual batch accepts a structured code", base: foundry, body: `{"error":{"code":555420}}`, batch: true, pass: true},
		{name: "single test accepts a string code", base: foundry, body: `{"error":{"code":"555420"}}`, manual: true, pass: true},
		{name: "direct Anthropic still fails", base: "https://api.anthropic.com", body: rejection},
		{name: "other Azure endpoint still fails", base: "https://fixture.services.ai.azure.com", body: rejection},
		{name: "OpenAI endpoint on Foundry host still fails", base: foundry, channelType: constant.ChannelTypeOpenAI, body: rejection},
		{name: "lookalike domain still fails", base: "https://fixture.services.ai.azure.com.example.test/anthropic", body: rejection},
		{name: "unrelated bad request still fails", base: foundry, body: `{"error":{"message":"Invalid request mentioning 555420: elsewhere"}}`},
		{name: "different HTTP status still fails", base: foundry, body: rejection, httpStatus: http.StatusServiceUnavailable},
		{name: "connection failure still fails", base: foundry},
		{name: "routing recovery stays unconfirmed", base: foundry, body: rejection, recovery: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := setupAutoCheckTest(t)
			user := newAutoCheckUser(t, db)
			status := tc.channelStatus
			if status == 0 {
				status = common.ChannelStatusEnabled
			}
			channelType := tc.channelType
			if channelType == 0 {
				channelType = constant.ChannelTypeAnthropic
			}
			channel := &model.Channel{Type: channelType, Models: "gpt-4o", Group: "default", Status: status, Key: "fixture", BaseURL: &tc.base, AutoBan: common.GetPointer(1)}
			require.NoError(t, db.Create(channel).Error)
			upstreamStatus := tc.httpStatus
			if upstreamStatus == 0 && tc.body != "" {
				upstreamStatus = http.StatusBadRequest
			}
			useFetchModelsTransport(t, func(request *http.Request) (*http.Response, error) {
				path := "/v1/messages"
				if channelType == constant.ChannelTypeOpenAI {
					path = "/v1/chat/completions"
				}
				assert.Equal(t, tc.base+path, request.URL.String())
				trace := httptrace.ContextClientTrace(request.Context())
				trace.GetConn(request.URL.Host)
				if tc.body == "" {
					return nil, errors.New("fixture connection failure")
				}
				trace.GotFirstResponseByte()
				w := httptest.NewRecorder()
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("request-id", "req_fixture")
				w.WriteHeader(upstreamStatus)
				_, _ = w.WriteString(tc.body)
				response := w.Result()
				response.Request = request
				return response, nil
			})
			if tc.recovery {
				ctx := context.WithValue(t.Context(), routingRecoveryProbeContextKey{}, true)
				result := testChannel(ctx, channel, user.Id, channel.Models, string(constant.EndpointTypeAnthropic), true)
				require.Error(t, result.localErr)
				assert.True(t, service.ChannelTestFoundryRejection(result.context))
				assert.Equal(t, http.StatusBadRequest, result.upstreamStatus)
				assert.Equal(t, "unconfirmed", result.recoveryEvidence)
				return
			}
			persisted := registerAutoCheckEventWait(t, db)
			if tc.manual {
				latencyUpdated := make(chan struct{}, 1)
				require.NoError(t, db.Callback().Update().After("gorm:commit_or_rollback_transaction").Register("test:manual_probe_latency_saved", func(tx *gorm.DB) {
					if tx.Statement.Table == "channels" && tx.Error == nil {
						latencyUpdated <- struct{}{}
					}
				}))
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				id := strconv.Itoa(channel.Id)
				c.Request = httptest.NewRequest(http.MethodGet, "/api/channel/test/"+id+"?endpoint_type=anthropic", nil)
				c.Params = gin.Params{{Key: "id", Value: id}}
				c.Set("id", user.Id)
				TestChannel(c)
				require.Equal(t, http.StatusOK, w.Code)
				var body struct {
					Success            bool   `json:"success"`
					GenerationEvidence string `json:"generation_evidence"`
				}
				require.NoError(t, common.Unmarshal(w.Body.Bytes(), &body))
				assert.True(t, body.Success)
				assert.Equal(t, "rejected", body.GenerationEvidence)
				select {
				case <-latencyUpdated:
				case <-time.After(5 * time.Second):
					t.Fatal("manual probe latency was not persisted")
				}
			} else {
				threshold := int64(100000)
				if tc.pass {
					// Make both automatic stop paths eligible for every matching rejection.
					previousRanges := operation_setting.AutomaticDisableStatusCodeRanges
					t.Cleanup(func() { operation_setting.AutomaticDisableStatusCodeRanges = previousRanges })
					require.NoError(t, operation_setting.AutomaticDisableStatusCodesFromString("400"))
					threshold = -1
				}
				summary := testChannelForHealthCheck(context.Background(), channel, user.Id, true, threshold, !tc.batch)
				assert.Equal(t, 1, summary.Tested)
				if tc.pass {
					assert.Equal(t, 1, summary.Succeeded)
					assert.Zero(t, summary.Failed)
					assert.Zero(t, summary.Disabled)
					assert.Zero(t, summary.Enabled)
					assert.Empty(t, summary.mailFailures)
					require.Len(t, summary.Checks, 1)
					assert.Equal(t, "passed", summary.Checks[0].Detail["check_result"])
					assert.Equal(t, "rejected", summary.Checks[0].Detail["generation_evidence"])
				} else {
					assert.Zero(t, summary.Succeeded)
					assert.Equal(t, 1, summary.Failed)
				}
			}
			waitPersistedAutoCheckEvent(t, persisted)
			row, detail := loadAutoCheckEventDetail(t, db, channel.Id)
			if tc.pass {
				assert.Equal(t, "555420", row.PublicCode)
				assert.Equal(t, "req_fixture", row.UpstreamRequestId)
				assert.NotEmpty(t, row.RequestId)
				assert.Equal(t, "400", detail["upstream_status"])
				assert.Equal(t, "passed", detail["check_result"])
				assert.Equal(t, "rejected", detail["generation_evidence"])
				var saved model.Channel
				require.NoError(t, db.First(&saved, channel.Id).Error)
				assert.Equal(t, status, saved.Status)
			} else {
				assert.Equal(t, "failed", detail["check_result"])
				assert.NotContains(t, detail, "generation_evidence")
			}
		})
	}
}

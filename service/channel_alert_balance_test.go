package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaymode "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelAlertRecognizesUpstreamBalanceResponses(t *testing.T) {
	for _, tc := range []struct {
		name                                  string
		status                                int
		body                                  string
		funCloud, miniMax, billingQuota, want bool
	}{
		{name: "Ark 403 code", status: 403, body: `{"error":{"code":"AccountOverdueError"}}`, want: true},
		{name: "Ark via proxy", status: 502, body: `{"error":{"message":"status_code=403, The request failed because your account has an overdue balance. Request id: private"}}`, want: true},
		{name: "plain proxy response", status: 403, body: `余额不足，请充值`, want: true},
		{name: "payment required", status: 402, want: true},
		{name: "credits exhausted", status: 429, body: `{"error":{"code":"credit_balance_exhausted"}}`, want: true},
		{name: "official asset error", status: 200, body: `{"ResponseMetadata":{"Error":{"Code":"OperationDenied.ServiceOverdue"}}}`, want: true},
		{name: "MiniMax business error", status: 200, body: `{"base_resp":{"status_code":1008}}`, miniMax: true, want: true},
		{name: "FunCloud image business error", status: 200, body: `{"code":40001}`, funCloud: true, want: true},
		{name: "permissions", status: 403, body: `{"error":{"code":"AccessDenied","message":"Forbidden"}}`},
		{name: "compatible billing quota", status: 429, body: `{"error":{"code":"insufficient_quota"}}`, billingQuota: true, want: true},
		{name: "rate limit", status: 429, body: `{"error":{"code":"rate_limit_exceeded"}}`},
		{name: "ambiguous quota rejection", status: 429, body: `{"error":{"code":"insufficient_quota","message":"You exceeded your current quota, please check your plan and billing details."}}`},
		{name: "batch billing rejection", status: 200, body: `{"status":"failed","errors":{"data":[{"code":"credit_balance_exhausted"}]}}`, want: true},
		{name: "ambiguous numeric code", status: 200, body: `{"code":40001}`},
		{name: "successful response", status: 200, body: `{"code":0,"message":"余额不足","data":{"status":"ok"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, upstreamBalanceResponse(tc.status, channelBalanceProfile{funCloudImage: tc.funCloud, miniMax: tc.miniMax, billingQuota: tc.billingQuota}, []byte(tc.body), true))
		})
	}
}

func TestChannelAlertBalanceSendsOnceAcrossOperationsAndReopensAfterRecovery(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.ChannelAlertEpisode{}, &model.ChannelAlertDelivery{}))
	require.NoError(t, model.UpdateOption("error_report_setting.recipients", "ops@example.com"))
	inbox := newLocalMailInbox(t)
	channel := model.Channel{Name: "balance-fixture", Models: "chat,video", Key: "private-key-fixture"}
	require.NoError(t, db.Create(&channel).Error)
	priorEnabled, priorHard := channelAlerts.enabled.Load(), channelAlerts.hard
	channelAlerts.buffer.buckets = make(map[int64]map[channelAlertScope]*channelAlertObservation)
	channelAlerts.buffer.dropped = make(map[int64]bool)
	channelAlerts.buffer.sealed = 0
	channelAlerts.hard = make(chan channelAlertObservation, 4)
	channelAlerts.enabled.Store(true)
	t.Cleanup(func() { channelAlerts.enabled.Store(priorEnabled); channelAlerts.hard = priorHard })

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "request-one")
	common.SetContextKey(c, constant.ContextKeyChannelId, channel.Id)
	common.SetContextKey(c, constant.ContextKeyOriginalModel, "chat")
	SetChannelAlertSelection(c, &channel, 0)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelId: channel.Id}, OriginModelName: "chat", RelayMode: relaymode.RelayModeChatCompletions}
	BeginChannelAlertAttempt(c)
	ObserveChannelRelayResult(c, info, types.NewErrorWithStatusCode(io.ErrUnexpectedEOF, types.ErrorCodeInsufficientUserQuota, http.StatusForbidden))
	require.Empty(t, channelAlerts.hard, "customer quota rejection has no upstream response")

	body := `{"error":{"code":"AccountOverdueError","message":"private-provider-detail"}}`
	response := &http.Response{StatusCode: http.StatusForbidden, Body: io.NopCloser(strings.NewReader(body))}
	RecordChannelAlertRelayHTTP(c, info, response, nil)
	require.Empty(t, channelAlerts.hard, "the adapter still owns body reads")
	got, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	assert.Equal(t, body, string(got))
	require.Len(t, channelAlerts.hard, 1, "the first verified response qualifies immediately")
	observation := <-channelAlerts.hard
	sample := observation.Failures[0]
	ctx := context.Background()
	episode, confirmed := channelAlertCandidate(channelAlertGroup{scope: observation.Scope, kind: sample.Kind}, observation.Failures, 0)
	require.True(t, confirmed)
	require.NoError(t, confirmChannelAlert(ctx, episode, time.Now()))

	other := observation.Scope
	other.Model, other.Operation = "video", "video_submit"
	repeated, confirmed := channelAlertCandidate(channelAlertGroup{scope: other, kind: sample.Kind}, observation.Failures, 0)
	require.True(t, confirmed)
	require.NoError(t, confirmChannelAlert(ctx, repeated, time.Now()))
	sent, err := deliverChannelAlertMail("balance-mail", common.SendEmailContext, time.Now)
	require.NoError(t, err)
	require.True(t, sent)
	mail := requireLocalMail(t, inbox, "Upstream channel balance alert", "ops@example.com")
	assert.Contains(t, mail.body, "403")
	assert.NotContains(t, mail.body, "private-provider-detail")
	assert.NotContains(t, mail.body, channel.Key)
	sent, err = deliverChannelAlertMail("balance-mail", common.SendEmailContext, time.Now)
	require.NoError(t, err)
	assert.False(t, sent, "other operations share the open balance incident")

	start := channelAlertBucket(sample.At/1000) + 5
	successAt := start*1000 + 1000
	window := channelAlertWindowState{localNode: "fixture"}
	laterFailure := channelAlertSample{ID: "still-unpaid", Kind: channelAlertBalance, Status: 402, At: successAt + 1000}
	window.append(channelAlertRound{start: start, batches: []channelAlertBatch{{Scopes: []channelAlertObservation{
		{Scope: observation.Scope, Success: successAt}, {Scope: other, Failures: []channelAlertSample{laterFailure}},
	}}}}, 256)
	require.NoError(t, evaluateChannelAlertWindow(ctx, window, []string{"fixture"}, time.Unix(start+5, 0)))
	var stored model.ChannelAlertEpisode
	require.NoError(t, db.First(&stored).Error)
	assert.Zero(t, stored.ClosedAt, "a newer billing rejection on another operation keeps the incident open")

	start += 5
	successAt = start*1000 + 1000
	window.append(channelAlertRound{start: start, batches: []channelAlertBatch{{Scopes: []channelAlertObservation{{Scope: observation.Scope, Success: successAt}}}}}, 256)
	require.NoError(t, evaluateChannelAlertWindow(ctx, window, []string{"fixture"}, time.Unix(start+5, 0)))
	require.NoError(t, db.First(&stored).Error)
	assert.Equal(t, model.ChannelAlertClosedRecovered, stored.CloseReason)

	sample.At = successAt + 1000
	episode, confirmed = channelAlertCandidate(channelAlertGroup{scope: other, kind: sample.Kind}, []channelAlertSample{sample}, 0)
	require.True(t, confirmed)
	require.NoError(t, confirmChannelAlert(ctx, episode, time.Now()))
	sent, err = deliverChannelAlertMail("balance-mail", common.SendEmailContext, time.Now)
	require.NoError(t, err)
	require.True(t, sent)
	requireLocalMail(t, inbox, "Upstream channel balance alert", "ops@example.com")
}

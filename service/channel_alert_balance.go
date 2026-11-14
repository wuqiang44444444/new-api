package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/azurebatch"
	relaymode "github.com/QuantumNous/new-api/relay/constant"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
)

const channelAlertBalance channelAlertKind = "upstream_balance"
const channelAlertBalanceBodyLimit = 16 << 10

// Only pass facts from a provider response. Local customer quota errors can
// have the same code or message as an upstream gateway's funding rejection.
func upstreamBalanceRejected(status int, code, message string) bool {
	if status == http.StatusPaymentRequired {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "accountoverdueerror", "operationdenied.serviceoverdue", "accountarrears", "account_overdue", "account_overdue_error", "arrearage",
		"insufficient_balance", "insufficient_balance_error", "insufficientbalance", "insufficient_funds", "insufficient_credit", "insufficient_user_quota",
		"insufficient_credits", "credit_balance_exhausted", "billing_hard_limit_reached",
		"organization_spend_limit_exceeded", "project_spend_limit_exceeded":
		return true
	}
	if len(message) > channelAlertBalanceBodyLimit {
		return false
	}
	message = strings.ToLower(strings.TrimSpace(message))
	if _, after, ok := strings.Cut(message, ", "); ok && strings.HasPrefix(message, "status_code=") {
		message = after
	}
	for _, prefix := range []string{
		"the request failed because your account has an overdue balance",
		"operation is denied because your account balance is overdue",
		"your account has an overdue balance", "your credit balance is too low",
		"your account balance is insufficient", "your account is in arrears", "you have run out of balance",

		"insufficient balance", "insufficient funds", "insufficient credits", "not enough balance",
		"credit balance is too low", "credit balance is exhausted", "balance is insufficient",
		"账户余额不足", "账号余额不足", "您的账户余额不足", "余额不足", "账户欠费", "账号欠费", "您的账号已欠费", "当前账号欠费", "用户额度不足",
	} {
		if suffix, ok := strings.CutPrefix(message, prefix); ok {
			next, _ := utf8.DecodeRuneInString(suffix)
			if suffix == "" || strings.ContainsRune(" .,:;([{，。；：、（！!\r\n\t", next) {
				return true
			}
		}
	}
	return false
}

type channelBalanceProfile struct {
	funCloudImage bool
	miniMax       bool
	billingQuota  bool
}

type channelBalanceError struct {
	Code         json.RawMessage `json:"code"`
	Type         string          `json:"type"`
	Message      string          `json:"message"`
	Msg          string          `json:"msg"`
	ErrorCode    string          `json:"errorCode"`
	ErrorMessage string          `json:"errorMessage"`
}

func (e channelBalanceError) rejects(status int, allowMessage, billingQuota bool) bool {
	var code string
	if len(e.Code) != 0 && common.Unmarshal(e.Code, &code) != nil {
		code = string(e.Code)
	}
	if billingQuota && (strings.EqualFold(code, "insufficient_quota") || strings.EqualFold(e.Type, "insufficient_quota")) {
		return true
	}
	if !allowMessage && (code == "" || code == "0" || code == "null") {
		return upstreamBalanceRejected(status, e.Type, "") || upstreamBalanceRejected(status, e.ErrorCode, "")
	}
	return upstreamBalanceRejected(status, code, e.Message) || upstreamBalanceRejected(status, e.Type, e.Msg) ||
		upstreamBalanceRejected(status, e.ErrorCode, e.ErrorMessage)
}

func upstreamBalanceResponse(status int, profile channelBalanceProfile, body []byte, complete bool) bool {
	if status == http.StatusPaymentRequired {
		return true
	}
	if len(body) == 0 || len(body) > channelAlertBalanceBodyLimit {
		return false
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return false
	}
	var envelope struct {
		channelBalanceError
		Error    json.RawMessage `json:"error"`
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
		Response struct {
			Error channelBalanceError `json:"error"`
		} `json:"response"`
		Errors struct {
			Data []channelBalanceError `json:"data"`
		} `json:"errors"`
		ResponseMetadata struct {
			Error channelBalanceError `json:"Error"`
		} `json:"ResponseMetadata"`
	}
	if body[0] == '{' && common.Unmarshal(body, &envelope) == nil {
		if envelope.channelBalanceError.rejects(status, status >= 400 || envelope.ErrorCode != "", profile.billingQuota) ||
			envelope.ResponseMetadata.Error.rejects(status, true, profile.billingQuota) || envelope.Response.Error.rejects(status, true, profile.billingQuota) ||
			(envelope.BaseResp.StatusCode != 0 && upstreamBalanceRejected(status, "", envelope.BaseResp.StatusMsg)) ||
			(profile.miniMax && (envelope.BaseResp.StatusCode == 1008 || envelope.BaseResp.StatusCode == 2053)) ||
			(profile.funCloudImage && (string(envelope.Code) == "40001" || string(envelope.Code) == `"40001"`)) {
			return true
		}
		for _, item := range envelope.Errors.Data {
			if item.rejects(status, true, profile.billingQuota) {
				return true
			}
		}
		var detail channelBalanceError
		if common.Unmarshal(envelope.Error, &detail) == nil {
			return detail.rejects(status, true, profile.billingQuota)
		}
		var message string
		return common.Unmarshal(envelope.Error, &message) == nil && upstreamBalanceRejected(status, "", message)
	}
	if !complete || status < 400 {
		return false
	}
	if body[0] == '"' {
		var message string
		return common.Unmarshal(body, &message) == nil && upstreamBalanceRejected(status, "", message)
	}
	return upstreamBalanceRejected(status, "", string(body))
}

// Inspect only bytes already consumed by the adapter. The observer neither
// reads ahead nor retains the raw response in any alert or diagnostic record.
func watchChannelBalanceResponse(response *http.Response, profile channelBalanceProfile, applicationErrors bool, observe func(balance bool)) {
	if response.StatusCode == http.StatusPaymentRequired {
		observe(true)
		return
	}
	if response.Body != nil && response.ContentLength <= channelAlertBalanceBodyLimit && (response.StatusCode >= 400 || applicationErrors && response.StatusCode/100 == 2) {
		response.Body = &channelBalanceBody{ReadCloser: response.Body, status: response.StatusCode, profile: profile, observe: observe}
	}
}

type channelBalanceBody struct {
	io.ReadCloser
	mu      sync.Mutex
	status  int
	profile channelBalanceProfile
	observe func(bool)
	body    []byte
}

func (r *channelBalanceBody) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.observe != nil {
		if len(r.body)+n > channelAlertBalanceBodyLimit || err != nil && err != io.EOF {
			r.body, r.observe = nil, nil
		} else {
			r.body = append(r.body, p[:n]...)
			if err == io.EOF {
				r.finish(true)
			}
		}
	}
	return n, err
}

func (r *channelBalanceBody) Close() error {
	r.mu.Lock()
	r.finish(false) // A decoder may finish a full JSON value without reading EOF.
	r.mu.Unlock()
	return r.ReadCloser.Close()
}

func (r *channelBalanceBody) finish(complete bool) {
	if r.observe != nil {
		r.observe(upstreamBalanceResponse(r.status, r.profile, r.body, complete))
	}
	r.body, r.observe = nil, nil
}

func watchRelayChannelBalance(c *gin.Context, response *http.Response, operation string, applicationErrors bool) *atomic.Bool {
	if response == nil || c.Request == nil || response.StatusCode != http.StatusPaymentRequired &&
		(response.Body == nil || response.ContentLength > channelAlertBalanceBodyLimit || response.StatusCode < 400 && !applicationErrors) {
		return nil
	}
	scope, id := relayBalanceObservation(c, operation)
	if scope.Channel == 0 {
		return nil
	}
	settings, _ := common.GetContextKeyType[dto.ChannelOtherSettings](c, constant.ContextKeyChannelOtherSetting)
	status := response.StatusCode
	observed := new(atomic.Bool)
	channelType := common.GetContextKeyInt(c, constant.ContextKeyChannelType)
	profile := channelBalanceProfile{
		funCloudImage: settings.ImageUpstreamProtocol == kitdto.ImageUpstreamProtocolFunCloudAIGCV2,
		miniMax:       channelType == constant.ChannelTypeMiniMax || channelType == constant.ChannelTypeMiniMaxLink,
		billingQuota: channelType == constant.ChannelTypeOpenAI || channelType == constant.ChannelTypeOpenAIMax ||
			channelType == constant.ChannelTypeNewAPI || settings.ImageUpstreamProtocol == kitdto.ImageUpstreamProtocolMoxingImagesV1,
	}
	watchChannelBalanceResponse(response, profile, applicationErrors, func(balance bool) {
		if balance {
			observed.Store(true)
			recordChannelAlert(scope, id, channelAlertBalance, status)
		}
	})
	return observed
}

func recordRelayChannelBalance(c *gin.Context, operation string, status int) {
	scope, id := relayBalanceObservation(c, operation)
	recordChannelAlert(scope, id, channelAlertBalance, status)
}

func relayBalanceObservation(c *gin.Context, operation string) (channelAlertScope, string) {
	snapshot := ChannelAlertSelection(c)
	if snapshot == nil {
		return channelAlertScope{}, ""
	}
	scope := channelAlertScope{
		Channel: common.GetContextKeyInt(c, constant.ContextKeyChannelId), Revision: snapshot.Revision,
		Model: common.GetContextKeyString(c, constant.ContextKeyOriginalModel), Operation: operation, Slot: snapshot.Slot,
	}
	id := c.GetString(common.RequestIdKey)
	if id == "" {
		id = common.NewRequestId()
	}
	return scope, id
}

// ObserveChannelSocketBalance handles provider error frames without an HTTP error.
func ObserveChannelSocketBalance(c *gin.Context, operation string, body []byte) {
	if channelAlerts.enabled.Load() && upstreamBalanceResponse(0, channelBalanceProfile{billingQuota: true}, body, true) {
		recordRelayChannelBalance(c, operation, 0)
	}
}

func relayBalanceOperation(c *gin.Context) string {
	if c.Request == nil {
		return "relay"
	}
	if c.Request.Method == http.MethodGet && strings.EqualFold(c.Request.Header.Get("Upgrade"), "websocket") {
		if relaymode.Path2RelayMode(c.Request.URL.Path) == relaymode.RelayModeResponses {
			return "responses_websocket"
		}
	}
	return channelAlertOperation(relaymode.Path2RelayMode(c.Request.URL.Path))
}

// WatchTaskHTTPBalance handles adapters that consume or normalize raw HTTP responses.
func WatchTaskHTTPBalance(ctx context.Context, task *model.Task, operation string, response *http.Response) {
	if !channelAlerts.enabled.Load() || response == nil || task == nil || task.PrivateData.ChannelAlert == nil {
		return
	}
	status := response.StatusCode
	watchChannelBalanceResponse(response, channelBalanceProfile{miniMax: task.IsMiniMaxV2Task()}, true, func(balance bool) {
		if balance {
			observeFrozenChannelBalance(ctx, task, operation, status)
		}
	})
}

func observeFrozenChannelBalance(ctx context.Context, task *model.Task, operation string, status int) {
	if task == nil || task.PrivateData.ChannelAlert == nil || ctx.Err() != nil {
		return
	}
	snapshot := task.PrivateData.ChannelAlert
	recordChannelAlert(channelAlertScope{Channel: task.ChannelId, Revision: snapshot.Revision, Model: task.Properties.OriginModelName,
		Operation: operation, Slot: snapshot.Slot}, common.NewRequestId(), channelAlertBalance, status)
}

// WatchChannelProbeBalance observes a saved channel's read-only HTTP probe.
func WatchChannelProbeBalance(channel *model.Channel, slot int, response *http.Response) {
	if !channelAlerts.enabled.Load() || response == nil || channel == nil {
		return
	}
	if !channel.ChannelInfo.IsMultiKey {
		slot = -1
	}
	scope := channelAlertScope{Channel: channel.Id, Revision: model.ChannelAlertRevision(channel), Operation: "connection_probe", Slot: slot}
	status := response.StatusCode
	watchChannelBalanceResponse(response, channelBalanceProfile{billingQuota: channel.Type == constant.ChannelTypeOpenAI}, false, func(balance bool) {
		if balance {
			recordChannelAlert(scope, common.NewRequestId(), channelAlertBalance, status)
		}
	})
}

// ConfigureBatchProbeBalance observes the read-only Azure Batch test using the saved connection.
func ConfigureBatchProbeBalance(client *azurebatch.Client, channel *model.Channel) {
	if channelAlerts.enabled.Load() {
		configureBatchChannelObservation(client, &model.BatchFrozenSnapshot{
			ChannelId:    channel.Id,
			ChannelAlert: &model.ChannelAlertTaskScope{Revision: model.ChannelAlertRevision(channel), Slot: -1},
		})
	}
}

func configureBatchChannelObservation(client *azurebatch.Client, frozen *model.BatchFrozenSnapshot) {
	client.HTTPClient.Transport = WithAPIRequestEvidenceTransport(client.HTTPClient.Transport)
	if !channelAlerts.enabled.Load() || frozen.ChannelAlert == nil {
		return
	}
	client.HTTPClient.Transport = batchChannelAlertTransport{client.HTTPClient.Transport, channelAlertScope{
		Channel: frozen.ChannelId, Revision: frozen.ChannelAlert.Revision, Model: frozen.PublicModel,
		Operation: "azure_batch", Slot: frozen.ChannelAlert.Slot,
	}}
}

type batchChannelAlertTransport struct {
	transport http.RoundTripper
	scope     channelAlertScope
}

func (t batchChannelAlertTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.transport.RoundTrip(req)
	if err != nil || response == nil {
		return response, err
	}
	status := response.StatusCode
	successCandidate := status/100 == 2 && !strings.HasSuffix(req.URL.Path, "/content")
	if successCandidate || status >= 400 {
		watchChannelBalanceResponse(response, channelBalanceProfile{billingQuota: true}, successCandidate, func(balance bool) {
			if balance {
				recordChannelAlert(t.scope, common.NewRequestId(), channelAlertBalance, status)
			} else if successCandidate {
				recordChannelAlert(t.scope, "", channelAlertSuccess, status)
			}
		})
	}
	return response, nil
}

// ObserveChannelSocketHandshakeBalance uses the handshake error body already
// buffered by the WebSocket client. The caller closes this rejected response.
func ObserveChannelSocketHandshakeBalance(c *gin.Context, response *http.Response) {
	if !channelAlerts.enabled.Load() || response == nil || response.Body == nil || response.StatusCode < 400 || response.StatusCode == http.StatusPaymentRequired {
		return
	}
	if _, watching := response.Body.(*channelBalanceBody); watching {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, channelAlertBalanceBodyLimit+1))
	}
}

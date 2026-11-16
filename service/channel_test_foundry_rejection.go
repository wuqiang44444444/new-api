package service

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

const (
	foundryClaudeProbeRejectionKey  = "channel_test_foundry_rejection"
	ChannelTestFoundryRejectionCode = "555420"
)

// ChannelTestUpstreamError preserves the upstream error while recognizing the
// Foundry Claude rejection that proves reachability for this probe.
func ChannelTestUpstreamError(c *gin.Context, channelType int, response *http.Response) *types.NewAPIError {
	if !isFoundryClaudeProbeResponse(channelType, response) {
		return RelayErrorHandler(c.Request.Context(), response, true)
	}
	return relayErrorHandler(c.Request.Context(), response, true, func(raw json.RawMessage) {
		var upstream struct {
			Code    json.RawMessage `json:"code"`
			Message string          `json:"message"`
		}
		if common.Unmarshal(raw, &upstream) == nil && (string(upstream.Code) == `"`+ChannelTestFoundryRejectionCode+`"` ||
			string(upstream.Code) == ChannelTestFoundryRejectionCode || strings.HasPrefix(upstream.Message, ChannelTestFoundryRejectionCode+":")) {
			c.Set(foundryClaudeProbeRejectionKey, true)
		}
	})
}

func isFoundryClaudeProbeResponse(channelType int, response *http.Response) bool {
	if channelType != constant.ChannelTypeAnthropic || response.StatusCode != http.StatusBadRequest || response.Request == nil {
		return false
	}
	request := response.Request
	target := request.URL
	if request.Method != http.MethodPost || target == nil || target.Scheme != "https" || target.User != nil ||
		target.EscapedPath() != "/anthropic/v1/messages" || (target.Port() != "" && target.Port() != "443") ||
		(request.Host != "" && !strings.EqualFold(request.Host, target.Host)) {
		return false
	}
	resource, found := strings.CutSuffix(strings.ToLower(target.Hostname()), ".services.ai.azure.com")
	return found && resource != "" && !strings.Contains(resource, ".")
}

func ChannelTestFoundryRejection(c *gin.Context) bool {
	return c != nil && c.GetBool(foundryClaudeProbeRejectionKey)
}

func AddChannelTestFoundryRejectionDetail(detail map[string]string) {
	detail["check_result"] = "passed"
	detail["check_scope"] = "generation_probe"
	detail["config_check"] = "passed"
	detail["generation_evidence"] = "rejected"
	detail["upstream_request"] = "response_received"
	detail["check_code"] = ChannelTestFoundryRejectionCode
}

package relay

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func markAmbiguousTaskCreate(c *gin.Context, info *relaycommon.RelayInfo) {
	service.MarkTaskCreateAttemptOutcomeUnknown(c, info)
}

func taskCreateHTTPDisposition(
	info *relaycommon.RelayInfo,
	status int,
	body []byte,
) relaycommon.TaskCreateDisposition {
	if info == nil || info.ChannelMeta == nil {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	// The MiniMax Link typed channel classifies the documented JD error
	// envelope in its own narrow branch.
	if info.ChannelType == constant.ChannelTypeMiniMaxLink {
		return minimaxCreateDisposition(status, body)
	}
	if info.ChannelType != constant.ChannelTypeSeedanceLink {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	// A task ID or non-empty data envelope contradicts a terminal rejection.
	// Classify the complete structured response, not its public/redacted message.
	var response struct {
		ID      any    `json:"id"`
		TaskID  any    `json:"task_id"`
		Data    any    `json:"data"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if common.Unmarshal(body, &response) != nil || response.ID != nil || response.TaskID != nil || response.Data != nil {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	if isFunCloudModelArkCreateRejection(info, status, body) {
		return relaycommon.TaskCreateTerminalRejection
	}
	code, message := response.Code, response.Message
	if response.Error != nil {
		code, message = response.Error.Code, response.Error.Message
	}
	if strings.TrimSpace(message) == "" {
		return relaycommon.TaskCreateOutcomeUnknown
	}
	switch info.ChannelOtherSettings.VideoUpstreamProtocol {
	case dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, dto.VideoUpstreamProtocolMoxingModelArkV1:
		if status == http.StatusForbidden && code == "user_quota_insufficient" {
			return relaycommon.TaskCreateTerminalRejection
		}
		// TokenSave 2026-09-08 live verification: model denied by account group.
		if info.ChannelOtherSettings.VideoUpstreamProtocol == dto.VideoUpstreamProtocolTokenSaveMediaTaskV1 && status == http.StatusForbidden && code == "group_model_permission_denied" {
			return relaycommon.TaskCreateTerminalRejection
		}
	case dto.VideoUpstreamProtocolModelArkV3Volcengine:
		// Official Ark 2026-09-08: inactive API key, rejected before task creation.
		if status == http.StatusUnauthorized && code == "AuthenticationError" {
			return relaycommon.TaskCreateTerminalRejection
		}
	case dto.VideoUpstreamProtocolModelArkV3CMCC:
		// CMCC 2026-09-08 reports invalid credentials without a machine code.
		if status == http.StatusForbidden && code == "" && message == "api key is invalid" {
			return relaycommon.TaskCreateTerminalRejection
		}
	}
	// Unregistered status + provider-code combinations remain unknown. A new
	// terminal rejection requires exact provider evidence and a regression test.
	return relaycommon.TaskCreateOutcomeUnknown
}

package relay

import (
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func isChannelAlertCredentialRejection(info *relaycommon.RelayInfo, failure *taskUpstreamHTTPError) bool {
	switch info.ChannelOtherSettings.VideoUpstreamProtocol {
	case dto.VideoUpstreamProtocolModelArkV3Volcengine:
		return failure.statusCode == http.StatusUnauthorized && failure.providerCode == "AuthenticationError" &&
			failure.providerMessage == "The API key status is not active."
	case dto.VideoUpstreamProtocolModelArkV3CMCC:
		return failure.statusCode == http.StatusForbidden && failure.providerCode == "" && failure.providerMessage == "api key is invalid"
	}
	return false
}

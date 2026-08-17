package seedance

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
)

// BillingProbeValidationExtraFields returns fields that exist only for the
// selected protocol. Common task fields are owned by the billing validator;
// keeping this protocol-specific part beside BuildTaskBillingProbe prevents a
// Feicai-only field from being accepted for other Seedance channels.
func BillingProbeValidationExtraFields(protocol dto.VideoUpstreamProtocol) map[string]any {
	if protocol.TransportProfile() != dto.VideoUpstreamProfileThirdPartyFeicaiVideos {
		return nil
	}
	return map[string]any{
		"ratio":           "16:9",
		"size_multiplier": 1.0,
		"billing_mode":    feicai.BillingModePerSecond,
	}
}

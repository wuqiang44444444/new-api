package seedance

import (
	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Provider errors are never trusted public text. URL validation is also a host
// boundary; it does not select fields or infer a Provider status.
func validatePluginProviderObservation(body []byte, protocol dto.VideoUpstreamProtocol) ([]byte, error) {
	if protocol == dto.VideoUpstreamProtocolModelArkV3Volcengine || protocol == dto.VideoUpstreamProtocolModelArkV3BytePlus || protocol == dto.VideoUpstreamProtocolModelArkV3CMCC {
		return body, nil
	}
	var result map[string]any
	if err := common.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if failure, ok := result["error"].(map[string]any); ok {
		for _, key := range []string{"code", "message"} {
			if value, ok := failure[key].(string); ok {
				failure[key] = common.PublicTaskErrorMessage(value)
			}
		}
	}
	if protocol != dto.VideoUpstreamProtocolArkMediaV1 {
		if content, ok := result["content"].(map[string]any); ok {
			for _, key := range []string{"video_url", "last_frame_url"} {
				if raw, exists := content[key]; exists {
					value, ok := raw.(string)
					if !ok {
						return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid video result URL"}
					}
					validated, err := relaycommon.ValidateHTTPSVideoResultURL(value)
					if err != nil {
						return nil, &relaycommon.UpstreamContractViolation{Reason: "invalid video result URL"}
					}
					content[key] = validated
				}
			}
		}
	}
	return common.Marshal(result)
}

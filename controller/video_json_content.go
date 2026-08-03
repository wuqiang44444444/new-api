package controller

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func jsonVideoContentSource(task *model.Task) (string, string, bool, error) {
	if task == nil || task.PrivateData.VideoUpstreamProfile != dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference {
		return "", "", false, nil
	}
	version, err := relaycommon.ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeDoubaoVideo,
		task.PrivateData.VideoUpstreamProfile,
		task.PrivateData.SouthboundAdapterVersion,
	)
	if err != nil {
		return "", "", true, fmt.Errorf("frozen JSON video adapter version is invalid")
	}
	if !version.IsJSONVideoOmniV2() {
		return "", "", true, fmt.Errorf("frozen JSON video adapter version is unsupported")
	}
	baseURL := strings.TrimSpace(task.PrivateData.VideoUpstreamQueryBaseURL)
	resultURL, err := relaycommon.ValidateSameOriginHTTPSVideoResultURL(
		task.PrivateData.ResultURL,
		baseURL,
	)
	if err != nil {
		return "", "", true, fmt.Errorf("frozen JSON video result URL is invalid")
	}
	return resultURL, "", true, nil
}

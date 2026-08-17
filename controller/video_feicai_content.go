package controller

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func videoFeicaiContentSource(task *model.Task) (string, string, bool, error) {
	if task == nil {
		return "", "", false, nil
	}
	profile := task.PrivateData.VideoUpstreamProfile
	if profile != dto.VideoUpstreamProfileThirdPartyFeicaiVideos {
		return "", "", false, nil
	}
	version, err := relaycommon.ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeSeedanceLink,
		task.PrivateData.VideoUpstreamProfile,
		task.PrivateData.SouthboundAdapterVersion,
	)
	if err != nil || !version.IsFeicaiVideos() {
		return "", "", true, fmt.Errorf("frozen Feicai video adapter version is invalid")
	}
	key := strings.TrimSpace(task.PrivateData.Key)
	if key == "" {
		return "", "", true, fmt.Errorf("frozen Feicai video credential is unavailable")
	}
	resultURL, err := relaycommon.ValidateSameOriginVideoResultURL(
		task.PrivateData.ResultURL,
		strings.TrimSpace(task.PrivateData.VideoUpstreamQueryBaseURL),
	)
	if err != nil {
		return "", "", true, fmt.Errorf("frozen Feicai video result URL is invalid")
	}
	return resultURL, key, true, nil
}

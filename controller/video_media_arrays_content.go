package controller

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func videoMediaArraysContentSource(task *model.Task) (string, string, bool, error) {
	if task == nil || task.PrivateData.VideoUpstreamProfile != dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays {
		return "", "", false, nil
	}
	version, err := relaycommon.ResolveVideoSouthboundAdapterVersion(
		constant.ChannelTypeDoubaoVideo,
		task.PrivateData.VideoUpstreamProfile,
		task.PrivateData.SouthboundAdapterVersion,
	)
	if err != nil || !version.IsJSONVideoMediaArraysV2() {
		return "", "", true, fmt.Errorf("frozen JSON video media-arrays adapter version is invalid")
	}
	implementation, ok := model.ResolveLinkImplementation(dto.LinkImplementationRef{
		ID: task.PrivateData.LinkImplementationID, Version: task.PrivateData.LinkImplementationVersion,
	})
	if !ok || implementation.ID != model.LinkImplementationFeicaiSeedanceVideos ||
		implementation.ContentHash != strings.TrimSpace(task.PrivateData.LinkImplementationHash) {
		return "", "", true, fmt.Errorf("frozen JSON video media-arrays implementation is invalid")
	}
	key := strings.TrimSpace(task.PrivateData.Key)
	if key == "" {
		return "", "", true, fmt.Errorf("frozen JSON video media-arrays credential is unavailable")
	}
	resultURL, err := relaycommon.ValidateSameOriginHTTPSVideoResultURL(
		task.PrivateData.ResultURL,
		strings.TrimSpace(task.PrivateData.VideoUpstreamQueryBaseURL),
	)
	if err != nil {
		return "", "", true, fmt.Errorf("frozen JSON video media-arrays result URL is invalid")
	}
	return resultURL, key, true, nil
}

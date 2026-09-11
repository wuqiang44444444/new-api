package seedance

import (
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

const (
	modelSeedance20     = "doubao-seedance-2-0-260128"
	modelSeedance20Fast = "doubao-seedance-2-0-fast-260128"
	modelSeedance20Mini = "doubao-seedance-2-0-mini-260615"
	modelSeedance25     = "doubao-seedance-2-5-260628"
)

type providerModelSpec struct {
	defaultDuration          int
	minDuration              int
	maxDuration              int
	intelligentDuration      int
	allowIntelligentDuration bool
	resolutions              map[string]struct{}
	maxImages                int
	maxVideos                int
	maxAudios                int
	maxTotalMedia            int
	allowVideos              bool
	allowAudios              bool
	allowAudioOnly           bool
	defaultGenerateAudio     bool
	outputFormats            map[string]struct{}
}

func stringSet(values ...string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func providerModelFromRelayInfo(info *relaycommon.RelayInfo, requestModel string) string {
	if info != nil && info.ChannelMeta != nil && strings.TrimSpace(info.UpstreamModelName) != "" {
		return strings.TrimSpace(info.UpstreamModelName)
	}
	return strings.TrimSpace(requestModel)
}

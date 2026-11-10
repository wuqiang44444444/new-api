// Package minimax owns the host boundary of the MiniMax Link typed video
// channel: typed channel admission, plugin hook invocation, frozen execution,
// usage normalization and content source resolution. Wire conversion and
// observation hooks are declared in the versioned minimax-link artifact.
package minimax

import (
	"strconv"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// VideoUpstreamProtocolJdCloudTaskV1 is the first code-registered southbound
// protocol of the MiniMax Link channel type. It is intentionally not added to
// the Seedance protocol enum in relaykit/dto: validation for this protocol
// lives in this package and in the versioned artifact declaration.
const VideoUpstreamProtocolJdCloudTaskV1 dto.VideoUpstreamProtocol = constant.VideoUpstreamProtocolJdCloudTaskV1

// ChannelName mirrors the artifact key family naming of the typed channels.
const ChannelName = "minimax-link"

// ChannelType is the typed channel this package serves.
const ChannelType = constant.ChannelTypeMiniMaxLink

// Platform is the numeric task platform identity shared by typed video tasks.
var Platform = constant.TaskPlatform(strconv.Itoa(ChannelType))

// protocolName identifies the JD Cloud artifact contract in its fixtures.
func protocolName() string {
	return string(VideoUpstreamProtocolJdCloudTaskV1)
}

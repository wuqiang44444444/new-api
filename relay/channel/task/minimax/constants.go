// Package minimax owns the host boundary of the MiniMax Link typed video
// channel: typed channel admission, plugin hook invocation, frozen execution,
// credit usage normalization and content source resolution. The JD Cloud
// request conversion, response envelope parsing, status mapping and result
// field location live exclusively in the versioned minimax-link artifact;
// this package never re-implements them.
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
const VideoUpstreamProtocolJdCloudTaskV1 dto.VideoUpstreamProtocol = "jdcloud_video_task_v1"

// Fixed JD Cloud task API paths. They are code-registered per protocol and
// never administrator-configured.
const (
	jdCloudCreatePath        = "/v1/task/submit"
	jdCloudQueryPathTemplate = "/v1/task/{task_id}"
)

// ChannelName mirrors the artifact key family naming of the typed channels.
const ChannelName = "minimax-link"

// ChannelType is the typed channel this package serves.
const ChannelType = constant.ChannelTypeMiniMaxLink

// Platform is the numeric task platform identity shared by typed video tasks.
var Platform = constant.TaskPlatform(strconv.Itoa(ChannelType))

// protocolName returns the registered southbound protocol identity used for
// pinning and hook dispatch.
func protocolName() string {
	return string(VideoUpstreamProtocolJdCloudTaskV1)
}

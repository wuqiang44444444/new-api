// Package minimaxlink holds the leaf-level MiniMax Link billing contract so
// pricing helpers can reference it without importing the relay adaptor tree.
package minimaxlink

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
)

// IsChannelType reports whether the channel belongs to the MiniMax Link
// typed contract.
func IsChannelType(channelType int) bool {
	return channelType == constant.ChannelTypeMiniMaxLink
}

// IsProtocol reports whether the protocol is the registered JD Cloud task
// protocol.
func IsProtocol(protocol string) bool {
	return protocol == constant.VideoUpstreamProtocolJdCloudTaskV1
}

// UsageFields is the MiniMax usage field contract for expressions. The
// jdcloud protocol exposes host-derived probe facts only: request seconds,
// resolution/ratio selectors across supported artifact versions and the video-input flag.
// These billing selectors do not grant request capabilities: admission and
// public API enums always come from the pinned artifact declaration. Credit
// evidence is provider accounting, never a customer pricing meter, so no
// credit field is exposed and u("tokens") stays out of the schema.
func UsageFields() map[string]jsplugin.UsageFieldSchema {
	return map[string]jsplugin.UsageFieldSchema{
		"duration_seconds": {Type: "number", Unit: "second"},
		"resolution":       {Enum: []string{"768p", "2k"}},
		"ratio":            {Enum: []string{"16:9", "21:9", "4:3", "1:1", "3:4", "9:16", "adaptive"}},
		"has_video_input":  {Type: "boolean"},
	}
}

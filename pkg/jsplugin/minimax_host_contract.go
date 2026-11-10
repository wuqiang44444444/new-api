package jsplugin

import "github.com/QuantumNous/new-api/constant"

// MinimaxPluginKey identifies the MiniMax standard-video typed extension
// artifact ("minimax-link"). It shares the typed-extension artifact contract
// (meta shape, hook validation, channelConfiguration declaration) with
// seedance-link but is a fully separate key, artifact file, version store and
// lifecycle: the embedded artifact is deployed without auto-activation and an
// administrator must explicitly activate a version before the channel type
// admits new requests. Native plugin routing never consumes this key.
const MinimaxPluginKey = "minimax-link"

// MinimaxHostContract returns the compile contract for the minimax-link
// artifact. Asset capability stays at the "none" placeholder.
func MinimaxHostContract() SeedanceExtensionContract {
	return SeedanceExtensionContract{
		Key: MinimaxPluginKey,
		Protocols: []SeedanceExtensionProtocol{
			{Name: constant.VideoUpstreamProtocolJdCloudTaskV1, Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
			{Name: constant.VideoUpstreamProtocolMiniMaxOfficialV2, Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
			{Name: constant.VideoUpstreamProtocolNexhinaHailuoV2, Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
		},
		AssetProtocols: []string{"none"},
	}
}

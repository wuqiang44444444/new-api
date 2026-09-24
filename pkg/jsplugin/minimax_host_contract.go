package jsplugin

// MinimaxPluginKey identifies the MiniMax standard-video typed extension
// artifact ("minimax-link"). It shares the typed-extension artifact contract
// (meta shape, hook validation, channelConfiguration declaration) with
// seedance-link but is a fully separate key, artifact file, version store and
// lifecycle: the embedded artifact is deployed without auto-activation and an
// administrator must explicitly activate a version before the channel type
// admits new requests. Native plugin routing never consumes this key.
const MinimaxPluginKey = "minimax-link"

// MinimaxHostContract returns the compile contract for the minimax-link
// artifact. The first registered southbound protocol is the JD Cloud task API
// conversion; asset capability stays at the "none" placeholder because the
// MiniMax Link contract publishes no asset operations.
func MinimaxHostContract() SeedanceExtensionContract {
	return SeedanceExtensionContract{
		Key: MinimaxPluginKey,
		Protocols: []SeedanceExtensionProtocol{
			{Name: "jdcloud_video_task_v1", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
		},
		AssetProtocols: []string{"none"},
	}
}

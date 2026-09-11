package jsplugin

// SeedancePluginKey identifies the local typed extension. This host contract
// lists implemented operations only; Provider models and pairing belong to the
// versioned artifact. Native plugin routing does not consume this declaration.
const SeedancePluginKey = "seedance-link"

// SeedanceHostedAssetProtocol is the only asset protocol backed by the host's
// platform-hosted material capability. Declarations that pair hosted group
// policy with any other protocol cannot be served and are rejected at the
// compile/publication boundary instead of at request time.
const SeedanceHostedAssetProtocol = "funcloud_material_hosted"

func SeedanceHostContract() SeedanceExtensionContract {
	return SeedanceExtensionContract{
		Key: SeedancePluginKey,
		Protocols: []SeedanceExtensionProtocol{
			{Name: "funcloud_modelark_v3", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
			{Name: "synlink_video_v1", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
			{Name: "modelark_v3_cmcc", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
			{Name: "tokensave_media_task_v1", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
			{Name: "moxing_modelark_media_v1", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
			{Name: "ark_media_v1", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
			{Name: "modelark_v3_byteplus", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
			{Name: "modelark_v3_volcengine", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
			{Name: "feicai_videos_v1", Hooks: []string{"buildCreate", "parseCreateResponse", "parseTaskObservation"}},
		},
		AssetProtocols: []string{"funcloud_material", "funcloud_material_hosted", "cmcc_aicc_assets_v2", "tokensave_assets_v1", "moxing_volc_assets_v1", "ark_assets_v1", "volcengine_assets_action_v2024_01_01", "byteplus_assets_action_v2024_01_01"},
	}
}

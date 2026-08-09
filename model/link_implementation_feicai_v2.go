package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func feicaiSeedanceV2Implementation() LinkImplementation {
	return LinkImplementation{
		ID:         LinkImplementationFeicaiSeedanceVideos,
		Version:    LinkImplementationVersionV2,
		Provider:   "飞彩",
		PlanName:   "Seedance 2.0 Video（后缀模型）",
		ContractID: "modelark.contents.generations.v3",
		PublicSKUs: []string{
			VideoSKUSeedance20Mini720P,
			VideoSKUSeedance20SD2720P,
			VideoSKUSeedance20Fast720P,
			VideoSKUSeedance20Value720P,
			VideoSKUSeedance20Standard720P,
			VideoSKUSeedance20Value1080P,
			VideoSKUSeedance20Standard1080P,
			VideoSKUSeedance20Value4K,
			VideoSKUSeedance20Standard4K,
			VideoSKUSeedance20ProPI720P,
		},
		ChannelType:            constant.ChannelTypeDoubaoVideo,
		RequiredVideoProfile:   VideoProfileJSONMediaArrays,
		RequiredAssetProfile:   string(dto.AssetUpstreamProfileNone),
		RequiredCreatePath:     "/v1/videos",
		RequiredQueryPath:      "/v1/videos/{task_id}",
		RequiredAdapterVersion: "54:third_party_json_video_media_arrays:v2",
		ExecutionBindings: []LinkExecutionBinding{
			{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileJSONMediaArrays, ProviderModel: FeicaiProviderModelSeedance20Mini720P, LinkSKU: VideoSKUSeedance20Mini720P},
			{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileJSONMediaArrays, ProviderModel: FeicaiProviderModelSeedance20SD2720P, LinkSKU: VideoSKUSeedance20SD2720P},
			{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileJSONMediaArrays, ProviderModel: FeicaiProviderModelSeedance20Fast720P, LinkSKU: VideoSKUSeedance20Fast720P},
			{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileJSONMediaArrays, ProviderModel: FeicaiProviderModelSeedance20Value720P, LinkSKU: VideoSKUSeedance20Value720P},
			{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileJSONMediaArrays, ProviderModel: FeicaiProviderModelSeedance20Standard720P, LinkSKU: VideoSKUSeedance20Standard720P},
			{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileJSONMediaArrays, ProviderModel: FeicaiProviderModelSeedance20Value1080P, LinkSKU: VideoSKUSeedance20Value1080P},
			{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileJSONMediaArrays, ProviderModel: FeicaiProviderModelSeedance20Standard1080P, LinkSKU: VideoSKUSeedance20Standard1080P},
			{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileJSONMediaArrays, ProviderModel: FeicaiProviderModelSeedance20Value4K, LinkSKU: VideoSKUSeedance20Value4K},
			{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileJSONMediaArrays, ProviderModel: FeicaiProviderModelSeedance20Standard4K, LinkSKU: VideoSKUSeedance20Standard4K},
			{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileJSONMediaArrays, ProviderModel: FeicaiProviderModelSeedance20ProPI720P, LinkSKU: VideoSKUSeedance20ProPI720P},
		},
		AssetCapability: LinkAssetImplementationCapability{
			ResolutionModes:     []LinkAssetResolutionMode{LinkAssetResolutionSourceURL},
			SourceMinTTLSeconds: 3600,
			AssetKinds:          []string{AssetKindGeneral},
			MediaTypes:          []string{"image", "audio", "video"},
			MaxImages:           9,
			MaxVideos:           3,
			MaxAudio:            3,
		},
		TaskContract:    "shared_video_task",
		BillingContract: "newapi_quota",
	}
}

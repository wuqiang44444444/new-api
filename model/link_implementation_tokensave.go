package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func tokenSaveSeedanceV1Implementation() LinkImplementation {
	return LinkImplementation{
		ID: LinkImplementationTokenSaveSeedance, Version: LinkImplementationVersionV1, Provider: "Moxing", PlanName: "Seedance 2.0 Overseas (Official Key)",
		Deprecated: true,
		ContractID: "modelark.contents.generations.v3", PublicSKUs: []string{VideoSKUDoubaoSeedance20260128},
		ChannelType: constant.ChannelTypeDoubaoVideo, RequiredVideoProfile: VideoProfileThirdPartyRelay,
		RequiredAssetProfile: string(dto.AssetUpstreamProfileRelay), RequiredCreatePath: "/v1/media/generations", RequiredQueryPath: "/v1/media/tasks/{task_id}", RequiredAdapterVersion: "54:third_party_relay:v1",
		ExecutionBindings: []LinkExecutionBinding{{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileThirdPartyRelay, ProviderModel: VideoSKUDoubaoSeedance20260128, LinkSKU: VideoSKUDoubaoSeedance20260128}},
		AssetCapability:   LinkAssetImplementationCapability{SupportsManagedAssets: true, ResolutionModes: []LinkAssetResolutionMode{LinkAssetResolutionUpstreamBinding}, AssetKinds: []string{AssetKindGeneral, AssetKindRealPerson}, MediaTypes: []string{"image", "video", "audio"}},
		TaskContract:      "shared_video_task", BillingContract: "newapi_quota",
	}
}

func tokenSaveSeedanceV2Implementation() LinkImplementation {
	return LinkImplementation{
		ID: LinkImplementationTokenSaveSeedance, Version: LinkImplementationVersionV2, Provider: "Moxing", PlanName: "Seedance 2.0 Overseas (Official Key)",
		ContractID: "modelark.contents.generations.v3", PublicSKUs: []string{VideoSKUDoubaoSeedance20260128},
		ChannelType: constant.ChannelTypeDoubaoVideo, RequiredVideoProfile: VideoProfileThirdPartyRelay,
		RequiredAssetProfile: string(dto.AssetUpstreamProfileRelay), RequiredCreatePath: "/v1/media/generations", RequiredQueryPath: "/v1/media/tasks/{task_id}", RequiredAdapterVersion: "54:third_party_relay:v2",
		ExecutionBindings: []LinkExecutionBinding{{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileThirdPartyRelay, ProviderModel: VideoSKUDoubaoSeedance20260128, LinkSKU: VideoSKUDoubaoSeedance20260128}},
		AssetCapability: LinkAssetImplementationCapability{
			SupportsManagedAssets: true,
			ResolutionModes:       []LinkAssetResolutionMode{LinkAssetResolutionUpstreamBinding},
			AssetKinds:            []string{AssetKindGeneral},
			MediaTypes:            []string{"image"},
			MaxImages:             9,
		},
		TaskContract: "shared_video_task", BillingContract: "newapi_quota",
	}
}

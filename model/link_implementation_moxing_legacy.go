package model

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const LinkImplementationMoxingSeedanceArk = "moxing.seedance-ark-assets"

// moxingSeedanceArkV1Implementation preserves the immutable execution shape
// required to read fully frozen historical tasks. Deprecated implementations
// are excluded from new-task candidates and cannot be saved on a channel.
func moxingSeedanceArkV1Implementation() LinkImplementation {
	return LinkImplementation{
		ID: LinkImplementationMoxingSeedanceArk, Version: LinkImplementationVersionV1, Provider: "Moxing",
		Deprecated: true,
		ContractID: "modelark.contents.generations.v3", PublicSKUs: []string{VideoSKUSeedance20Oversea},
		ChannelType: constant.ChannelTypeDoubaoVideo, RequiredVideoProfile: VideoProfileThirdPartyReverse,
		RequiredAssetProfile: string(dto.AssetUpstreamProfileArk), RequiredAdapterVersion: "54:third_party_reverse_proxy:v1",
		ExecutionBindings: []LinkExecutionBinding{{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileThirdPartyReverse, ProviderModel: VideoSKUSeedance20Oversea, LinkSKU: VideoSKUSeedance20Oversea}},
		AssetCapability:   LinkAssetImplementationCapability{SupportsManagedAssets: true, ResolutionModes: []LinkAssetResolutionMode{LinkAssetResolutionUpstreamBinding}, AssetKinds: []string{AssetKindGeneral, AssetKindRealPerson}, MediaTypes: []string{"image", "video", "audio"}},
		TaskContract:      "shared_video_task", BillingContract: "newapi_quota",
	}
}

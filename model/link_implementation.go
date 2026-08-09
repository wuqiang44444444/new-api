package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

const linkImplementationHashVersion = "link-implementation-hash-v2"

const (
	LinkImplementationBytePlusSeedanceArk  = "byteplus.seedance-ark"
	LinkImplementationMoxingSeedanceMedia  = "moxing.seedance-media-task"
	LinkImplementationTokenSaveSeedance    = "tokensave.seedance-media-task"
	LinkImplementationFeicaiSeedanceVideos = "feicai.seedance-videos"
	LinkImplementationFunCloudSeedance     = "funcloud.seedance-json"
	LinkImplementationMoxingImages         = "moxing.images.media-task"
	LinkImplementationQihangImages         = "qihang.images.openai-compatible"
	LinkImplementationKlingVideos          = "kling.videos-official"
	LinkImplementationJimengVideos         = "jimeng.videos-official"

	LinkImplementationVersionV1 = "v1"
	LinkImplementationVersionV2 = "v2"
)

type LinkAssetResolutionMode string

const (
	LinkAssetResolutionUpstreamBinding LinkAssetResolutionMode = "upstream_binding"
	LinkAssetResolutionSourceURL       LinkAssetResolutionMode = "source_url"
)

type LinkAssetImplementationCapability struct {
	SupportsManagedAssets   bool                      `json:"supports_managed_assets"`
	ResolutionModes         []LinkAssetResolutionMode `json:"asset_resolution_modes,omitempty"`
	SourceMinTTLSeconds     int64                     `json:"asset_source_min_ttl_seconds,omitempty"`
	AssetKinds              []string                  `json:"asset_kinds,omitempty"`
	MediaTypes              []string                  `json:"media_types,omitempty"`
	MaxImages               int                       `json:"max_images,omitempty"`
	MaxVideos               int                       `json:"max_videos,omitempty"`
	MaxAudio                int                       `json:"max_audio,omitempty"`
	SupportsMixedMediaPaths bool                      `json:"supports_mixed_media_paths"`
}

func (capability LinkAssetImplementationCapability) Supports(mode LinkAssetResolutionMode) bool {
	return slices.Contains(capability.ResolutionModes, mode)
}

type LinkRouteRequirement struct {
	PublicSKU    string `json:"public_sku"`
	IncomingPath string `json:"incoming_path"`
	UpstreamPath string `json:"upstream_path"`
	Converter    string `json:"converter"`
	AuthType     string `json:"auth_type"`
}

type LinkSKUPathRequirement struct {
	PublicSKU  string `json:"public_sku"`
	CreatePath string `json:"create_path"`
}

// LinkImplementation is the code-owned implementation contract selected by a
// channel. Provider and PlanName are display-only; execution identity is ID +
// Version + ContentHash.
type LinkImplementation struct {
	ID                     string                            `json:"id"`
	Version                string                            `json:"version"`
	ContentHash            string                            `json:"content_hash"`
	Provider               string                            `json:"provider"`
	PlanName               string                            `json:"plan_name,omitempty"`
	Deprecated             bool                              `json:"deprecated,omitempty"`
	ContractID             string                            `json:"contract_id"`
	PublicSKUs             []string                          `json:"public_skus"`
	ChannelType            int                               `json:"channel_type"`
	RequiredVideoProfile   string                            `json:"required_video_profile,omitempty"`
	RequiredAssetProfile   string                            `json:"required_asset_profile,omitempty"`
	RequiredCreatePath     string                            `json:"required_create_path,omitempty"`
	RequiredSKUCreatePaths []LinkSKUPathRequirement          `json:"required_sku_create_paths,omitempty"`
	RequiredQueryPath      string                            `json:"required_query_path,omitempty"`
	RequiredAdapterVersion string                            `json:"required_adapter_version,omitempty"`
	RequiredRoutes         []LinkRouteRequirement            `json:"required_routes,omitempty"`
	ExecutionBindings      []LinkExecutionBinding            `json:"execution_bindings"`
	AssetCapability        LinkAssetImplementationCapability `json:"asset_capability"`
	TaskContract           string                            `json:"task_contract"`
	BillingContract        string                            `json:"billing_contract"`
}

type linkImplementationHashMaterial struct {
	HashVersion            string                            `json:"hash_version"`
	ID                     string                            `json:"id"`
	Version                string                            `json:"version"`
	ContractID             string                            `json:"contract_id"`
	PublicSKUs             []string                          `json:"public_skus"`
	ChannelType            int                               `json:"channel_type"`
	RequiredVideoProfile   string                            `json:"required_video_profile,omitempty"`
	RequiredAssetProfile   string                            `json:"required_asset_profile,omitempty"`
	RequiredCreatePath     string                            `json:"required_create_path,omitempty"`
	RequiredSKUCreatePaths []LinkSKUPathRequirement          `json:"required_sku_create_paths,omitempty"`
	RequiredQueryPath      string                            `json:"required_query_path,omitempty"`
	RequiredAdapterVersion string                            `json:"required_adapter_version,omitempty"`
	RequiredRoutes         []LinkRouteRequirement            `json:"required_routes,omitempty"`
	ExecutionBindings      []LinkExecutionBinding            `json:"execution_bindings"`
	AssetCapability        LinkAssetImplementationCapability `json:"asset_capability"`
	TaskContract           string                            `json:"task_contract"`
	BillingContract        string                            `json:"billing_contract"`
}

var linkImplementationRegistry, linkImplementationRegistryBuildErr = buildLinkImplementationRegistry()

func buildLinkImplementationRegistry() (map[string]LinkImplementation, error) {
	implementations := []LinkImplementation{
		{
			ID: LinkImplementationBytePlusSeedanceArk, Version: LinkImplementationVersionV1, Provider: "BytePlus", PlanName: "Seedance Official",
			ContractID: "modelark.contents.generations.v3", PublicSKUs: []string{VideoSKUSeedanceBytePlus},
			ChannelType: constant.ChannelTypeDoubaoVideo, RequiredVideoProfile: VideoProfileOfficial,
			RequiredAssetProfile: string(dto.AssetUpstreamProfileOfficial), RequiredAdapterVersion: "54:official:v1",
			ExecutionBindings: []LinkExecutionBinding{{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileOfficial, ProviderModel: VideoSKUSeedanceBytePlus, LinkSKU: VideoSKUSeedanceBytePlus}},
			AssetCapability:   LinkAssetImplementationCapability{SupportsManagedAssets: true, ResolutionModes: []LinkAssetResolutionMode{LinkAssetResolutionUpstreamBinding}, AssetKinds: []string{AssetKindGeneral, AssetKindRealPerson}, MediaTypes: []string{"image", "video", "audio"}},
			TaskContract:      "shared_video_task", BillingContract: "newapi_quota",
		},
		{
			ID: LinkImplementationMoxingSeedanceMedia, Version: LinkImplementationVersionV2, Provider: "Moxing", PlanName: "Seedance 2.0 Overseas",
			ContractID: "modelark.contents.generations.v3", PublicSKUs: []string{VideoSKUSeedance20Oversea},
			ChannelType: constant.ChannelTypeDoubaoVideo, RequiredVideoProfile: VideoProfileThirdPartyRelay,
			RequiredAssetProfile: string(dto.AssetUpstreamProfileRelay), RequiredCreatePath: "/v1/media/generations", RequiredQueryPath: "/v1/media/tasks/{task_id}", RequiredAdapterVersion: "54:third_party_relay:v2",
			ExecutionBindings: []LinkExecutionBinding{{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileThirdPartyRelay, ProviderModel: VideoSKUSeedance20Oversea, LinkSKU: VideoSKUSeedance20Oversea}},
			AssetCapability: LinkAssetImplementationCapability{
				SupportsManagedAssets: true,
				ResolutionModes:       []LinkAssetResolutionMode{LinkAssetResolutionUpstreamBinding},
				AssetKinds:            []string{AssetKindGeneral},
				MediaTypes:            []string{"image"},
				MaxImages:             9,
			},
			TaskContract: "shared_video_task", BillingContract: "newapi_quota",
		},
		moxingSeedanceArkV1Implementation(),
		tokenSaveSeedanceV1Implementation(),
		tokenSaveSeedanceV2Implementation(),
		feicaiSeedanceV2Implementation(),
		{
			ID: LinkImplementationFunCloudSeedance, Version: LinkImplementationVersionV1, Provider: "FunCloud", PlanName: "Seedance 2.0 Video",
			ContractID: "modelark.contents.generations.v3", PublicSKUs: []string{VideoSKUSeedance20Standard, VideoSKUSeedance20Fast},
			ChannelType: constant.ChannelTypeDoubaoVideo, RequiredVideoProfile: VideoProfileFunCloudSeedanceV2,
			RequiredAssetProfile: string(dto.AssetUpstreamProfileNone), RequiredQueryPath: "/api/v2/open/aigc/{task_id}", RequiredAdapterVersion: "54:third_party_funcloud_seedance_v2:v2",
			RequiredSKUCreatePaths: []LinkSKUPathRequirement{{PublicSKU: VideoSKUSeedance20Standard, CreatePath: "/api/v2/open/aigc/seedance2-0"}, {PublicSKU: VideoSKUSeedance20Fast, CreatePath: "/api/v2/open/aigc/seedance2-0-fast"}},
			ExecutionBindings: []LinkExecutionBinding{
				{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileFunCloudSeedanceV2, ProviderModel: VideoSKUSeedance20Standard, LinkSKU: VideoSKUSeedance20Standard},
				{RouteFamily: LinkRouteFamilyModelArkVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileFunCloudSeedanceV2, ProviderModel: VideoSKUSeedance20Fast, LinkSKU: VideoSKUSeedance20Fast},
			},
			AssetCapability: LinkAssetImplementationCapability{ResolutionModes: []LinkAssetResolutionMode{LinkAssetResolutionSourceURL}, SourceMinTTLSeconds: 300, AssetKinds: []string{AssetKindGeneral}, MediaTypes: []string{"image", "video", "audio"}, MaxImages: 3, MaxVideos: 1, MaxAudio: 1},
			TaskContract:    "shared_video_task", BillingContract: "newapi_quota",
		},
		{
			ID: LinkImplementationMoxingImages, Version: LinkImplementationVersionV1, Provider: "Moxing", PlanName: "Image Generation",
			ContractID: "newapi.images.generations.v1", PublicSKUs: []string{"seedream-5-moxing", "nano-banana-2"},
			ChannelType: constant.ChannelTypeAdvancedCustom,
			RequiredRoutes: []LinkRouteRequirement{
				{PublicSKU: "seedream-5-moxing", IncomingPath: "/v1/images/generations", UpstreamPath: "/v1/images/generations", Converter: dto.AdvancedCustomConverterMediaTaskImageBlocking, AuthType: dto.AdvancedCustomAuthTypeHeader},
				{PublicSKU: "nano-banana-2", IncomingPath: "/v1/images/generations", UpstreamPath: "/v1/media/generations", Converter: dto.AdvancedCustomConverterMediaTaskImageBlocking, AuthType: dto.AdvancedCustomAuthTypeHeader},
			},
			ExecutionBindings: []LinkExecutionBinding{
				{RouteFamily: LinkRouteFamilyImageGeneration, Action: LinkExecutionActionCreate, Profile: dto.AdvancedCustomConverterMediaTaskImageBlocking, ProviderModel: "seedream-5-0-260128", LinkSKU: "seedream-5-moxing"},
				{RouteFamily: LinkRouteFamilyImageGeneration, Action: LinkExecutionActionCreate, Profile: dto.AdvancedCustomConverterMediaTaskImageBlocking, ProviderModel: "gemini-3.1-flash-image-preview-usage", LinkSKU: "nano-banana-2"},
			},
			AssetCapability: LinkAssetImplementationCapability{}, TaskContract: "shared_image_task", BillingContract: "newapi_quota",
		},
		{
			ID: LinkImplementationQihangImages, Version: LinkImplementationVersionV1, Provider: "Qihang", PlanName: "Image Generation",
			ContractID: "newapi.images.generations.v1", PublicSKUs: []string{"seedream-5-qihang"},
			ChannelType:       constant.ChannelTypeAdvancedCustom,
			RequiredRoutes:    []LinkRouteRequirement{{PublicSKU: "seedream-5-qihang", IncomingPath: "/v1/images/generations", UpstreamPath: "/v1/images/generations", Converter: "none", AuthType: dto.AdvancedCustomAuthTypeHeader}},
			ExecutionBindings: []LinkExecutionBinding{{RouteFamily: LinkRouteFamilyImageGeneration, Action: LinkExecutionActionCreate, Profile: "none", ProviderModel: "seedream-5", LinkSKU: "seedream-5-qihang"}},
			AssetCapability:   LinkAssetImplementationCapability{}, TaskContract: "synchronous_image_or_shared_task", BillingContract: "newapi_quota",
		},
		{
			ID: LinkImplementationKlingVideos, Version: LinkImplementationVersionV1, Provider: "Kling", PlanName: "Video Generation",
			ContractID: "kling.v1.videos", PublicSKUs: []string{VideoSKUKlingV1, VideoSKUKlingV16, VideoSKUKlingV2Master},
			ChannelType: constant.ChannelTypeKling, RequiredVideoProfile: VideoProfileOfficial, RequiredAdapterVersion: "50:official:v1",
			ExecutionBindings: []LinkExecutionBinding{
				{RouteFamily: LinkRouteFamilyKlingVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileOfficial, ProviderModel: VideoSKUKlingV1, LinkSKU: VideoSKUKlingV1},
				{RouteFamily: LinkRouteFamilyKlingVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileOfficial, ProviderModel: VideoSKUKlingV16, LinkSKU: VideoSKUKlingV16},
				{RouteFamily: LinkRouteFamilyKlingVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileOfficial, ProviderModel: VideoSKUKlingV2Master, LinkSKU: VideoSKUKlingV2Master},
			},
			AssetCapability: LinkAssetImplementationCapability{ResolutionModes: []LinkAssetResolutionMode{LinkAssetResolutionSourceURL}, SourceMinTTLSeconds: 300, AssetKinds: []string{AssetKindGeneral}, MediaTypes: []string{"image"}, MaxImages: 2},
			TaskContract:    "shared_video_task", BillingContract: "newapi_quota",
		},
		{
			ID: LinkImplementationJimengVideos, Version: LinkImplementationVersionV1, Provider: "Jimeng", PlanName: "Video Generation",
			ContractID: "jimeng.cv.async.2022-08-31", PublicSKUs: []string{VideoSKUJimengVGFMT2VL20},
			ChannelType: constant.ChannelTypeJimeng, RequiredVideoProfile: VideoProfileOfficial, RequiredAdapterVersion: "51:official:v1",
			ExecutionBindings: []LinkExecutionBinding{{RouteFamily: LinkRouteFamilyJimengVideo, Action: LinkExecutionActionCreate, Profile: VideoProfileOfficial, ProviderModel: VideoSKUJimengVGFMT2VL20, LinkSKU: VideoSKUJimengVGFMT2VL20}},
			AssetCapability:   LinkAssetImplementationCapability{ResolutionModes: []LinkAssetResolutionMode{LinkAssetResolutionSourceURL}, SourceMinTTLSeconds: 300, AssetKinds: []string{AssetKindGeneral}, MediaTypes: []string{"image"}},
			TaskContract:      "shared_video_task", BillingContract: "newapi_quota",
		},
	}
	return buildLinkImplementationRegistryFrom(implementations)
}

func buildLinkImplementationRegistryFrom(implementations []LinkImplementation) (map[string]LinkImplementation, error) {
	registry := make(map[string]LinkImplementation, len(implementations))
	displayNames := make(map[string]struct{}, len(implementations))
	for _, implementation := range implementations {
		implementation = normalizeLinkImplementation(implementation)
		if implementation.ID == "" {
			return nil, fmt.Errorf("Link implementation ID is required")
		}
		key := linkImplementationRegistryKey(implementation.ID, implementation.Version)
		if _, exists := registry[key]; exists {
			return nil, fmt.Errorf("duplicate Link implementation identity %q/%q", implementation.ID, implementation.Version)
		}
		if !implementation.Deprecated && implementation.PlanName != "" {
			displayKey := implementation.Provider + "\x00" + implementation.PlanName
			if _, exists := displayNames[displayKey]; exists {
				return nil, fmt.Errorf("duplicate selectable Link plan display name %q", implementation.Provider+" · "+implementation.PlanName)
			}
			displayNames[displayKey] = struct{}{}
		}
		implementation.ContentHash = linkImplementationContentHash(implementation)
		registry[key] = implementation
	}
	return registry, nil
}

func normalizeLinkImplementation(implementation LinkImplementation) LinkImplementation {
	implementation.ID = strings.TrimSpace(implementation.ID)
	implementation.Version = strings.TrimSpace(implementation.Version)
	implementation.Provider = strings.TrimSpace(implementation.Provider)
	implementation.PlanName = strings.TrimSpace(implementation.PlanName)
	implementation.ContractID = strings.TrimSpace(implementation.ContractID)
	implementation.RequiredVideoProfile = strings.TrimSpace(implementation.RequiredVideoProfile)
	implementation.RequiredAssetProfile = strings.TrimSpace(implementation.RequiredAssetProfile)
	implementation.RequiredCreatePath = strings.TrimSpace(implementation.RequiredCreatePath)
	implementation.RequiredQueryPath = strings.TrimSpace(implementation.RequiredQueryPath)
	implementation.RequiredAdapterVersion = strings.TrimSpace(implementation.RequiredAdapterVersion)
	implementation.TaskContract = strings.TrimSpace(implementation.TaskContract)
	implementation.BillingContract = strings.TrimSpace(implementation.BillingContract)
	implementation.PublicSKUs = normalizedStringSet(implementation.PublicSKUs)
	implementation.AssetCapability.AssetKinds = normalizedStringSet(implementation.AssetCapability.AssetKinds)
	implementation.AssetCapability.MediaTypes = normalizedStringSet(implementation.AssetCapability.MediaTypes)
	implementation.AssetCapability.ResolutionModes = normalizedResolutionModes(implementation.AssetCapability.ResolutionModes)
	sort.Slice(implementation.RequiredRoutes, func(i, j int) bool {
		left, right := implementation.RequiredRoutes[i], implementation.RequiredRoutes[j]
		return left.PublicSKU+"\x00"+left.IncomingPath+"\x00"+left.UpstreamPath < right.PublicSKU+"\x00"+right.IncomingPath+"\x00"+right.UpstreamPath
	})
	for index := range implementation.ExecutionBindings {
		implementation.ExecutionBindings[index] = normalizeLinkExecutionBinding(implementation.ExecutionBindings[index])
	}
	sort.Slice(implementation.ExecutionBindings, func(i, j int) bool {
		return linkExecutionBindingKey(implementation.ExecutionBindings[i]) < linkExecutionBindingKey(implementation.ExecutionBindings[j])
	})
	sort.Slice(implementation.RequiredSKUCreatePaths, func(i, j int) bool {
		return implementation.RequiredSKUCreatePaths[i].PublicSKU < implementation.RequiredSKUCreatePaths[j].PublicSKU
	})
	return implementation
}

func normalizedStringSet(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizedResolutionModes(values []LinkAssetResolutionMode) []LinkAssetResolutionMode {
	stringsSet := make([]string, 0, len(values))
	for _, value := range values {
		stringsSet = append(stringsSet, string(value))
	}
	stringsSet = normalizedStringSet(stringsSet)
	result := make([]LinkAssetResolutionMode, len(stringsSet))
	for i, value := range stringsSet {
		result[i] = LinkAssetResolutionMode(value)
	}
	return result
}

func linkImplementationContentHash(implementation LinkImplementation) string {
	implementation = normalizeLinkImplementation(implementation)
	material := linkImplementationHashMaterial{
		HashVersion: linkImplementationHashVersion,
		ID:          implementation.ID, Version: implementation.Version, ContractID: implementation.ContractID,
		PublicSKUs: implementation.PublicSKUs, ChannelType: implementation.ChannelType,
		RequiredVideoProfile: implementation.RequiredVideoProfile, RequiredAssetProfile: implementation.RequiredAssetProfile,
		RequiredCreatePath: implementation.RequiredCreatePath, RequiredSKUCreatePaths: implementation.RequiredSKUCreatePaths, RequiredQueryPath: implementation.RequiredQueryPath,
		RequiredAdapterVersion: implementation.RequiredAdapterVersion, RequiredRoutes: implementation.RequiredRoutes,
		ExecutionBindings: implementation.ExecutionBindings, AssetCapability: implementation.AssetCapability,
		TaskContract: implementation.TaskContract, BillingContract: implementation.BillingContract,
	}
	payload, err := common.Marshal(material)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func linkImplementationRegistryKey(id, version string) string {
	return strings.TrimSpace(id) + "\x00" + strings.TrimSpace(version)
}

func ResolveLinkImplementation(ref dto.LinkImplementationRef) (LinkImplementation, bool) {
	implementation, ok := linkImplementationRegistry[linkImplementationRegistryKey(ref.ID, ref.Version)]
	if !ok {
		return LinkImplementation{}, false
	}
	return cloneLinkImplementation(implementation), true
}

func LinkImplementationsForSKU(publicSKU string) []LinkImplementation {
	publicSKU = strings.TrimSpace(publicSKU)
	result := make([]LinkImplementation, 0)
	for _, implementation := range linkImplementationRegistry {
		if !implementation.Deprecated && slices.Contains(implementation.PublicSKUs, publicSKU) {
			result = append(result, cloneLinkImplementation(implementation))
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID+"\x00"+result[i].Version < result[j].ID+"\x00"+result[j].Version
	})
	return result
}

func ListLinkImplementations() []LinkImplementation {
	result := make([]LinkImplementation, 0, len(linkImplementationRegistry))
	for _, implementation := range linkImplementationRegistry {
		result = append(result, cloneLinkImplementation(implementation))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID+"\x00"+result[i].Version < result[j].ID+"\x00"+result[j].Version
	})
	return result
}

// ListSelectableLinkImplementations returns only implementations that may be
// assigned to channels creating new work. Deprecated versions remain in the
// registry exclusively for resolving immutable historical task snapshots.
func ListSelectableLinkImplementations() []LinkImplementation {
	result := make([]LinkImplementation, 0, len(linkImplementationRegistry))
	for _, implementation := range linkImplementationRegistry {
		if implementation.Deprecated {
			continue
		}
		result = append(result, cloneLinkImplementation(implementation))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID+"\x00"+result[i].Version < result[j].ID+"\x00"+result[j].Version
	})
	return result
}

func cloneLinkImplementation(implementation LinkImplementation) LinkImplementation {
	implementation.PublicSKUs = append([]string(nil), implementation.PublicSKUs...)
	implementation.RequiredRoutes = append([]LinkRouteRequirement(nil), implementation.RequiredRoutes...)
	implementation.ExecutionBindings = append([]LinkExecutionBinding(nil), implementation.ExecutionBindings...)
	implementation.RequiredSKUCreatePaths = append([]LinkSKUPathRequirement(nil), implementation.RequiredSKUCreatePaths...)
	implementation.AssetCapability.ResolutionModes = append([]LinkAssetResolutionMode(nil), implementation.AssetCapability.ResolutionModes...)
	implementation.AssetCapability.AssetKinds = append([]string(nil), implementation.AssetCapability.AssetKinds...)
	implementation.AssetCapability.MediaTypes = append([]string(nil), implementation.AssetCapability.MediaTypes...)
	return implementation
}

func ValidateLinkImplementationRegistry() error {
	if linkImplementationRegistryBuildErr != nil {
		return linkImplementationRegistryBuildErr
	}
	seenSKU := make(map[string]struct{})
	for key, implementation := range linkImplementationRegistry {
		if key == "" || key != linkImplementationRegistryKey(implementation.ID, implementation.Version) || implementation.Version == "" || implementation.ContractID == "" || implementation.ChannelType <= 0 || len(implementation.PublicSKUs) == 0 || len(implementation.ExecutionBindings) == 0 || (!implementation.Deprecated && implementation.PlanName == "") {
			return fmt.Errorf("Link implementation %q is incomplete", key)
		}
		if err := validateLinkExecutionBindings(implementation); err != nil {
			return fmt.Errorf("Link implementation %q/%q execution bindings: %w", implementation.ID, implementation.Version, err)
		}
		if implementation.ContentHash == "" || implementation.ContentHash != linkImplementationContentHash(implementation) {
			return fmt.Errorf("Link implementation %q/%q content hash is invalid", implementation.ID, implementation.Version)
		}
		if implementation.Deprecated {
			continue
		}
		for _, publicSKU := range implementation.PublicSKUs {
			seenSKU[publicSKU] = struct{}{}
		}
	}
	for publicSKU := range videoSKUCapabilities {
		if _, exists := seenSKU[publicSKU]; !exists {
			return fmt.Errorf("video Link SKU %q has no registered implementation", publicSKU)
		}
	}
	for publicSKU := range imageSKUCapabilities {
		if _, exists := seenSKU[publicSKU]; !exists {
			return fmt.Errorf("image Link SKU %q has no registered implementation", publicSKU)
		}
	}
	return nil
}

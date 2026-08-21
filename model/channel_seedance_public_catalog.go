package model

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/asset_setting"
)

type SeedancePublicModel struct {
	ModelName string
	Enabled   bool
	Groups    []string
	API       dto.PublicModelAPI
}

func GetConfiguredSeedancePublicModels() ([]SeedancePublicModel, error) {
	var channels []Channel
	if err := DB.Where("type = ?", constant.ChannelTypeSeedanceLink).Order("id").Find(&channels).Error; err != nil {
		return nil, err
	}

	models := make([]SeedancePublicModel, 0)
	modelIndex := make(map[string]int)
	for i := range channels {
		settings := channels[i].GetOtherSettings()
		for _, modelName := range channels[i].GetModels() {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			candidate := SeedancePublicModel{
				ModelName: modelName,
				Enabled:   channels[i].Status == common.ChannelStatusEnabled,
				Groups:    normalizedPublicModelGroups(channels[i].GetGroups()),
				API: seedancePublicModelAPI(
					&channels[i],
					modelName,
					settings.AssetUpstreamProtocol,
					settings.AssetMinURLTTLSeconds,
				),
			}
			if index, exists := modelIndex[modelName]; exists {
				models[index].Groups = mergePublicModelGroups(models[index].Groups, candidate.Groups)
				if candidate.Enabled && !models[index].Enabled {
					models[index].Enabled = true
					models[index].API = candidate.API
				}
				continue
			}
			modelIndex[modelName] = len(models)
			models = append(models, candidate)
		}
	}
	return models, nil
}

func normalizedPublicModelGroups(groups []string) []string {
	result := make([]string, 0, len(groups))
	seen := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		result = append(result, group)
	}
	return result
}

func mergePublicModelGroups(existing, additional []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(additional))
	for _, group := range existing {
		seen[group] = struct{}{}
	}
	for _, group := range additional {
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		existing = append(existing, group)
	}
	return existing
}

func seedancePublicModelAPI(
	channel *Channel,
	modelName string,
	assetProtocol dto.AssetUpstreamProtocol,
	assetMinURLTTLSeconds int64,
) dto.PublicModelAPI {
	return dto.PublicModelAPI{
		Video: dto.PublicVideoAPI{
			Protocol:          "modelark_v3",
			DocumentationPath: "/docs/api-reference/videos/modelark",
			Operations: []dto.PublicAPIOperation{
				publicAPIOperation("create_video", http.MethodPost, "/api/v3/contents/generations/tasks", true),
				publicAPIOperation("list_videos", http.MethodGet, "/api/v3/contents/generations/tasks", true),
				publicAPIOperation("get_video", http.MethodGet, "/api/v3/contents/generations/tasks/{task_id}", true),
				publicAPIOperation("delete_video", http.MethodDelete, "/api/v3/contents/generations/tasks/{task_id}", true),
				publicAPIOperation("get_video_content", http.MethodGet, "/v1/videos/{task_id}/content", true),
			},
			Creation: dto.PublicVideoCreation{
				Method: http.MethodPost, Path: "/api/v3/contents/generations/tasks", ContentType: "application/json",
				RequiredFields: []string{"model", "content"}, Model: modelName,
			},
		},
		Assets: seedancePublicAssetAPI(
			modelName,
			assetProtocol,
			assetMinURLTTLSeconds,
			seedancePublicAssetReuseScope(channel, assetProtocol),
		),
	}
}

func seedancePublicAssetAPI(
	modelName string,
	protocol dto.AssetUpstreamProtocol,
	assetMinURLTTLSeconds int64,
	reuseScope string,
) dto.PublicAssetAPI {
	assetCreate, assetRead, assetUpdate, assetDelete := false, false, false, false
	groupCreate, groupRead, groupDelete := false, false, false
	realPerson := false
	assetGroupRequirement := dto.PublicAssetGroupUnsupported
	media := make([]dto.PublicAssetMedia, 0)

	switch protocol {
	case dto.AssetUpstreamProtocolVolcengineAction,
		dto.AssetUpstreamProtocolBytePlusAction,
		dto.AssetUpstreamProtocolMoxingVolcAssetsV1:
		assetCreate, assetRead, assetUpdate, assetDelete = true, true, true, true
		groupCreate, groupRead, groupDelete = true, true, true
		realPerson = true
		assetGroupRequirement = dto.PublicAssetGroupOptional
	case dto.AssetUpstreamProtocolArkAssetsV1:
		assetCreate, assetRead, assetUpdate, assetDelete = true, true, true, true
		groupCreate, groupRead = true, true
		realPerson = true
		assetGroupRequirement = dto.PublicAssetGroupOptional
	case dto.AssetUpstreamProtocolTokenSaveAssetsV1:
		assetCreate, assetRead, assetUpdate, assetDelete = true, true, true, true
	case dto.AssetUpstreamProtocolMoxingJoyCreatorV1:
		assetCreate, assetRead, assetUpdate, assetDelete = true, true, true, true
		groupCreate, groupRead = true, true
		assetGroupRequirement = dto.PublicAssetGroupOptional
	case dto.AssetUpstreamProtocolFunCloudMaterial:
		assetCreate, assetRead = true, true
		groupCreate, groupRead, groupDelete = true, true, true
		assetGroupRequirement = dto.PublicAssetGroupRequired
	case dto.AssetUpstreamProtocolCMCCAICCV2:
		assetCreate, assetRead, assetUpdate, assetDelete = true, true, true, true
		groupCreate, groupRead, groupDelete = true, true, true
		realPerson = true
		assetGroupRequirement = dto.PublicAssetGroupRequired
	}
	if assetCreate {
		media = publicAssetMedia(realPerson, assetGroupRequirement)
		if protocol == dto.AssetUpstreamProtocolCMCCAICCV2 {
			media = append(media,
				dto.PublicAssetMedia{Kind: AssetKindRealPerson, MediaType: "video", AssetGroupRequirement: dto.PublicAssetGroupRequired},
				dto.PublicAssetMedia{Kind: AssetKindRealPerson, MediaType: "audio", AssetGroupRequirement: dto.PublicAssetGroupRequired},
			)
		}
	}

	supported := protocol != "" && protocol != dto.AssetUpstreamProtocolNone
	api := dto.PublicAssetAPI{
		Supported:         supported,
		DocumentationPath: "/docs/api-reference/assets",
		ManagementMode:    "caller_managed_stateless",
		RequiresModel:     true,
		ReferenceFormat:   "asset://{opaque_upstream_asset_id}",
		ReuseScope:        reuseScope,
		Media:             media,
		Operations: []dto.PublicAPIOperation{
			publicAPIOperation("create_asset", http.MethodPost, "/v1/assets", assetCreate),
			publicAPIOperation("list_assets", http.MethodGet, "/v1/assets", false),
			publicAPIOperation("get_asset", http.MethodGet, "/v1/assets/{asset_id}?model={model}", assetRead),
			publicAPIOperation("update_asset", http.MethodPatch, "/v1/assets/{asset_id}", assetUpdate),
			publicAPIOperation("delete_asset", http.MethodDelete, "/v1/assets/{asset_id}?model={model}", assetDelete),
			publicAPIOperation("create_asset_group", http.MethodPost, "/v1/asset-groups", groupCreate),
			publicAPIOperation("list_asset_groups", http.MethodGet, "/v1/asset-groups", false),
			publicAPIOperation("get_asset_group", http.MethodGet, "/v1/asset-groups/{group_id}?model={model}", groupRead),
			publicAPIOperation("get_asset_group_verification", http.MethodGet, "/v1/asset-groups/{session_id}?model={model}&verification_session=true", realPerson),
			publicAPIOperation("delete_asset_group", http.MethodDelete, "/v1/asset-groups/{group_id}?model={model}", groupDelete),
		},
	}
	if assetCreate {
		config := asset_setting.Current()
		requiredFields := []string{"name", "asset_kind", "media_type", "model", "source.type", "source.url"}
		example := dto.PublicAssetCreateExample{
			Name: "example-asset", AssetKind: AssetKindGeneral, MediaType: "image", Model: modelName,
			Source: dto.PublicAssetSourceExample{Type: "url", URL: "https://cdn.example.com/path/to/image.png"},
		}
		if assetGroupRequirement == dto.PublicAssetGroupRequired {
			requiredFields = append(requiredFields, "asset_group_id")
			example.AssetGroupID = "provider-group-id"
		}
		source := dto.PublicAssetSourceContract{
			Type: "url", URLScheme: "https", PublicNetworkOnly: true, Port: 443,
			MaxURLLength: config.RemoteURLMaxLength, ExpiresAtMinRemainingSeconds: assetMinURLTTLSeconds,
		}
		if protocol == dto.AssetUpstreamProtocolFunCloudMaterial {
			source.MaxBytes = dto.PublicAssetFunCloudMaxBytes
			source.RedirectLimit = dto.PublicAssetFunCloudRedirectLimit
			source.ContentTypeMustMatchMedia = true
			source.AcceptedContentTypes = []dto.PublicAssetSourceMediaTypes{
				{MediaType: "image", ContentTypes: []string{"image/jpeg", "image/png", "image/webp", "image/bmp", "image/tiff", "image/gif"}},
				{MediaType: "video", ContentTypes: []string{"video/mp4", "video/quicktime"}},
				{MediaType: "audio", ContentTypes: []string{"audio/mpeg", "audio/wav", "audio/x-wav"}},
			}
		}
		api.Creation = &dto.PublicAssetCreation{
			Method: http.MethodPost, Path: "/v1/assets", ContentType: "application/json",
			RequiredFields: requiredFields, NameMaxCharacters: dto.PublicAssetNameMaxCharacters,
			Source: source, Example: example,
		}
	}
	return api
}

func seedancePublicAssetReuseScope(channel *Channel, protocol dto.AssetUpstreamProtocol) string {
	if channel == nil || protocol == "" || protocol == dto.AssetUpstreamProtocolNone {
		return ""
	}
	settings := channel.GetOtherSettings()
	baseURL := channel.GetBaseURL()
	if protocol.TransportProfile() == dto.AssetUpstreamProfileOfficial {
		baseURL = AssetActionBaseURL(protocol, settings.AssetRegion)
	}
	if protocol == dto.AssetUpstreamProtocolCMCCAICCV2 {
		scope, err := CMCCAssetReuseScope(channel.Id)
		if err != nil {
			common.SysError("CMCC asset reuse scope is unavailable")
			return ""
		}
		return scope
	}
	fingerprint := AssetCredentialFingerprint(
		baseURL,
		"",
		string(protocol),
		settings.AssetProviderProject,
		settings.AssetRegion,
	)
	return "asset_scope_" + fingerprint
}

func publicAssetMedia(realPerson bool, generalGroupRequirement string) []dto.PublicAssetMedia {
	media := []dto.PublicAssetMedia{
		{Kind: AssetKindGeneral, MediaType: "image", AssetGroupRequirement: generalGroupRequirement},
		{Kind: AssetKindGeneral, MediaType: "video", AssetGroupRequirement: generalGroupRequirement},
		{Kind: AssetKindGeneral, MediaType: "audio", AssetGroupRequirement: generalGroupRequirement},
	}
	if realPerson {
		media = append(media, dto.PublicAssetMedia{
			Kind: AssetKindRealPerson, MediaType: "image", AssetGroupRequirement: dto.PublicAssetGroupRequired,
		})
	}
	return media
}

func publicAPIOperation(operation, method, path string, supported bool) dto.PublicAPIOperation {
	return dto.PublicAPIOperation{Operation: operation, Method: method, Path: path, Supported: supported}
}

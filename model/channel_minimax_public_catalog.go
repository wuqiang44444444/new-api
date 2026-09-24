package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/publicmodel"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// GetConfiguredMiniMaxPublicModels projects the configured MiniMax Link
// customer models from the same pinned declaration that drives runtime
// validation. Only api.video is published: the MiniMax Link contract
// declares no asset operations and no reuse scopes. When the active plugin
// declaration is unavailable, nothing is projected (fail closed) and the
// error is surfaced to the caller.
func GetConfiguredMiniMaxPublicModels() ([]SeedancePublicModel, error) {
	var channels []Channel
	if err := DB.Where("type = ?", constant.ChannelTypeMiniMaxLink).Order("id").Find(&channels).Error; err != nil {
		return nil, err
	}
	if len(channels) == 0 {
		return []SeedancePublicModel{}, nil
	}
	published, err := GetMinimaxPluginConfiguration()
	if err != nil {
		return nil, fmt.Errorf("MiniMax plugin declaration is unavailable for model projection: %w", err)
	}
	models := make([]SeedancePublicModel, 0)
	modelIndex := make(map[string]int)
	for i := range channels {
		settings := channels[i].GetOtherSettings()
		if settings.VideoUpstreamProtocol != dto.VideoUpstreamProtocol(constant.VideoUpstreamProtocolJdCloudTaskV1) {
			return nil, fmt.Errorf("MiniMax channel has an unregistered video protocol")
		}
		for _, modelName := range channels[i].GetModels() {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			providerModel, err := mappedCustomerModel(&channels[i], modelName)
			if err != nil {
				return nil, fmt.Errorf("resolve MiniMax customer model contract: %w", err)
			}
			video, err := minimaxPublicVideoAPI(published.Configuration, modelName, providerModel, settings.AllowServiceTier)
			if err != nil {
				return nil, fmt.Errorf("MiniMax model has no published parameter declaration: %w", err)
			}
			candidate := SeedancePublicModel{
				ModelName: modelName,
				Enabled:   channels[i].Status == common.ChannelStatusEnabled,
				Groups:    normalizedPublicModelGroups(channels[i].GetGroups()),
				API: dto.PublicModelAPI{
					Video: video,
				},
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

// minimaxPublicVideoAPI builds the public creation contract from the pinned
// declaration and registered host request shape. Provider names and channel identity
// cannot escape through this projection.
func minimaxPublicVideoAPI(configuration *jsplugin.SeedanceChannelConfiguration, customerModel, providerModel string, allowServiceTier bool) (*dto.PublicVideoAPI, error) {
	metadata, err := minimaxDeclaredMetadata(configuration, providerModel)
	if err != nil {
		return nil, err
	}
	api, ok := publicmodel.VideoAPIFromPlugin(customerModel, constantVideoProtocol(), *metadata, allowServiceTier)
	if !ok {
		return nil, fmt.Errorf("provider model %q cannot be projected", providerModel)
	}
	api.DocumentationPath = "/docs/api-reference/videos/minimax"
	// The shared projector has historical Seedance defaults. MiniMax's
	// optional parameters must describe the same declaration used by its adapter.
	for i := range api.Creation.Parameters {
		parameter := &api.Creation.Parameters[i]
		switch parameter.Name {
		case "duration":
			parameter.DefaultValue = nil
			if !parameter.Required && metadata.DefaultDuration > 0 {
				parameter.DefaultValue = metadata.DefaultDuration
			}
		case "resolution", "ratio":
			parameter.DefaultValue = nil
			if !parameter.Required && len(parameter.Enum) > 0 {
				parameter.DefaultValue = parameter.Enum[0]
			}
		case "content":
			parameter.ItemType = "object"
			parameter.MaxItems = common.GetPointer(1 + metadata.MaxImages + metadata.MaxVideos + metadata.MaxAudios)
		}
	}
	// Only publish media declared by the pinned artifact; old text-only
	// versions keep their original scope; frame roles require an explicit declaration.
	api.Creation.ContentTypes = []dto.PublicVideoContentType{{Type: "text", RequiredFields: []string{"type", "text"}, MinItems: 1, MaxItems: 1}}
	if metadata.MaxImages > 0 {
		roles := []string{"reference_image"}
		if metadata.AllowFrameImages {
			roles = append(roles, "first_frame", "last_frame")
		}
		api.Creation.ContentTypes = append(api.Creation.ContentTypes, dto.PublicVideoContentType{Type: "image_url", Roles: roles, RequiredFields: []string{"type", "role", "image_url.url"}, MaxItems: metadata.MaxImages})
	}
	if metadata.AllowVideos && metadata.MaxVideos > 0 {
		api.Creation.ContentTypes = append(api.Creation.ContentTypes, dto.PublicVideoContentType{Type: "video_url", Roles: []string{"reference_video"}, RequiredFields: []string{"type", "role", "video_url.url"}, MaxItems: metadata.MaxVideos})
	}
	if metadata.AllowAudios && metadata.MaxAudios > 0 {
		api.Creation.ContentTypes = append(api.Creation.ContentTypes, dto.PublicVideoContentType{Type: "audio_url", Roles: []string{"reference_audio"}, RequiredFields: []string{"type", "role", "audio_url.url"}, MaxItems: metadata.MaxAudios})
	}
	if metadata.MaxPromptLength > 0 {
		api.Creation.Parameters = append(api.Creation.Parameters, dto.PublicAPIParameter{Name: "content[].text", Type: "string", MinLength: common.GetPointer(1), MaxLength: common.GetPointer(metadata.MaxPromptLength)})
	}
	api.Creation.Parameters = append(api.Creation.Parameters, dto.PublicAPIParameter{Name: "watermark", Type: "boolean", FixedValue: false, DefaultValue: false})
	return api, nil
}

func constantVideoProtocol() dto.VideoUpstreamProtocol {
	return dto.VideoUpstreamProtocol(constant.VideoUpstreamProtocolJdCloudTaskV1)
}

// minimaxDeclaredMetadata resolves the provider model's declared metadata
// from the active minimax-link declaration.
func minimaxDeclaredMetadata(configuration *jsplugin.SeedanceChannelConfiguration, providerModel string) (*jsplugin.SeedanceVideoModelMetadata, error) {
	if configuration == nil {
		return nil, fmt.Errorf("MiniMax plugin declaration is unavailable")
	}
	for i := range configuration.Videos {
		if configuration.Videos[i].Protocol != constant.VideoUpstreamProtocolJdCloudTaskV1 {
			continue
		}
		if metadata, ok := configuration.Videos[i].ModelMetadata[providerModel]; ok {
			return &metadata, nil
		}
		break
	}
	return nil, fmt.Errorf("provider model %q has no declared metadata", providerModel)
}

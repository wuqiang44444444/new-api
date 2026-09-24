package jsplugin

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// SeedanceConfigurationAPIVersion extends the local Seedance contract only.
// Native task plugin v1 and historical Seedance v1 artifacts keep their ABI.
const SeedanceConfigurationAPIVersion = 2

// SeedanceUsageScanAPIVersion moves usage derivation to the host: artifacts at
// this version only locate the Provider usage subtree (usage_scan_root) and
// the host derives billable usage with its own normalization. Frozen artifacts
// at older versions keep their embedded-usage observation contract.
const SeedanceUsageScanAPIVersion = 3

// SeedanceChannelConfiguration describes Provider configuration, never Channel
// instance values or native model/route eligibility. The compiled declaration
// is consumed by management, validation and the pinned execution version.
type SeedanceChannelConfiguration struct {
	Videos []SeedanceVideoConfiguration `json:"videos"`
	Assets []SeedanceAssetConfiguration `json:"assets"`
}

type SeedanceVideoConfiguration struct {
	ModelMetadata        map[string]SeedanceVideoModelMetadata `json:"modelMetadata,omitempty"`
	DefaultModelMetadata *SeedanceVideoModelMetadata           `json:"defaultModelMetadata,omitempty"`
	Protocol             string                                `json:"protocol"`
	Label                string                                `json:"label"`
	ModelPolicy          string                                `json:"modelPolicy,omitempty"`
	Models               []string                              `json:"models"`
	AssetProtocols       []string                              `json:"assetProtocols"`
	DefaultAssetProtocol string                                `json:"defaultAssetProtocol"`
}

type SeedanceAssetMedia struct {
	Kind      string `json:"kind"`
	MediaType string `json:"mediaType"`
}

type SeedanceAssetConfiguration struct {
	DeleteNotFoundIsSuccess bool                       `json:"deleteNotFoundIsSuccess,omitempty"`
	Connectivity            bool                       `json:"connectivity,omitempty"`
	GroupSearch             bool                       `json:"groupSearch,omitempty"`
	Operations              []string                   `json:"operations,omitempty"`
	Media                   []SeedanceAssetMedia       `json:"media,omitempty"`
	Protocol                string                     `json:"protocol"`
	Label                   string                     `json:"label"`
	GroupPolicy             string                     `json:"groupPolicy"`
	Credential              string                     `json:"credential"`
	Project                 *SeedanceConfigurationText `json:"project,omitempty"`
	Region                  *SeedanceConfigurationText `json:"region,omitempty"`
	DefaultURLTTLSeconds    *int64                     `json:"defaultURLTTLSeconds,omitempty"`
}

// Text rules address the two existing non-secret Channel fields. They cannot
// introduce arbitrary fields, expressions, auth targets or credential values.
// Defaults are suggestions for explicit management edits, never runtime writes.
type SeedanceConfigurationText struct {
	Required bool   `json:"required"`
	Fixed    string `json:"fixed,omitempty"`
	Default  string `json:"default,omitempty"`
	Format   string `json:"format,omitempty"`
}

var seedanceConfigurationRegion = regexp.MustCompile(`^[a-z]{2}(?:-[a-z]+)+-[0-9]+$`)

// ValidateChannel validates only Provider configuration. Customer permissions,
// model mapping, URL safety and credential storage remain host responsibilities.
// In particular it never applies defaults or rewrites persisted Channel values.
func (configuration *SeedanceChannelConfiguration) ValidateChannel(videoProtocol, assetProtocol string, providerModels []string, project, region string, urlTTLSeconds int64) error {
	var video *SeedanceVideoConfiguration
	for i := range configuration.Videos {
		if configuration.Videos[i].Protocol == videoProtocol {
			video = &configuration.Videos[i]
			break
		}
	}
	if video == nil {
		return fmt.Errorf("video protocol is absent from the selected plugin version")
	}
	if !slices.Contains(video.AssetProtocols, assetProtocol) {
		return fmt.Errorf("asset protocol is not paired with the selected video protocol")
	}
	if len(providerModels) == 0 {
		return fmt.Errorf("at least one mapped Provider model is required")
	}
	for _, model := range providerModels {
		if strings.TrimSpace(model) == "" || (video.ModelPolicy != "configured" && !slices.Contains(video.Models, model)) {
			return fmt.Errorf("mapped Provider model is absent from the selected plugin version")
		}
	}
	for _, asset := range configuration.Assets {
		if asset.Protocol != assetProtocol {
			continue
		}
		if asset.DefaultURLTTLSeconds != nil && urlTTLSeconds <= 0 {
			return fmt.Errorf("remote asset configuration requires a positive URL TTL")
		}
		for _, field := range []struct {
			name  string
			value string
			rule  *SeedanceConfigurationText
		}{{"project", project, asset.Project}, {"region", region, asset.Region}} {
			if field.rule == nil {
				continue
			}
			value := strings.TrimSpace(field.value)
			if field.rule.Required && value == "" {
				return fmt.Errorf("asset %s is required", field.name)
			}
			if field.rule.Fixed != "" && value != field.rule.Fixed {
				return fmt.Errorf("asset %s does not match the declared fixed value", field.name)
			}
			if value != "" && field.rule.Format == "region_id" && !seedanceConfigurationRegion.MatchString(value) {
				return fmt.Errorf("asset %s has an invalid format", field.name)
			}
		}
		return nil
	}
	return fmt.Errorf("asset protocol is absent from the selected plugin version")
}

func decodeSeedanceChannelConfiguration(value any, apiVersion int, protocols, hostAssets []string) (*SeedanceChannelConfiguration, error) {
	meta := value.(map[string]any) // already validated by decodeSeedanceExtensionMeta
	raw, exists := meta["channelConfiguration"]
	if apiVersion == APIVersion1 {
		if exists {
			return nil, fmt.Errorf("Seedance channelConfiguration requires apiVersion 2")
		}
		return nil, nil
	}
	object, err := seedanceConfigurationObject(raw, "channelConfiguration", "videos", "assets")
	if err != nil {
		return nil, err
	}
	videos, ok := object["videos"].([]any)
	if !ok || len(videos) == 0 {
		return nil, fmt.Errorf("channelConfiguration videos must be a non-empty array")
	}
	assets, ok := object["assets"].([]any)
	if !ok || len(assets) == 0 {
		return nil, fmt.Errorf("channelConfiguration assets must be a non-empty array")
	}
	for _, rawVideo := range videos {
		if _, err = seedanceConfigurationObject(rawVideo, "video configuration", "protocol", "label", "models", "modelPolicy", "modelMetadata", "defaultModelMetadata", "assetProtocols", "defaultAssetProtocol"); err != nil {
			return nil, err
		}

		video := rawVideo.(map[string]any)
		specs := []any{}
		if value, exists := video["defaultModelMetadata"]; exists {
			specs = append(specs, value)
		}
		if value, exists := video["modelMetadata"]; exists {
			entries, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("modelMetadata must be an object")
			}
			for _, spec := range entries {
				specs = append(specs, spec)
			}
		}
		for _, spec := range specs {
			if _, err = seedanceConfigurationObject(spec, "ModelArk metadata", "allowFrameImages", "maxPromptLength", "omitDurationMaximum", "publishGenerateAudioDefault", "allowReturnLastFrame", "allowPriority", "deleteVideo", "defaultDuration", "intelligentDurationSeconds", "defaultGenerateAudio", "allowAudioOnly", "maxTotalMedia", "minDuration", "maxDuration", "intelligentDuration", "durationRequired", "resolutions", "suggestedResolutions", "resolutionRequired", "freeResolution", "ratios", "ratioRequired", "maxImages", "maxVideos", "minImages", "maxAudios", "allowVideos", "allowAudios", "allowGenerateAudio", "allowWatermark", "allowSeed", "allowCameraFixed", "outputFormats", "omitOutputFormat", "fullModelArk"); err != nil {
				return nil, err
			}
		}
	}
	for _, rawAsset := range assets {
		asset, err := seedanceConfigurationObject(rawAsset, "asset configuration", "protocol", "label", "groupPolicy", "credential", "project", "region", "defaultURLTTLSeconds", "operations", "media", "groupSearch", "connectivity", "deleteNotFoundIsSuccess")
		if err != nil {
			return nil, err
		}
		if rawMedia, exists := asset["media"]; exists {
			media, ok := rawMedia.([]any)
			if !ok {
				return nil, fmt.Errorf("asset media must be an array")
			}
			for _, item := range media {
				if _, err := seedanceConfigurationObject(item, "asset media", "kind", "mediaType"); err != nil {
					return nil, err
				}
			}
		}
		for _, field := range []string{"project", "region"} {
			if rule, present := asset[field]; present {
				if _, err = seedanceConfigurationObject(rule, field, "required", "fixed", "default", "format"); err != nil {
					return nil, err
				}
			}
		}
	}
	encoded, err := common.Marshal(object)
	if err != nil {
		return nil, err
	}
	var configuration SeedanceChannelConfiguration
	if err = common.Unmarshal(encoded, &configuration); err != nil {
		return nil, fmt.Errorf("invalid channelConfiguration field type")
	}
	if err = configuration.validate(protocols, hostAssets); err != nil {
		return nil, &seedanceConfigurationValidationError{reason: err.Error()}
	}
	return &configuration, nil
}

func seedanceConfigurationObject(raw any, name string, fields ...string) (map[string]any, error) {
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", name)
	}
	for key, value := range object {
		if !slices.Contains(fields, key) {
			return nil, fmt.Errorf("%s has unknown field %q", name, key)
		}
		if value == nil {
			return nil, fmt.Errorf("%s field %q must not be null", name, key)
		}
	}
	return object, nil
}

func (configuration *SeedanceChannelConfiguration) validate(protocols, hostAssets []string) error {
	assets := make(map[string]bool, len(configuration.Assets))
	for _, asset := range configuration.Assets {
		if assets[asset.Protocol] || (asset.Protocol != "none" && !slices.Contains(hostAssets, asset.Protocol)) {
			return fmt.Errorf("duplicate or unsupported asset configuration")
		}
		assets[asset.Protocol] = true
		operations := map[string]bool{}
		for _, operation := range asset.Operations {
			if operations[operation] || !slices.Contains([]string{"create_asset", "get_asset", "update_asset", "delete_asset", "create_asset_group", "get_asset_group", "get_asset_group_verification"}, operation) {
				return fmt.Errorf("unsupported or duplicate asset operation")
			}
			operations[operation] = true
		}
		media := map[string]bool{}
		for _, item := range asset.Media {
			key := item.Kind + "/" + item.MediaType
			if media[key] || !slices.Contains([]string{"general", "real_person"}, item.Kind) || !slices.Contains([]string{"image", "video", "audio"}, item.MediaType) {
				return fmt.Errorf("invalid asset media declaration")
			}
			media[key] = true
		}

		if strings.TrimSpace(asset.Label) == "" {
			return fmt.Errorf("asset configuration label is required")
		}
		if !slices.Contains([]string{"none", "default_fallback", "hosted"}, asset.GroupPolicy) {
			return fmt.Errorf("unsupported asset group policy")
		}
		if (asset.GroupPolicy == "hosted") != (asset.Protocol == SeedanceHostedAssetProtocol) {
			return fmt.Errorf("hosted group policy must match the platform-hosted asset protocol")
		}
		if !slices.Contains([]string{"none", "channel", "asset_key_pair"}, asset.Credential) {
			return fmt.Errorf("unsupported asset credential slot")
		}
		if asset.Protocol == "none" || asset.GroupPolicy == "hosted" {
			if asset.Credential != "none" || asset.Project != nil || asset.Region != nil || asset.DefaultURLTTLSeconds != nil {
				return fmt.Errorf("non-remote asset configuration cannot require Provider fields")
			}
			if asset.Protocol == "none" && asset.GroupPolicy != "none" {
				return fmt.Errorf("none asset protocol requires none group policy")
			}
		} else if asset.Credential == "none" || asset.DefaultURLTTLSeconds == nil || *asset.DefaultURLTTLSeconds <= 0 {
			return fmt.Errorf("remote asset configuration requires credentials and positive URL TTL")
		}
		for _, rule := range []*SeedanceConfigurationText{asset.Project, asset.Region} {
			if rule == nil {
				continue
			}
			if rule.Format != "" && rule.Format != "region_id" {
				return fmt.Errorf("unsupported configuration text format")
			}
			if rule.Fixed != "" && rule.Default != "" && rule.Fixed != rule.Default {
				return fmt.Errorf("configuration default conflicts with fixed value")
			}
			for _, value := range []string{rule.Fixed, rule.Default} {
				if value != "" && (strings.TrimSpace(value) != value || (rule.Format == "region_id" && !seedanceConfigurationRegion.MatchString(value))) {
					return fmt.Errorf("invalid configuration text default or fixed value")
				}
			}
		}
	}
	seen := make(map[string]bool, len(configuration.Videos))
	usedAssets := make(map[string]bool, len(assets))
	for _, video := range configuration.Videos {
		if seen[video.Protocol] || !slices.Contains(protocols, video.Protocol) {
			return fmt.Errorf("duplicate or unimplemented video configuration")
		}
		seen[video.Protocol] = true
		if video.ModelPolicy != "" && video.ModelPolicy != "listed" && video.ModelPolicy != "configured" {
			return fmt.Errorf("unsupported Provider model policy")
		}
		if strings.TrimSpace(video.Label) == "" || (len(video.Models) == 0 && video.ModelPolicy != "configured") {
			return fmt.Errorf("video configuration requires a label and Provider models")
		}
		models := make(map[string]bool, len(video.Models))
		for _, model := range video.Models {
			if model == "" || strings.TrimSpace(model) != model || models[model] {
				return fmt.Errorf("video configuration contains an empty, untrimmed or duplicate Provider model")
			}
			models[model] = true
		}
		// The public projection has no Go-side default table to fall back on,
		// so a listed model without metadata could never be published instead
		// of silently disappearing from the model list.
		if video.ModelPolicy != "configured" {
			for _, model := range video.Models {
				if _, ok := video.ModelMetadata[model]; !ok {
					return fmt.Errorf("listed Provider model requires declared model metadata")
				}
			}
		} else if video.DefaultModelMetadata == nil {
			return fmt.Errorf("configured Provider models require default model metadata")
		}
		for model, spec := range video.ModelMetadata {
			if !models[model] {
				return fmt.Errorf("model metadata must reference a declared Provider model")
			}
			if err := spec.validate(); err != nil {
				return err
			}
		}
		if video.DefaultModelMetadata != nil && video.ModelPolicy != "configured" {
			return fmt.Errorf("default model metadata requires explicitly configured Provider models")
		}
		if err := video.DefaultModelMetadata.validate(); err != nil {
			return err
		}
		pairs := make(map[string]bool, len(video.AssetProtocols))
		for _, asset := range video.AssetProtocols {
			if !assets[asset] || pairs[asset] {
				return fmt.Errorf("video configuration contains an unknown or duplicate asset pairing")
			}
			pairs[asset] = true
			usedAssets[asset] = true
		}
		if !pairs[video.DefaultAssetProtocol] {
			return fmt.Errorf("default asset protocol must be a declared pairing")
		}
	}
	if len(seen) != len(protocols) {
		return fmt.Errorf("every implemented video protocol requires a configuration")
	}
	if len(usedAssets) != len(assets) {
		return fmt.Errorf("asset configuration must be paired with an implemented video protocol")
	}
	return nil
}

// Asset returns the declaration bound to the same compiled video version.
func (configuration *SeedanceChannelConfiguration) Asset(protocol string) *SeedanceAssetConfiguration {
	if configuration == nil {
		return nil
	}
	for i := range configuration.Assets {
		if configuration.Assets[i].Protocol == protocol {
			return &configuration.Assets[i]
		}
	}
	return nil
}

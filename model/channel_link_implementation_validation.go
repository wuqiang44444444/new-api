package model

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/provider_exposure_setting"
)

func ValidateLinkImplementationRegistration(channel *Channel, settings *dto.ChannelOtherSettings) error {
	if channel == nil || settings == nil {
		return fmt.Errorf("Link implementation channel settings are required")
	}
	linkModels := channelRegisteredLinkModels(channel)
	ref := settings.LinkImplementation
	if len(linkModels) == 0 {
		if !ref.Empty() {
			return fmt.Errorf("Link implementation is configured but the channel has no registered Link SKU")
		}
		return nil
	}
	if !ref.Valid() {
		return fmt.Errorf("registered Link SKUs require an explicit Link implementation ID and version")
	}
	implementation, ok := ResolveLinkImplementation(ref)
	if !ok {
		return fmt.Errorf("Link implementation %q version %q is not registered", strings.TrimSpace(ref.ID), strings.TrimSpace(ref.Version))
	}
	if implementation.ChannelType != channel.Type {
		return fmt.Errorf("Link implementation %s/%s requires channel type %d", implementation.ID, implementation.Version, implementation.ChannelType)
	}
	if channel.ChannelInfo.IsMultiKey {
		return fmt.Errorf("Link implementation %s/%s requires a single channel credential", implementation.ID, implementation.Version)
	}
	if channel.Status == common.ChannelStatusEnabled && !provider_exposure_setting.Current().ActiveForImplementation(implementation.ID, implementation.Version) {
		return fmt.Errorf("Link implementation %s/%s has no enabled exposure policy", implementation.ID, implementation.Version)
	}
	for _, publicSKU := range linkModels {
		if !slices.Contains(implementation.PublicSKUs, publicSKU) {
			return fmt.Errorf("Link implementation %s/%s does not implement public SKU %q", implementation.ID, implementation.Version, publicSKU)
		}
	}
	if err := validateLinkImplementationSettings(channel, settings, implementation, linkModels); err != nil {
		return err
	}
	return nil
}

func ValidateChannelLinkImplementationForSKU(channel *Channel, publicSKU string) error {
	if channel == nil {
		return fmt.Errorf("Link implementation channel is required")
	}
	publicSKU = strings.TrimSpace(publicSKU)
	if !IsRegisteredLinkSKU(publicSKU) {
		return nil
	}
	settings := channel.GetOtherSettings()
	implementation, ok := ResolveLinkImplementation(settings.LinkImplementation)
	if !ok || !slices.Contains(implementation.PublicSKUs, publicSKU) {
		return fmt.Errorf("channel has no registered Link implementation for public SKU %q", publicSKU)
	}
	if !provider_exposure_setting.Current().ActiveForImplementation(implementation.ID, implementation.Version) {
		return fmt.Errorf("Link implementation %s/%s has no enabled exposure policy", implementation.ID, implementation.Version)
	}
	if err := validateLinkImplementationSettings(channel, &settings, implementation, []string{publicSKU}); err != nil {
		return err
	}
	return nil
}

func ResolveChannelLinkImplementation(channel *Channel) (LinkImplementation, bool) {
	if channel == nil {
		return LinkImplementation{}, false
	}
	return ResolveLinkImplementation(channel.GetOtherSettings().LinkImplementation)
}

func channelRegisteredLinkModels(channel *Channel) []string {
	if channel == nil {
		return nil
	}
	result := make([]string, 0)
	for _, rawModel := range strings.Split(channel.Models, ",") {
		publicSKU := strings.TrimSpace(rawModel)
		if IsRegisteredLinkSKU(publicSKU) {
			result = append(result, publicSKU)
		}
	}
	return normalizedStringSet(result)
}

func validateLinkImplementationSettings(channel *Channel, settings *dto.ChannelOtherSettings, implementation LinkImplementation, publicSKUs []string) error {
	videoProfile := normalizedVideoProfile(string(settings.VideoUpstreamProfile))
	if implementation.RequiredVideoProfile != "" && videoProfile != normalizedVideoProfile(implementation.RequiredVideoProfile) {
		return fmt.Errorf("Link implementation %s/%s requires video profile %q", implementation.ID, implementation.Version, implementation.RequiredVideoProfile)
	}
	assetProfile := strings.TrimSpace(string(settings.AssetUpstreamProfile))
	if assetProfile == "" {
		assetProfile = string(dto.AssetUpstreamProfileNone)
	}
	if implementation.RequiredAssetProfile != "" && assetProfile != implementation.RequiredAssetProfile {
		return fmt.Errorf("Link implementation %s/%s requires asset profile %q", implementation.ID, implementation.Version, implementation.RequiredAssetProfile)
	}
	if implementation.RequiredCreatePath != "" && strings.TrimSpace(settings.VideoUpstreamCreatePath) != implementation.RequiredCreatePath {
		return fmt.Errorf("Link implementation %s/%s requires create path %q", implementation.ID, implementation.Version, implementation.RequiredCreatePath)
	}
	for _, requirement := range implementation.RequiredSKUCreatePaths {
		if slices.Contains(publicSKUs, requirement.PublicSKU) && strings.TrimSpace(settings.VideoUpstreamCreatePath) != requirement.CreatePath {
			return fmt.Errorf("Link implementation %s/%s requires create path %q for %q", implementation.ID, implementation.Version, requirement.CreatePath, requirement.PublicSKU)
		}
	}
	if implementation.RequiredQueryPath != "" && strings.TrimSpace(settings.VideoUpstreamQueryPathTemplate) != implementation.RequiredQueryPath {
		return fmt.Errorf("Link implementation %s/%s requires query path %q", implementation.ID, implementation.Version, implementation.RequiredQueryPath)
	}
	if implementation.RequiredAdapterVersion != "" && relaycommon.CurrentVideoSouthboundAdapterVersion(channel.Type, settings.VideoUpstreamProfile) != implementation.RequiredAdapterVersion {
		return fmt.Errorf("Link implementation %s/%s requires adapter version %q", implementation.ID, implementation.Version, implementation.RequiredAdapterVersion)
	}
	if len(implementation.RequiredRoutes) > 0 {
		if settings.AdvancedCustom == nil {
			return fmt.Errorf("Link implementation %s/%s requires Advanced Custom routes", implementation.ID, implementation.Version)
		}
		for _, publicSKU := range publicSKUs {
			requirement, required := linkRouteRequirementForSKU(implementation.RequiredRoutes, publicSKU)
			if !required {
				return fmt.Errorf("Link implementation %s/%s has no route declaration for %q", implementation.ID, implementation.Version, publicSKU)
			}
			if !advancedCustomRouteMatchesRequirement(settings.AdvancedCustom.Routes, requirement) {
				return fmt.Errorf("Link implementation %s/%s route for %q is incomplete or mismatched", implementation.ID, implementation.Version, publicSKU)
			}
		}
	}
	if len(implementation.RequiredModelMappings) > 0 {
		mapping := make(map[string]string)
		if channel.ModelMapping != nil && strings.TrimSpace(*channel.ModelMapping) != "" {
			if err := common.Unmarshal([]byte(*channel.ModelMapping), &mapping); err != nil {
				return fmt.Errorf("Link implementation model mapping is invalid: %w", err)
			}
		}
		for _, requirement := range implementation.RequiredModelMappings {
			if !slices.Contains(publicSKUs, requirement.PublicSKU) {
				continue
			}
			if strings.TrimSpace(mapping[requirement.PublicSKU]) != requirement.UpstreamModel {
				return fmt.Errorf("Link implementation %s/%s requires model mapping %q -> %q", implementation.ID, implementation.Version, requirement.PublicSKU, requirement.UpstreamModel)
			}
		}
	}
	return nil
}

func linkRouteRequirementForSKU(requirements []LinkRouteRequirement, publicSKU string) (LinkRouteRequirement, bool) {
	for _, requirement := range requirements {
		if requirement.PublicSKU == publicSKU {
			return requirement, true
		}
	}
	return LinkRouteRequirement{}, false
}

func advancedCustomRouteMatchesRequirement(routes []dto.AdvancedCustomRoute, requirement LinkRouteRequirement) bool {
	for _, route := range routes {
		if strings.TrimSpace(route.IncomingPath) != requirement.IncomingPath ||
			strings.TrimSpace(route.UpstreamPath) != requirement.UpstreamPath ||
			strings.TrimSpace(route.Converter) != requirement.Converter ||
			!slices.Contains(normalizedStringSet(route.Models), requirement.PublicSKU) {
			continue
		}
		authType := dto.AdvancedCustomAuthTypeNone
		if route.Auth != nil && strings.TrimSpace(route.Auth.Type) != "" {
			authType = strings.TrimSpace(route.Auth.Type)
		}
		if authType == requirement.AuthType {
			return true
		}
	}
	return false
}

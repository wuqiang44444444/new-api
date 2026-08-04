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
	ref := settings.LinkImplementation
	if ref.Empty() {
		for _, rawModel := range strings.Split(channel.Models, ",") {
			if IsRegisteredLinkSKU(strings.TrimSpace(rawModel)) {
				return fmt.Errorf("registered Link SKUs require an explicit Link access plan")
			}
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
	executions, err := DeriveChannelLinkExecutions(channel, settings)
	if err != nil {
		return err
	}
	if err := validateLinkImplementationSettings(channel, settings, implementation, executions); err != nil {
		return err
	}
	for _, execution := range executions {
		if err := ValidateLinkImplementationAssetCoverage(implementation, execution.LinkSKU); err != nil {
			return err
		}
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
	executions, err := DeriveChannelLinkExecutions(channel, &settings)
	if err != nil {
		return err
	}
	found := false
	for _, execution := range executions {
		if execution.LinkSKU == publicSKU {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("channel has no execution binding for Link SKU %q", publicSKU)
	}
	if err := validateLinkImplementationSettings(channel, &settings, implementation, executions); err != nil {
		return err
	}
	return ValidateLinkImplementationAssetCoverage(implementation, publicSKU)
}

func ResolveChannelLinkExecution(channel *Channel, customerModel string, routeFamily LinkRouteFamily) (ChannelLinkExecution, error) {
	if channel == nil {
		return ChannelLinkExecution{}, fmt.Errorf("Link implementation channel is required")
	}
	settings := channel.GetOtherSettings()
	executions, err := DeriveChannelLinkExecutions(channel, &settings)
	if err != nil {
		return ChannelLinkExecution{}, err
	}
	matches := make([]ChannelLinkExecution, 0, 1)
	for _, execution := range executions {
		if execution.CustomerModel == strings.TrimSpace(customerModel) && execution.Binding.RouteFamily == routeFamily {
			matches = append(matches, execution)
		}
	}
	if len(matches) != 1 {
		return ChannelLinkExecution{}, fmt.Errorf("customer model %q resolved %d Link executions for route family %q", customerModel, len(matches), routeFamily)
	}
	return matches[0], nil
}

func ValidateChannelLinkExecution(channel *Channel, customerModel string, routeFamily LinkRouteFamily, expectedSKU string) error {
	execution, err := ResolveChannelLinkExecution(channel, customerModel, routeFamily)
	if err != nil {
		return err
	}
	if execution.LinkSKU != strings.TrimSpace(expectedSKU) {
		return fmt.Errorf("customer model %q resolves to Link SKU %q instead of published SKU %q", customerModel, execution.LinkSKU, expectedSKU)
	}
	return ValidateChannelLinkImplementationForSKU(channel, execution.LinkSKU)
}

func ResolveChannelLinkImplementation(channel *Channel) (LinkImplementation, bool) {
	if channel == nil {
		return LinkImplementation{}, false
	}
	return ResolveLinkImplementation(channel.GetOtherSettings().LinkImplementation)
}

func validateLinkImplementationSettings(channel *Channel, settings *dto.ChannelOtherSettings, implementation LinkImplementation, executions []ChannelLinkExecution) error {
	publicSKUs := make([]string, 0, len(executions))
	for _, execution := range executions {
		publicSKUs = append(publicSKUs, execution.LinkSKU)
	}
	publicSKUs = normalizedStringSet(publicSKUs)
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
		for _, execution := range executions {
			requirement, required := linkRouteRequirementForSKU(implementation.RequiredRoutes, execution.LinkSKU)
			if !required {
				return fmt.Errorf("Link implementation %s/%s has no route declaration for %q", implementation.ID, implementation.Version, execution.LinkSKU)
			}
			if !advancedCustomRouteMatchesRequirement(settings.AdvancedCustom.Routes, execution.CustomerModel, requirement) {
				return fmt.Errorf("Link implementation %s/%s route for customer model %q is incomplete or mismatched", implementation.ID, implementation.Version, execution.CustomerModel)
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

func advancedCustomRouteMatchesRequirement(routes []dto.AdvancedCustomRoute, customerModel string, requirement LinkRouteRequirement) bool {
	for _, route := range routes {
		if strings.TrimSpace(route.IncomingPath) != requirement.IncomingPath ||
			strings.TrimSpace(route.UpstreamPath) != requirement.UpstreamPath ||
			strings.TrimSpace(route.Converter) != requirement.Converter ||
			!slices.Contains(normalizedStringSet(route.Models), customerModel) {
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

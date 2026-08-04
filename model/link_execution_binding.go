package model

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

type LinkRouteFamily string

const (
	LinkContractNamespaceDefault                   = "link"
	LinkExecutionActionCreate                      = "create"
	LinkRouteFamilyImageGeneration LinkRouteFamily = "image_generation"
	LinkRouteFamilyModelArkVideo   LinkRouteFamily = "modelark_video"
	LinkRouteFamilyKlingVideo      LinkRouteFamily = "kling_video"
	LinkRouteFamilyJimengVideo     LinkRouteFamily = "jimeng_video"
)

type LinkExecutionBinding struct {
	RouteFamily   LinkRouteFamily `json:"route_family"`
	Action        string          `json:"action"`
	Profile       string          `json:"profile"`
	ProviderModel string          `json:"provider_model"`
	LinkSKU       string          `json:"link_sku"`
}

type ChannelLinkExecution struct {
	CustomerModel string               `json:"customer_model"`
	ProviderModel string               `json:"provider_model"`
	LinkSKU       string               `json:"link_sku"`
	Binding       LinkExecutionBinding `json:"binding"`
}

func normalizeLinkExecutionBinding(binding LinkExecutionBinding) LinkExecutionBinding {
	binding.RouteFamily = LinkRouteFamily(strings.TrimSpace(string(binding.RouteFamily)))
	binding.Action = strings.TrimSpace(binding.Action)
	binding.Profile = strings.TrimSpace(binding.Profile)
	binding.ProviderModel = strings.TrimSpace(binding.ProviderModel)
	binding.LinkSKU = strings.TrimSpace(binding.LinkSKU)
	return binding
}

func linkExecutionBindingKey(binding LinkExecutionBinding) string {
	return strings.Join([]string{string(binding.RouteFamily), binding.Action, binding.Profile, binding.ProviderModel, binding.LinkSKU}, "\x00")
}

func validateLinkExecutionBindings(implementation LinkImplementation) error {
	seen := make(map[string]struct{}, len(implementation.ExecutionBindings))
	for _, raw := range implementation.ExecutionBindings {
		binding := normalizeLinkExecutionBinding(raw)
		if binding.RouteFamily == "" || binding.Action == "" || binding.Profile == "" || binding.ProviderModel == "" || binding.LinkSKU == "" {
			return errors.New("binding identity is incomplete")
		}
		if !slices.Contains(implementation.PublicSKUs, binding.LinkSKU) {
			return fmt.Errorf("binding SKU %q is not implemented", binding.LinkSKU)
		}
		key := strings.Join([]string{string(binding.RouteFamily), binding.Action, binding.Profile, binding.ProviderModel}, "\x00")
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate or ambiguous binding for %s", key)
		}
		seen[key] = struct{}{}
	}
	for _, publicSKU := range implementation.PublicSKUs {
		found := false
		for _, binding := range implementation.ExecutionBindings {
			if binding.LinkSKU == publicSKU {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("SKU %q has no execution binding", publicSKU)
		}
	}
	return nil
}

func resolveImplementationExecutionBinding(implementation LinkImplementation, routeFamily LinkRouteFamily, action, profile, providerModel string) (LinkExecutionBinding, error) {
	action = strings.TrimSpace(action)
	profile = strings.TrimSpace(profile)
	providerModel = strings.TrimSpace(providerModel)
	matches := make([]LinkExecutionBinding, 0, 1)
	for _, binding := range implementation.ExecutionBindings {
		if binding.RouteFamily == routeFamily && binding.Action == action && binding.Profile == profile && binding.ProviderModel == providerModel {
			matches = append(matches, binding)
		}
	}
	if len(matches) != 1 {
		return LinkExecutionBinding{}, fmt.Errorf("execution binding resolved %d matches for route family %q, action %q, profile %q, Provider model %q", len(matches), routeFamily, action, profile, providerModel)
	}
	return matches[0], nil
}

func DeriveChannelLinkExecutions(channel *Channel, settings *dto.ChannelOtherSettings) ([]ChannelLinkExecution, error) {
	if channel == nil || settings == nil || settings.LinkImplementation.Empty() {
		return nil, nil
	}
	implementation, ok := ResolveLinkImplementation(settings.LinkImplementation)
	if !ok {
		return nil, fmt.Errorf("Link implementation %q version %q is not registered", settings.LinkImplementation.ID, settings.LinkImplementation.Version)
	}
	mapping, err := channelModelMapping(channel)
	if err != nil {
		return nil, err
	}
	executions := make([]ChannelLinkExecution, 0)
	for _, rawModel := range strings.Split(channel.Models, ",") {
		customerModel := strings.TrimSpace(rawModel)
		if customerModel == "" {
			continue
		}
		providerModel, err := resolveMappedChannelModel(customerModel, mapping)
		if err != nil {
			return nil, err
		}
		routeFamily, profile, err := channelExecutionShape(channel, settings, implementation, customerModel)
		if err != nil {
			return nil, err
		}
		binding, err := resolveImplementationExecutionBinding(implementation, routeFamily, LinkExecutionActionCreate, profile, providerModel)
		if err != nil {
			return nil, fmt.Errorf("Link customer model %q: %w", customerModel, err)
		}
		executions = append(executions, ChannelLinkExecution{CustomerModel: customerModel, ProviderModel: providerModel, LinkSKU: binding.LinkSKU, Binding: binding})
	}
	if len(executions) == 0 {
		return nil, errors.New("Link access plan requires at least one customer model")
	}
	return executions, nil
}

func channelModelMapping(channel *Channel) (map[string]string, error) {
	mapping := make(map[string]string)
	if channel.ModelMapping == nil || strings.TrimSpace(*channel.ModelMapping) == "" {
		return mapping, nil
	}
	if err := common.Unmarshal([]byte(*channel.ModelMapping), &mapping); err != nil {
		return nil, fmt.Errorf("Link implementation model mapping is invalid: %w", err)
	}
	return mapping, nil
}

func resolveMappedChannelModel(customerModel string, mapping map[string]string) (string, error) {
	current := strings.TrimSpace(customerModel)
	visited := map[string]struct{}{current: {}}
	for {
		next := strings.TrimSpace(mapping[current])
		if next == "" || next == current {
			return current, nil
		}
		if _, exists := visited[next]; exists {
			return "", fmt.Errorf("model mapping for %q contains a cycle", customerModel)
		}
		visited[next] = struct{}{}
		current = next
	}
}

func channelExecutionShape(channel *Channel, settings *dto.ChannelOtherSettings, implementation LinkImplementation, customerModel string) (LinkRouteFamily, string, error) {
	switch implementation.ChannelType {
	case constant.ChannelTypeAdvancedCustom:
		if settings.AdvancedCustom == nil {
			return "", "", errors.New("Advanced Custom Link access plan requires routes")
		}
		profiles := make([]string, 0, 1)
		for _, route := range settings.AdvancedCustom.Routes {
			if strings.TrimSpace(route.IncomingPath) == "/v1/images/generations" && slices.Contains(normalizedStringSet(route.Models), customerModel) {
				profile := strings.TrimSpace(route.Converter)
				if profile == "" {
					profile = "none"
				}
				profiles = append(profiles, profile)
			}
		}
		if len(profiles) != 1 {
			return "", "", fmt.Errorf("Link customer model %q must match exactly one image generation route", customerModel)
		}
		return LinkRouteFamilyImageGeneration, profiles[0], nil
	case constant.ChannelTypeKling:
		return LinkRouteFamilyKlingVideo, normalizedVideoProfile(string(settings.VideoUpstreamProfile)), nil
	case constant.ChannelTypeJimeng:
		return LinkRouteFamilyJimengVideo, normalizedVideoProfile(string(settings.VideoUpstreamProfile)), nil
	default:
		return LinkRouteFamilyModelArkVideo, normalizedVideoProfile(string(settings.VideoUpstreamProfile)), nil
	}
}

func LinkRouteFamilySupportsChannel(routeFamily LinkRouteFamily, channel *Channel, customerModel string) bool {
	if channel == nil {
		return false
	}
	switch routeFamily {
	case LinkRouteFamilyModelArkVideo:
		return channel.Type == constant.ChannelTypeDoubaoVideo
	case LinkRouteFamilyKlingVideo:
		return channel.Type == constant.ChannelTypeKling
	case LinkRouteFamilyJimengVideo:
		return channel.Type == constant.ChannelTypeJimeng
	case LinkRouteFamilyImageGeneration:
		if channel.Type != constant.ChannelTypeAdvancedCustom {
			return true
		}
		config := channel.GetOtherSettings().AdvancedCustom
		return config != nil && config.SupportsPathForModel("/v1/images/generations", customerModel)
	default:
		return false
	}
}

package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

func mappedCustomerModel(channel *Channel, customerModel string) (string, error) {
	if channel.Type == constant.ChannelTypeMiniMaxLink {
		return ResolveMiniMaxChannelModelMapping(customerModel, channel.GetModelMapping())
	}
	providerModel, _, err := ResolveChannelModelMapping(customerModel, channel.GetModelMapping())
	return providerModel, err
}

// ResolveMiniMaxChannelModelMapping looks up the customer ID once and
// preserves the configured Provider ID verbatim.
func ResolveMiniMaxChannelModelMapping(customerModel, rawMapping string) (string, error) {
	mapping, err := parseChannelModelMapping(rawMapping)
	if err != nil {
		return "", err
	}
	if providerModel, ok := mapping[customerModel]; ok {
		if strings.TrimSpace(providerModel) == "" {
			return "", fmt.Errorf("MiniMax Link model mapping for %q requires an upstream model ID", customerModel)
		}
		return providerModel, nil
	}
	return customerModel, nil
}

// ResolveChannelModelMapping resolves the saved mapping without interpreting
// modifiers in customer or Provider model IDs.
func ResolveChannelModelMapping(customerModel, rawMapping string) (string, bool, error) {
	mapping, err := parseChannelModelMapping(rawMapping)
	if err != nil {
		return "", false, err
	}
	return ResolveModelMapping(customerModel, mapping)
}

func parseChannelModelMapping(rawMapping string) (map[string]string, error) {
	mapping := make(map[string]string)
	if raw := strings.TrimSpace(rawMapping); raw != "" && raw != "{}" {
		if err := common.UnmarshalJsonStr(raw, &mapping); err != nil {
			return nil, err
		}
	}
	return mapping, nil
}

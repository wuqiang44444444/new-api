package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

// SeedancePricingChannels selects the pricing contract for each exact customer
// model from an already loaded channel set. Enabled channels take precedence;
// only when none is enabled do all disabled channels constrain the price.
// This request-scoped projection is shared by saving, display and offline
// migration. It neither selects runtime routes nor checks channel uniqueness.
func SeedancePricingChannels(channels []Channel) map[string][]Channel {
	selected := make(map[string][]Channel)
	enabled := make(map[string]bool)
	for _, channel := range channels {
		if channel.Type != constant.ChannelTypeSeedanceLink && channel.Type != constant.ChannelTypeMiniMaxLink {
			continue
		}
		active := channel.Status == common.ChannelStatusEnabled
		for _, name := range channel.GetModels() {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if enabled[name] && !active {
				continue
			}
			if active && !enabled[name] {
				selected[name] = nil
			}
			selected[name] = append(selected[name], channel)
			enabled[name] = active
		}
	}
	return selected
}

package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

// BatchPricingChannels contains only public routing membership for read-only
// pricing. It never grants native Ability membership.
func BatchPricingChannels() ([]Channel, error) {
	var channels []Channel
	err := DB.Select([]string{"models", "group"}).Where("type = ? AND status = ?", constant.ChannelTypeAzureBatch, common.ChannelStatusEnabled).Find(&channels).Error
	return channels, err
}

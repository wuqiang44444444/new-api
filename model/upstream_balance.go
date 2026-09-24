package model

import "context"

type UpstreamBalanceURLGroup struct {
	URLKey string
	Name   string
}

// Read the same identity and display-name source as upstream reconciliation.
// No billing summary or billing permissions are needed for these channel labels.
func GetUpstreamBalanceURLGroups(channels []*Channel) (map[int]UpstreamBalanceURLGroup, error) {
	names, err := getProviderURLGroupNames()
	if err != nil {
		return nil, err
	}
	groups := make(map[int]UpstreamBalanceURLGroup, len(channels))
	for _, channel := range channels {
		key, name := billingURLGroupIdentity(*channel, true, channel.Id)
		if alias := names[key]; alias != "" {
			name = alias
		}
		groups[channel.Id] = UpstreamBalanceURLGroup{URLKey: key, Name: name}
	}
	return groups, nil
}

// GetUpstreamBalanceChannels includes disabled channels and keys. Balance queries
// are administrative observations, independent of relay availability.
func GetUpstreamBalanceChannels(ctx context.Context) ([]*Channel, error) {
	channels := make([]*Channel, 0)
	err := DB.WithContext(ctx).Select("id", "type", "key", "name", "status", "base_url", "setting", "channel_info").Order("id ASC").Find(&channels).Error
	return channels, err
}

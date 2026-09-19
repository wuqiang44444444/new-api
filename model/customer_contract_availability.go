package model

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
)

type contractRouteSource struct {
	channelID int
	group     string
	model     string
}

// These facts exist only for the current read. Current channel eligibility is
// never cached with the frozen contract definition or granted by that snapshot.
type contractRouteFacts struct {
	channels  map[int]Channel
	abilities map[contractRouteSource]bool
}

func loadContractRouteFacts(tx *gorm.DB, rules []ContractEntityRule) (*contractRouteFacts, error) {
	facts := &contractRouteFacts{channels: make(map[int]Channel), abilities: make(map[contractRouteSource]bool)}
	ids := make([]int, 0, len(rules))
	for _, rule := range rules {
		if rule.ChannelId > 0 && ratio_setting.ContainsGroupRatio(rule.RouteGroup) {
			ids = append(ids, rule.ChannelId)
		}
	}
	if len(ids) == 0 {
		return facts, nil
	}
	var channels []Channel
	if err := tx.Select("id", "type", "status", "models", "group").Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		facts.channels[channel.Id] = channel
	}
	query := tx.Session(&gorm.Session{NewDB: true}).Where("1 = 0")
	sources := make(map[contractRouteSource]bool)
	for _, rule := range rules {
		channel, exists := facts.channels[rule.ChannelId]
		if !exists || channel.Status != common.ChannelStatusEnabled || channelSkipsGenericAbilities(channel.Type) || !ratio_setting.ContainsGroupRatio(rule.RouteGroup) {
			continue
		}
		source := contractRouteSource{rule.ChannelId, rule.RouteGroup, rule.PublicModel}
		if !sources[source] {
			query = query.Or(map[string]any{"channel_id": rule.ChannelId, "group": rule.RouteGroup, "model": rule.PublicModel})
			sources[source] = true
		}
	}
	if len(sources) == 0 {
		return facts, nil
	}
	var abilities []Ability
	if err := tx.Select("channel_id", "group", "model").Where("enabled = ?", true).Where(query).Find(&abilities).Error; err != nil {
		return nil, err
	}
	for _, ability := range abilities {
		facts.abilities[contractRouteSource{ability.ChannelId, ability.Group, ability.Model}] = true
	}
	return facts, nil
}

// validate is the common source policy for new management rules and derived
// availability. Ordinary sources require Ability; typed sources use Channel.
func (facts *contractRouteFacts) validate(rule ContractEntityRule) error {
	if !ratio_setting.ContainsGroupRatio(rule.RouteGroup) {
		return fmt.Errorf("%w: route group %q has no native ratio", ErrCustomerContractInvalidRule, rule.RouteGroup)
	}
	if rule.ChannelId <= 0 {
		return fmt.Errorf("%w: channel is required", ErrCustomerContractEntityInvalidChannel)
	}
	channel, exists := facts.channels[rule.ChannelId]
	if !exists {
		return fmt.Errorf("%w: channel %d does not exist", ErrCustomerContractEntityInvalidChannel, rule.ChannelId)
	}
	if channel.Status != common.ChannelStatusEnabled {
		return fmt.Errorf("%w: channel %d is disabled", ErrCustomerContractEntityInvalidChannel, rule.ChannelId)
	}
	if channelSkipsGenericAbilities(channel.Type) {
		if !slices.Contains(channel.GetGroups(), rule.RouteGroup) {
			return fmt.Errorf("%w: channel %d is not in group %q", ErrCustomerContractEntityInvalidChannel, rule.ChannelId, rule.RouteGroup)
		}
		for _, candidate := range strings.Split(channel.Models, ",") {
			if strings.TrimSpace(candidate) == rule.PublicModel {
				return nil
			}
		}
	} else if facts.abilities[contractRouteSource{rule.ChannelId, rule.RouteGroup, rule.PublicModel}] {
		return nil
	}
	return fmt.Errorf("%w: channel %d does not serve model %q in group %q", ErrCustomerContractEntityInvalidChannel, rule.ChannelId, rule.PublicModel, rule.RouteGroup)
}

// GetContractRouteAvailability preserves rule order and returns no partial
// projection if a source read fails. It does not filter user or token permissions.
func GetContractRouteAvailability(rules []ContractEntityRule) ([]ContractEntityRule, error) {
	return readContractRouteAvailability(DB, rules)
}

func readContractRouteAvailability(tx *gorm.DB, rules []ContractEntityRule) ([]ContractEntityRule, error) {
	result := make([]ContractEntityRule, len(rules))
	copy(result, rules)
	// At most 200 channel parameters and 601 Ability parameters per query;
	// the OR tree and source rows stay bounded on every supported database.
	for start := 0; start < len(result); start += 200 {
		batch := result[start:min(start+200, len(result))]
		facts, err := loadContractRouteFacts(tx, batch)
		if err != nil {
			return nil, err
		}
		for i := range batch {
			batch[i].Available = facts.validate(batch[i]) == nil
		}
	}
	return result, nil
}

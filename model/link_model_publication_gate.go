package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

func EnsureChannelLinkModelPublications(tx *gorm.DB, channel *Channel, actorID int) error {
	if channel == nil {
		return fmt.Errorf("Link publication channel is required")
	}
	settings := channel.GetOtherSettings()
	if settings.LinkImplementation.Empty() {
		return nil
	}
	executions, err := DeriveChannelLinkExecutions(channel, &settings)
	if err != nil {
		return err
	}
	for _, execution := range executions {
		if err := rejectOrdinaryLinkModelConflict(tx, channel, execution); err != nil {
			return err
		}
		if _, err := EnsureLinkModelPublication(tx, LinkModelPublicationKey{
			ContractNamespace: LinkContractNamespaceDefault,
			RouteFamily:       execution.Binding.RouteFamily,
			CustomerModel:     execution.CustomerModel,
		}, execution.LinkSKU, actorID, channel.Id, "published from enabled Link access plan"); err != nil {
			return err
		}
	}
	return nil
}

func rejectOrdinaryLinkModelConflict(tx *gorm.DB, channel *Channel, execution ChannelLinkExecution) error {
	if tx == nil {
		tx = DB
	}
	groups := normalizedStringSet(strings.Split(channel.Group, ","))
	if len(groups) == 0 {
		return nil
	}
	var abilities []Ability
	if err := tx.Where(commonGroupCol+" IN ? AND model = ? AND enabled = ? AND channel_id <> ?", groups, execution.CustomerModel, true, channel.Id).Find(&abilities).Error; err != nil {
		return err
	}
	seen := make(map[int]struct{}, len(abilities))
	channelIDs := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		if _, exists := seen[ability.ChannelId]; exists {
			continue
		}
		seen[ability.ChannelId] = struct{}{}
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	candidates, err := loadLinkPublicationChannels(tx, channelIDs)
	if err != nil {
		return err
	}
	processed := make(map[int]struct{}, len(candidates))
	for _, ability := range abilities {
		if _, exists := processed[ability.ChannelId]; exists {
			continue
		}
		processed[ability.ChannelId] = struct{}{}
		candidate, exists := candidates[ability.ChannelId]
		if !exists {
			return fmt.Errorf("load Link publication channel %d: %w", ability.ChannelId, gorm.ErrRecordNotFound)
		}
		if !LinkRouteFamilySupportsChannel(execution.Binding.RouteFamily, &candidate, execution.CustomerModel) {
			continue
		}
		candidateSettings := candidate.GetOtherSettings()
		if candidateSettings.LinkImplementation.Empty() {
			return fmt.Errorf("Link customer model %q conflicts with ordinary channel %d in group %q", execution.CustomerModel, candidate.Id, ability.Group)
		}
		candidateExecution, err := ResolveChannelLinkExecution(&candidate, execution.CustomerModel, execution.Binding.RouteFamily)
		if err != nil || candidateExecution.LinkSKU != execution.LinkSKU {
			return fmt.Errorf("Link customer model %q has a non-equivalent channel %d in group %q", execution.CustomerModel, candidate.Id, ability.Group)
		}
	}
	return nil
}

func LinkPublicationHasOrdinaryConflict(publication *LinkModelPublication, group string) (bool, error) {
	availability, err := GetLinkModelPublicationAvailability(publication, group)
	return availability.RoutingConflict, err
}

type LinkModelPublicationAvailability struct {
	CurrentlyFulfillable bool
	RoutingConflict      bool
}

func GetLinkModelPublicationAvailability(publication *LinkModelPublication, group string) (LinkModelPublicationAvailability, error) {
	if publication == nil {
		return LinkModelPublicationAvailability{}, nil
	}
	availabilities, err := GetLinkModelPublicationAvailabilities([]LinkModelPublication{*publication}, group)
	if err != nil {
		return LinkModelPublicationAvailability{}, err
	}
	return availabilities[0], nil
}

func GetLinkModelPublicationAvailabilities(publications []LinkModelPublication, group string) ([]LinkModelPublicationAvailability, error) {
	availabilities := make([]LinkModelPublicationAvailability, len(publications))
	if len(publications) == 0 {
		return availabilities, nil
	}
	models := make([]string, 0, len(publications))
	seenModels := make(map[string]struct{}, len(publications))
	for i := range publications {
		if _, exists := seenModels[publications[i].CustomerModel]; exists {
			continue
		}
		seenModels[publications[i].CustomerModel] = struct{}{}
		models = append(models, publications[i].CustomerModel)
	}
	query := DB.Where("model IN ? AND enabled = ?", models, true)
	if group = strings.TrimSpace(group); group != "" && group != "auto" {
		query = query.Where(commonGroupCol+" = ?", group)
	}
	var abilities []Ability
	if err := query.Find(&abilities).Error; err != nil {
		return nil, err
	}
	channelIDs := make([]int, 0, len(abilities))
	seenChannelIDs := make(map[int]struct{}, len(abilities))
	abilitiesByModel := make(map[string][]Ability, len(models))
	for _, ability := range abilities {
		abilitiesByModel[ability.Model] = append(abilitiesByModel[ability.Model], ability)
		if _, exists := seenChannelIDs[ability.ChannelId]; !exists {
			seenChannelIDs[ability.ChannelId] = struct{}{}
			channelIDs = append(channelIDs, ability.ChannelId)
		}
	}
	channels, err := loadLinkPublicationChannels(DB, channelIDs)
	if err != nil {
		return nil, err
	}
	for i := range publications {
		publication := &publications[i]
		seen := make(map[int]struct{}, len(abilitiesByModel[publication.CustomerModel]))
		for _, ability := range abilitiesByModel[publication.CustomerModel] {
			if _, exists := seen[ability.ChannelId]; exists {
				continue
			}
			seen[ability.ChannelId] = struct{}{}
			channel, exists := channels[ability.ChannelId]
			if !exists {
				return nil, fmt.Errorf("load Link publication channel %d: %w", ability.ChannelId, gorm.ErrRecordNotFound)
			}
			if channel.Status != common.ChannelStatusEnabled || !LinkRouteFamilySupportsChannel(publication.RouteFamily, &channel, publication.CustomerModel) {
				continue
			}
			if channel.GetOtherSettings().LinkImplementation.Empty() {
				availabilities[i].RoutingConflict = true
				continue
			}
			if ValidateChannelLinkExecution(&channel, publication.CustomerModel, publication.RouteFamily, publication.LinkSKU) == nil {
				availabilities[i].CurrentlyFulfillable = true
			}
		}
		if availabilities[i].RoutingConflict {
			availabilities[i].CurrentlyFulfillable = false
		}
	}
	return availabilities, nil
}

func loadLinkPublicationChannels(tx *gorm.DB, channelIDs []int) (map[int]Channel, error) {
	channelsByID := make(map[int]Channel, len(channelIDs))
	if len(channelIDs) == 0 {
		return channelsByID, nil
	}
	var channels []Channel
	if err := tx.Where("id IN ?", channelIDs).Find(&channels).Error; err != nil {
		return nil, err
	}
	for i := range channels {
		channelsByID[channels[i].Id] = channels[i]
	}
	return channelsByID, nil
}

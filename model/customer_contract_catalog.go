package model

import (
	"slices"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

// Management config-diff diagnostics for ordinary channels. They describe a
// Channel configuration vs Ability mismatch for display only: they never gate
// saving or availability, and the shared route validation remains the only
// qualification for any source.
const (
	// ContractCatalogDiffConfigOnly marks a source declared in the channel
	// configuration whose Ability is missing or disabled.
	ContractCatalogDiffConfigOnly = "channel_config_only"
	// ContractCatalogDiffAbilityOnly marks an enabled Ability source whose
	// model or group is missing from the channel configuration.
	ContractCatalogDiffAbilityOnly = "ability_only"
)

// CustomerContractCatalogSource is one deduplicated (public model, route
// group, channel) management source. Available/UnavailableCategory come from
// the shared route validation; origin and config-diff fields are display-only
// diagnostics and may be present on available sources.
type CustomerContractCatalogSource struct {
	PublicModel         string
	RouteGroup          string
	ChannelId           int
	ChannelName         string
	Available           bool
	UnavailableCategory string
	FromChannelConfig   bool
	FromAbility         bool
	ConfigDiff          string
}

// CustomerContractCatalogNoGroupRecord flattens the models of one channel
// without any configured route group. These records are diagnostics only:
// they never join the group aggregation, never become submittable rules and
// never carry a group price.
type CustomerContractCatalogNoGroupRecord struct {
	ChannelId     int
	ChannelName   string
	ChannelStatus int
	Models        []string
}

// CustomerContractCatalog is the read-only management projection of every
// connected model source. It is rebuilt per request from current facts; no
// table, sync job or long-lived cache backs it.
type CustomerContractCatalog struct {
	Sources        []CustomerContractCatalogSource
	NoGroupRecords []CustomerContractCatalogNoGroupRecord
}

type customerContractCatalogCombo struct {
	fromChannel   bool
	activeAbility bool
	seenAbility   bool
}

// GetCustomerContractCatalog enumerates every connected model source from
// current channel configuration and ordinary-channel Ability rows, then marks
// each source with the shared route availability judgment. Typed channels are
// enumerated from their own configuration only: residual abilities neither
// add sources nor restore removed models. Abilities of deleted channels are
// not sources. Any read failure fails the whole catalog.
func GetCustomerContractCatalog() (*CustomerContractCatalog, error) {
	return readCustomerContractCatalog(DB)
}

func readCustomerContractCatalog(tx *gorm.DB) (*CustomerContractCatalog, error) {
	var channels []Channel
	if err := tx.Select("id", "name", "type", "status", "models", "group").Find(&channels).Error; err != nil {
		return nil, err
	}
	ordinaryIds := make([]int, 0, len(channels))
	channelById := make(map[int]Channel, len(channels))
	for i := range channels {
		channelById[channels[i].Id] = channels[i]
		if !channelSkipsGenericAbilities(channels[i].Type) {
			ordinaryIds = append(ordinaryIds, channels[i].Id)
		}
	}
	abilityRows, err := loadCatalogAbilityRows(tx, ordinaryIds)
	if err != nil {
		return nil, err
	}

	combos := make(map[string]*customerContractCatalogCombo)
	noGroupRecords := make([]CustomerContractCatalogNoGroupRecord, 0)
	for i := range channels {
		channel := &channels[i]
		models := splitCatalogModels(channel.Models)
		groups := catalogGroups(channel.GetGroups())
		if len(groups) == 0 {
			if len(models) > 0 {
				noGroupRecords = append(noGroupRecords, CustomerContractCatalogNoGroupRecord{
					ChannelId: channel.Id, ChannelName: channel.Name,
					ChannelStatus: channel.Status, Models: models,
				})
			}
		}
		for _, group := range groups {
			for _, modelName := range models {
				addCombo(combos, modelName, group, channel.Id).fromChannel = true
			}
		}
	}
	for _, row := range abilityRows {
		combo := addCombo(combos, row.Model, row.Group, row.ChannelId)
		combo.seenAbility = true
		if row.Enabled {
			combo.activeAbility = true
		}
	}

	abilityFacts := make(map[contractRouteSource]bool, len(abilityRows))
	for _, row := range abilityRows {
		if row.Enabled {
			abilityFacts[contractRouteSource{row.ChannelId, row.Group, row.Model}] = true
		}
	}
	facts := &contractRouteFacts{channels: channelById, abilities: abilityFacts}

	sources := make([]CustomerContractCatalogSource, 0, len(combos))
	for key, combo := range combos {
		parts := strings.SplitN(key, "\x00", 3)
		channelId, _ := strconv.Atoi(parts[2])
		rule := ContractEntityRule{PublicModel: parts[0], RouteGroup: parts[1], ChannelId: channelId}
		validateErr := facts.validate(rule)
		source := CustomerContractCatalogSource{
			PublicModel: rule.PublicModel, RouteGroup: rule.RouteGroup,
			ChannelId: channelId, ChannelName: channelById[channelId].Name,
			Available: validateErr == nil, UnavailableCategory: contractRouteUnavailableCategory(validateErr),
			FromChannelConfig: combo.fromChannel,
			FromAbility:       combo.seenAbility,
		}
		// Dedicated routes intentionally have no generic Ability projection.
		if !channelSkipsGenericAbilities(channelById[channelId].Type) {
			switch {
			case combo.fromChannel && !combo.activeAbility:
				source.ConfigDiff = ContractCatalogDiffConfigOnly
			case !combo.fromChannel && combo.seenAbility:
				source.ConfigDiff = ContractCatalogDiffAbilityOnly
			}
		}
		sources = append(sources, source)
	}
	sortCatalogSources(sources)
	sortCatalogNoGroupRecords(noGroupRecords)
	return &CustomerContractCatalog{Sources: sources, NoGroupRecords: noGroupRecords}, nil
}

// loadCatalogAbilityRows reads the Ability identity rows of existing ordinary
// channels in bounded batches. Disabled rows are kept: they carry the
// config-diff diagnosis and the unavailable reason. Rows of deleted channels
// and typed channels are never loaded.
func loadCatalogAbilityRows(tx *gorm.DB, ordinaryIds []int) ([]Ability, error) {
	rows := make([]Ability, 0)
	const batchSize = 500
	for start := 0; start < len(ordinaryIds); start += batchSize {
		batch := ordinaryIds[start:min(start+batchSize, len(ordinaryIds))]
		var partial []Ability
		if err := tx.Model(&Ability{}).
			Select("channel_id", "group", "model", "enabled").
			Where("channel_id IN ?", batch).
			Find(&partial).Error; err != nil {
			return nil, err
		}
		rows = append(rows, partial...)
	}
	return rows, nil
}

func addCombo(combos map[string]*customerContractCatalogCombo, modelName string, group string, channelId int) *customerContractCatalogCombo {
	key := modelName + "\x00" + group + "\x00" + strconv.Itoa(channelId)
	combo := combos[key]
	if combo == nil {
		combo = &customerContractCatalogCombo{}
		combos[key] = combo
	}
	return combo
}

// catalogGroups trims the raw group list and drops empty entries. Entries stay
// exact: the shared validation decides membership and ratio facts.
func catalogGroups(raw []string) []string {
	groups := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, group := range raw {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		if _, dup := seen[group]; dup {
			continue
		}
		seen[group] = struct{}{}
		groups = append(groups, group)
	}
	return groups
}

// splitCatalogModels splits the configured model list, trimming edge blanks
// and exact duplicates. Different letter cases stay distinct identities.
func splitCatalogModels(raw string) []string {
	models := make([]string, 0)
	seen := make(map[string]struct{})
	for _, modelName := range strings.Split(raw, ",") {
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			continue
		}
		if _, dup := seen[modelName]; dup {
			continue
		}
		seen[modelName] = struct{}{}
		models = append(models, modelName)
	}
	return models
}

func sortCatalogSources(sources []CustomerContractCatalogSource) {
	slices.SortFunc(sources, func(a, b CustomerContractCatalogSource) int {
		if c := strings.Compare(a.PublicModel, b.PublicModel); c != 0 {
			return c
		}
		if c := strings.Compare(a.RouteGroup, b.RouteGroup); c != 0 {
			return c
		}
		return a.ChannelId - b.ChannelId
	})
}

func sortCatalogNoGroupRecords(records []CustomerContractCatalogNoGroupRecord) {
	slices.SortFunc(records, func(a, b CustomerContractCatalogNoGroupRecord) int {
		return a.ChannelId - b.ChannelId
	})
}

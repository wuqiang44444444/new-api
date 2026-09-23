package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	hosttypes "github.com/QuantumNous/new-api/types"

	"gorm.io/gorm"
)

var (
	ErrCustomerContractEntityNotFound       = errors.New("customer contract entity not found")
	ErrCustomerContractEntityInvalidChannel = errors.New("customer contract rule channel is unavailable for the model")
)

// CustomerContract is one contract entity owned by a user. A user may hold
// multiple contracts; each API key binds at most one of them. Contract state,
// version and rules live here — the legacy user-level contract columns on
// users are no longer read at request time.
type CustomerContract struct {
	Id      int    `json:"id"`
	UserId  int    `json:"user_id" gorm:"index"`
	Name    string `json:"name" gorm:"type:varchar(128)"`
	Enabled bool   `json:"enabled"`
	Version int64  `json:"version" gorm:"type:bigint"`
	// Optional creation-time template provenance. Zero/empty on ordinary
	// creates; never written again after creation and never read at runtime.
	SourceTemplateId      int    `json:"source_template_id" gorm:"index"`
	SourceTemplateVersion int64  `json:"source_template_version" gorm:"type:bigint"`
	SourceTemplateName    string `json:"source_template_name" gorm:"type:varchar(128)"`
	CreatedAt             int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt             int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// CustomerContractEntityRule binds one public model inside one contract to an
// explicit channel source, a management route group and an eight-decimal
// fixed point discount. (contract_id, public_model, channel_id) is unique
// within a contract: the same public model may list several channels, but
// only when every rule of that model carries the identical discount. Channel
// and route group define allowed routing combinations; discount matching
// remains exact-model and independent of the selected combination.
type CustomerContractEntityRule struct {
	Id          int    `json:"id"`
	ContractId  int    `json:"contract_id" gorm:"index;uniqueIndex:idx_cc_entity_rule_contract_model_channel"`
	PublicModel string `json:"public_model" gorm:"type:varchar(255);uniqueIndex:idx_cc_entity_rule_contract_model_channel"`
	ChannelId   int    `json:"channel_id" gorm:"index;uniqueIndex:idx_cc_entity_rule_contract_model_channel"`
	RouteGroup  string `json:"route_group" gorm:"type:varchar(64);index"`
	RatioUnits  int64  `json:"ratio_units" gorm:"type:bigint"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// CustomerContractEntityAudit is the append-only audit trail for contract
// entity writes. States are redacted JSON snapshots via common.Marshal.
type CustomerContractEntityAudit struct {
	Id              int    `json:"id"`
	ContractId      int    `json:"contract_id" gorm:"index"`
	UserId          int    `json:"user_id" gorm:"index"`
	ContractVersion int64  `json:"contract_version" gorm:"type:bigint;index"`
	AdminUserId     int    `json:"admin_user_id" gorm:"index"`
	Operation       string `json:"operation" gorm:"type:varchar(32)"`
	Reason          string `json:"reason" gorm:"type:varchar(500)"`
	BeforeState     string `json:"-" gorm:"type:text"`
	AfterState      string `json:"-" gorm:"type:text"`
	CreatedAt       int64  `json:"created_at" gorm:"autoCreateTime"`
	AdminUsername   string `json:"admin_username" gorm:"->;-:migration"`
	BeforeEnabled   bool   `json:"before_enabled" gorm:"-"`
	AfterEnabled    bool   `json:"after_enabled" gorm:"-"`
	BeforeRuleCount int    `json:"before_rule_count" gorm:"-"`
	AfterRuleCount  int    `json:"after_rule_count" gorm:"-"`
}

// ContractEntityRule is the read model for one contract rule. Availability is
// derived from current channel/model facts and is never persisted.
type ContractEntityRule struct {
	PublicModel string `json:"public_model"`
	ChannelId   int    `json:"channel_id"`
	RouteGroup  string `json:"route_group"`
	RatioUnits  int64  `json:"ratio_units"`
	Available   bool   `json:"available"`
	// UnavailableCategory carries the controlled rejection category produced by
	// the same availability validation for request-side diagnostics. It is a
	// read-only annotation: never persisted and never serialized.
	UnavailableCategory string `json:"-" gorm:"-"`
}

// ContractEntitySnapshot is one immutable contract definition plus its rules.
type ContractEntitySnapshot struct {
	Id                    int                  `json:"id"`
	UserId                int                  `json:"user_id"`
	Name                  string               `json:"name"`
	Enabled               bool                 `json:"enabled"`
	Version               int64                `json:"version"`
	SourceTemplateId      int                  `json:"source_template_id,omitempty"`
	SourceTemplateVersion int64                `json:"source_template_version,omitempty"`
	SourceTemplateName    string               `json:"source_template_name,omitempty"`
	Rules                 []ContractEntityRule `json:"rules"`
}

type CustomerContractEntityRuleInput struct {
	PublicModel string
	ChannelId   int
	RouteGroup  string
	RatioUnits  int64
}

type CreateCustomerContractParams struct {
	UserId      int
	AdminUserId int
	Name        string
	Enabled     bool
	Reason      string
	Rules       []CustomerContractEntityRuleInput
	// Optional template provenance: both fields must be provided together.
	// When set, the creation transaction verifies the template still exists,
	// is enabled and matches the confirmed version before writing.
	SourceTemplateId      int
	SourceTemplateVersion int64
}

type ReplaceCustomerContractEntityParams struct {
	ContractId      int
	AdminUserId     int
	ExpectedVersion int64
	Name            string
	Enabled         *bool
	Reason          string
	Rules           []CustomerContractEntityRuleInput
}

type customerContractEntityAuditState struct {
	Name    string               `json:"name"`
	Enabled bool                 `json:"enabled"`
	Version int64                `json:"version"`
	Rules   []ContractEntityRule `json:"rules"`
}

// normalizeCustomerContractEntityRules validates one complete rule set at the
// entity save boundary. The same public model may appear on several channels,
// but only under one identical discount; identical model+channel pairs and
// names differing only by letter case are rejected.
func normalizeCustomerContractEntityRules(rules []CustomerContractEntityRuleInput) ([]CustomerContractEntityRuleInput, error) {
	if len(rules) == 0 {
		return nil, fmt.Errorf("%w: contract requires at least one rule", ErrCustomerContractInvalidRule)
	}
	normalized := make([]CustomerContractEntityRuleInput, 0, len(rules))
	modelDiscounts := make(map[string]int64, len(rules))
	modelNames := make(map[string]string, len(rules))
	usedPairs := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		rule.PublicModel = strings.TrimSpace(rule.PublicModel)
		rule.RouteGroup = strings.TrimSpace(rule.RouteGroup)
		if rule.PublicModel == "" || len(rule.PublicModel) > 255 {
			return nil, fmt.Errorf("%w: public model is required and must not exceed 255 characters", ErrCustomerContractInvalidRule)
		}
		if rule.RouteGroup == "" || len(rule.RouteGroup) > 64 || strings.EqualFold(rule.RouteGroup, "auto") || NormalizeChannelGroupFilter(rule.RouteGroup) == "" {
			return nil, fmt.Errorf("%w: route group must be a concrete group", ErrCustomerContractInvalidRule)
		}
		if rule.RatioUnits <= 0 || rule.RatioUnits > hosttypes.CustomerContractRatioScale {
			return nil, fmt.Errorf("%w: ratio must be greater than zero and no greater than one", ErrCustomerContractInvalidRule)
		}
		if rule.ChannelId <= 0 {
			return nil, fmt.Errorf("%w: rule for model %q must bind a concrete channel", ErrCustomerContractInvalidRule, rule.PublicModel)
		}
		modelKey := strings.ToLower(rule.PublicModel)
		if existingName, exists := modelNames[modelKey]; exists && existingName != rule.PublicModel {
			return nil, fmt.Errorf("%w: duplicate or case-only duplicate public model %q", ErrCustomerContractInvalidRule, rule.PublicModel)
		}
		if existingUnits, exists := modelDiscounts[modelKey]; exists && existingUnits != rule.RatioUnits {
			return nil, fmt.Errorf("%w: model %q must keep one identical discount across all of its channels", ErrCustomerContractInvalidRule, rule.PublicModel)
		}
		pairKey := modelKey + "\x00" + strconv.Itoa(rule.ChannelId)
		if _, exists := usedPairs[pairKey]; exists {
			return nil, fmt.Errorf("%w: model %q already binds channel %d", ErrCustomerContractInvalidRule, rule.PublicModel, rule.ChannelId)
		}
		modelNames[modelKey] = rule.PublicModel
		modelDiscounts[modelKey] = rule.RatioUnits
		usedPairs[pairKey] = struct{}{}
		normalized = append(normalized, rule)
	}
	return normalized, nil
}

// validateCustomerContractEntityChannel verifies that one rule's channel is an
// enabled channel that serves the public model in the rule's route group. It
// validates newly added or changed management sources using the same facts and
// policy as runtime availability. Historical sources remain editable.
func validateCustomerContractEntityChannel(tx *gorm.DB, channelId int, routeGroup string, publicModel string) error {
	rule := ContractEntityRule{ChannelId: channelId, RouteGroup: routeGroup, PublicModel: publicModel}
	facts, err := loadContractRouteFacts(tx, []ContractEntityRule{rule})
	if err != nil {
		return err
	}
	return facts.validate(rule)
}

// validateCustomerContractEntityRules verifies the channel reference of every
// submitted rule.
func validateCustomerContractEntityRules(tx *gorm.DB, rules []CustomerContractEntityRuleInput) error {
	for _, rule := range rules {
		if err := validateCustomerContractEntityChannel(tx, rule.ChannelId, rule.RouteGroup, rule.PublicModel); err != nil {
			return err
		}
	}
	return nil
}

// validateCustomerContractEntityNewSources validates only the channel
// references whose (public model, channel, route group) triple is not already
// stored for this contract. Already accepted sources stay accepted even when
// they later become unavailable: they must never block saving other discounts
// or re-enabling the contract.
func validateCustomerContractEntityNewSources(tx *gorm.DB, contractId int, rules []CustomerContractEntityRuleInput) error {
	existing, err := loadCustomerContractEntityRules(tx, contractId, false)
	if err != nil {
		return err
	}
	known := make(map[string]struct{}, len(existing))
	for _, rule := range existing {
		known[customerContractEntitySourceKey(rule.PublicModel, rule.ChannelId, rule.RouteGroup)] = struct{}{}
	}
	for _, rule := range rules {
		if _, stored := known[customerContractEntitySourceKey(rule.PublicModel, rule.ChannelId, rule.RouteGroup)]; stored {
			continue
		}
		if err := validateCustomerContractEntityChannel(tx, rule.ChannelId, rule.RouteGroup, rule.PublicModel); err != nil {
			return err
		}
	}
	return nil
}

// customerContractEntitySourceKey builds the dedup key for one rule source
// triple (case-insensitive public model).
func customerContractEntitySourceKey(publicModel string, channelId int, routeGroup string) string {
	return strings.ToLower(publicModel) + "\x00" + strconv.Itoa(channelId) + "\x00" + routeGroup
}

func customerContractEntityRulesFromInputs(inputs []CustomerContractEntityRuleInput) []ContractEntityRule {
	rules := make([]ContractEntityRule, 0, len(inputs))
	for _, input := range inputs {
		rules = append(rules, ContractEntityRule{
			PublicModel: input.PublicModel,
			ChannelId:   input.ChannelId,
			RouteGroup:  input.RouteGroup,
			RatioUnits:  input.RatioUnits,
		})
	}
	return rules
}

func createCustomerContractEntityAudit(tx *gorm.DB, contract *CustomerContract, adminUserId int, operation string, reason string, before *customerContractEntityAuditState, afterRules []ContractEntityRule) error {
	after := customerContractEntityAuditState{Name: contract.Name, Enabled: contract.Enabled, Version: contract.Version, Rules: afterRules}
	var beforeJSON, afterJSON []byte
	var err error
	if before != nil {
		beforeJSON, err = common.Marshal(before)
		if err != nil {
			return err
		}
	}
	afterJSON, err = common.Marshal(after)
	if err != nil {
		return err
	}
	return tx.Create(&CustomerContractEntityAudit{
		ContractId: contract.Id, UserId: contract.UserId, ContractVersion: contract.Version,
		AdminUserId: adminUserId, Operation: operation, Reason: reason,
		BeforeState: string(beforeJSON), AfterState: string(afterJSON),
	}).Error
}

func CreateCustomerContractEntity(params CreateCustomerContractParams) (*ContractEntitySnapshot, error) {
	if params.UserId <= 0 || params.AdminUserId <= 0 {
		return nil, fmt.Errorf("invalid contract owner or administrator")
	}
	if (params.SourceTemplateId <= 0) != (params.SourceTemplateVersion <= 0) {
		return nil, fmt.Errorf("template source requires both template id and version")
	}
	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" || len(params.Name) > 128 {
		return nil, fmt.Errorf("contract name is required and must not exceed 128 characters")
	}
	params.Reason = strings.TrimSpace(params.Reason)
	if params.Reason == "" || len(params.Reason) > 500 {
		return nil, fmt.Errorf("contract change reason is required and must not exceed 500 characters")
	}
	rules, err := normalizeCustomerContractEntityRules(params.Rules)
	if err != nil {
		return nil, err
	}
	useTemplate := params.SourceTemplateId > 0
	if !useTemplate {
		if err := validateCustomerContractEntityRules(DB, rules); err != nil {
			return nil, err
		}
	}
	contract := &CustomerContract{UserId: params.UserId, Name: params.Name, Enabled: params.Enabled, Version: 1}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if useTemplate {
			// Template-sourced creations verify the confirmed template and the
			// final rules against the same transaction facts. The plain path
			// keeps its existing pre-transaction validation timing.
			template, err := checkCustomerContractTemplateSource(tx, params.SourceTemplateId, params.SourceTemplateVersion)
			if err != nil {
				return err
			}
			if err := validateCustomerContractEntityRules(tx, rules); err != nil {
				return err
			}
			contract.SourceTemplateId = template.Id
			contract.SourceTemplateVersion = template.Version
			contract.SourceTemplateName = template.Name
		}
		if err := tx.Create(contract).Error; err != nil {
			return err
		}
		if err := saveCustomerContractEntityRules(tx, contract.Id, rules); err != nil {
			return err
		}
		return createCustomerContractEntityAudit(tx, contract, params.AdminUserId, "create", params.Reason, nil, customerContractEntityRulesFromInputs(rules))
	})
	if err != nil {
		return nil, err
	}
	return GetContractEntitySnapshot(contract.Id, true)
}

func saveCustomerContractEntityRules(tx *gorm.DB, contractId int, rules []CustomerContractEntityRuleInput) error {
	if err := tx.Where("contract_id = ?", contractId).Delete(&CustomerContractEntityRule{}).Error; err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	rows := make([]CustomerContractEntityRule, 0, len(rules))
	for _, rule := range rules {
		rows = append(rows, CustomerContractEntityRule{
			ContractId: contractId, PublicModel: rule.PublicModel,
			ChannelId: rule.ChannelId, RouteGroup: rule.RouteGroup, RatioUnits: rule.RatioUnits,
		})
	}
	return tx.Create(&rows).Error
}

// ReplaceCustomerContractEntity is the single authoritative contract write.
// It commits expected_version fencing, the fail-closed auth fence, the rule
// replacement and the audit in one transaction, then refreshes caches.
func ReplaceCustomerContractEntity(params ReplaceCustomerContractEntityParams) (*ContractEntitySnapshot, error) {
	if params.ContractId <= 0 || params.AdminUserId <= 0 {
		return nil, fmt.Errorf("invalid contract or administrator")
	}
	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" || len(params.Name) > 128 {
		return nil, fmt.Errorf("contract name is required and must not exceed 128 characters")
	}
	params.Reason = strings.TrimSpace(params.Reason)
	if params.Reason == "" || len(params.Reason) > 500 {
		return nil, fmt.Errorf("contract change reason is required and must not exceed 500 characters")
	}
	rules, err := normalizeCustomerContractEntityRules(params.Rules)
	if err != nil {
		return nil, err
	}
	var nextVersion int64
	err = DB.Transaction(func(tx *gorm.DB) error {
		var contract CustomerContract
		if err := lockForUpdate(tx).First(&contract, params.ContractId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCustomerContractEntityNotFound
			}
			return err
		}
		if contract.Version != params.ExpectedVersion {
			return ErrCustomerContractVersionConflict
		}
		enabled := contract.Enabled
		if params.Enabled != nil {
			enabled = *params.Enabled
		}
		// Only new or changed rule sources are reference-validated. Sources
		// accepted in an earlier version stay accepted even when they later
		// become unavailable, so they cannot block saving other discounts or
		// re-enabling the contract.
		if err := validateCustomerContractEntityNewSources(tx, contract.Id, rules); err != nil {
			return err
		}
		beforeRules, err := loadCustomerContractEntityRules(tx, contract.Id, false)
		if err != nil {
			return err
		}
		before := customerContractEntityAuditState{Name: contract.Name, Enabled: contract.Enabled, Version: contract.Version, Rules: beforeRules}
		if _, err := IncrementUserAuthVersionWithTx(tx, contract.UserId); err != nil {
			return err
		}
		nextVersion = contract.Version + 1
		if err := tx.Model(&CustomerContract{}).Where("id = ?", contract.Id).Updates(map[string]any{
			"name": params.Name, "enabled": enabled, "version": nextVersion,
		}).Error; err != nil {
			return err
		}
		contract.Name, contract.Enabled, contract.Version = params.Name, enabled, nextVersion
		if err := saveCustomerContractEntityRules(tx, contract.Id, rules); err != nil {
			return err
		}
		operation := "update"
		if contract.Enabled != before.Enabled {
			if contract.Enabled {
				operation = "enable"
			} else {
				operation = "disable"
			}
		}
		return createCustomerContractEntityAudit(tx, &contract, params.AdminUserId, operation, params.Reason, &before, customerContractEntityRulesFromInputs(rules))
	})
	if err != nil {
		return nil, err
	}
	if err := PublishUserAuthCache(userIdOfContract(params.ContractId)); err != nil {
		return nil, fmt.Errorf("contract version %d committed with auth cache publication failure: %w", nextVersion, err)
	}
	return GetContractEntitySnapshot(params.ContractId, true)
}

func userIdOfContract(contractId int) int {
	var contract CustomerContract
	if err := DB.Select("user_id").First(&contract, contractId).Error; err != nil {
		return 0
	}
	return contract.UserId
}

func loadCustomerContractEntityRules(tx *gorm.DB, contractId int, includeAvailability bool) ([]ContractEntityRule, error) {
	var rows []CustomerContractEntityRule
	if err := tx.Where("contract_id = ?", contractId).Order("public_model").Find(&rows).Error; err != nil {
		return nil, err
	}
	rules := make([]ContractEntityRule, 0, len(rows))
	for _, row := range rows {
		rule := ContractEntityRule{
			PublicModel: row.PublicModel, ChannelId: row.ChannelId,
			RouteGroup: row.RouteGroup, RatioUnits: row.RatioUnits,
		}
		rules = append(rules, rule)
	}
	if includeAvailability {
		return readContractRouteAvailability(tx, rules)
	}
	return rules, nil
}

// GetContractEntitySnapshot loads one contract with its rules, optionally
// deriving current availability. Used by admin and self-service views.
func GetContractEntitySnapshot(contractId int, includeAvailability bool) (*ContractEntitySnapshot, error) {
	if contractId <= 0 {
		return nil, fmt.Errorf("invalid contract id")
	}
	var snapshot *ContractEntitySnapshot
	err := DB.Transaction(func(tx *gorm.DB) error {
		var contract CustomerContract
		if err := lockForUpdate(tx).First(&contract, contractId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCustomerContractEntityNotFound
			}
			return err
		}
		rules, err := loadCustomerContractEntityRules(tx, contractId, includeAvailability)
		if err != nil {
			return err
		}
		snapshot = &ContractEntitySnapshot{
			Id: contract.Id, UserId: contract.UserId, Name: contract.Name, Enabled: contract.Enabled,
			Version: contract.Version, SourceTemplateId: contract.SourceTemplateId,
			SourceTemplateVersion: contract.SourceTemplateVersion, SourceTemplateName: contract.SourceTemplateName,
			Rules: rules,
		}
		return nil
	})
	return snapshot, err
}

// ListContractEntitiesForUser returns all contracts of one user ordered by id.
func ListContractEntitiesForUser(userId int, includeAvailability bool) ([]ContractEntitySnapshot, error) {
	if userId <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	var contracts []CustomerContract
	if err := DB.Where("user_id = ?", userId).Order("id").Find(&contracts).Error; err != nil {
		return nil, err
	}
	result := make([]ContractEntitySnapshot, 0, len(contracts))
	for i := range contracts {
		rules, err := loadCustomerContractEntityRules(DB, contracts[i].Id, includeAvailability)
		if err != nil {
			return nil, err
		}
		result = append(result, ContractEntitySnapshot{
			Id: contracts[i].Id, UserId: contracts[i].UserId, Name: contracts[i].Name,
			Enabled: contracts[i].Enabled, Version: contracts[i].Version,
			SourceTemplateId:      contracts[i].SourceTemplateId,
			SourceTemplateVersion: contracts[i].SourceTemplateVersion,
			SourceTemplateName:    contracts[i].SourceTemplateName,
			Rules:                 rules,
		})
	}
	return result, nil
}

// GetContractEntityOwnedByUser validates a key binding target: the contract
// must exist and belong to the key's owner.
func GetContractEntityOwnedByUser(contractId int, userId int) (*CustomerContract, error) {
	if contractId <= 0 || userId <= 0 {
		return nil, fmt.Errorf("invalid contract or user")
	}
	var contract CustomerContract
	if err := DB.Where("id = ? AND user_id = ?", contractId, userId).First(&contract).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomerContractEntityNotFound
		}
		return nil, err
	}
	return &contract, nil
}

// CountTokensBoundToContract reports how many keys currently reference a
// contract, for admin impact display.
func CountTokensBoundToContract(contractId int) (int64, error) {
	var count int64
	if err := DB.Model(&Token{}).Where("contract_id = ?", contractId).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// RefreshContractEntityAvailability refreshes the derived availability of an
// already loaded snapshot in place.
func RefreshContractEntityAvailability(snapshot *ContractEntitySnapshot) error {
	if snapshot == nil {
		return fmt.Errorf("contract snapshot is nil")
	}
	rules, err := GetContractRouteAvailability(snapshot.Rules)
	if err != nil {
		return err
	}
	snapshot.Rules = rules
	return nil
}

// CustomerContractEntityChannelOption is one qualifying channel for a public
// model inside a route group, for admin contract-rule drawers.
type CustomerContractEntityChannelOption struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

// CustomerContractEntityGroupModelChannels lists, for one route group, every
// model with the channels qualified to serve it.
type CustomerContractEntityGroupModelChannels struct {
	Model    string                                `json:"model"`
	Channels []CustomerContractEntityChannelOption `json:"channels"`
}

// GetContractEntityAudits returns one contract entity's audit page. Counts are
// derived from the redacted JSON snapshots.
func GetContractEntityAudits(contractId int, offset int, limit int) ([]CustomerContractEntityAudit, int64, error) {
	if contractId <= 0 {
		return nil, 0, fmt.Errorf("invalid contract id")
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := DB.Model(&CustomerContractEntityAudit{}).Where("contract_id = ?", contractId).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var audits []CustomerContractEntityAudit
	err := DB.Model(&CustomerContractEntityAudit{}).
		Select("customer_contract_entity_audits.*, COALESCE(users.username, '') AS admin_username").
		Joins("LEFT JOIN users ON users.id = customer_contract_entity_audits.admin_user_id").
		Where("customer_contract_entity_audits.contract_id = ?", contractId).
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&audits).Error
	if err != nil {
		return nil, 0, err
	}
	for i := range audits {
		var before customerContractEntityAuditState
		var after customerContractEntityAuditState
		// Migration audits carry no before state; empty snapshots are valid.
		if strings.TrimSpace(audits[i].BeforeState) != "" {
			if err := common.Unmarshal([]byte(audits[i].BeforeState), &before); err != nil {
				return nil, 0, err
			}
		}
		if strings.TrimSpace(audits[i].AfterState) != "" {
			if err := common.Unmarshal([]byte(audits[i].AfterState), &after); err != nil {
				return nil, 0, err
			}
		}
		audits[i].BeforeEnabled = before.Enabled
		audits[i].AfterEnabled = after.Enabled
		audits[i].BeforeRuleCount = len(before.Rules)
		audits[i].AfterRuleCount = len(after.Rules)
	}
	return audits, total, nil
}

// GetCustomerContractEntityChannelOptions lists, for one route group, every
// model with the enabled channels qualified to serve it: native ability
// channels plus Seedance Link channels of the group.
func GetCustomerContractEntityChannelOptions(group string) ([]CustomerContractEntityGroupModelChannels, error) {
	group = strings.TrimSpace(group)
	if group == "" || strings.EqualFold(group, "auto") {
		return nil, fmt.Errorf("route group must be a concrete group")
	}
	type channelRow struct {
		Model     string
		ChannelId int
		Name      string
	}
	var rows []channelRow
	err := DB.Model(&Ability{}).
		Select("abilities.model AS model, abilities.channel_id AS channel_id, channels.name AS name").
		Joins("JOIN channels ON channels.id = abilities.channel_id AND channels.status = ?", common.ChannelStatusEnabled).
		Where(&Ability{Group: group, Enabled: true}).
		Order("model ASC, channel_id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	orderedModels := make([]string, 0, len(rows))
	channelIdsByModel := make(map[string][]int)
	seenPair := make(map[string]struct{})
	for _, row := range rows {
		key := row.Model + "\x00" + strconv.Itoa(row.ChannelId)
		if _, dup := seenPair[key]; dup {
			continue
		}
		seenPair[key] = struct{}{}
		if _, known := channelIdsByModel[row.Model]; !known {
			orderedModels = append(orderedModels, row.Model)
		}
		channelIdsByModel[row.Model] = append(channelIdsByModel[row.Model], row.ChannelId)
	}
	var seedanceChannels []Channel
	if err := ApplyChannelGroupFilter(DB.Model(&Channel{}), group).
		Where("type IN ? AND status = ?", []int{constant.ChannelTypeSeedanceLink, constant.ChannelTypeAzureBatch}, common.ChannelStatusEnabled).
		Find(&seedanceChannels).Error; err != nil {
		return nil, err
	}
	channelName := make(map[int]string, len(rows))
	for _, row := range rows {
		channelName[row.ChannelId] = row.Name
	}
	for i := range seedanceChannels {
		channelName[seedanceChannels[i].Id] = seedanceChannels[i].Name
		for _, modelName := range strings.Split(seedanceChannels[i].Models, ",") {
			modelName = strings.TrimSpace(modelName)
			if modelName == "" {
				continue
			}
			key := modelName + "\x00" + strconv.Itoa(seedanceChannels[i].Id)
			if _, dup := seenPair[key]; dup {
				continue
			}
			seenPair[key] = struct{}{}
			if _, known := channelIdsByModel[modelName]; !known {
				orderedModels = append(orderedModels, modelName)
			}
			channelIdsByModel[modelName] = append(channelIdsByModel[modelName], seedanceChannels[i].Id)
		}
	}
	result := make([]CustomerContractEntityGroupModelChannels, 0, len(orderedModels))
	for _, modelName := range orderedModels {
		options := make([]CustomerContractEntityChannelOption, 0, len(channelIdsByModel[modelName]))
		for _, channelId := range channelIdsByModel[modelName] {
			options = append(options, CustomerContractEntityChannelOption{Id: channelId, Name: channelName[channelId]})
		}
		result = append(result, CustomerContractEntityGroupModelChannels{
			Model: modelName, Channels: options,
		})
	}
	return result, nil
}

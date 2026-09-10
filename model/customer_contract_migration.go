package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

var (
	// ErrCustomerContractMigrationNothingToDo reports a user without legacy
	// user-level contract rules.
	ErrCustomerContractMigrationNothingToDo = errors.New("user has no legacy customer contract rules")
	// ErrCustomerContractMigrationAlreadyDone keeps the migration idempotent:
	// a user is migrated at most once.
	ErrCustomerContractMigrationAlreadyDone = errors.New("legacy customer contract was already migrated")
	// ErrCustomerContractMigrationChannelRequired reports a rule whose channel
	// cannot be chosen automatically.
	ErrCustomerContractMigrationChannelRequired = errors.New("contract rule has no unique channel candidate; an explicit channel is required")
)

// CustomerContractMigrationRulePreview is one legacy rule with its channel
// candidates. Legacy rules only carried a route group, so the explicit channel
// must be resolved during migration: exactly one enabled candidate is chosen
// automatically; zero or multiple candidates require an admin decision.
type CustomerContractMigrationRulePreview struct {
	PublicModel       string `json:"public_model"`
	RouteGroup        string `json:"route_group"`
	RatioUnits        int64  `json:"ratio_units"`
	ChannelIds        []int  `json:"channel_ids"`
	ResolvedChannelId int    `json:"resolved_channel_id"`
	NeedsDecision     bool   `json:"needs_decision"`
}

type CustomerContractMigrationPreview struct {
	UserId          int                                    `json:"user_id"`
	Username        string                                 `json:"username"`
	ContractEnabled bool                                   `json:"contract_enabled"`
	BoundTokenCount int64                                  `json:"bound_token_count"`
	AlreadyMigrated bool                                   `json:"already_migrated"`
	Rules           []CustomerContractMigrationRulePreview `json:"rules"`
}

type MigrateCustomerContractParams struct {
	UserId           int
	AdminUserId      int
	ContractName     string
	Reason           string
	ChannelOverrides map[string]int
}

func legacyCustomerContractChannelCandidates(tx *gorm.DB, channelId int, routeGroup string, publicModel string) ([]int, error) {
	var channels []Channel
	query := ApplyChannelGroupFilter(tx.Model(&Channel{}), routeGroup).
		Where("status = ?", common.ChannelStatusEnabled)
	if channelId > 0 {
		query = query.Where("id = ?", channelId)
	}
	if err := query.Find(&channels).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(channels))
	for i := range channels {
		if validateCustomerContractEntityChannel(tx, channels[i].Id, routeGroup, publicModel) == nil {
			ids = append(ids, channels[i].Id)
		}
	}
	return ids, nil
}

// PreviewLegacyCustomerContractMigration lists every user that still has
// legacy user-level contract rules together with per-rule channel candidates.
// It is read-only and safe to run repeatedly.
func PreviewLegacyCustomerContractMigration() ([]CustomerContractMigrationPreview, error) {
	var rows []CustomerModelContract
	if err := DB.Order("user_id, public_model").Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []CustomerContractMigrationPreview{}, nil
	}
	byUser := make(map[int][]CustomerModelContract)
	userIds := make([]int, 0)
	for _, row := range rows {
		if _, exists := byUser[row.UserId]; !exists {
			userIds = append(userIds, row.UserId)
		}
		byUser[row.UserId] = append(byUser[row.UserId], row)
	}
	var users []User
	if err := DB.Select("id", "username", "contract_mode").Where("id IN ?", userIds).Find(&users).Error; err != nil {
		return nil, err
	}
	migrated := make(map[int]struct{})
	var migratedAudits []CustomerContractEntityAudit
	if err := DB.Select("user_id").Where("operation = ?", "migrate").Find(&migratedAudits).Error; err == nil {
		for _, audit := range migratedAudits {
			migrated[audit.UserId] = struct{}{}
		}
	}

	previews := make([]CustomerContractMigrationPreview, 0, len(users))
	for _, user := range users {
		preview := CustomerContractMigrationPreview{
			UserId: user.Id, Username: user.Username, ContractEnabled: user.ContractMode,
			AlreadyMigrated: true,
		}
		if err := DB.Model(&Token{}).Where("user_id = ? AND deleted_at IS NULL", user.Id).Count(&preview.BoundTokenCount).Error; err != nil {
			return nil, err
		}
		for _, legacy := range byUser[user.Id] {
			rule := CustomerContractMigrationRulePreview{
				PublicModel: legacy.PublicModel, RouteGroup: legacy.RouteGroup, RatioUnits: legacy.RatioUnits,
			}
			candidates, err := legacyCustomerContractChannelCandidates(DB, 0, legacy.RouteGroup, legacy.PublicModel)
			if err != nil {
				return nil, err
			}
			rule.ChannelIds = candidates
			if len(candidates) == 1 {
				rule.ResolvedChannelId = candidates[0]
			} else {
				rule.NeedsDecision = true
			}
			preview.Rules = append(preview.Rules, rule)
		}
		if _, done := migrated[user.Id]; !done {
			preview.AlreadyMigrated = false
		}
		previews = append(previews, preview)
	}
	return previews, nil
}

// MigrateLegacyCustomerContractForUser converts one user's legacy user-level
// contract into a contract entity and binds the user's existing API keys to
// it. The original enabled state and discount precision are preserved; the
// user's original key groups are never rewritten. Legacy rule rows are kept
// as history and are no longer read at request time.
func MigrateLegacyCustomerContractForUser(params MigrateCustomerContractParams) (*ContractEntitySnapshot, error) {
	if params.UserId <= 0 || params.AdminUserId <= 0 {
		return nil, fmt.Errorf("invalid contract owner or administrator")
	}
	params.Reason = strings.TrimSpace(params.Reason)
	if params.Reason == "" || len(params.Reason) > 500 {
		return nil, fmt.Errorf("contract change reason is required and must not exceed 500 characters")
	}
	contractName := strings.TrimSpace(params.ContractName)
	if contractName == "" {
		contractName = "Legacy contract"
	}
	if len(contractName) > 128 {
		return nil, fmt.Errorf("contract name must not exceed 128 characters")
	}

	var result *ContractEntitySnapshot
	err := DB.Transaction(func(tx *gorm.DB) error {
		var legacyRules []CustomerModelContract
		if err := lockForUpdate(tx).Where("user_id = ?", params.UserId).Order("public_model").Find(&legacyRules).Error; err != nil {
			return err
		}
		if len(legacyRules) == 0 {
			return ErrCustomerContractMigrationNothingToDo
		}
		var migratedCount int64
		if err := tx.Model(&CustomerContractEntityAudit{}).
			Where("user_id = ? AND operation = ?", params.UserId, "migrate").
			Count(&migratedCount).Error; err != nil {
			return err
		}
		if migratedCount > 0 {
			return ErrCustomerContractMigrationAlreadyDone
		}

		var user User
		if err := lockForUpdate(tx).
			Select("id", "auth_version", "contract_mode", "contract_version").
			First(&user, params.UserId).Error; err != nil {
			return err
		}

		rules := make([]CustomerContractEntityRuleInput, 0, len(legacyRules))
		for _, legacy := range legacyRules {
			channelId := params.ChannelOverrides[legacy.PublicModel]
			if channelId == 0 {
				candidates, err := legacyCustomerContractChannelCandidates(tx, 0, legacy.RouteGroup, legacy.PublicModel)
				if err != nil {
					return err
				}
				if len(candidates) != 1 {
					return fmt.Errorf("%w: model %q candidates %v", ErrCustomerContractMigrationChannelRequired, legacy.PublicModel, candidates)
				}
				channelId = candidates[0]
			}
			if err := validateCustomerContractEntityChannel(tx, channelId, legacy.RouteGroup, legacy.PublicModel); err != nil {
				return err
			}
			rules = append(rules, CustomerContractEntityRuleInput{
				PublicModel: legacy.PublicModel, ChannelId: channelId,
				RouteGroup: legacy.RouteGroup, RatioUnits: legacy.RatioUnits,
			})
		}

		contract := &CustomerContract{
			UserId: params.UserId, Name: contractName, Enabled: user.ContractMode, Version: max(user.ContractVersion, 1),
		}
		if err := tx.Create(contract).Error; err != nil {
			return err
		}
		if err := saveCustomerContractEntityRules(tx, contract.Id, rules); err != nil {
			return err
		}
		var tokens []Token
		if err := tx.Select("key").Where("user_id = ? AND contract_id = 0", params.UserId).Find(&tokens).Error; err != nil {
			return err
		}
		for _, token := range tokens {
			if err := invalidateTokenCacheForMutation(token.Key); err != nil {
				return err
			}
		}
		if err := tx.Model(&Token{}).
			Where("user_id = ? AND deleted_at IS NULL AND contract_id = 0", params.UserId).
			Update("contract_id", contract.Id).Error; err != nil {
			return err
		}
		if _, err := IncrementUserAuthVersionWithTx(tx, params.UserId); err != nil {
			return err
		}

		after := customerContractEntityAuditState{
			Name: contract.Name, Enabled: contract.Enabled, Version: contract.Version,
			Rules: customerContractEntityRulesFromInputs(rules),
		}
		afterJSON, err := common.Marshal(after)
		if err != nil {
			return err
		}
		if err := tx.Create(&CustomerContractEntityAudit{
			ContractId: contract.Id, UserId: params.UserId, ContractVersion: contract.Version,
			AdminUserId: params.AdminUserId, Operation: "migrate", Reason: params.Reason,
			AfterState: string(afterJSON),
		}).Error; err != nil {
			return err
		}
		result = &ContractEntitySnapshot{
			Id: contract.Id, UserId: contract.UserId, Name: contract.Name,
			Enabled: contract.Enabled, Version: contract.Version,
			Rules: customerContractEntityRulesFromInputs(rules),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := PublishUserAuthCache(params.UserId); err != nil {
		return nil, fmt.Errorf("migration committed but authentication cache publication failed: %w", err)
	}
	return GetContractEntitySnapshotBySnapshot(result)
}

func GetContractEntitySnapshotBySnapshot(snapshot *ContractEntitySnapshot) (*ContractEntitySnapshot, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("migration snapshot is nil")
	}
	return GetContractEntitySnapshot(snapshot.Id, true)
}

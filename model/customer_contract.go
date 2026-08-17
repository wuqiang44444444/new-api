package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"

	"gorm.io/gorm"
)

var (
	ErrCustomerContractVersionConflict = errors.New("customer contract version conflict")
	ErrCustomerContractInvalidRule     = errors.New("invalid customer contract rule")
	ErrCustomerContractRuleUnavailable = errors.New("customer contract rule is unavailable")
)

type CustomerModelContract struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id" gorm:"index;uniqueIndex:idx_customer_model_contract_user_model"`
	PublicModel string `json:"public_model" gorm:"type:varchar(255);uniqueIndex:idx_customer_model_contract_user_model"`
	RouteGroup  string `json:"route_group" gorm:"type:varchar(64);index"`
	RatioUnits  int64  `json:"ratio_units" gorm:"type:bigint"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type CustomerContractAudit struct {
	Id              int    `json:"id"`
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

type CustomerContractRule struct {
	PublicModel string `json:"public_model"`
	RouteGroup  string `json:"route_group"`
	RatioUnits  int64  `json:"ratio_units"`
	Available   bool   `json:"available"`
}

type CustomerContractSnapshot struct {
	UserId  int                    `json:"user_id"`
	Enabled bool                   `json:"enabled"`
	Version int64                  `json:"version"`
	Rules   []CustomerContractRule `json:"rules"`
}

type ReplaceCustomerContractParams struct {
	UserId          int
	AdminUserId     int
	ExpectedVersion int64
	Enabled         bool
	Reason          string
	Rules           []CustomerContractRule
}

type customerContractAuditState struct {
	Enabled      bool                               `json:"enabled"`
	Version      int64                              `json:"version"`
	Rules        []CustomerContractRule             `json:"rules"`
	PricingFacts []customerContractAuditPricingFact `json:"pricing_facts,omitempty"`
}

type customerContractAuditPricingFact struct {
	PublicModel         string `json:"public_model"`
	RouteGroup          string `json:"route_group"`
	ContractDiscount    string `json:"contract_discount"`
	NativeGroupRatio    string `json:"native_group_ratio"`
	EffectiveMultiplier string `json:"effective_multiplier"`
	SpecialGroupRatio   bool   `json:"special_group_ratio"`
}

func InitializeUserContractVersions() error {
	if err := DB.Model(&User{}).
		Where("contract_version IS NULL OR contract_version < ?", 0).
		Update("contract_version", 0).Error; err != nil {
		return err
	}
	return DB.Model(&User{}).
		Where("contract_mode IS NULL").
		Updates(map[string]any{"contract_mode": false}).Error
}

func GetCustomerContractSnapshot(userId int) (*CustomerContractSnapshot, error) {
	return getCustomerContractSnapshot(userId, true)
}

func GetCustomerContractSnapshotWithoutAvailability(userId int) (*CustomerContractSnapshot, error) {
	return getCustomerContractSnapshot(userId, false)
}

func getCustomerContractSnapshot(userId int, includeAvailability bool) (*CustomerContractSnapshot, error) {
	if userId <= 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	var user User
	if err := DB.Select("id", "contract_mode", "contract_version").First(&user, userId).Error; err != nil {
		return nil, err
	}
	rules, err := loadCustomerContractRules(DB, userId, includeAvailability)
	if err != nil {
		return nil, err
	}
	return &CustomerContractSnapshot{
		UserId:  user.Id,
		Enabled: user.ContractMode,
		Version: user.ContractVersion,
		Rules:   rules,
	}, nil
}

func ReplaceCustomerContract(params ReplaceCustomerContractParams) (*CustomerContractSnapshot, error) {
	if params.UserId <= 0 || params.AdminUserId <= 0 {
		return nil, fmt.Errorf("invalid contract owner or administrator")
	}
	params.Reason = strings.TrimSpace(params.Reason)
	if params.Reason == "" || len(params.Reason) > 500 {
		return nil, fmt.Errorf("contract change reason is required and must not exceed 500 characters")
	}

	normalizedRules, err := normalizeCustomerContractRules(params.Rules)
	if err != nil {
		return nil, err
	}
	if params.Enabled {
		if err := validateCustomerContractRulesAvailable(DB, normalizedRules); err != nil {
			return nil, err
		}
	}

	var nextVersion int64
	err = DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).
			Select("id", "auth_version", "group", "contract_mode", "contract_version").
			First(&user, params.UserId).Error; err != nil {
			return err
		}
		if user.ContractVersion != params.ExpectedVersion {
			return ErrCustomerContractVersionConflict
		}
		beforeRules, err := loadCustomerContractRules(tx, params.UserId, false)
		if err != nil {
			return err
		}
		before := customerContractAuditState{
			Enabled:      user.ContractMode,
			Version:      user.ContractVersion,
			Rules:        beforeRules,
			PricingFacts: customerContractAuditPricingFacts(user.Group, beforeRules),
		}

		if _, err := IncrementUserAuthVersionWithTx(tx, params.UserId); err != nil {
			return err
		}
		nextVersion = user.ContractVersion + 1
		if err := tx.Model(&User{}).Where("id = ?", params.UserId).Updates(map[string]any{
			"contract_mode":    params.Enabled,
			"contract_version": nextVersion,
		}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", params.UserId).Delete(&CustomerModelContract{}).Error; err != nil {
			return err
		}
		if len(normalizedRules) > 0 {
			rows := make([]CustomerModelContract, 0, len(normalizedRules))
			for _, rule := range normalizedRules {
				rows = append(rows, CustomerModelContract{
					UserId:      params.UserId,
					PublicModel: rule.PublicModel,
					RouteGroup:  rule.RouteGroup,
					RatioUnits:  rule.RatioUnits,
				})
			}
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}

		after := customerContractAuditState{
			Enabled:      params.Enabled,
			Version:      nextVersion,
			Rules:        normalizedRules,
			PricingFacts: customerContractAuditPricingFacts(user.Group, normalizedRules),
		}
		beforeJSON, err := common.Marshal(before)
		if err != nil {
			return err
		}
		afterJSON, err := common.Marshal(after)
		if err != nil {
			return err
		}
		operation := "update"
		if user.ContractVersion == 0 {
			operation = "create"
		} else if user.ContractMode != params.Enabled {
			if params.Enabled {
				operation = "enable"
			} else {
				operation = "disable"
			}
		}
		return tx.Create(&CustomerContractAudit{
			UserId:          params.UserId,
			ContractVersion: nextVersion,
			AdminUserId:     params.AdminUserId,
			Operation:       operation,
			Reason:          params.Reason,
			BeforeState:     string(beforeJSON),
			AfterState:      string(afterJSON),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	if err := PublishUserAuthCache(params.UserId); err != nil {
		return nil, fmt.Errorf("contract version %d committed but authentication cache publication failed: %w", nextVersion, err)
	}
	return GetCustomerContractSnapshot(params.UserId)
}

func customerContractAuditPricingFacts(userGroup string, rules []CustomerContractRule) []customerContractAuditPricingFact {
	facts := make([]customerContractAuditPricingFact, 0, len(rules))
	scale := decimal.NewFromInt(hosttypes.CustomerContractRatioScale)
	for _, rule := range rules {
		groupRatio := ratio_setting.GetGroupRatio(rule.RouteGroup)
		specialRatio, hasSpecialRatio := ratio_setting.GetGroupGroupRatio(userGroup, rule.RouteGroup)
		if hasSpecialRatio {
			groupRatio = specialRatio
		}
		discount := decimal.NewFromInt(rule.RatioUnits).Div(scale)
		nativeRatio := decimal.NewFromFloat(groupRatio)
		facts = append(facts, customerContractAuditPricingFact{
			PublicModel: rule.PublicModel, RouteGroup: rule.RouteGroup,
			ContractDiscount: discount.String(), NativeGroupRatio: nativeRatio.String(),
			EffectiveMultiplier: nativeRatio.Mul(discount).String(),
			SpecialGroupRatio:   hasSpecialRatio && specialRatio != 1,
		})
	}
	return facts
}

func GetCustomerContractAudits(userId int, offset int, limit int) ([]CustomerContractAudit, int64, error) {
	if userId <= 0 {
		return nil, 0, fmt.Errorf("invalid user id")
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := DB.Model(&CustomerContractAudit{}).Where("user_id = ?", userId).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var audits []CustomerContractAudit
	err := DB.Model(&CustomerContractAudit{}).
		Select("customer_contract_audits.*, COALESCE(users.username, '') AS admin_username").
		Joins("LEFT JOIN users ON users.id = customer_contract_audits.admin_user_id").
		Where("customer_contract_audits.user_id = ?", userId).
		Order("contract_version DESC").
		Offset(offset).
		Limit(limit).
		Find(&audits).Error
	if err != nil {
		return nil, 0, err
	}
	for i := range audits {
		var before customerContractAuditState
		var after customerContractAuditState
		if err := common.Unmarshal([]byte(audits[i].BeforeState), &before); err != nil {
			return nil, 0, err
		}
		if err := common.Unmarshal([]byte(audits[i].AfterState), &after); err != nil {
			return nil, 0, err
		}
		audits[i].BeforeEnabled = before.Enabled
		audits[i].AfterEnabled = after.Enabled
		audits[i].BeforeRuleCount = len(before.Rules)
		audits[i].AfterRuleCount = len(after.Rules)
	}
	return audits, total, err
}

func DeleteCurrentCustomerContractRulesWithTx(tx *gorm.DB, userId int) error {
	return tx.Unscoped().Where("user_id = ?", userId).Delete(&CustomerModelContract{}).Error
}

func normalizeCustomerContractRules(rules []CustomerContractRule) ([]CustomerContractRule, error) {
	normalized := make([]CustomerContractRule, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		rule.PublicModel = strings.TrimSpace(rule.PublicModel)
		rule.RouteGroup = strings.TrimSpace(rule.RouteGroup)
		rule.Available = false
		if rule.PublicModel == "" || len(rule.PublicModel) > 255 {
			return nil, fmt.Errorf("%w: public model is required and must not exceed 255 characters", ErrCustomerContractInvalidRule)
		}
		if rule.RouteGroup == "" || len(rule.RouteGroup) > 64 || strings.EqualFold(rule.RouteGroup, "auto") || NormalizeChannelGroupFilter(rule.RouteGroup) == "" {
			return nil, fmt.Errorf("%w: route group must be a concrete group", ErrCustomerContractInvalidRule)
		}
		if !ratio_setting.ContainsGroupRatio(rule.RouteGroup) {
			return nil, fmt.Errorf("%w: route group %q has no native ratio", ErrCustomerContractInvalidRule, rule.RouteGroup)
		}
		if rule.RatioUnits <= 0 || rule.RatioUnits > hosttypes.CustomerContractRatioScale {
			return nil, fmt.Errorf("%w: ratio must be greater than zero and no greater than one", ErrCustomerContractInvalidRule)
		}
		modelKey := strings.ToLower(rule.PublicModel)
		if _, exists := seen[modelKey]; exists {
			return nil, fmt.Errorf("%w: duplicate or case-only duplicate public model %q", ErrCustomerContractInvalidRule, rule.PublicModel)
		}
		seen[modelKey] = struct{}{}
		normalized = append(normalized, rule)
	}
	return normalized, nil
}

func loadCustomerContractRules(tx *gorm.DB, userId int, includeAvailability bool) ([]CustomerContractRule, error) {
	var rows []CustomerModelContract
	if err := tx.Where("user_id = ?", userId).Order("public_model").Find(&rows).Error; err != nil {
		return nil, err
	}
	rules := make([]CustomerContractRule, 0, len(rows))
	availableByGroup := make(map[string]map[string]struct{})
	for _, row := range rows {
		rule := CustomerContractRule{
			PublicModel: row.PublicModel,
			RouteGroup:  row.RouteGroup,
			RatioUnits:  row.RatioUnits,
		}
		if includeAvailability {
			availableModels, ok := availableByGroup[row.RouteGroup]
			if !ok {
				var err error
				availableModels, err = customerContractAvailableModelsForGroup(tx, row.RouteGroup)
				if err != nil {
					return nil, err
				}
				availableByGroup[row.RouteGroup] = availableModels
			}
			_, rule.Available = availableModels[row.PublicModel]
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func customerContractAvailableModelsForGroup(tx *gorm.DB, group string) (map[string]struct{}, error) {
	models := make(map[string]struct{})
	var abilities []Ability
	if err := tx.Where(&Ability{Group: group, Enabled: true}).Find(&abilities).Error; err != nil {
		return nil, err
	}
	channelIDs := make([]int, 0, len(abilities))
	for _, ability := range abilities {
		channelIDs = append(channelIDs, ability.ChannelId)
	}
	enabledChannels := make(map[int]struct{})
	if len(channelIDs) > 0 {
		var ids []int
		if err := tx.Model(&Channel{}).
			Where("id IN ? AND status = ?", channelIDs, common.ChannelStatusEnabled).
			Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		for _, id := range ids {
			enabledChannels[id] = struct{}{}
		}
	}
	for _, ability := range abilities {
		if _, enabled := enabledChannels[ability.ChannelId]; enabled {
			models[ability.Model] = struct{}{}
		}
	}
	var channels []Channel
	query := ApplyChannelGroupFilter(tx.Model(&Channel{}), group).
		Where("type = ? AND status = ?", constant.ChannelTypeSeedanceLink, common.ChannelStatusEnabled)
	if err := query.Find(&channels).Error; err != nil {
		return nil, err
	}
	for i := range channels {
		for _, modelName := range strings.Split(channels[i].Models, ",") {
			modelName = strings.TrimSpace(modelName)
			if modelName != "" {
				models[modelName] = struct{}{}
			}
		}
	}
	return models, nil
}

func validateCustomerContractRulesAvailable(tx *gorm.DB, rules []CustomerContractRule) error {
	availableByGroup := make(map[string]map[string]struct{})
	for _, rule := range rules {
		models, ok := availableByGroup[rule.RouteGroup]
		if !ok {
			var err error
			models, err = customerContractAvailableModelsForGroup(tx, rule.RouteGroup)
			if err != nil {
				return err
			}
			availableByGroup[rule.RouteGroup] = models
		}
		if _, available := models[rule.PublicModel]; !available {
			return fmt.Errorf("%w: model %q in group %q", ErrCustomerContractRuleUnavailable, rule.PublicModel, rule.RouteGroup)
		}
	}
	return nil
}

func GetCustomerContractAvailableModelsForGroup(group string) ([]string, error) {
	available, err := customerContractAvailableModelsForGroup(DB, group)
	if err != nil {
		return nil, err
	}
	models := make([]string, 0, len(available))
	for modelName := range available {
		models = append(models, modelName)
	}
	return models, nil
}

func IsChannelEnabledForExactCustomerContractModel(group string, publicModel string, channelID int) bool {
	if group == "" || publicModel == "" || channelID <= 0 {
		return false
	}
	var abilities []Ability
	if err := DB.Where(&Ability{Group: group, ChannelId: channelID, Enabled: true}).
		Find(&abilities).Error; err != nil {
		return false
	}
	for _, ability := range abilities {
		if ability.Model == publicModel {
			return true
		}
	}
	return false
}

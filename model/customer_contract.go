package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"

	"gorm.io/gorm"
)

var (
	ErrCustomerContractVersionConflict = errors.New("customer contract version conflict")
	ErrCustomerContractInvalidRule     = errors.New("invalid customer contract rule")
)

// CustomerModelContract is the legacy user-level contract rule table. It is
// retained for the admin contract list, the ratio-impact preview and the user
// rule-count summary; the request path resolves contract entities instead.
type CustomerModelContract struct {
	Id          int    `json:"id"`
	UserId      int    `json:"user_id" gorm:"index;uniqueIndex:idx_customer_model_contract_user_model"`
	PublicModel string `json:"public_model" gorm:"type:varchar(255);uniqueIndex:idx_customer_model_contract_user_model"`
	RouteGroup  string `json:"route_group" gorm:"type:varchar(64);index"`
	RatioUnits  int64  `json:"ratio_units" gorm:"type:bigint"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// CustomerContractAudit is the legacy user-level contract audit trail. The
// admin audits endpoint still serves it; contract entities keep their own
// append-only audit in customer_contract_entity.go.
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

// CustomerContractRule is the shared rule value type used by contract rule
// normalization and the legacy audit pricing facts.
type CustomerContractRule struct {
	PublicModel string `json:"public_model"`
	RouteGroup  string `json:"route_group"`
	RatioUnits  int64  `json:"ratio_units"`
	Available   bool   `json:"available"`
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
	return audits, total, nil
}

func DeleteCurrentCustomerContractRulesWithTx(tx *gorm.DB, userId int) error {
	return tx.Unscoped().Where("user_id = ?", userId).Delete(&CustomerModelContract{}).Error
}

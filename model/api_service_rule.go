package model

import (
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	APIServiceRuleDraft   = "draft"
	APIServiceRuleActive  = "active"
	APIServiceRuleRetired = "retired"
)

var (
	ErrAPIServiceRuleNotEffective        = errors.New("API service rule is not effective yet")
	ErrAPIServiceRuleAcceptanceImmutable = errors.New("API service rule acceptance is immutable")
)

// APIServiceRule is the single application-facing agreement accepted by an
// API client. Provider agreements and real-person H5 details are deliberately
// excluded from this Link contract record.
type APIServiceRule struct {
	ID            int64  `json:"id" gorm:"primaryKey"`
	Version       string `json:"version" gorm:"type:varchar(64);uniqueIndex"`
	Title         string `json:"title" gorm:"type:varchar(191)"`
	Content       string `json:"content" gorm:"type:text"`
	ContentSHA256 string `json:"content_sha256" gorm:"type:varchar(64)"`
	Status        string `json:"status" gorm:"type:varchar(32);index"`
	EffectiveAt   int64  `json:"effective_at" gorm:"bigint;index"`
	CreatedAt     int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt     int64  `json:"updated_at" gorm:"bigint"`
}

// ApplicationAPIRuleAcceptance binds one API token (the current app_id) to
// one immutable rule version. It never contains an end-user consent record.
type ApplicationAPIRuleAcceptance struct {
	ID                     int64  `json:"-" gorm:"primaryKey"`
	UserID                 int    `json:"-" gorm:"index"`
	AppID                  int    `json:"app_id" gorm:"uniqueIndex:idx_app_api_rule_acceptance;index"`
	RuleID                 int64  `json:"-" gorm:"uniqueIndex:idx_app_api_rule_acceptance"`
	RuleVersion            string `json:"rule_version" gorm:"type:varchar(64)"`
	ContentSHA256          string `json:"content_sha256" gorm:"type:varchar(64)"`
	AcceptedAt             int64  `json:"accepted_at" gorm:"bigint"`
	AcceptedBy             string `json:"accepted_by" gorm:"type:varchar(128)"`
	AcceptanceMethod       string `json:"acceptance_method" gorm:"type:varchar(32)"`
	ComplianceOwner        string `json:"compliance_owner" gorm:"type:varchar(191)"`
	ConsentRecordSystemRef string `json:"consent_record_system_ref" gorm:"type:varchar(300)"`
	CreatedAt              int64  `json:"created_at" gorm:"bigint"`
	UpdatedAt              int64  `json:"updated_at" gorm:"bigint"`
}

func (r *APIServiceRule) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if r.CreatedAt == 0 {
		r.CreatedAt = now
	}
	r.UpdatedAt = now
	if r.Status == "" {
		r.Status = APIServiceRuleDraft
	}
	return nil
}

func (a *ApplicationAPIRuleAcceptance) BeforeCreate(_ *gorm.DB) error {
	now := common.GetTimestamp()
	if a.CreatedAt == 0 {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	return nil
}

func (*ApplicationAPIRuleAcceptance) BeforeUpdate(_ *gorm.DB) error {
	return ErrAPIServiceRuleAcceptanceImmutable
}

func GetActiveAPIServiceRule() (*APIServiceRule, error) {
	var rule APIServiceRule
	err := DB.Where("status = ? AND effective_at <= ?", APIServiceRuleActive, common.GetTimestamp()).
		Order("effective_at desc, id desc").First(&rule).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &rule, err
}

func ActivateAPIServiceRule(id int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var rule APIServiceRule
		if err := lockForUpdate(tx).First(&rule, "id = ?", id).Error; err != nil {
			return err
		}
		now := common.GetTimestamp()
		if rule.EffectiveAt > now {
			return ErrAPIServiceRuleNotEffective
		}
		if err := tx.Model(&APIServiceRule{}).
			Where("status = ? AND id <> ?", APIServiceRuleActive, rule.ID).
			Updates(map[string]any{"status": APIServiceRuleRetired, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&rule).Updates(map[string]any{"status": APIServiceRuleActive, "updated_at": now}).Error
	})
}

func GetApplicationAPIRuleAcceptance(userID, appID int, ruleID int64) (*ApplicationAPIRuleAcceptance, error) {
	var acceptance ApplicationAPIRuleAcceptance
	err := DB.Where("user_id = ? AND app_id = ? AND rule_id = ?", userID, appID, ruleID).First(&acceptance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &acceptance, err
}

func SaveApplicationAPIRuleAcceptance(acceptance *ApplicationAPIRuleAcceptance) error {
	acceptance.ComplianceOwner = strings.TrimSpace(acceptance.ComplianceOwner)
	acceptance.ConsentRecordSystemRef = strings.TrimSpace(acceptance.ConsentRecordSystemRef)
	now := common.GetTimestamp()
	acceptance.UpdatedAt = now
	if acceptance.CreatedAt == 0 {
		acceptance.CreatedAt = now
	}
	result := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "app_id"}, {Name: "rule_id"}},
		DoNothing: true,
	}).Create(acceptance)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 1 {
		return nil
	}
	existing, err := GetApplicationAPIRuleAcceptance(acceptance.UserID, acceptance.AppID, acceptance.RuleID)
	if err != nil {
		return err
	}
	if existing == nil {
		return errors.New("API service rule acceptance conflict was not readable")
	}
	*acceptance = *existing
	return nil
}

func ListAPIServiceRules() ([]APIServiceRule, error) {
	var rules []APIServiceRule
	err := DB.Order("effective_at desc, id desc").Find(&rules).Error
	return rules, err
}

func ListApplicationAPIRuleAcceptances(appID, offset, limit int) ([]ApplicationAPIRuleAcceptance, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := DB.Model(&ApplicationAPIRuleAcceptance{})
	if appID > 0 {
		query = query.Where("app_id = ?", appID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var acceptances []ApplicationAPIRuleAcceptance
	err := query.Order("accepted_at desc, id desc").Offset(offset).Limit(limit).Find(&acceptances).Error
	return acceptances, total, err
}

package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

var (
	ErrCustomerContractTemplateNotFound        = errors.New("customer contract template not found")
	ErrCustomerContractTemplateDisabled        = errors.New("customer contract template is disabled and cannot create contracts")
	ErrCustomerContractTemplateVersionConflict = errors.New("customer contract template version conflict")
)

// CustomerContractTemplate is a site-wide shared template that administrators
// copy into a new user contract. Templates carry no customer attribution,
// never take part in runtime authorization, and have no delete path: they are
// only disabled and restored.
type CustomerContractTemplate struct {
	Id        int    `json:"id"`
	Name      string `json:"name" gorm:"type:varchar(128)"`
	Enabled   bool   `json:"enabled"`
	Version   int64  `json:"version" gorm:"type:bigint"`
	CreatorId int    `json:"creator_id" gorm:"index"`
	UpdaterId int    `json:"updater_id" gorm:"index"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// CustomerContractTemplateRule binds one public model inside one template to
// an explicit channel source, a management route group and an eight-decimal
// fixed point discount. Rule semantics are identical to contract entity
// rules; (template_id, public_model, channel_id) is unique within a template.
type CustomerContractTemplateRule struct {
	Id          int    `json:"id"`
	TemplateId  int    `json:"template_id" gorm:"index;uniqueIndex:idx_cct_rule_template_model_channel"`
	PublicModel string `json:"public_model" gorm:"type:varchar(255);uniqueIndex:idx_cct_rule_template_model_channel"`
	ChannelId   int    `json:"channel_id" gorm:"index;uniqueIndex:idx_cct_rule_template_model_channel"`
	RouteGroup  string `json:"route_group" gorm:"type:varchar(64);index"`
	RatioUnits  int64  `json:"ratio_units" gorm:"type:bigint"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

// CustomerContractTemplateAudit is the append-only audit trail for template
// writes. States are redacted JSON snapshots via common.Marshal.
type CustomerContractTemplateAudit struct {
	Id              int    `json:"id"`
	TemplateId      int    `json:"template_id" gorm:"index"`
	TemplateVersion int64  `json:"template_version" gorm:"type:bigint;index"`
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

// ContractTemplateSnapshot is one template definition plus its rules.
// Availability is derived from current channel/model facts and never persisted.
type ContractTemplateSnapshot struct {
	Id        int                  `json:"id"`
	Name      string               `json:"name"`
	Enabled   bool                 `json:"enabled"`
	Version   int64                `json:"version"`
	CreatorId int                  `json:"creator_id"`
	UpdaterId int                  `json:"updater_id"`
	Rules     []ContractEntityRule `json:"rules"`
}

type CustomerContractTemplateListItem struct {
	Id          int    `json:"id"`
	Name        string `json:"name"`
	Enabled     bool   `json:"enabled"`
	Version     int64  `json:"version"`
	ModelCount  int    `json:"model_count"`
	RuleCount   int    `json:"rule_count"`
	StaleCount  int    `json:"stale_rule_count"`
	CreatorId   int    `json:"creator_id"`
	UpdaterId   int    `json:"updater_id"`
	UpdaterName string `json:"updater_name"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
}

type CreateCustomerContractTemplateParams struct {
	AdminUserId int
	Name        string
	Enabled     bool
	Reason      string
	Rules       []CustomerContractEntityRuleInput
}

type ReplaceCustomerContractTemplateParams struct {
	TemplateId      int
	AdminUserId     int
	ExpectedVersion int64
	Name            string
	Enabled         *bool
	Reason          string
	Rules           []CustomerContractEntityRuleInput
}

type customerContractTemplateAuditState struct {
	Name    string               `json:"name"`
	Enabled bool                 `json:"enabled"`
	Version int64                `json:"version"`
	Rules   []ContractEntityRule `json:"rules"`
}

func validateCustomerContractTemplateWrite(name string, reason string, adminUserId int) error {
	if adminUserId <= 0 {
		return fmt.Errorf("invalid administrator")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return fmt.Errorf("template name is required and must not exceed 128 characters")
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len(reason) > 500 {
		return fmt.Errorf("template change reason is required and must not exceed 500 characters")
	}
	return nil
}

// validateCustomerContractTemplateNewSources validates only the channel
// references whose (public model, channel, route group) triple is not already
// stored for this template. It reuses the shared contract source policy;
// already accepted sources stay accepted when they later become unavailable,
// so stale rules never block editing or disabling the template.
func validateCustomerContractTemplateNewSources(tx *gorm.DB, templateId int, rules []CustomerContractEntityRuleInput) error {
	existing, err := loadCustomerContractTemplateRules(tx, templateId, false)
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

func saveCustomerContractTemplateRules(tx *gorm.DB, templateId int, rules []CustomerContractEntityRuleInput) error {
	if err := tx.Where("template_id = ?", templateId).Delete(&CustomerContractTemplateRule{}).Error; err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}
	rows := make([]CustomerContractTemplateRule, 0, len(rules))
	for _, rule := range rules {
		rows = append(rows, CustomerContractTemplateRule{
			TemplateId: templateId, PublicModel: rule.PublicModel,
			ChannelId: rule.ChannelId, RouteGroup: rule.RouteGroup, RatioUnits: rule.RatioUnits,
		})
	}
	return tx.Create(&rows).Error
}

func createCustomerContractTemplateAudit(tx *gorm.DB, template *CustomerContractTemplate, adminUserId int, operation string, reason string, before *customerContractTemplateAuditState, afterRules []ContractEntityRule) error {
	after := customerContractTemplateAuditState{Name: template.Name, Enabled: template.Enabled, Version: template.Version, Rules: afterRules}
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
	return tx.Create(&CustomerContractTemplateAudit{
		TemplateId: template.Id, TemplateVersion: template.Version,
		AdminUserId: adminUserId, Operation: operation, Reason: reason,
		BeforeState: string(beforeJSON), AfterState: string(afterJSON),
	}).Error
}

func CreateCustomerContractTemplate(params CreateCustomerContractTemplateParams) (*ContractTemplateSnapshot, error) {
	if err := validateCustomerContractTemplateWrite(params.Name, params.Reason, params.AdminUserId); err != nil {
		return nil, err
	}
	// Rule normalization and source qualification are the shared contract
	// semantics; templates never define their own looser variant.
	rules, err := normalizeCustomerContractEntityRules(params.Rules)
	if err != nil {
		return nil, err
	}
	if err := validateCustomerContractEntityRules(DB, rules); err != nil {
		return nil, err
	}
	template := &CustomerContractTemplate{
		Name: strings.TrimSpace(params.Name), Enabled: params.Enabled,
		Version: 1, CreatorId: params.AdminUserId, UpdaterId: params.AdminUserId,
	}
	var snapshot *ContractTemplateSnapshot
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(template).Error; err != nil {
			return err
		}
		if err := saveCustomerContractTemplateRules(tx, template.Id, rules); err != nil {
			return err
		}
		if err := createCustomerContractTemplateAudit(tx, template, params.AdminUserId, "create", strings.TrimSpace(params.Reason), nil, customerContractEntityRulesFromInputs(rules)); err != nil {
			return err
		}
		snapshot, err = getContractTemplateSnapshot(tx, template.Id, true)
		return err
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// ReplaceCustomerContractTemplate is the single authoritative template write.
// It commits expected_version fencing, the incremental source validation, the
// rule replacement, the version increment (including disable/restore) and the
// audit in one transaction.
func ReplaceCustomerContractTemplate(params ReplaceCustomerContractTemplateParams) (*ContractTemplateSnapshot, error) {
	if params.TemplateId <= 0 {
		return nil, fmt.Errorf("invalid template id")
	}
	if err := validateCustomerContractTemplateWrite(params.Name, params.Reason, params.AdminUserId); err != nil {
		return nil, err
	}
	rules, err := normalizeCustomerContractEntityRules(params.Rules)
	if err != nil {
		return nil, err
	}
	var nextVersion int64
	var snapshot *ContractTemplateSnapshot
	err = DB.Transaction(func(tx *gorm.DB) error {
		var template CustomerContractTemplate
		if err := lockForUpdate(tx).First(&template, params.TemplateId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrCustomerContractTemplateNotFound
			}
			return err
		}
		if template.Version != params.ExpectedVersion {
			return ErrCustomerContractTemplateVersionConflict
		}
		enabled := template.Enabled
		if params.Enabled != nil {
			enabled = *params.Enabled
		}
		if err := validateCustomerContractTemplateNewSources(tx, template.Id, rules); err != nil {
			return err
		}
		beforeRules, err := loadCustomerContractTemplateRules(tx, template.Id, false)
		if err != nil {
			return err
		}
		before := customerContractTemplateAuditState{Name: template.Name, Enabled: template.Enabled, Version: template.Version, Rules: beforeRules}
		nextVersion = template.Version + 1
		if err := tx.Model(&CustomerContractTemplate{}).Where("id = ?", template.Id).Updates(map[string]any{
			"name": strings.TrimSpace(params.Name), "enabled": enabled,
			"version": nextVersion, "updater_id": params.AdminUserId,
		}).Error; err != nil {
			return err
		}
		template.Name, template.Enabled, template.Version = strings.TrimSpace(params.Name), enabled, nextVersion
		template.UpdaterId = params.AdminUserId
		if err := saveCustomerContractTemplateRules(tx, template.Id, rules); err != nil {
			return err
		}
		operation := "update"
		if template.Enabled != before.Enabled {
			if template.Enabled {
				operation = "enable"
			} else {
				operation = "disable"
			}
		}
		if err := createCustomerContractTemplateAudit(tx, &template, params.AdminUserId, operation, strings.TrimSpace(params.Reason), &before, customerContractEntityRulesFromInputs(rules)); err != nil {
			return err
		}
		snapshot, err = getContractTemplateSnapshot(tx, template.Id, true)
		return err
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func loadCustomerContractTemplateRules(tx *gorm.DB, templateId int, includeAvailability bool) ([]ContractEntityRule, error) {
	var rows []CustomerContractTemplateRule
	if err := tx.Where("template_id = ?", templateId).Order("public_model").Find(&rows).Error; err != nil {
		return nil, err
	}
	rules := make([]ContractEntityRule, 0, len(rows))
	for _, row := range rows {
		rules = append(rules, ContractEntityRule{
			PublicModel: row.PublicModel, ChannelId: row.ChannelId,
			RouteGroup: row.RouteGroup, RatioUnits: row.RatioUnits,
		})
	}
	if includeAvailability {
		return readContractRouteAvailability(tx, rules)
	}
	return rules, nil
}

// GetContractTemplateSnapshot loads one template with its rules, optionally
// deriving current availability for the admin editor.
func GetContractTemplateSnapshot(templateId int, includeAvailability bool) (*ContractTemplateSnapshot, error) {
	return getContractTemplateSnapshot(DB, templateId, includeAvailability)
}

func getContractTemplateSnapshot(tx *gorm.DB, templateId int, includeAvailability bool) (*ContractTemplateSnapshot, error) {
	if templateId <= 0 {
		return nil, fmt.Errorf("invalid template id")
	}
	var template CustomerContractTemplate
	if err := tx.First(&template, templateId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomerContractTemplateNotFound
		}
		return nil, err
	}
	rules, err := loadCustomerContractTemplateRules(tx, templateId, includeAvailability)
	if err != nil {
		return nil, err
	}
	return &ContractTemplateSnapshot{
		Id: template.Id, Name: template.Name, Enabled: template.Enabled,
		Version: template.Version, CreatorId: template.CreatorId,
		UpdaterId: template.UpdaterId, Rules: rules,
	}, nil
}

// checkCustomerContractTemplateSource runs inside the contract creation
// transaction: it locks the template row and verifies the template still
// exists, is enabled and matches the version the administrator confirmed.
func checkCustomerContractTemplateSource(tx *gorm.DB, templateId int, expectedVersion int64) (*CustomerContractTemplate, error) {
	var template CustomerContractTemplate
	if err := lockForUpdate(tx).First(&template, templateId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCustomerContractTemplateNotFound
		}
		return nil, err
	}
	if !template.Enabled {
		return nil, ErrCustomerContractTemplateDisabled
	}
	if template.Version != expectedVersion {
		return nil, ErrCustomerContractTemplateVersionConflict
	}
	return &template, nil
}

// GetCustomerContractTemplateList returns one admin template page with
// derived model/rule/stale counts. Availability comes from current channel
// and group facts; a read failure fails the whole page instead of hiding
// stale rules behind an empty list.
func GetCustomerContractTemplateList(keyword string, enabled *bool, offset int, limit int) ([]CustomerContractTemplateListItem, int64, error) {
	keyword = strings.TrimSpace(keyword)
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	applyFilters := func(query *gorm.DB) *gorm.DB {
		if keyword != "" {
			query = query.Where("LOWER(name) LIKE ?", "%"+strings.ToLower(keyword)+"%")
		}
		if enabled != nil {
			query = query.Where("enabled = ?", *enabled)
		}
		return query
	}
	var total int64
	if err := applyFilters(DB.Model(&CustomerContractTemplate{})).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var templates []CustomerContractTemplate
	if err := applyFilters(DB.Model(&CustomerContractTemplate{})).Order("id DESC").Offset(offset).Limit(limit).Find(&templates).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]int, 0, len(templates))
	updaterIds := make([]int, 0, len(templates))
	for _, template := range templates {
		ids = append(ids, template.Id)
		updaterIds = append(updaterIds, template.UpdaterId)
	}
	ruleRows := make([]CustomerContractTemplateRule, 0)
	if len(ids) > 0 {
		if err := DB.Where("template_id IN ?", ids).Find(&ruleRows).Error; err != nil {
			return nil, 0, err
		}
	}
	updaterNames := make(map[int]string, len(updaterIds))
	if len(updaterIds) > 0 {
		var users []User
		if err := DB.Select("id", "username").Where("id IN ?", updaterIds).Find(&users).Error; err != nil {
			return nil, 0, err
		}
		for _, user := range users {
			updaterNames[user.Id] = user.Username
		}
	}
	rulesByTemplate := make(map[int][]ContractEntityRule, len(templates))
	for _, row := range ruleRows {
		rulesByTemplate[row.TemplateId] = append(rulesByTemplate[row.TemplateId], ContractEntityRule{
			PublicModel: row.PublicModel, ChannelId: row.ChannelId,
			RouteGroup: row.RouteGroup, RatioUnits: row.RatioUnits,
		})
	}
	items := make([]CustomerContractTemplateListItem, 0, len(templates))
	for _, template := range templates {
		rules := rulesByTemplate[template.Id]
		modelNames := make(map[string]struct{}, len(rules))
		for _, rule := range rules {
			modelNames[strings.ToLower(rule.PublicModel)] = struct{}{}
		}
		staleCount := 0
		if len(rules) > 0 {
			available, err := readContractRouteAvailability(DB, rules)
			if err != nil {
				return nil, 0, err
			}
			for _, rule := range available {
				if !rule.Available {
					staleCount++
				}
			}
		}
		items = append(items, CustomerContractTemplateListItem{
			Id: template.Id, Name: template.Name, Enabled: template.Enabled,
			Version: template.Version, ModelCount: len(modelNames), RuleCount: len(rules),
			StaleCount: staleCount, CreatorId: template.CreatorId, UpdaterId: template.UpdaterId,
			UpdaterName: updaterNames[template.UpdaterId], CreatedAt: template.CreatedAt, UpdatedAt: template.UpdatedAt,
		})
	}
	return items, total, nil
}

// GetContractTemplateAudits returns one template's audit page. Counts are
// derived from the redacted JSON snapshots.
func GetContractTemplateAudits(templateId int, offset int, limit int) ([]CustomerContractTemplateAudit, int64, error) {
	if templateId <= 0 {
		return nil, 0, fmt.Errorf("invalid template id")
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := DB.Model(&CustomerContractTemplateAudit{}).Where("template_id = ?", templateId).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var audits []CustomerContractTemplateAudit
	err := DB.Model(&CustomerContractTemplateAudit{}).
		Select("customer_contract_template_audits.*, COALESCE(users.username, '') AS admin_username").
		Joins("LEFT JOIN users ON users.id = customer_contract_template_audits.admin_user_id").
		Where("customer_contract_template_audits.template_id = ?", templateId).
		Order("id DESC").
		Offset(offset).
		Limit(limit).
		Find(&audits).Error
	if err != nil {
		return nil, 0, err
	}
	for i := range audits {
		var before, after customerContractTemplateAuditState
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

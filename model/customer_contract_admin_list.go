package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	CustomerContractAdminStatusActive     = "active"
	CustomerContractAdminStatusZeroAccess = "zero_access"
	CustomerContractAdminStatusInactive   = "inactive"
)

type CustomerContractAdminListFilter struct {
	AdminRole int
	Keyword   string
	Status    string
	Offset    int
	Limit     int
}

// CustomerContractAdminListItem is one contract entity in the admin overview.
// The overview is a read-only projection; writes only happen through the
// entity replace endpoint.
type CustomerContractAdminListItem struct {
	ContractId           int    `json:"contract_id" gorm:"column:contract_id"`
	ContractName         string `json:"contract_name" gorm:"column:contract_name"`
	UserId               int    `json:"user_id" gorm:"column:user_id"`
	Username             string `json:"username"`
	DisplayName          string `json:"display_name" gorm:"column:display_name"`
	ContractEnabled      bool   `json:"contract_enabled" gorm:"column:contract_enabled"`
	ContractStatus       string `json:"contract_status" gorm:"-"`
	ContractVersion      int64  `json:"contract_version" gorm:"column:contract_version"`
	RuleCount            int    `json:"rule_count" gorm:"column:rule_count"`
	UnavailableRuleCount int    `json:"unavailable_rule_count" gorm:"-"`
	BoundTokenCount      int    `json:"bound_token_count" gorm:"column:bound_token_count"`
	UpdatedAt            int64  `json:"updated_at" gorm:"column:updated_at"`
	AdminUserId          int    `json:"admin_user_id" gorm:"column:admin_user_id"`
	AdminUsername        string `json:"admin_username" gorm:"column:admin_username"`
}

type CustomerContractAdminSummary struct {
	Total      int64 `json:"total"`
	Active     int64 `json:"active"`
	ZeroAccess int64 `json:"zero_access"`
	Inactive   int64 `json:"inactive"`
}

func IsCustomerContractAdminStatus(value string) bool {
	switch value {
	case "", CustomerContractAdminStatusActive, CustomerContractAdminStatusZeroAccess, CustomerContractAdminStatusInactive:
		return true
	default:
		return false
	}
}

func GetCustomerContractAdminList(filter CustomerContractAdminListFilter) ([]CustomerContractAdminListItem, int64, CustomerContractAdminSummary, error) {
	if !IsCustomerContractAdminStatus(filter.Status) {
		return nil, 0, CustomerContractAdminSummary{}, fmt.Errorf("invalid customer contract status")
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = common.ItemsPerPage
	}

	ruleCounts := DB.Model(&CustomerContractEntityRule{}).
		Select("contract_id, COUNT(*) AS rule_count").
		Group("contract_id")

	query := customerContractAdminBaseQuery(filter.AdminRole, ruleCounts)
	query = applyCustomerContractAdminKeyword(query, filter.Keyword)
	query = applyCustomerContractAdminStatus(query, filter.Status)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, CustomerContractAdminSummary{}, err
	}

	var items []CustomerContractAdminListItem
	err := query.
		Select(`customer_contracts.id AS contract_id, customer_contracts.name AS contract_name,
			customer_contracts.user_id, customer_contracts.enabled AS contract_enabled,
			customer_contracts.version AS contract_version, customer_contracts.updated_at AS updated_at,
			users.username, users.display_name,
			COALESCE(contract_rule_counts.rule_count, 0) AS rule_count,
			(SELECT COUNT(*) FROM tokens WHERE tokens.contract_id = customer_contracts.id AND tokens.deleted_at IS NULL) AS bound_token_count,
			COALESCE(current_contract_audit.admin_user_id, 0) AS admin_user_id,
			COALESCE(contract_admin.username, '') AS admin_username`).
		Joins(`LEFT JOIN customer_contract_entity_audits AS current_contract_audit
			ON current_contract_audit.contract_id = customer_contracts.id
			AND current_contract_audit.contract_version = customer_contracts.version`).
		Joins("LEFT JOIN users AS contract_admin ON contract_admin.id = current_contract_audit.admin_user_id").
		Order("customer_contracts.updated_at DESC").
		Order("customer_contracts.id DESC").
		Offset(filter.Offset).
		Limit(filter.Limit).
		Scan(&items).Error
	if err != nil {
		return nil, 0, CustomerContractAdminSummary{}, err
	}
	for i := range items {
		items[i].ContractStatus = customerContractAdminStatus(items[i].ContractEnabled, items[i].RuleCount)
	}
	if err := populateCustomerContractAdminAvailability(items); err != nil {
		return nil, 0, CustomerContractAdminSummary{}, err
	}

	summary, err := getCustomerContractAdminSummary(filter.AdminRole, ruleCounts)
	if err != nil {
		return nil, 0, CustomerContractAdminSummary{}, err
	}
	return items, total, summary, nil
}

func customerContractAdminBaseQuery(adminRole int, ruleCounts *gorm.DB) *gorm.DB {
	query := DB.Model(&CustomerContract{}).
		Joins("JOIN users ON users.id = customer_contracts.user_id AND users.deleted_at IS NULL").
		Joins("LEFT JOIN (?) AS contract_rule_counts ON contract_rule_counts.contract_id = customer_contracts.id", ruleCounts)
	if adminRole != common.RoleRootUser {
		query = query.Where("users.role < ?", adminRole)
	}
	return query
}

func applyCustomerContractAdminKeyword(query *gorm.DB, keyword string) *gorm.DB {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return query
	}
	pattern := "%" + strings.ToLower(keyword) + "%"
	condition := `LOWER(users.username) LIKE ? OR LOWER(users.display_name) LIKE ?
		OR LOWER(customer_contracts.name) LIKE ? OR EXISTS (
		SELECT 1 FROM customer_contract_entity_rules AS searched_contract_rule
		WHERE searched_contract_rule.contract_id = customer_contracts.id
		AND LOWER(searched_contract_rule.public_model) LIKE ?
	)`
	args := []any{pattern, pattern, pattern, pattern}
	if userId, err := strconv.Atoi(keyword); err == nil {
		condition = "customer_contracts.user_id = ? OR " + condition
		args = append([]any{userId}, args...)
	}
	return query.Where("("+condition+")", args...)
}

func applyCustomerContractAdminStatus(query *gorm.DB, status string) *gorm.DB {
	switch status {
	case CustomerContractAdminStatusActive:
		return query.Where("customer_contracts.enabled = ?", true).
			Where("COALESCE(contract_rule_counts.rule_count, 0) > 0")
	case CustomerContractAdminStatusZeroAccess:
		return query.Where("customer_contracts.enabled = ?", true).
			Where("COALESCE(contract_rule_counts.rule_count, 0) = 0")
	case CustomerContractAdminStatusInactive:
		return query.Where("customer_contracts.enabled = ?", false)
	default:
		return query
	}
}

func customerContractAdminStatus(enabled bool, ruleCount int) string {
	if !enabled {
		return CustomerContractAdminStatusInactive
	}
	if ruleCount == 0 {
		return CustomerContractAdminStatusZeroAccess
	}
	return CustomerContractAdminStatusActive
}

func populateCustomerContractAdminAvailability(items []CustomerContractAdminListItem) error {
	if len(items) == 0 {
		return nil
	}
	contractIds := make([]int, 0, len(items))
	itemIndex := make(map[int]int, len(items))
	for i := range items {
		contractIds = append(contractIds, items[i].ContractId)
		itemIndex[items[i].ContractId] = i
	}

	var rules []CustomerContractEntityRule
	if err := DB.Select("contract_id", "public_model", "route_group", "channel_id").
		Where("contract_id IN ?", contractIds).
		Find(&rules).Error; err != nil {
		return err
	}
	for _, rule := range rules {
		if validateCustomerContractEntityChannel(DB, rule.ChannelId, rule.RouteGroup, rule.PublicModel) == nil {
			continue
		}
		if index, ok := itemIndex[rule.ContractId]; ok {
			items[index].UnavailableRuleCount++
		}
	}
	return nil
}

func getCustomerContractAdminSummary(adminRole int, ruleCounts *gorm.DB) (CustomerContractAdminSummary, error) {
	var summary CustomerContractAdminSummary
	counts := []struct {
		target *int64
		status string
	}{
		{target: &summary.Total},
		{target: &summary.Active, status: CustomerContractAdminStatusActive},
		{target: &summary.ZeroAccess, status: CustomerContractAdminStatusZeroAccess},
		{target: &summary.Inactive, status: CustomerContractAdminStatusInactive},
	}
	for _, count := range counts {
		query := customerContractAdminBaseQuery(adminRole, ruleCounts)
		query = applyCustomerContractAdminStatus(query, count.status)
		if err := query.Count(count.target).Error; err != nil {
			return CustomerContractAdminSummary{}, err
		}
	}
	return summary, nil
}

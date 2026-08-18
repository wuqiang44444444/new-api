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

type CustomerContractAdminListItem struct {
	UserId               int    `json:"user_id" gorm:"column:user_id"`
	Username             string `json:"username"`
	DisplayName          string `json:"display_name" gorm:"column:display_name"`
	ContractMode         bool   `json:"contract_mode" gorm:"column:contract_mode"`
	ContractStatus       string `json:"contract_status" gorm:"-"`
	ContractVersion      int64  `json:"contract_version" gorm:"column:contract_version"`
	RuleCount            int    `json:"rule_count" gorm:"column:rule_count"`
	UnavailableRuleCount int    `json:"unavailable_rule_count" gorm:"-"`
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

	ruleCounts := DB.Model(&CustomerModelContract{}).
		Select("user_id, COUNT(*) AS rule_count").
		Group("user_id")

	query := customerContractAdminBaseQuery(filter.AdminRole, ruleCounts)
	query = applyCustomerContractAdminKeyword(query, filter.Keyword)
	query = applyCustomerContractAdminStatus(query, filter.Status)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, CustomerContractAdminSummary{}, err
	}

	var items []CustomerContractAdminListItem
	err := query.
		Select(`users.id AS user_id, users.username, users.display_name,
			users.contract_mode, users.contract_version,
			COALESCE(contract_rule_counts.rule_count, 0) AS rule_count,
			COALESCE(current_contract_audit.created_at, 0) AS updated_at,
			COALESCE(current_contract_audit.admin_user_id, 0) AS admin_user_id,
			COALESCE(contract_admin.username, '') AS admin_username`).
		Joins(`LEFT JOIN customer_contract_audits AS current_contract_audit
			ON current_contract_audit.user_id = users.id
			AND current_contract_audit.contract_version = users.contract_version`).
		Joins("LEFT JOIN users AS contract_admin ON contract_admin.id = current_contract_audit.admin_user_id").
		Order("updated_at DESC").
		Order("users.id DESC").
		Offset(filter.Offset).
		Limit(filter.Limit).
		Scan(&items).Error
	if err != nil {
		return nil, 0, CustomerContractAdminSummary{}, err
	}
	for i := range items {
		items[i].ContractStatus = customerContractAdminStatus(items[i].ContractMode, items[i].RuleCount)
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
	query := DB.Model(&User{}).
		Joins("LEFT JOIN (?) AS contract_rule_counts ON contract_rule_counts.user_id = users.id", ruleCounts).
		Where("users.contract_version > ?", 0)
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
	condition := `LOWER(users.username) LIKE ? OR LOWER(users.display_name) LIKE ? OR EXISTS (
		SELECT 1 FROM customer_model_contracts AS searched_contract_rule
		WHERE searched_contract_rule.user_id = users.id
		AND LOWER(searched_contract_rule.public_model) LIKE ?
	)`
	args := []any{pattern, pattern, pattern}
	if userId, err := strconv.Atoi(keyword); err == nil {
		condition = "users.id = ? OR " + condition
		args = append([]any{userId}, args...)
	}
	return query.Where("("+condition+")", args...)
}

func applyCustomerContractAdminStatus(query *gorm.DB, status string) *gorm.DB {
	switch status {
	case CustomerContractAdminStatusActive:
		return query.Where("users.contract_mode = ?", true).
			Where("COALESCE(contract_rule_counts.rule_count, 0) > 0")
	case CustomerContractAdminStatusZeroAccess:
		return query.Where("users.contract_mode = ?", true).
			Where("COALESCE(contract_rule_counts.rule_count, 0) = 0")
	case CustomerContractAdminStatusInactive:
		return query.Where("users.contract_mode = ?", false)
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
	userIds := make([]int, 0, len(items))
	itemIndex := make(map[int]int, len(items))
	for i := range items {
		userIds = append(userIds, items[i].UserId)
		itemIndex[items[i].UserId] = i
	}

	var rules []CustomerModelContract
	if err := DB.Select("user_id", "public_model", "route_group").
		Where("user_id IN ?", userIds).
		Find(&rules).Error; err != nil {
		return err
	}
	availableByGroup := make(map[string]map[string]struct{})
	for _, rule := range rules {
		availableModels, ok := availableByGroup[rule.RouteGroup]
		if !ok {
			var err error
			availableModels, err = customerContractAvailableModelsForGroup(DB, rule.RouteGroup)
			if err != nil {
				return err
			}
			availableByGroup[rule.RouteGroup] = availableModels
		}
		if _, available := availableModels[rule.PublicModel]; available {
			continue
		}
		if index, ok := itemIndex[rule.UserId]; ok {
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

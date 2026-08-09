package model

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

type BillingCustomerStatementListItem struct {
	UserId         int                               `json:"user_id"`
	Username       string                            `json:"username"`
	DisplayName    string                            `json:"display_name"`
	Deleted        bool                              `json:"deleted,omitempty"`
	Usage          BillingReconciliationUsage        `json:"usage"`
	OriginalQuota  *int64                            `json:"original_quota,omitempty"`
	DiscountQuota  *int64                            `json:"discount_quota,omitempty"`
	DataQuality    *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	LastActivityAt int64                             `json:"last_activity_at"`
}

type BillingReconciliationUserIdentity struct {
	Id             int    `json:"id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	Deleted        bool   `json:"deleted,omitempty"`
	CurrentBalance int    `json:"current_balance"`
}

type BillingCustomerStatementListSummary struct {
	CustomerCount int64                             `json:"customer_count"`
	Usage         BillingReconciliationUsage        `json:"usage"`
	OriginalQuota *int64                            `json:"original_quota,omitempty"`
	DiscountQuota *int64                            `json:"discount_quota,omitempty"`
	DataQuality   *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
}

type BillingCustomerStatementList struct {
	Summary   BillingCustomerStatementListSummary `json:"summary"`
	Items     []BillingCustomerStatementListItem  `json:"items"`
	Page      int                                 `json:"page"`
	PageSize  int                                 `json:"page_size"`
	Total     int64                               `json:"total"`
	SortBy    string                              `json:"sort_by"`
	SortOrder string                              `json:"sort_order"`
}

type billingCustomerStatementListAccumulator struct {
	item  BillingCustomerStatementListItem
	price billingReconciliationModelAccumulator
}

func GetBillingReconciliationUserById(userId int) (BillingReconciliationUserIdentity, error) {
	var user struct {
		Id          int
		Username    string
		DisplayName string
		Quota       int
		DeletedAt   gorm.DeletedAt
	}
	err := DB.Unscoped().Model(&User{}).
		Select("id, username, display_name, quota, deleted_at").
		Where("id = ?", userId).
		Take(&user).Error
	return BillingReconciliationUserIdentity{
		Id:             user.Id,
		Username:       user.Username,
		DisplayName:    user.DisplayName,
		Deleted:        user.DeletedAt.Valid,
		CurrentBalance: user.Quota,
	}, err
}

func GetBillingCustomerStatementList(
	startTimestamp int64,
	endTimestamp int64,
	search string,
	qualityStatus string,
	sortBy string,
	sortOrder string,
	page int,
	pageSize int,
) (BillingCustomerStatementList, error) {
	result := BillingCustomerStatementList{
		Items:     make([]BillingCustomerStatementListItem, 0),
		Page:      page,
		PageSize:  pageSize,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}

	rows, err := LOG_DB.Model(&Log{}).
		Select("user_id, type, created_at, prompt_tokens, completion_tokens, quota, COALESCE(other, '') AS other").
		Where("type IN ? AND created_at >= ? AND created_at <= ?", []int{LogTypeConsume, LogTypeRefund}, startTimestamp, endTimestamp).
		Rows()
	if err != nil {
		return result, err
	}
	defer rows.Close()

	accumulators := make(map[int]*billingCustomerStatementListAccumulator)
	for rows.Next() {
		var log billingReconciliationLog
		if err := rows.Scan(&log.UserId, &log.Type, &log.CreatedAt, &log.PromptTokens, &log.CompletionTokens, &log.Quota, &log.Other); err != nil {
			return result, err
		}
		accumulator, ok := accumulators[log.UserId]
		if !ok {
			accumulator = &billingCustomerStatementListAccumulator{
				item: BillingCustomerStatementListItem{UserId: log.UserId},
				price: billingReconciliationModelAccumulator{
					model:                 BillingReconciliationModelSummary{},
					originalQuotaComplete: true,
					priceSnapshotMarkers:  make(map[string]struct{}),
				},
			}
			accumulators[log.UserId] = accumulator
		}

		parsed := parseBillingReconciliationLog(log)
		accumulateBillingReconciliationLog(&accumulator.item.Usage, log, parsed)
		if parsed.unavailable {
			ensureBillingReconciliationQuality(&accumulator.price.model.DataQuality).UnavailableRequests++
		}
		if parsed.billingMode == BillingReconciliationModeUnknown {
			ensureBillingReconciliationQuality(&accumulator.price.model.DataQuality).UnknownBillingModeRequests++
		}
		accumulateBillingReconciliationPrice(&accumulator.price, log, parsed)
		if log.CreatedAt > accumulator.item.LastActivityAt {
			accumulator.item.LastActivityAt = log.CreatedAt
		}
	}
	if err := rows.Err(); err != nil {
		return result, err
	}

	userIds := make([]int, 0, len(accumulators))
	for userId := range accumulators {
		userIds = append(userIds, userId)
	}
	if len(userIds) > 0 {
		var users []struct {
			Id          int
			Username    string
			DisplayName string
			DeletedAt   gorm.DeletedAt
		}
		if err := DB.Unscoped().Model(&User{}).
			Select("id, username, display_name, deleted_at").
			Where("id IN ?", userIds).
			Scan(&users).Error; err != nil {
			return result, err
		}
		for _, user := range users {
			accumulator := accumulators[user.Id]
			if accumulator == nil {
				continue
			}
			accumulator.item.Username = user.Username
			accumulator.item.DisplayName = user.DisplayName
			accumulator.item.Deleted = user.DeletedAt.Valid
		}
	}

	search = strings.ToLower(strings.TrimSpace(search))
	items := make([]BillingCustomerStatementListItem, 0, len(accumulators))
	for _, accumulator := range accumulators {
		if strings.TrimSpace(accumulator.item.Username) == "" {
			accumulator.item.Username = fmt.Sprintf("User #%d", accumulator.item.UserId)
			accumulator.item.Deleted = true
		}
		finalizeBillingReconciliationUsage(&accumulator.item.Usage)
		finalizeBillingReconciliationPrice(&accumulator.price)
		accumulator.item.OriginalQuota = accumulator.price.model.OriginalQuota
		if accumulator.item.Usage.GrossQuota == 0 && accumulator.item.OriginalQuota == nil {
			zero := int64(0)
			accumulator.item.OriginalQuota = &zero
		}
		if accumulator.item.OriginalQuota != nil {
			discountQuota := *accumulator.item.OriginalQuota - accumulator.item.Usage.GrossQuota
			accumulator.item.DiscountQuota = &discountQuota
		}
		accumulator.item.DataQuality = accumulator.price.model.DataQuality
		finalizeBillingReconciliationQuality(&accumulator.item.DataQuality)

		if search != "" {
			identity := strings.ToLower(strings.Join([]string{
				accumulator.item.Username,
				accumulator.item.DisplayName,
				strconv.Itoa(accumulator.item.UserId),
			}, " "))
			if !strings.Contains(identity, search) {
				continue
			}
		}
		if qualityStatus != "" && accumulator.item.DataQuality.Status != qualityStatus {
			continue
		}
		items = append(items, accumulator.item)
	}

	sortBillingCustomerStatementList(items, sortBy, sortOrder)
	result.Total = int64(len(items))
	result.Summary = summarizeBillingCustomerStatementList(items)

	start := (page - 1) * pageSize
	if start >= len(items) {
		return result, nil
	}
	end := min(start+pageSize, len(items))
	result.Items = append(result.Items, items[start:end]...)
	return result, nil
}

func summarizeBillingCustomerStatementList(items []BillingCustomerStatementListItem) BillingCustomerStatementListSummary {
	summary := BillingCustomerStatementListSummary{CustomerCount: int64(len(items))}
	originalQuota := int64(0)
	originalQuotaComplete := true
	for _, item := range items {
		accumulateBillingReconciliationUsage(&summary.Usage, item.Usage)
		accumulateBillingReconciliationQuality(&summary.DataQuality, item.DataQuality)
		if item.Usage.GrossQuota > 0 && item.OriginalQuota == nil {
			originalQuotaComplete = false
		} else if item.OriginalQuota != nil {
			originalQuota += *item.OriginalQuota
		}
	}
	finalizeBillingReconciliationUsage(&summary.Usage)
	finalizeBillingReconciliationQuality(&summary.DataQuality)
	if originalQuotaComplete {
		summary.OriginalQuota = &originalQuota
		discountQuota := originalQuota - summary.Usage.GrossQuota
		summary.DiscountQuota = &discountQuota
	}
	return summary
}

func sortBillingCustomerStatementList(items []BillingCustomerStatementListItem, sortBy string, sortOrder string) {
	descending := sortOrder != "asc"
	sort.SliceStable(items, func(i, j int) bool {
		comparison := 0
		switch sortBy {
		case "requests":
			comparison = compareInt64(items[i].Usage.Requests, items[j].Usage.Requests)
		case "original_quota":
			if items[i].OriginalQuota == nil || items[j].OriginalQuota == nil {
				if items[i].OriginalQuota == nil && items[j].OriginalQuota == nil {
					return items[i].UserId < items[j].UserId
				}
				return items[i].OriginalQuota != nil
			}
			comparison = compareInt64(*items[i].OriginalQuota, *items[j].OriginalQuota)
		case "username":
			comparison = strings.Compare(strings.ToLower(items[i].Username), strings.ToLower(items[j].Username))
		default:
			comparison = compareInt64(items[i].Usage.NetQuota, items[j].Usage.NetQuota)
		}
		if comparison == 0 {
			return items[i].UserId < items[j].UserId
		}
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
}

func compareInt64(left int64, right int64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

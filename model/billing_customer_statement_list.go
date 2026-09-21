package model

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type BillingCustomerStatementListItem struct {
	MoneyUSD       *BillingStatementListMoney        `json:"money_usd,omitempty"`
	UserId         int                               `json:"user_id"`
	Username       string                            `json:"username"`
	DisplayName    string                            `json:"display_name"`
	Deleted        bool                              `json:"deleted,omitempty"`
	Usage          BillingReconciliationUsage        `json:"usage"`
	OriginalQuota  *int64                            `json:"original_quota,omitempty"`
	DiscountQuota  *int64                            `json:"discount_quota,omitempty"`
	DataQuality    *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	LastActivityAt int64                             `json:"last_activity_at"`
	// BillingVersion 标注该客户月已有已确认版本：金额列已切换为确认版冻结金额（方案 6.3）。
	BillingVersion *BillingCustomerStatementListVersion `json:"billing_version,omitempty"`
	originalQuota  decimal.Decimal
	quotaPerUnit   float64
}

// BillingCustomerStatementListVersion 是列表行的确认版本标注。
type BillingCustomerStatementListVersion struct {
	VersionNumber *int  `json:"version_number"`
	ConfirmedAt   int64 `json:"confirmed_at"`
}

type BillingReconciliationUserIdentity struct {
	Id             int    `json:"id"`
	Username       string `json:"username"`
	DisplayName    string `json:"display_name"`
	Deleted        bool   `json:"deleted,omitempty"`
	CurrentBalance *int   `json:"current_balance"`
}

type BillingCustomerStatementListSummary struct {
	MoneyUSD      *BillingStatementListMoney        `json:"money_usd,omitempty"`
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
	if errors.Is(err, gorm.ErrRecordNotFound) {
		var history struct{ UserId int }
		if err := LOG_DB.Model(&Log{}).Scopes(customerSettlementLogs).
			Select("user_id").Where("user_id = ? AND type IN ?", userId, []int{LogTypeConsume, LogTypeRefund}).
			Take(&history).Error; err != nil {
			return BillingReconciliationUserIdentity{}, err
		}
		return BillingReconciliationUserIdentity{Id: userId, Username: fmt.Sprintf("User #%d", userId), Deleted: true}, nil
	}
	if err != nil {
		return BillingReconciliationUserIdentity{}, err
	}
	return BillingReconciliationUserIdentity{
		Id:             user.Id,
		Username:       user.Username,
		DisplayName:    user.DisplayName,
		Deleted:        user.DeletedAt.Valid,
		CurrentBalance: &user.Quota,
	}, nil
}

func GetBillingCustomerStatementList(ctx context.Context,
	startTimestamp int64,
	endTimestamp int64,
	search string,
	qualityStatus string,
	sortBy string,
	sortOrder string,
	page int,
	pageSize int,
	frozen ...BillingCustomerStatementListItem,
) (BillingCustomerStatementList, error) {
	result := BillingCustomerStatementList{
		Items:     make([]BillingCustomerStatementListItem, 0),
		Page:      page,
		PageSize:  pageSize,
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}

	query := LOG_DB.WithContext(ctx).Model(&Log{}).
		Scopes(customerSettlementLogs).
		Select("user_id, token_id, channel_id, COALESCE(model_name, '') AS model_name, type, created_at, prompt_tokens, completion_tokens, quota, COALESCE(other, '') AS other").
		Where("type IN ? AND created_at >= ? AND created_at <= ?", []int{LogTypeConsume, LogTypeRefund}, startTimestamp, endTimestamp)
	frozenItems := make(map[int]BillingCustomerStatementListItem, len(frozen))
	frozenIDs := make([]int, 0, len(frozen))
	for _, item := range frozen {
		frozenItems[item.UserId] = item
		frozenIDs = append(frozenIDs, item.UserId)
	}
	// Keep SQL parameters bounded. For large frozen sets the existing map
	// excludes rows during scanning, including split-database deployments.
	if len(frozenIDs) > 0 && len(frozenIDs) <= 500 {
		query = query.Where("user_id NOT IN ?", frozenIDs)
	}
	refundEvidence, err := loadBillingStatementRefundEvidence(ctx, query)
	if err != nil {
		return result, err
	}
	rows, err := query.Rows()
	if err != nil {
		return result, err
	}
	defer rows.Close()

	accumulators := make(map[int]*billingCustomerStatementListAccumulator)
	for rows.Next() {
		var log billingReconciliationLog
		if err := rows.Scan(&log.UserId, &log.TokenId, &log.ChannelId, &log.ModelName, &log.Type, &log.CreatedAt, &log.PromptTokens, &log.CompletionTokens, &log.Quota, &log.Other); err != nil {
			return result, err
		}
		if _, frozen := frozenItems[log.UserId]; frozen {
			continue
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
		refundEvidence.apply(log, &parsed)
		accumulateBillingReconciliationLog(&accumulator.item.Usage, log, parsed)
		if parsed.inputTokensUnavailable {
			ensureBillingReconciliationQuality(&accumulator.price.model.DataQuality).InputTokensUnavailableRequests++
		}
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

	for userID, item := range frozenItems {
		accumulators[userID] = &billingCustomerStatementListAccumulator{item: item}
	}
	userIds := make([]int, 0, len(accumulators))
	for userId := range accumulators {
		userIds = append(userIds, userId)
	}
	for start := 0; start < len(userIds); start += 500 {
		var users []struct {
			Id          int
			Username    string
			DisplayName string
			DeletedAt   gorm.DeletedAt
		}
		if err := DB.Unscoped().Model(&User{}).
			Select("id, username, display_name, deleted_at").
			Where("id IN ?", userIds[start:min(start+500, len(userIds))]).
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
		if accumulator.item.BillingVersion == nil {
			finalizeBillingReconciliationUsage(&accumulator.item.Usage)
			finalizeBillingReconciliationPrice(&accumulator.price)
			accumulator.item.OriginalQuota = accumulator.price.model.OriginalQuota
			accumulator.item.originalQuota = accumulator.price.model.originalQuota
			if accumulator.item.Usage.GrossQuota == 0 && accumulator.item.Usage.RefundQuota == 0 && accumulator.item.OriginalQuota == nil {
				zero := int64(0)
				accumulator.item.OriginalQuota = &zero
			}
			if accumulator.item.OriginalQuota != nil {
				discountQuota := *accumulator.item.OriginalQuota - accumulator.item.Usage.NetQuota
				accumulator.item.DiscountQuota = &discountQuota
			}
			accumulator.item.DataQuality = accumulator.price.model.DataQuality
			finalizeBillingReconciliationQuality(&accumulator.item.DataQuality)

		}

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
		if len(frozen) > 0 {
			accumulator.item.MoneyUSD = billingStatementListMoney(accumulator.item)
		}
		items = append(items, accumulator.item)
	}

	sortBillingCustomerStatementList(items, sortBy, sortOrder)
	result.Total = int64(len(items))
	result.Summary = summarizeBillingCustomerStatementList(items)
	if len(frozen) > 0 {
		result.Summary.MoneyUSD = sumBillingStatementListMoney(items)
	}

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
	originalQuota := decimal.Zero
	originalQuotaComplete := true
	for _, item := range items {
		accumulateBillingReconciliationUsage(&summary.Usage, item.Usage)
		accumulateBillingReconciliationQuality(&summary.DataQuality, item.DataQuality)
		if (item.Usage.GrossQuota > 0 || item.Usage.RefundQuota > 0) && item.OriginalQuota == nil {
			originalQuotaComplete = false
		} else if item.OriginalQuota != nil {
			originalQuota = originalQuota.Add(item.originalQuota)
		}
	}
	finalizeBillingReconciliationUsage(&summary.Usage)
	if originalQuotaComplete {
		summary.OriginalQuota = billingStatementOriginalQuota(originalQuota)
		if summary.OriginalQuota == nil {
			accumulateBillingEstimateReasonQuality(ensureBillingReconciliationQuality(&summary.DataQuality), []string{BillingEstimateAmountOutOfRange})
		} else {
			discountQuota := *summary.OriginalQuota - summary.Usage.NetQuota
			summary.DiscountQuota = &discountQuota
		}
	}
	finalizeBillingReconciliationQuality(&summary.DataQuality)
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
			if items[i].MoneyUSD != nil && items[j].MoneyUSD != nil {
				a, _ := decimal.NewFromString(*items[i].MoneyUSD.Original)
				b, _ := decimal.NewFromString(*items[j].MoneyUSD.Original)
				comparison = a.Cmp(b)
			}
		case "username":
			comparison = strings.Compare(strings.ToLower(items[i].Username), strings.ToLower(items[j].Username))
		default:
			comparison = compareInt64(items[i].Usage.NetQuota, items[j].Usage.NetQuota)
			if items[i].MoneyUSD != nil && items[j].MoneyUSD != nil {
				a, _ := decimal.NewFromString(items[i].MoneyUSD.Net)
				b, _ := decimal.NewFromString(items[j].MoneyUSD.Net)
				comparison = a.Cmp(b)
			}
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

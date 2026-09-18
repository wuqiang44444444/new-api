package model

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

// 保存整份客户安全投影；十进制中间值仅供筛选后聚合，避免逐模型舍入再求和。
type billingStatementFrozenProjection struct {
	Statement BillingCustomerStatement `json:"statement"`
	Original  map[string]string        `json:"original"`
}

func billingStatementOriginalKey(group int64, model, mode string) string {
	raw, _ := common.Marshal([]string{strconv.FormatInt(group, 10), model, mode})
	return string(raw)
}
func FreezeBillingStatementProjection(statement BillingCustomerStatement) (BillingStatementJSON, error) {
	snapshot := billingStatementFrozenProjection{Statement: statement, Original: map[string]string{}}
	snapshot.Statement.CurrentBalance = nil // 当前余额始终是独立实时字段，不成为历史金额。
	for _, group := range statement.Groups {
		for _, item := range group.Models {
			if item.OriginalQuota != nil {
				snapshot.Original[billingStatementOriginalKey(group.Id, item.ModelName, item.BillingMode)] = item.originalQuota.String()
			}
		}
	}
	raw, err := common.Marshal(snapshot)
	return BillingStatementJSON(raw), err
}
func ReadBillingStatementProjection(v *BillingStatementVersion, dimension string, groupID *int, modelName, billingMode string) (*BillingCustomerStatement, error) {
	raw := v.SummaryProjection
	if dimension == "channel" {
		raw = v.ChannelProjection
	}
	var snapshot billingStatementFrozenProjection
	if raw == "" {
		return nil, errors.New("statement projection is missing")
	}
	if err := common.UnmarshalJsonStr(string(raw), &snapshot); err != nil {
		return nil, err
	}
	if groupID == nil && modelName == "" && billingMode == "" {
		return &snapshot.Statement, nil
	}
	source := snapshot.Statement
	result := source
	result.Groups = nil
	result.Summary = BillingReconciliationUsage{}
	result.DataQuality = nil
	result.OriginalQuota = nil
	result.DiscountQuota = nil
	result.DiscountCombinations = nil
	for _, group := range source.Groups {
		if groupID != nil && group.Id != int64(*groupID) {
			continue
		}
		next := group
		next.Models = nil
		next.Usage = BillingReconciliationUsage{}
		next.OriginalQuota = nil
		next.DiscountQuota = nil
		for _, item := range group.Models {
			if (modelName != "" && item.ModelName != modelName) || (billingMode != "" && item.BillingMode != billingMode) {
				continue
			}
			if item.OriginalQuota != nil {
				value, ok := snapshot.Original[billingStatementOriginalKey(group.Id, item.ModelName, item.BillingMode)]
				if !ok {
					return nil, errors.New("statement exact original subtotal is missing")
				}
				exact, err := decimal.NewFromString(value)
				if err != nil {
					return nil, err
				}
				item.originalQuota = exact
			}
			next.Models = append(next.Models, item)
			accumulateBillingReconciliationUsage(&next.Usage, item.Usage)
			accumulateBillingReconciliationQuality(&result.DataQuality, item.DataQuality)
		}
		if len(next.Models) == 0 {
			continue
		}
		finalizeBillingReconciliationUsage(&next.Usage)
		finalizeBillingReconciliationOriginalQuota(&next)
		result.Groups = append(result.Groups, next)
		accumulateBillingReconciliationUsage(&result.Summary, next.Usage)
	}
	// 原聚合器溢出合并行不保留成员维度，筛选时不能把缺少的组合当完整分解。
	combinationsComplete := true
	for _, item := range source.DiscountCombinations {
		if item.Other {
			combinationsComplete = false
			break
		}
	}
	for _, item := range source.DiscountCombinations {
		if (groupID != nil && item.GroupId != int64(*groupID)) || (modelName != "" && item.ModelName != modelName) || (billingMode != "" && item.BillingMode != billingMode) {
			continue
		}
		result.DiscountCombinations = append(result.DiscountCombinations, item)
	}
	if !combinationsComplete {
		result.DiscountCombinations = nil
	}
	finalizeBillingReconciliationUsage(&result.Summary)
	finalizeBillingCustomerStatementOriginalQuota(&result)
	finalizeBillingReconciliationQuality(&result.DataQuality)
	return &result, nil
}

// 当前余额单独读取主库，不冻结，也不为已删除客户探测历史日志。
func GetBillingStatementCurrentBalance(ctx context.Context, userID int) (*int, error) {
	var user User
	err := DB.WithContext(ctx).Select("id,quota").First(&user, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user.Quota, nil
}

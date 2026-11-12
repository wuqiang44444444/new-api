package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type billingCustomerModelKey struct {
	groupId int64
	model   string
	mode    string
}

type billingCustomerSummary struct {
	groups       map[int64]*BillingReconciliationGroupSummary
	models       map[billingCustomerModelKey]*billingReconciliationModelAccumulator
	names        map[int64]string
	nameSource   map[int64]int64
	combinations *billingDiscountCombinationAccumulator
}

func newBillingCustomerSummary() *billingCustomerSummary {
	return &billingCustomerSummary{
		groups:       make(map[int64]*BillingReconciliationGroupSummary),
		models:       make(map[billingCustomerModelKey]*billingReconciliationModelAccumulator),
		names:        make(map[int64]string),
		nameSource:   make(map[int64]int64),
		combinations: newBillingDiscountCombinationAccumulator(),
	}
}

func (a *billingCustomerSummary) observe(dimension string, log billingReconciliationLog, parsed parsedBillingReconciliationLog, sourceID int64) {
	log.ModelName = parsed.customerModel
	groupID := int64(log.TokenId)
	if dimension == "channel" {
		groupID = int64(log.ChannelId)
	} else if log.TokenName != "" {
		a.names[groupID], a.nameSource[groupID] = log.TokenName, sourceID
	}
	a.combinations.observe(log, groupID, parsed, sourceID)
	acc := a.model(billingCustomerModelKey{groupID, parsed.customerModel, parsed.billingMode})
	accumulateBillingReconciliationLog(&acc.model.Usage, log, parsed)
	accumulateCustomerBillingQuality(&acc.model.DataQuality, parsed)
	accumulateBillingReconciliationPrice(acc, log, parsed)
}

func (a *billingCustomerSummary) model(key billingCustomerModelKey) *billingReconciliationModelAccumulator {
	if a.groups[key.groupId] == nil {
		a.groups[key.groupId] = &BillingReconciliationGroupSummary{Id: key.groupId, Models: []BillingReconciliationModelSummary{}}
	}
	acc := a.models[key]
	if acc == nil {
		acc = &billingReconciliationModelAccumulator{
			model:                BillingReconciliationModelSummary{ModelName: key.model, BillingMode: key.mode},
			priceSnapshotMarkers: make(map[string]struct{}), originalQuotaComplete: true,
		}
		a.models[key] = acc
	}
	return acc
}

func accumulateCustomerBillingQuality(quality **BillingReconciliationDataQuality, parsed parsedBillingReconciliationLog) {
	if parsed.inputTokensUnavailable {
		ensureBillingReconciliationQuality(quality).InputTokensUnavailableRequests++
	}
	if parsed.unavailable {
		ensureBillingReconciliationQuality(quality).UnavailableRequests++
	}
	if parsed.billingMode == BillingReconciliationModeUnknown {
		ensureBillingReconciliationQuality(quality).UnknownBillingModeRequests++
	}
}

func (a *billingCustomerSummary) finish(ctx context.Context, statement BillingCustomerStatement, start, end int64) (BillingCustomerStatement, error) {
	userId, dimension := statement.UserId, statement.Dimension
	for key, acc := range a.models {
		filter := BillingReconciliationDetailFilter{StartTimestamp: start, EndTimestamp: end, UserId: userId, ModelName: key.model, BillingMode: key.mode}
		if dimension == "channel" {
			filter.ChannelId = int(key.groupId)
		} else {
			filter.TokenId = int(key.groupId)
		}
		acc.model.DetailFilter = filter
	}
	if dimension == "channel" {
		channelIds := make([]int, 0, len(a.groups))
		for key := range a.groups {
			if key > 0 {
				channelIds = append(channelIds, int(key))
			}
		}
		if len(channelIds) > 0 {
			var channels []struct {
				Id   int
				Name string
			}
			if err := DB.WithContext(ctx).Model(&Channel{}).Select("id, name").Where("id IN ?", channelIds).Scan(&channels).Error; err != nil {
				return statement, err
			}
			for _, channel := range channels {
				a.names[int64(channel.Id)] = channel.Name
			}
		}
	}

	for key, accumulator := range a.models {
		finalizeBillingReconciliationUsage(&accumulator.model.Usage)
		finalizeBillingReconciliationPrice(accumulator)
		finalizeBillingReconciliationQuality(&accumulator.model.DataQuality)
		accumulateBillingReconciliationQuality(&statement.DataQuality, accumulator.model.DataQuality)
		group := a.groups[key.groupId]
		group.Models = append(group.Models, accumulator.model)
		accumulateBillingReconciliationUsage(&group.Usage, accumulator.model.Usage)
	}
	for key, group := range a.groups {
		name, ok := a.names[key]
		if !ok || strings.TrimSpace(name) == "" {
			group.Deleted = key > 0 && !ok
			if dimension == "api_key" {
				name = fmt.Sprintf("API Key #%d", key)
			} else {
				name = fmt.Sprintf("Channel #%d", key)
			}
		}
		group.Name = name
		finalizeBillingReconciliationUsage(&group.Usage)
		finalizeBillingReconciliationOriginalQuota(group)
		sort.Slice(group.Models, func(i, j int) bool {
			if group.Models[i].Usage.NetQuota != group.Models[j].Usage.NetQuota {
				return group.Models[i].Usage.NetQuota > group.Models[j].Usage.NetQuota
			}
			if group.Models[i].ModelName != group.Models[j].ModelName {
				return group.Models[i].ModelName < group.Models[j].ModelName
			}
			return group.Models[i].BillingMode < group.Models[j].BillingMode
		})
		statement.Groups = append(statement.Groups, *group)
		accumulateBillingReconciliationUsage(&statement.Summary, group.Usage)
	}
	finalizeBillingReconciliationUsage(&statement.Summary)
	finalizeBillingCustomerStatementOriginalQuota(&statement)
	finalizeBillingReconciliationQuality(&statement.DataQuality)
	statement.DiscountCombinations = a.combinations.finalize()
	sort.Slice(statement.Groups, func(i, j int) bool {
		if statement.Groups[i].Usage.NetQuota != statement.Groups[j].Usage.NetQuota {
			return statement.Groups[i].Usage.NetQuota > statement.Groups[j].Usage.NetQuota
		}
		return statement.Groups[i].Name < statement.Groups[j].Name
	})

	identity, err := GetBillingReconciliationUser(ctx, userId)
	if err != nil {
		return statement, err
	}
	statement.Username = identity.Username
	statement.DisplayName = identity.DisplayName
	statement.Deleted = identity.Deleted
	statement.CurrentBalance = identity.CurrentBalance
	return statement, nil
}

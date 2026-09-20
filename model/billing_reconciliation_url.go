package model

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// billingURLGroupChannelFallbackPrefix marks the per-channel group of a
// channel whose base URL is missing, unreadable or unsafe to merge (deleted
// channel, credentials, query or fragment). Normalized URLs always start with
// a scheme, so the prefix cannot collide with an identified URL group.
const billingURLGroupChannelFallbackPrefix = "channel:"

// ProviderURLSummary is a read-only management projection that merges the
// platform-recorded upstream usage of channels sharing the same currently
// configured base URL. The grouping is a reporting aid only: it never proves
// the URL used at request time, supplier identity, account, contract or
// pricing, and it must not feed routing, billing or settlement.
type ProviderURLSummary struct {
	Groups      []ProviderURLGroupSummary         `json:"url_groups"`
	DataQuality *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
}

type ProviderURLGroupSummary struct {
	UrlKey       string                            `json:"url_key"`
	DisplayName  string                            `json:"display_name"`
	BaseURL      string                            `json:"base_url,omitempty"`
	Unidentified bool                              `json:"unidentified,omitempty"`
	Deleted      bool                              `json:"deleted,omitempty"`
	ChannelIds   []int                             `json:"channel_ids"`
	ChannelCount int64                             `json:"channel_count"`
	ModelCount   int64                             `json:"model_count"`
	Usage        ProviderBillingUsage              `json:"usage"`
	Models       []ProviderURLModelSummary         `json:"models"`
	DataQuality  *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	// ChannelDiscounts is the per-URL editing area: one slot per channel with
	// usage in the group, pending slots included.
	ChannelDiscounts []ProviderChannelDiscountStatus `json:"channel_discounts"`
	// OriginalAmount / ReferenceAmount follow the same completeness rules as
	// the model rows; a pending channel discount keeps the reference amount
	// incomplete even when that channel's original amount happens to be zero.
	OriginalAmount          *int64   `json:"original_amount,omitempty"`
	ReferenceAmount         *int64   `json:"reference_amount,omitempty"`
	ReferenceKnown          bool     `json:"reference_known"`
	DiscountPendingChannels int      `json:"discount_pending_channels"`
	EstimateReasons         []string `json:"estimate_reasons,omitempty"`
}

// ProviderURLModelSummary merges one upstream model identity across the
// channels of a URL group. Records whose provider model identity fell back to
// the customer model stay in separate fallback rows, and token, per-call and
// unknown billing modes always remain separate rows.
type ProviderURLModelSummary struct {
	ProviderModel         string                            `json:"provider_model"`
	ProviderModelFallback bool                              `json:"provider_model_fallback,omitempty"`
	BillingMode           string                            `json:"billing_mode"`
	Usage                 ProviderBillingUsage              `json:"usage"`
	Channels              []ProviderURLChannelSummary       `json:"channels"`
	DataQuality           *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	// OriginalAmount is the signed official-price quota of this model row
	// within the URL group; nil means at least one money-bearing row could not
	// be restored from frozen historical facts.
	OriginalAmount *int64 `json:"original_amount,omitempty"`
	// ReferenceAmount is the discount-adjusted reference amount (original
	// price times the channel-month coefficient). It is nil — never zero —
	// when any contributing channel lacks a discount or any amount is unknown.
	ReferenceAmount *int64   `json:"reference_amount,omitempty"`
	EstimateReasons []string `json:"estimate_reasons,omitempty"`
}

// ProviderURLChannelSummary is one channel's contribution to a model row. It
// carries usage facts plus the local official-price amount and the reference
// amount after the channel's month coefficient; it never implies what the
// supplier actually charged.
type ProviderURLChannelSummary struct {
	ChannelId             int                                `json:"channel_id"`
	ChannelName           string                             `json:"channel_name"`
	ProviderModel         string                             `json:"provider_model"`
	CustomerModels        []string                           `json:"customer_models"`
	ProviderModelFallback bool                               `json:"provider_model_fallback,omitempty"`
	BillingMode           string                             `json:"billing_mode"`
	Usage                 ProviderBillingUsage               `json:"usage"`
	DataQuality           *BillingReconciliationDataQuality  `json:"data_quality,omitempty"`
	DetailFilter          BillingReconciliationDetailFilter  `json:"detail_filter"`
	OriginalAmount        *int64                             `json:"original_amount,omitempty"`
	ReferenceAmount       *int64                             `json:"reference_amount,omitempty"`
	Discount              *ProviderBillingDiscountProjection `json:"discount"`
	EstimateReasons       []string                           `json:"estimate_reasons,omitempty"`
	// UsageOnly rows keep usage but no customer settlement, so the amount is
	// not a completeness gap for their parents.
	UsageOnly bool `json:"usage_only,omitempty"`
}

// ProviderChannelDiscountStatus is one channel's slot in the per-URL discount
// editing area. A nil Discount means the month is pending manual fill — it is
// never presented as coefficient 1.
type ProviderChannelDiscountStatus struct {
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	// Discount is nil when the channel-month is pending manual fill.
	Discount  *ProviderBillingDiscountProjection `json:"discount"`
	UpdatedAt int64                              `json:"updated_at,omitempty"`
	UpdatedBy int                                `json:"updated_by,omitempty"`
}

// GetProviderBillingURLSummary aggregates the platform-recorded upstream
// usage, the local official-price amounts restored from customer settlement
// facts, and the channel-month discount coefficients into URL groups. It is a
// read-only reporting view: it never materializes discounts, never writes
// audit rows and never feeds routing, billing or settlement. The reference
// amount is original price times the channel-month coefficient; it is a
// reference display figure under the "same official price on both sides"
// business premise, not a verified supplier cost.
func GetProviderBillingURLSummary(startTimestamp int64, endTimestamp int64, periodStart int64, urlKey string) (ProviderURLSummary, error) {
	summary := ProviderURLSummary{Groups: make([]ProviderURLGroupSummary, 0)}
	items, err := scanProviderBillingURLPlatformItems(startTimestamp, endTimestamp)
	if err != nil {
		return summary, err
	}

	channelIds := make([]int, 0, len(items))
	seenChannels := make(map[int]struct{}, len(items))
	for key := range items {
		if _, ok := seenChannels[key.channelId]; !ok {
			seenChannels[key.channelId] = struct{}{}
			channelIds = append(channelIds, key.channelId)
		}
	}
	channels, err := getBillingURLChannelsById(channelIds)
	if err != nil {
		return summary, err
	}
	discounts, err := GetProviderChannelBillingDiscounts(periodStart, channelIds)
	if err != nil {
		return summary, err
	}

	type modelKey struct {
		model    string
		mode     string
		fallback bool
	}
	type channelBuild struct {
		usage           ProviderBillingUsage
		original        decimal.Decimal
		allKnown        bool
		usageOnly       bool
		leafCount       int
		estimateReasons []string
	}
	type groupBuild struct {
		group      ProviderURLGroupSummary
		models     map[modelKey]*ProviderURLModelSummary
		channelIds map[int]struct{}
		channels   map[int]*channelBuild
	}
	groups := make(map[string]*groupBuild)

	for _, item := range items {
		finalizeBillingReconciliationQuality(&item.DataQuality)
		finalizeProviderBillingItemAmount(item)
		item.CustomerModels = make([]string, 0, len(item.customerModels))
		for customerModel := range item.customerModels {
			item.CustomerModels = append(item.CustomerModels, customerModel)
		}
		sort.Strings(item.CustomerModels)
		name := strings.TrimSpace(channels[item.ChannelId].Name)
		if name == "" {
			name = fmt.Sprintf("Channel #%d", item.ChannelId)
		}
		item.ChannelName = name
	}

	for itemKey, item := range items {
		channel, found := channels[itemKey.channelId]
		groupKey, displayName := billingURLGroupIdentity(channel, found, itemKey.channelId)
		group, ok := groups[groupKey]
		if !ok {
			group = &groupBuild{
				group: ProviderURLGroupSummary{
					UrlKey:           groupKey,
					DisplayName:      displayName,
					Unidentified:     strings.HasPrefix(groupKey, billingURLGroupChannelFallbackPrefix),
					Deleted:          strings.HasPrefix(groupKey, billingURLGroupChannelFallbackPrefix) && itemKey.channelId > 0 && !found,
					ChannelIds:       make([]int, 0),
					Models:           make([]ProviderURLModelSummary, 0),
					ChannelDiscounts: make([]ProviderChannelDiscountStatus, 0),
				},
				models:     make(map[modelKey]*ProviderURLModelSummary),
				channelIds: make(map[int]struct{}),
				channels:   make(map[int]*channelBuild),
			}
			if !group.group.Unidentified {
				group.group.BaseURL = groupKey
			}
			groups[groupKey] = group
		}
		if _, seen := group.channelIds[itemKey.channelId]; !seen {
			group.channelIds[itemKey.channelId] = struct{}{}
			group.group.ChannelIds = append(group.group.ChannelIds, itemKey.channelId)
		}
		cb := group.channels[itemKey.channelId]
		if cb == nil {
			cb = &channelBuild{allKnown: true, usageOnly: true}
			group.channels[itemKey.channelId] = cb
		}

		mk := modelKey{model: itemKey.model, mode: itemKey.mode, fallback: itemKey.fallback}
		modelRow, ok := group.models[mk]
		if !ok {
			modelRow = &ProviderURLModelSummary{
				ProviderModel:         itemKey.model,
				ProviderModelFallback: itemKey.fallback,
				BillingMode:           itemKey.mode,
				Channels:              make([]ProviderURLChannelSummary, 0),
			}
			group.models[mk] = modelRow
		}
		accumulateBillingReconciliationQuality(&group.group.DataQuality, item.DataQuality)
		accumulateBillingReconciliationQuality(&modelRow.DataQuality, item.DataQuality)
		var discountProjection *ProviderBillingDiscountProjection
		if record, ok := discounts[itemKey.channelId]; ok {
			projection := providerChannelDiscountProjection(record)
			discountProjection = &projection
		}
		leaf := ProviderURLChannelSummary{
			ChannelId: item.ChannelId, ChannelName: item.ChannelName,
			ProviderModel: item.ProviderModel, CustomerModels: item.CustomerModels,
			ProviderModelFallback: item.ProviderModelFallback, BillingMode: item.BillingMode,
			Usage: item.Usage, DataQuality: item.DataQuality, DetailFilter: item.DetailFilter,
			OriginalAmount: item.OriginalAmount, Discount: discountProjection,
			EstimateReasons: item.EstimateReasons, UsageOnly: item.UsageOnly,
		}
		modelRow.Channels = append(modelRow.Channels, leaf)
		accumulateProviderBillingUsage(&modelRow.Usage, item.Usage)
		accumulateProviderBillingUsage(&group.group.Usage, item.Usage)
		// 纯渠道测试叶子没有客户结算金额，对父级金额既不贡献也不造成不完整。
		cb.usageOnly = cb.usageOnly && item.UsageOnly
		if item.UsageOnly {
			cb.allKnown = cb.allKnown
		} else if item.OriginalAmount != nil {
			cb.original = cb.original.Add(item.originalQuota)
		} else {
			cb.allKnown = false
		}
		cb.leafCount++
		cb.estimateReasons = mergeBillingEstimateReasons(cb.estimateReasons, item.EstimateReasons...)
	}

	result := make([]ProviderURLGroupSummary, 0, len(groups))
	for _, build := range groups {
		sort.Ints(build.group.ChannelIds)
		build.group.ChannelCount = int64(len(build.channelIds))

		// Channel-level finals: original amount, discount slot and reference
		// amount per channel.
		groupOriginal := decimal.Zero
		groupOriginalKnown := true
		groupReference := decimal.Zero
		groupReferenceKnown := true
		for _, channelId := range build.group.ChannelIds {
			cb := build.channels[channelId]
			name := strings.TrimSpace(channels[channelId].Name)
			if name == "" {
				name = fmt.Sprintf("Channel #%d", channelId)
			}
			status := ProviderChannelDiscountStatus{ChannelId: channelId, ChannelName: name}
			var discountValue decimal.Decimal
			hasDiscount := false
			if record, ok := discounts[channelId]; ok {
				projection := providerChannelDiscountProjection(record)
				status.Discount = &projection
				status.UpdatedAt = record.UpdatedAt
				status.UpdatedBy = record.UpdatedBy
				discountValue = record.Discount
				hasDiscount = true
			} else if !cb.usageOnly {
				build.group.DiscountPendingChannels++
			}
			build.group.ChannelDiscounts = append(build.group.ChannelDiscounts, status)

			channelReference := decimal.Zero
			channelReferenceKnown := cb.usageOnly || (cb.allKnown && hasDiscount)
			if cb.allKnown {
				groupOriginal = groupOriginal.Add(cb.original)
			} else {
				groupOriginalKnown = false
			}
			channelReference = cb.original.Mul(discountValue)
			if !channelReferenceKnown {
				groupReferenceKnown = false
			} else {
				groupReference = groupReference.Add(channelReference)
			}
			build.group.EstimateReasons = mergeBillingEstimateReasons(build.group.EstimateReasons, cb.estimateReasons...)
		}
		if groupOriginalKnown && len(build.channels) > 0 {
			build.group.OriginalAmount = billingStatementOriginalQuota(groupOriginal)
			if build.group.OriginalAmount == nil {
				build.group.EstimateReasons = mergeBillingEstimateReasons(build.group.EstimateReasons, BillingEstimateAmountOutOfRange)
				groupOriginalKnown = false
			}
		}
		build.group.ReferenceAmount = nil
		if groupReferenceKnown && len(build.channels) > 0 {
			build.group.ReferenceAmount = billingStatementOriginalQuota(groupReference)
			build.group.ReferenceKnown = build.group.ReferenceAmount != nil
		}

		// Model-row finals aggregate the same leaves.
		type modelIdentity struct {
			name     string
			fallback bool
		}
		modelIdentities := make(map[modelIdentity]struct{})
		for mk, modelRow := range build.models {
			modelIdentities[modelIdentity{modelRow.ProviderModel, modelRow.ProviderModelFallback}] = struct{}{}
			finalizeBillingReconciliationQuality(&modelRow.DataQuality)
			modelOriginal := decimal.Zero
			modelOriginalKnown := true
			modelReference := decimal.Zero
			modelReferenceKnown := true
			sort.Slice(modelRow.Channels, func(i, j int) bool {
				if modelRow.Channels[i].ChannelName != modelRow.Channels[j].ChannelName {
					return modelRow.Channels[i].ChannelName < modelRow.Channels[j].ChannelName
				}
				return modelRow.Channels[i].ChannelId < modelRow.Channels[j].ChannelId
			})
			for i := range modelRow.Channels {
				leaf := &modelRow.Channels[i]
				if leaf.UsageOnly {
					continue
				}
				leafKnown := leaf.OriginalAmount != nil
				leafOriginal := decimal.Zero
				if leafKnown {
					leafOriginal = items[providerBillingSummaryKey{channelId: leaf.ChannelId, model: mk.model, mode: mk.mode, fallback: mk.fallback}].originalQuota
					modelOriginal = modelOriginal.Add(leafOriginal)
				} else {
					modelOriginalKnown = false
				}
				if leafKnown && leaf.Discount != nil {
					leafReference := leafOriginal.Mul(leaf.Discount.Value)
					leaf.ReferenceAmount = billingStatementOriginalQuota(leafReference)
					modelReference = modelReference.Add(leafReference)
				} else {
					modelReferenceKnown = false
				}
			}
			if modelOriginalKnown && len(modelRow.Channels) > 0 {
				modelRow.OriginalAmount = billingStatementOriginalQuota(modelOriginal)
				if modelRow.OriginalAmount == nil {
					modelRow.EstimateReasons = mergeBillingEstimateReasons(modelRow.EstimateReasons, BillingEstimateAmountOutOfRange)
				}
			}
			if modelReferenceKnown && len(modelRow.Channels) > 0 && modelRow.OriginalAmount != nil {
				modelRow.ReferenceAmount = billingStatementOriginalQuota(modelReference)
			}
			build.group.Models = append(build.group.Models, *modelRow)
		}
		sort.Slice(build.group.Models, func(i, j int) bool {
			if build.group.Models[i].ProviderModel != build.group.Models[j].ProviderModel {
				return build.group.Models[i].ProviderModel < build.group.Models[j].ProviderModel
			}
			if build.group.Models[i].BillingMode != build.group.Models[j].BillingMode {
				return build.group.Models[i].BillingMode < build.group.Models[j].BillingMode
			}
			return build.group.Models[i].ProviderModelFallback
		})
		build.group.ModelCount = int64(len(modelIdentities))
		finalizeBillingReconciliationQuality(&build.group.DataQuality)
		if urlKey == "" || build.group.UrlKey == urlKey {
			result = append(result, build.group)
			accumulateBillingReconciliationQuality(&summary.DataQuality, build.group.DataQuality)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Unidentified != result[j].Unidentified {
			return !result[i].Unidentified
		}
		return result[i].DisplayName < result[j].DisplayName
	})
	summary.Groups = result
	finalizeBillingReconciliationQuality(&summary.DataQuality)
	return summary, nil
}
func billingURLGroupIdentity(channel Channel, found bool, channelId int) (string, string) {
	if found {
		if normalized := normalizeBillingURLGroupKey(channel.GetBaseURL()); normalized != "" {
			return normalized, normalized
		}
	}
	fallbackKey := fmt.Sprintf("%s%d", billingURLGroupChannelFallbackPrefix, channelId)
	displayName := strings.TrimSpace(channel.Name)
	if displayName == "" {
		displayName = fmt.Sprintf("Channel #%d", channelId)
	}
	return fallbackKey, displayName
}

// normalizeBillingURLGroupKey canonicalizes only the parts of a base URL that
// are unambiguously equivalent — scheme and host case, the scheme's default
// port and trailing slashes in the path. Protocol, explicit non-default port
// and base path stay part of the identity. It returns an empty string for
// URLs that are missing, unparseable or unsafe to treat as one identity
// (credentials, query parameters or fragment), so callers keep those channels
// in separate per-channel groups and never display the sensitive parts.
func normalizeBillingURLGroupKey(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Opaque != "" ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	host := parsed.Hostname()
	if host == "" {
		return ""
	}
	port := parsed.Port()
	if (parsed.Scheme == "http" && port == "80") || (parsed.Scheme == "https" && port == "443") {
		port = ""
	}
	hostPart := strings.ToLower(host)
	if strings.Contains(host, ":") {
		hostPart = "[" + hostPart + "]"
	}
	if port != "" {
		hostPart += ":" + port
	}
	escapedPath := strings.TrimRight(parsed.EscapedPath(), "/")
	path, err := url.PathUnescape(escapedPath)
	if err != nil {
		return ""
	}
	normalized := url.URL{Scheme: parsed.Scheme, Host: hostPart, Path: path, RawPath: escapedPath}
	return normalized.String()
}

func getBillingURLChannelsById(channelIds []int) (map[int]Channel, error) {
	channels := make(map[int]Channel, len(channelIds))
	if len(channelIds) == 0 {
		return channels, nil
	}
	var rows []struct {
		Id      int
		Name    string
		BaseURL *string
		Type    int
	}
	if err := DB.Model(&Channel{}).Select("id, name, base_url, type").Where("id IN ?", channelIds).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		channels[row.Id] = Channel{Id: row.Id, Name: row.Name, BaseURL: row.BaseURL, Type: row.Type}
	}
	return channels, nil
}

// scanProviderBillingURLPlatformItems shares signed customer settlement facts
// with details and exports, while provider usage includes only consume rows and
// provable task settlement usage. Keys retain provider identity and billing mode.
func scanProviderBillingURLPlatformItems(startTimestamp int64, endTimestamp int64) (map[providerBillingSummaryKey]*ProviderBillingPlatformSummary, error) {
	platform := make(map[providerBillingSummaryKey]*ProviderBillingPlatformSummary)
	err := scanUpstreamBillingFacts(context.Background(), UpstreamBillingDetailFilter{Start: startTimestamp, End: endTimestamp}, BillingStatementReadPolicy{}, func(_ Log, log billingReconciliationLog, parsed parsedBillingReconciliationLog) error {
		providerModel := strings.TrimSpace(parsed.providerModel)
		fallback := false
		if providerModel == "" {
			providerModel = log.ModelName
			fallback = true
		}
		itemKey := providerBillingSummaryKey{channelId: log.ChannelId, model: providerModel, mode: parsed.billingMode, fallback: fallback}
		item, ok := platform[itemKey]
		if !ok {
			item = &ProviderBillingPlatformSummary{
				ChannelId:             log.ChannelId,
				ProviderModel:         providerModel,
				ProviderModelFallback: fallback,
				BillingMode:           parsed.billingMode,
				DetailFilter: BillingReconciliationDetailFilter{
					StartTimestamp: startTimestamp,
					EndTimestamp:   endTimestamp,
					ChannelId:      log.ChannelId,
					ModelName:      log.ModelName,
					BillingMode:    parsed.billingMode,
				},
				detailModelName:  log.ModelName,
				customerModels:   make(map[string]struct{}),
				originalComplete: true,
			}
			platform[itemKey] = item
		} else if item.detailModelName != log.ModelName {
			item.DetailFilter.ModelName = ""
		}
		if log.ModelName != "" {
			item.customerModels[log.ModelName] = struct{}{}
		}
		if log.Type != LogTypeRefund || isProviderTaskUsageAdjustment(log) {
			accumulateProviderBillingLog(&item.Usage, log, parsed)
		}
		accumulateProviderBillingItemAmount(item, log, parsed)
		if fallback {
			ensureBillingReconciliationQuality(&item.DataQuality).ProviderModelFallbackRows++
		}
		if parsed.billingMode == BillingReconciliationModeUnknown {
			ensureBillingReconciliationQuality(&item.DataQuality).UnknownBillingModeRequests++
		}
		if parsed.unavailable {
			ensureBillingReconciliationQuality(&item.DataQuality).UnavailableRequests++
		}
		if parsed.cacheWriteUnavailable {
			ensureBillingReconciliationQuality(&item.DataQuality).CacheWriteUnavailableRequests++
		}
		return nil
	})
	return platform, err
}

// accumulateProviderBillingItemAmount restores the official-price amount of
// one upstream usage row using the customer statement algorithm: settled
// quota divided by the frozen group ratio and contract discount, refunds
// negative. Native channel test rows carry usage but no customer settlement,
// so they stay in the usage view, never enter the amount, and are counted
// separately. Rows whose frozen price evidence is incomplete leave the amount
// unknown instead of being dropped or zero-filled.
func accumulateProviderBillingItemAmount(item *ProviderBillingPlatformSummary, log billingReconciliationLog, parsed parsedBillingReconciliationLog) {
	if isNativeChannelTestLog(log.Type, log.TokenId, log.TokenName, log.Content) {
		ensureBillingReconciliationQuality(&item.DataQuality).UsageWithoutAmountRows++
		return
	}
	if log.Type != LogTypeConsume && log.Type != LogTypeRefund {
		return
	}
	item.settlementRows++
	quota := max(int64(log.Quota), int64(0))
	if quota == 0 {
		return
	}
	item.moneyRows++
	original, reasons := upstreamOriginalQuota(log, parsed)
	if len(reasons) > 0 {
		item.EstimateReasons = mergeBillingEstimateReasons(item.EstimateReasons, reasons...)
		item.originalComplete = false
		ensureBillingReconciliationQuality(&item.DataQuality).MissingHistoricalPriceRows++
		return
	}
	item.originalQuota = item.originalQuota.Add(original)
	item.originalQuotaKnown = true
}

// finalizeProviderBillingItemAmount rounds an item's restored original once,
// from the accumulated decimals, and keeps estimate reasons as the quality
// marker when any money-bearing row could not be restored.
func finalizeProviderBillingItemAmount(item *ProviderBillingPlatformSummary) {
	item.UsageOnly = item.settlementRows == 0
	if item.originalComplete && (item.originalQuotaKnown || (item.settlementRows > 0 && item.moneyRows == 0)) {
		item.OriginalAmount = billingStatementOriginalQuota(item.originalQuota)
		if item.OriginalAmount == nil {
			item.EstimateReasons = mergeBillingEstimateReasons(item.EstimateReasons, BillingEstimateAmountOutOfRange)
		}
	}
	item.originalComplete = true
}

// providerChannelDiscountProjection renders a channel-month record for
// display: a copied month points at its source month, a migrated month points
// at the migration, and everything else is a plain database value.
func providerChannelDiscountProjection(record ProviderChannelBillingDiscount) ProviderBillingDiscountProjection {
	source := "database"
	sourcePeriod := record.PeriodStart
	if record.Reason == reasonChannelDiscountDefault {
		source = "default"
	} else if strings.HasPrefix(record.Reason, reasonChannelDiscountMigrated) {
		source = "migrated"
		sourcePeriod = record.PeriodStart
	} else if record.CopiedFromPeriod > 0 && record.Version == 1 {
		source = "previous_period"
		sourcePeriod = record.CopiedFromPeriod
	}
	return ProviderBillingDiscountProjection{Value: record.Discount, Version: record.Version, Source: source, SourcePeriod: sourcePeriod}
}

// GetChannelIdsByNormalizedBaseURL resolves a URL grouping key back to the
// channels whose current base URL normalizes to it. Detail filters reuse the
// exact summary grouping identity; regrouping after a channel URL edit is a
// documented reporting behavior and never rewrites history.
func GetChannelIdsByNormalizedBaseURL(urlKey string) ([]int, error) {
	var channels []Channel
	if err := DB.Select("id, base_url, type").Find(&channels).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0)
	for _, channel := range channels {
		key, _ := billingURLGroupIdentity(channel, true, channel.Id)
		if key == urlKey {
			ids = append(ids, channel.Id)
		}
	}
	return ids, nil
}

package model

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
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
	UsageOnly bool `json:"usage_only,omitempty"`
	// Known subtotals remain separate from complete amounts when evidence is missing.
	KnownOriginalAmount  *int64 `json:"known_original_amount,omitempty"`
	KnownReferenceAmount *int64 `json:"known_reference_amount,omitempty"`
	UrlKey               string `json:"url_key"`
	DisplayName          string `json:"display_name"`
	// CustomName is the admin-defined alias persisted per grouping key. It
	// only replaces the card title; unidentified and deleted markers always
	// stay visible, and the safe base URL keeps being shown below the title.
	CustomName   string               `json:"custom_name,omitempty"`
	BaseURL      string               `json:"base_url,omitempty"`
	Unidentified bool                 `json:"unidentified,omitempty"`
	Deleted      bool                 `json:"deleted,omitempty"`
	ChannelIds   []int                `json:"channel_ids"`
	ChannelCount int64                `json:"channel_count"`
	ModelCount   int64                `json:"model_count"`
	Usage        ProviderBillingUsage `json:"usage"`
	// Channels is the single channel table of the group: one parent row per
	// channel carrying its month coefficient, its usage totals and its
	// model × billing-mode leaves. It replaces the old parallel discount
	// editing area and the old URL → model → channel tree.
	Channels    []ProviderURLChannelGroupSummary  `json:"channels"`
	DataQuality *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	// OriginalAmount / ReferenceAmount follow the same completeness rules as
	// the channel rows; a pending channel discount keeps the reference amount
	// incomplete even when that channel's original amount happens to be zero.
	OriginalAmount          *int64   `json:"original_amount,omitempty"`
	ReferenceAmount         *int64   `json:"reference_amount,omitempty"`
	ReferenceKnown          bool     `json:"reference_known"`
	DiscountPendingChannels int      `json:"discount_pending_channels"`
	EstimateReasons         []string `json:"estimate_reasons,omitempty"`
}

// ProviderURLChannelGroupSummary is one channel parent row. A nil Discount
// means the channel-month is pending manual fill — it is never presented as
// coefficient 1. Usage totals server-side over the same leaves shown below;
// the frontend never recomputes amounts.
type ProviderURLChannelGroupSummary struct {
	// Known subtotals remain separate from complete amounts when evidence is missing.
	KnownOriginalAmount  *int64 `json:"known_original_amount,omitempty"`
	KnownReferenceAmount *int64 `json:"known_reference_amount,omitempty"`
	ChannelId            int    `json:"channel_id"`
	ChannelName          string `json:"channel_name"`
	// Discount is nil when the channel-month is pending manual fill.
	Discount  *ProviderBillingDiscountProjection `json:"discount"`
	UpdatedAt int64                              `json:"updated_at,omitempty"`
	UpdatedBy int                                `json:"updated_by,omitempty"`
	Usage     ProviderBillingUsage               `json:"usage"`
	// Models are this channel's provider model × billing-mode leaves. Same-named
	// models of different channels stay inside their own channel row; confirmed
	// and fallback model identities never merge.
	Models      []ProviderURLChannelModelSummary  `json:"models"`
	DataQuality *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	// OriginalAmount is the signed official-price quota of the channel; nil
	// means at least one money-bearing row could not be restored from frozen
	// historical facts.
	OriginalAmount *int64 `json:"original_amount,omitempty"`
	// ReferenceAmount is the discount-adjusted reference amount (original
	// price times the channel-month coefficient). It is nil — never zero for
	// money-bearing rows — when the channel lacks a discount or any amount is
	// unknown.
	ReferenceAmount *int64   `json:"reference_amount,omitempty"`
	EstimateReasons []string `json:"estimate_reasons,omitempty"`
	// UsageOnly identifies channels without settled money rows. Pending test
	// amounts still make the channel and its parent totals incomplete.
	UsageOnly bool `json:"usage_only,omitempty"`
}

// ProviderURLChannelModelSummary is one model × billing-mode leaf of a channel
// row: the platform-recorded usage of that channel for one upstream model
// identity, plus the restored official-price amount and the reference amount
// after the channel's month coefficient. It never implies what the supplier
// actually charged.
type ProviderURLChannelModelSummary struct {
	// Known subtotals remain separate from complete amounts when evidence is missing.
	KnownOriginalAmount   *int64                            `json:"known_original_amount,omitempty"`
	KnownReferenceAmount  *int64                            `json:"known_reference_amount,omitempty"`
	UsageOnly             bool                              `json:"usage_only,omitempty"`
	ProviderModel         string                            `json:"provider_model"`
	ProviderModelFallback bool                              `json:"provider_model_fallback,omitempty"`
	BillingMode           string                            `json:"billing_mode"`
	CustomerModels        []string                          `json:"customer_models"`
	Usage                 ProviderBillingUsage              `json:"usage"`
	DataQuality           *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	DetailFilter          BillingReconciliationDetailFilter `json:"detail_filter"`
	OriginalAmount        *int64                            `json:"original_amount,omitempty"`
	ReferenceAmount       *int64                            `json:"reference_amount,omitempty"`
	EstimateReasons       []string                          `json:"estimate_reasons,omitempty"`
}

// GetProviderBillingURLSummary aggregates the platform-recorded upstream
// usage, the local official-price amounts restored from customer settlement
// facts, and the channel-month discount coefficients into URL groups of
// channel rows with model leaves. It is a read-only reporting view: it never
// materializes discounts, never writes audit rows and never feeds routing,
// billing or settlement. The reference amount is original price times the
// channel-month coefficient; it is a reference display figure under the "same
// official price on both sides" business premise, not a verified supplier
// cost.
func GetProviderBillingURLSummary(startTimestamp int64, endTimestamp int64, periodStart int64, urlKey string) (ProviderURLSummary, error) {
	return getProviderBillingURLSummary(context.Background(), startTimestamp, endTimestamp, periodStart, urlKey, 0)
}

func getProviderBillingURLSummary(ctx context.Context, startTimestamp, endTimestamp, periodStart int64, urlKey string, channelID int) (ProviderURLSummary, error) {
	summary := ProviderURLSummary{Groups: make([]ProviderURLGroupSummary, 0), DataQuality: &BillingReconciliationDataQuality{Status: "complete"}}
	records, err := loadProviderChannelBillingDiscounts(ctx, periodStart, nil)
	if err != nil {
		return summary, err
	}
	var scopeIDs []int
	if channelID > 0 {
		scopeIDs = []int{channelID}
	} else if urlKey != "" {
		if raw, fallback := strings.CutPrefix(urlKey, billingURLGroupChannelFallbackPrefix); fallback {
			id, parseErr := strconv.Atoi(raw)
			if parseErr != nil || id <= 0 {
				return summary, nil
			}
			scopeIDs = []int{id}
		} else {
			scopeIDs, err = GetChannelIdsByNormalizedBaseURL(urlKey)
			if err != nil {
				return summary, err
			}
			if len(scopeIDs) == 0 {
				return summary, nil
			}
		}
	}
	items, err := scanProviderBillingURLPlatformItems(ctx, startTimestamp, endTimestamp, records, scopeIDs, BillingStatementReadPolicy{})
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
	discounts := make(map[int]ProviderChannelBillingDiscount, len(channelIds))
	for _, id := range channelIds {
		if record, valid := providerChannelBillingDiscountFor(records, periodStart, id); valid {
			discounts[id] = record
		}
	}
	names, err := getProviderURLGroupNames()
	if err != nil {
		return summary, err
	}

	type modelKey struct {
		model    string
		mode     string
		fallback bool
	}
	type modelIdentity struct {
		name     string
		fallback bool
	}
	type channelBuild struct {
		group           ProviderURLChannelGroupSummary
		models          map[modelKey]*ProviderURLChannelModelSummary
		original        decimal.Decimal
		reference       decimal.Decimal
		allKnown        bool
		hasKnown        bool
		usageOnly       bool
		estimateReasons []string
	}
	type groupBuild struct {
		group           ProviderURLGroupSummary
		channelIds      map[int]struct{}
		channels        map[int]*channelBuild
		modelIdentities map[modelIdentity]struct{}
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
		build, ok := groups[groupKey]
		if !ok {
			build = &groupBuild{
				group: ProviderURLGroupSummary{
					UrlKey:       groupKey,
					DisplayName:  displayName,
					CustomName:   names[groupKey],
					Unidentified: strings.HasPrefix(groupKey, billingURLGroupChannelFallbackPrefix),
					Deleted:      strings.HasPrefix(groupKey, billingURLGroupChannelFallbackPrefix) && itemKey.channelId > 0 && !found,
					ChannelIds:   make([]int, 0),
					Channels:     make([]ProviderURLChannelGroupSummary, 0),
				},
				channelIds:      make(map[int]struct{}),
				channels:        make(map[int]*channelBuild),
				modelIdentities: make(map[modelIdentity]struct{}),
			}
			if !build.group.Unidentified {
				build.group.BaseURL = groupKey
			}
			groups[groupKey] = build
		}
		if _, seen := build.channelIds[itemKey.channelId]; !seen {
			build.channelIds[itemKey.channelId] = struct{}{}
			build.group.ChannelIds = append(build.group.ChannelIds, itemKey.channelId)
		}
		cb := build.channels[itemKey.channelId]
		if cb == nil {
			cb = &channelBuild{
				group: ProviderURLChannelGroupSummary{
					ChannelId: itemKey.channelId, ChannelName: item.ChannelName,
				},
				models:   make(map[modelKey]*ProviderURLChannelModelSummary),
				allKnown: true, usageOnly: true,
			}
			build.channels[itemKey.channelId] = cb
		}

		mk := modelKey{model: itemKey.model, mode: itemKey.mode, fallback: itemKey.fallback}
		leaf, ok := cb.models[mk]
		if !ok {
			leaf = &ProviderURLChannelModelSummary{
				UsageOnly:             item.UsageOnly,
				ProviderModel:         item.ProviderModel,
				ProviderModelFallback: item.ProviderModelFallback,
				BillingMode:           item.BillingMode,
				CustomerModels:        item.CustomerModels,
				Usage:                 item.Usage,
				DataQuality:           item.DataQuality,
				DetailFilter:          item.DetailFilter,
				OriginalAmount:        item.OriginalAmount,
				EstimateReasons:       item.EstimateReasons,
			}
			var discountProjection *ProviderBillingDiscountProjection
			if record, ok := discounts[itemKey.channelId]; ok {
				projection := providerChannelDiscountProjection(record)
				discountProjection = &projection
			}
			if item.OriginalAmount != nil && discountProjection != nil {
				leaf.ReferenceAmount = billingStatementOriginalQuota(item.referenceQuota)
			}
			if item.originalQuotaKnown {
				leaf.KnownOriginalAmount = billingStatementOriginalQuota(item.originalQuota)
				if discountProjection != nil {
					leaf.KnownReferenceAmount = billingStatementOriginalQuota(item.referenceQuota)
				}
			}
			cb.models[mk] = leaf
		}
		build.modelIdentities[modelIdentity{name: itemKey.model, fallback: itemKey.fallback}] = struct{}{}
		accumulateProviderBillingUsage(&cb.group.Usage, item.Usage)
		accumulateBillingReconciliationQuality(&cb.group.DataQuality, item.DataQuality)
		accumulateProviderBillingUsage(&build.group.Usage, item.Usage)
		accumulateBillingReconciliationQuality(&build.group.DataQuality, item.DataQuality)
		// Every pending test is a money gap, even in an otherwise usage-only leaf.
		cb.usageOnly = cb.usageOnly && item.UsageOnly
		cb.hasKnown = cb.hasKnown || item.originalQuotaKnown || item.OriginalAmount != nil
		cb.original = cb.original.Add(item.originalQuota)
		cb.reference = cb.reference.Add(item.referenceQuota)
		if item.OriginalAmount == nil {
			cb.allKnown = false
		}
		cb.estimateReasons = mergeBillingEstimateReasons(cb.estimateReasons, item.EstimateReasons...)
	}

	result := make([]ProviderURLGroupSummary, 0, len(groups))
	for _, build := range groups {
		sort.Ints(build.group.ChannelIds)
		build.group.ChannelCount = int64(len(build.channelIds))

		// Channel finals: model leaves, discount slot, amounts and completeness
		// flags; group totals only ever add the same per-leaf rounded amounts.
		knownOriginal, knownReference := decimal.Zero, decimal.Zero
		hasKnownOriginal, hasKnownReference := false, false
		groupOriginal := decimal.Zero
		groupOriginalKnown := true
		groupReference := decimal.Zero
		groupReferenceKnown := true
		for _, channelId := range build.group.ChannelIds {
			cb := build.channels[channelId]
			for _, leaf := range cb.models {
				finalizeBillingReconciliationQuality(&leaf.DataQuality)
				cb.group.Models = append(cb.group.Models, *leaf)
			}
			sort.Slice(cb.group.Models, func(i, j int) bool {
				if cb.group.Models[i].ProviderModel != cb.group.Models[j].ProviderModel {
					return cb.group.Models[i].ProviderModel < cb.group.Models[j].ProviderModel
				}
				if cb.group.Models[i].BillingMode != cb.group.Models[j].BillingMode {
					return cb.group.Models[i].BillingMode < cb.group.Models[j].BillingMode
				}
				return cb.group.Models[i].ProviderModelFallback
			})
			finalizeBillingReconciliationQuality(&cb.group.DataQuality)

			hasDiscount := false
			if record, ok := discounts[channelId]; ok {
				projection := providerChannelDiscountProjection(record)
				cb.group.Discount = &projection
				cb.group.UpdatedAt = record.UpdatedAt
				cb.group.UpdatedBy = record.UpdatedBy
				hasDiscount = true
			} else if !cb.usageOnly {
				build.group.DiscountPendingChannels++
			}
			if cb.hasKnown {
				cb.group.KnownOriginalAmount = billingStatementOriginalQuota(cb.original)
				knownOriginal = knownOriginal.Add(cb.original)
				hasKnownOriginal = true
				if hasDiscount {
					cb.group.KnownReferenceAmount = billingStatementOriginalQuota(cb.reference)
					knownReference = knownReference.Add(cb.reference)
					hasKnownReference = true
				}
			}
			channelReferenceKnown := cb.allKnown && hasDiscount
			if cb.allKnown {
				groupOriginal = groupOriginal.Add(cb.original)
			} else {
				groupOriginalKnown = false
			}
			if channelReferenceKnown {
				groupReference = groupReference.Add(cb.reference)
			} else {
				groupReferenceKnown = false
			}
			// Usage-only is a display classification, never an amount-completeness exception.
			if !cb.usageOnly && cb.allKnown {
				cb.group.OriginalAmount = billingStatementOriginalQuota(cb.original)
				if cb.group.OriginalAmount == nil {
					cb.group.EstimateReasons = mergeBillingEstimateReasons(cb.group.EstimateReasons, BillingEstimateAmountOutOfRange)
				}
			}
			if !cb.usageOnly && channelReferenceKnown {
				cb.group.ReferenceAmount = billingStatementOriginalQuota(cb.reference)
			}
			cb.group.EstimateReasons = cb.estimateReasons
			cb.group.UsageOnly = cb.usageOnly
			build.group.Channels = append(build.group.Channels, cb.group)
			build.group.EstimateReasons = mergeBillingEstimateReasons(build.group.EstimateReasons, cb.estimateReasons...)
		}
		sort.Slice(build.group.Channels, func(i, j int) bool {
			if build.group.Channels[i].ChannelName != build.group.Channels[j].ChannelName {
				return build.group.Channels[i].ChannelName < build.group.Channels[j].ChannelName
			}
			return build.group.Channels[i].ChannelId < build.group.Channels[j].ChannelId
		})
		if hasKnownOriginal {
			build.group.KnownOriginalAmount = billingStatementOriginalQuota(knownOriginal)
		}
		if hasKnownReference {
			build.group.KnownReferenceAmount = billingStatementOriginalQuota(knownReference)
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
		build.group.ModelCount = int64(len(build.modelIdentities))
		finalizeBillingReconciliationQuality(&build.group.DataQuality)
		if urlKey == "" || build.group.UrlKey == urlKey {
			result = append(result, build.group)
			accumulateBillingReconciliationQuality(&summary.DataQuality, build.group.DataQuality)
		}
	}
	sort.Slice(result, func(i, j int) bool { return providerURLGroupLess(result[i], result[j]) })

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
	for offset := 0; offset < len(channelIds); offset += 500 {
		ids := channelIds[offset:min(offset+500, len(channelIds))]
		if err := DB.Model(&Channel{}).Select("id, name, base_url, type").Where("id IN ?", ids).Limit(500).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			channels[row.Id] = Channel{Id: row.Id, Name: row.Name, BaseURL: row.BaseURL, Type: row.Type}
		}
	}
	return channels, nil
}

// scanProviderBillingURLPlatformItems shares signed customer settlement facts
// with details and exports, while provider usage includes only consume rows and
// provable task settlement usage. Keys retain provider identity and billing mode.
func scanProviderBillingURLPlatformItems(ctx context.Context, startTimestamp, endTimestamp int64, discounts map[int]ProviderChannelBillingDiscount, channelIDs []int, policy BillingStatementReadPolicy) (map[providerBillingSummaryKey]*ProviderBillingPlatformSummary, error) {
	platform := make(map[providerBillingSummaryKey]*ProviderBillingPlatformSummary)
	taskCache := newUpstreamTaskCacheTracker()
	coverage := upstreamEvidenceCoverage{}
	err := scanUpstreamBillingFacts(ctx, UpstreamBillingDetailFilter{Start: startTimestamp, End: endTimestamp, ChannelIds: channelIDs}, policy, func(_ Log, log billingReconciliationLog, parsed parsedBillingReconciliationLog) error {
		providerModel := strings.TrimSpace(parsed.providerModel)
		fallback := false
		if providerModel == "" {
			providerModel = log.ModelName
			fallback = true
		}
		itemKey := providerBillingSummaryKey{channelId: log.ChannelId, model: providerModel, mode: parsed.billingMode, fallback: fallback}
		item, ok := platform[itemKey]
		if !ok {
			if policy.MaxGroups > 0 && len(platform) >= policy.MaxGroups {
				return fmt.Errorf("upstream summary group limit exceeded")
			}
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
			secondsMissing, secondsUnitKnown := accumulateProviderBillingLog(&item.Usage, log, parsed)
			if secondsMissing {
				quality := ensureBillingReconciliationQuality(&item.DataQuality)
				quality.SecondsUnavailableRows++
				if secondsUnitKnown {
					quality.SecondsValueMissingRows++
				}
			}
			// Cache quality measures final metering rows: creation pre-holds are
			// estimates and one task's rows dedupe to a single missing count.
			if !upstreamTaskCreatePreHold(parsed) {
				if parsed.taskID != "" {
					taskCache.observe(itemKey, log.UserId, parsed)
				} else {
					accumulateUpstreamCacheQuality(&item.DataQuality, parsed)
				}
			}

		}
		discount, _ := providerChannelBillingDiscountFor(discounts, 0, log.ChannelId)
		accumulateProviderBillingItemAmount(item, log, parsed, discount.Discount)
		if fallback {
			ensureBillingReconciliationQuality(&item.DataQuality).ProviderModelFallbackRows++
		}
		if parsed.billingMode == BillingReconciliationModeUnknown {
			ensureBillingReconciliationQuality(&item.DataQuality).UnknownBillingModeRequests++
		}
		if parsed.unavailable {
			ensureBillingReconciliationQuality(&item.DataQuality).UnavailableRequests++
		}
		coverage.observe(itemKey, log, parsed)
		if len(coverage.rows) >= upstreamDetailFlushSize {
			return coverage.flush(ctx, platform)
		}
		return nil
	})
	if err == nil {
		err = coverage.flush(ctx, platform)
	}
	taskCache.flushInto(platform)

	return platform, err
}

// accumulateProviderBillingItemAmount restores the official-price amount of
// one upstream usage row using the customer statement algorithm: settled
// quota divided by the frozen group ratio and contract discount, refunds
// negative. Native channel test rows carry usage but no customer settlement,
// so they stay in the usage view, never enter the amount, and are counted
// separately. Rows whose frozen price evidence is incomplete leave the amount
// unknown instead of being dropped or zero-filled.
func accumulateProviderBillingItemAmount(item *ProviderBillingPlatformSummary, log billingReconciliationLog, parsed parsedBillingReconciliationLog, coefficient decimal.Decimal) {
	if parsed.isChannelTest && log.Type == LogTypeConsume {
		accumulateUpstreamTestAmount(item, log, parsed, coefficient)
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
		accumulateBillingEstimateReasonQuality(ensureBillingReconciliationQuality(&item.DataQuality), reasons)
		return
	}
	item.originalQuota = item.originalQuota.Add(original)
	item.referenceQuota = item.referenceQuota.Add(UpstreamReferenceQuota(original, coefficient))
	item.originalQuotaKnown = true
}

// finalizeProviderBillingItemAmount rounds an item's restored original once,
// from the accumulated decimals, and keeps estimate reasons as the quality
// marker when any money-bearing row could not be restored.
func finalizeProviderBillingItemAmount(item *ProviderBillingPlatformSummary) {
	item.UsageOnly = item.settlementRows == 0 && item.testMoneyRows == 0
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
	} else if record.CopiedFromPeriod > 0 && record.Version <= 1 {
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
	ids := make([]int, 0)
	if err := DB.Select("id, base_url, type").FindInBatches(&channels, 500, func(tx *gorm.DB, batch int) error {
		for _, channel := range channels {
			key, _ := billingURLGroupIdentity(channel, true, channel.Id)
			if key == urlKey {
				ids = append(ids, channel.Id)
			}
		}
		return nil
	}).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

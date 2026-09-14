package model

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
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
}

// ProviderURLChannelSummary exposes usage facts only; this read-only view does
// not load or imply a supplier discount.
type ProviderURLChannelSummary struct {
	ChannelId             int                               `json:"channel_id"`
	ChannelName           string                            `json:"channel_name"`
	ProviderModel         string                            `json:"provider_model"`
	CustomerModels        []string                          `json:"customer_models"`
	ProviderModelFallback bool                              `json:"provider_model_fallback,omitempty"`
	BillingMode           string                            `json:"billing_mode"`
	Usage                 ProviderBillingUsage              `json:"usage"`
	DataQuality           *BillingReconciliationDataQuality `json:"data_quality,omitempty"`
	DetailFilter          BillingReconciliationDetailFilter `json:"detail_filter"`
}

// GetProviderBillingURLSummary aggregates the same platform usage rows as
// GetProviderBillingSummary into URL groups. It deliberately performs its own
// read-only scan instead of calling GetProviderBillingSummary, because that
// function materializes monthly discounts as a side effect, which a pure
// reporting view must not trigger. The scan reuses the identical log parsing
// and accumulation helpers, so URL group totals equal the sums of the
// corresponding channel rows.
func GetProviderBillingURLSummary(startTimestamp int64, endTimestamp int64, urlKey string) (ProviderURLSummary, error) {
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

	type modelKey struct {
		model    string
		mode     string
		fallback bool
	}
	type groupBuild struct {
		group      ProviderURLGroupSummary
		models     map[modelKey]*ProviderURLModelSummary
		channelIds map[int]struct{}
	}
	groups := make(map[string]*groupBuild)

	for _, item := range items {
		finalizeBillingReconciliationQuality(&item.DataQuality)
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
					UrlKey:       groupKey,
					DisplayName:  displayName,
					Unidentified: strings.HasPrefix(groupKey, billingURLGroupChannelFallbackPrefix),
					Deleted:      strings.HasPrefix(groupKey, billingURLGroupChannelFallbackPrefix) && itemKey.channelId > 0 && !found,
					ChannelIds:   make([]int, 0),
					Models:       make([]ProviderURLModelSummary, 0),
				},
				models:     make(map[modelKey]*ProviderURLModelSummary),
				channelIds: make(map[int]struct{}),
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
		modelRow.Channels = append(modelRow.Channels, ProviderURLChannelSummary{
			ChannelId: item.ChannelId, ChannelName: item.ChannelName,
			ProviderModel: item.ProviderModel, CustomerModels: item.CustomerModels,
			ProviderModelFallback: item.ProviderModelFallback, BillingMode: item.BillingMode,
			Usage: item.Usage, DataQuality: item.DataQuality, DetailFilter: item.DetailFilter,
		})
		accumulateProviderBillingUsage(&modelRow.Usage, item.Usage)
		accumulateProviderBillingUsage(&group.group.Usage, item.Usage)
	}

	result := make([]ProviderURLGroupSummary, 0, len(groups))
	for _, build := range groups {
		sort.Ints(build.group.ChannelIds)
		build.group.ChannelCount = int64(len(build.group.ChannelIds))
		type modelIdentity struct {
			name     string
			fallback bool
		}
		modelIdentities := make(map[modelIdentity]struct{})
		for _, modelRow := range build.models {
			modelIdentities[modelIdentity{modelRow.ProviderModel, modelRow.ProviderModelFallback}] = struct{}{}
			finalizeBillingReconciliationQuality(&modelRow.DataQuality)
			sort.Slice(modelRow.Channels, func(i, j int) bool {
				if modelRow.Channels[i].ChannelName != modelRow.Channels[j].ChannelName {
					return modelRow.Channels[i].ChannelName < modelRow.Channels[j].ChannelName
				}
				return modelRow.Channels[i].ChannelId < modelRow.Channels[j].ChannelId
			})
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

// billingURLGroupIdentity resolves the URL group of one channel. Identified
// channels share the normalized current base URL; channels with a missing or
// unsafe URL, and deleted channels without any stored configuration, each
// keep an isolated per-channel group instead of merging into one unknown
// supplier.
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

// scanProviderBillingURLPlatformItems mirrors the row handling of
// GetProviderBillingSummary: consume rows plus refunds that provably carry
// task settlement usage, keyed by channel, provider model identity and
// billing mode.
func scanProviderBillingURLPlatformItems(startTimestamp int64, endTimestamp int64) (map[providerBillingSummaryKey]*ProviderBillingPlatformSummary, error) {
	query := LOG_DB.Model(&Log{}).
		Select("user_id, token_id, COALESCE(token_name, '') AS token_name, channel_id, COALESCE(model_name, '') AS model_name, type, created_at, prompt_tokens, completion_tokens, quota, COALESCE(content, '') AS content, COALESCE(other, '') AS other").
		Where("type IN ? AND created_at >= ? AND created_at <= ?", []int{LogTypeConsume, LogTypeRefund}, startTimestamp, endTimestamp)
	rows, err := query.Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	platform := make(map[providerBillingSummaryKey]*ProviderBillingPlatformSummary)
	for rows.Next() {
		var log billingReconciliationLog
		if err := rows.Scan(&log.UserId, &log.TokenId, &log.TokenName, &log.ChannelId, &log.ModelName, &log.Type, &log.CreatedAt, &log.PromptTokens, &log.CompletionTokens, &log.Quota, &log.Content, &log.Other); err != nil {
			return nil, err
		}
		parsed := parseBillingReconciliationLog(log)
		if log.Type == LogTypeRefund && !isProviderTaskUsageAdjustment(log) {
			continue
		}
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
				detailModelName: log.ModelName,
				customerModels:  make(map[string]struct{}),
			}
			platform[itemKey] = item
		} else if item.detailModelName != log.ModelName {
			item.DetailFilter.ModelName = ""
		}
		if log.ModelName != "" {
			item.customerModels[log.ModelName] = struct{}{}
		}
		accumulateProviderBillingLog(&item.Usage, log, parsed)
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
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return platform, nil
}

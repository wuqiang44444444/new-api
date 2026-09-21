package model

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/common"
	"sort"
)

const CustomerExportJobTypeUpstreamDetails = "upstream_details"
const CustomerExportJobTypeUpstreamSummary = "upstream_summary"

// Submission-time channel membership and coefficients are immutable evidence.
// TargetUserId remains zero; this scope is only admitted by the admin endpoint.
type UpstreamExportScope struct {
	AllChannels           bool   `json:"all_channels,omitempty"`
	UpperLogID            *int64 `json:"upper_log_id,omitempty"`
	EvidenceFilter        string `json:"evidence_filter,omitempty"`
	ProviderModelFallback *bool  `json:"provider_model_fallback,omitempty"`
	URLKey                string `json:"url_key,omitempty"`
	// GroupName freezes the admin-defined upstream display name at submission
	// time; regenerated jobs keep the frozen value instead of re-reading the
	// current alias.
	GroupName         string                        `json:"group_name,omitempty"`
	ChannelIds        []int                         `json:"channel_ids"`
	Channels          map[int]UpstreamExportChannel `json:"channels"`
	IncludeRootFields bool                          `json:"include_root_fields"`
}
type UpstreamExportChannel struct {
	Name     string                             `json:"name"`
	Discount *ProviderBillingDiscountProjection `json:"discount,omitempty"`
}

func AuthorizeCustomerExportJob(ctx context.Context, job *CustomerExportJob) error {
	// 用量汇总导出中视角为上游的任务 TargetUserId=0，仅管理员保持读取资格；
	// 角色降级后立即失效，与 upstream_details 相同。
	if job.JobType == CustomerExportJobTypeUsageSummary && job.TargetUserId == 0 {
		var actor User
		if err := DB.WithContext(ctx).Select("id, role, status").First(&actor, job.UserId).Error; err != nil {
			return err
		}
		if actor.Status != common.UserStatusEnabled || actor.Role < common.RoleAdminUser {
			return ErrCustomerExportNotFound
		}
		return nil
	}
	if job.JobType != CustomerExportJobTypeUpstreamDetails && job.JobType != CustomerExportJobTypeUpstreamSummary {
		return AuthorizeCustomerExport(ctx, job.UserId, job.TargetUserId)
	}
	filters, err := job.DecodeFilters()
	if err != nil {
		return err
	}
	if filters.Upstream == nil || job.TargetUserId != 0 {
		return ErrCustomerExportNotFound
	}
	global := filters.Upstream.AllChannels && job.JobType == CustomerExportJobTypeUpstreamDetails && filters.Upstream.URLKey == "" && filters.Upstream.EvidenceFilter != "" && len(filters.Upstream.ChannelIds) == 0
	if (!global && len(filters.Upstream.ChannelIds) == 0) || (filters.Upstream.AllChannels && !global) {
		return ErrCustomerExportNotFound
	}
	var actor User
	if err := DB.WithContext(ctx).Select("id, role, status").First(&actor, job.UserId).Error; err != nil {
		return err
	}
	required := common.RoleAdminUser
	if filters.Upstream.IncludeRootFields {
		required = common.RoleRootUser
	}
	if actor.Status != common.UserStatusEnabled || actor.Role < required {
		return ErrCustomerExportNotFound
	}
	return nil
}

func FreezeUpstreamExportScope(ctx context.Context, actorID int, periodStart int64, channelIDs []int, urlKey string, allChannels bool) (*UpstreamExportScope, error) {
	var actor User
	if err := DB.WithContext(ctx).Select("id, role, status").First(&actor, actorID).Error; err != nil {
		return nil, err
	}
	if actor.Status != common.UserStatusEnabled || actor.Role < common.RoleAdminUser || (len(channelIDs) == 0 && !allChannels) || (allChannels && (len(channelIDs) != 0 || urlKey != "")) {
		return nil, ErrCustomerExportNotFound
	}
	if allChannels {
		channelIDs = []int{}
	}
	upper, err := CustomerExportLogUpperBound(ctx)
	if err != nil {
		return nil, err
	}
	channels, err := freezeUpstreamExportChannels(ctx, periodStart, channelIDs, allChannels)
	if err != nil {
		return nil, err
	}
	var name ProviderURLGroupDisplayName
	if urlKey != "" {
		if err := DB.WithContext(ctx).Where("url_hash = ?", providerURLGroupHash(urlKey)).Limit(1).Find(&name).Error; err != nil {
			return nil, err
		}
	}
	scope := &UpstreamExportScope{AllChannels: allChannels, UpperLogID: &upper, URLKey: urlKey, GroupName: name.Name, ChannelIds: channelIDs, Channels: channels, IncludeRootFields: actor.Role >= common.RoleRootUser}
	return scope, nil
}

// ScanUpstreamExportSummary shares the page's accounting projection but reads
// one frozen channel at a time, under the shared export queue's batch policy.
func ScanUpstreamExportSummary(ctx context.Context, filters CustomerExportFilters, policy BillingStatementReadPolicy, consume func(int, ProviderURLChannelModelSummary) error) error {
	if filters.Upstream == nil || filters.Upstream.URLKey == "" || len(filters.Upstream.ChannelIds) == 0 {
		return errors.New("missing upstream summary scope")
	}
	policy.UpperLogID = filters.Upstream.UpperLogID
	if policy.UpperLogID == nil {
		upper, err := CustomerExportLogUpperBound(ctx)
		if err != nil {
			return err
		}
		policy.UpperLogID = &upper
	}
	for _, id := range filters.Upstream.ChannelIds {
		if err := ctx.Err(); err != nil {
			return err
		}
		discounts := make(map[int]ProviderChannelBillingDiscount)
		discount := filters.Upstream.Channels[id].Discount
		if discount != nil {
			discounts[id] = ProviderChannelBillingDiscount{ChannelId: id, PeriodStart: filters.StartTimestamp, Discount: discount.Value, Version: discount.Version}
		}
		items, err := scanProviderBillingURLPlatformItems(ctx, filters.StartTimestamp, filters.EndTimestamp-1, discounts, []int{id}, policy)
		if err != nil {
			return err
		}
		rows := make([]ProviderURLChannelModelSummary, 0, len(items))
		for _, item := range items {
			finalizeBillingReconciliationQuality(&item.DataQuality)
			finalizeProviderBillingItemAmount(item)
			row := ProviderURLChannelModelSummary{ProviderModel: item.ProviderModel, ProviderModelFallback: item.ProviderModelFallback, BillingMode: item.BillingMode, Usage: item.Usage, DataQuality: item.DataQuality, OriginalAmount: item.OriginalAmount, EstimateReasons: item.EstimateReasons, UsageOnly: item.UsageOnly}
			if item.OriginalAmount != nil && discount != nil {
				row.ReferenceAmount = billingStatementOriginalQuota(item.referenceQuota)
			}
			if item.originalQuotaKnown {
				row.KnownOriginalAmount = billingStatementOriginalQuota(item.originalQuota)
				if discount != nil {
					row.KnownReferenceAmount = billingStatementOriginalQuota(item.referenceQuota)
				}
			}
			for name := range item.customerModels {
				row.CustomerModels = append(row.CustomerModels, name)
			}
			sort.Strings(row.CustomerModels)
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool {
			if rows[i].ProviderModel != rows[j].ProviderModel {
				return rows[i].ProviderModel < rows[j].ProviderModel
			}
			if rows[i].BillingMode != rows[j].BillingMode {
				return rows[i].BillingMode < rows[j].BillingMode
			}
			return rows[i].ProviderModelFallback && !rows[j].ProviderModelFallback
		})
		for _, row := range rows {
			if err := consume(id, row); err != nil {
				return err
			}
		}
	}
	return nil
}

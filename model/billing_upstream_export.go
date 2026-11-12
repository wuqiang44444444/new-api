package model

import (
	"context"
	"errors"
	"github.com/QuantumNous/new-api/common"
)

const CustomerExportJobTypeUpstreamDetails = "upstream_details"
const CustomerExportJobTypeUpstreamSummary = "upstream_summary"

// Submission-time channel membership and coefficients are immutable evidence.
// TargetUserId remains zero; this scope is only admitted by the admin endpoint.
type UpstreamExportScope struct {
	AllChannels           bool   `json:"all_channels,omitempty"`
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
	UpstreamName string                             `json:"upstream_name,omitempty"`
	Name         string                             `json:"name"`
	Discount     *ProviderBillingDiscountProjection `json:"discount,omitempty"`
}

func AuthorizeCustomerExportJob(ctx context.Context, job *CustomerExportJob) error {
	if job.JobType == CustomerExportJobTypeCustomerStatements {
		if job.TargetUserId != 0 {
			return ErrCustomerExportNotFound
		}
		return AuthorizeCustomerStatementsExport(ctx, job.UserId)
	}
	if job.JobType == CustomerExportJobTypeUsageSummary {
		filters, err := job.DecodeFilters()
		if err != nil {
			return err
		}
		return AuthorizeUsageAnalyticsExport(ctx, job.UserId, job.TargetUserId, filters.UsageView)
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
	global := filters.Upstream.AllChannels && job.JobType == CustomerExportJobTypeUpstreamDetails && filters.Upstream.URLKey == "" && (filters.Upstream.EvidenceFilter != "" || filters.BillingReportVersion != "") && len(filters.Upstream.ChannelIds) == 0
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

// A customer-view export belongs to the admin workflow even when the target
// customer is the administrator themselves.
func AuthorizeUsageAnalyticsExport(ctx context.Context, actorID, targetID int, view string) error {
	if targetID != 0 && (view != "customer" || actorID != targetID) {
		return AuthorizeCustomerExport(ctx, actorID, targetID)
	}
	var actor User
	if err := DB.WithContext(ctx).Select("id, role, status").First(&actor, actorID).Error; err != nil {
		return err
	}
	if actor.Status != common.UserStatusEnabled || actor.Role < common.RoleAdminUser {
		return ErrCustomerExportNotFound
	}
	return nil
}

func FreezeUpstreamExportScope(ctx context.Context, actorID int, start int64, channelIDs []int, urlKey string, allChannels bool) (*UpstreamExportScope, error) {
	var actor User
	if err := DB.WithContext(ctx).Select("id, role, status").First(&actor, actorID).Error; err != nil {
		return nil, err
	}
	if actor.Status != common.UserStatusEnabled || actor.Role < common.RoleAdminUser || (len(channelIDs) == 0 && !allChannels) || (allChannels && (len(channelIDs) != 0 || urlKey != "")) {
		return nil, ErrCustomerExportNotFound
	}
	report := BillingReportFromContext(ctx)
	if report == nil {
		return nil, common.NewMessage("Billing report version is unavailable")
	}
	return report.freezeUpstreamExportScope(ctx, start, actor.Role, channelIDs, urlKey, allChannels)
}

// ScanUpstreamExportSummary shares the page's accounting projection but reads
// one frozen channel at a time, under the shared export queue's batch policy.
func ScanUpstreamExportSummary(ctx context.Context, filters CustomerExportFilters, policy BillingStatementReadPolicy, consume func(int, ProviderURLChannelModelSummary) error) error {
	if filters.Upstream == nil || len(filters.Upstream.ChannelIds) == 0 {
		return errors.New("missing upstream summary scope")
	}
	report := BillingReportFromContext(ctx)
	if report == nil {
		return common.NewMessage("Billing report version is unavailable")
	}
	for _, channelID := range filters.Upstream.ChannelIds {
		if policy.BeforeBatch != nil {
			if err := policy.BeforeBatch(ctx); err != nil {
				return err
			}
		}
		rows, err := report.upstreamRows(ctx, []int{channelID})
		if err != nil {
			return err
		}
		discounts := make(map[int]ProviderChannelBillingDiscount, 1)
		if discount := filters.Upstream.Channels[channelID].Discount; discount != nil {
			discounts[channelID] = ProviderChannelBillingDiscount{Discount: discount.Value, Version: discount.Version, Changes: discount.Changes}
		}
		summary, err := summarizeReportUpstream(rows, discounts, filters.StartTimestamp, filters.EndTimestamp-1, filters.Upstream.URLKey, true)
		if err != nil {
			return err
		}
		for _, group := range summary.Groups {
			for _, channel := range group.Channels {
				for _, leaf := range channel.Models {
					if err := consume(channel.ChannelId, leaf); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

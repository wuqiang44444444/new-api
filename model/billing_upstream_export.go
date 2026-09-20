package model

import (
	"context"
	"github.com/QuantumNous/new-api/common"
)

const CustomerExportJobTypeUpstreamDetails = "upstream_details"

// Submission-time channel membership and coefficients are immutable evidence.
// TargetUserId remains zero; this scope is only admitted by the admin endpoint.
type UpstreamExportScope struct {
	URLKey            string                        `json:"url_key,omitempty"`
	ChannelIds        []int                         `json:"channel_ids"`
	Channels          map[int]UpstreamExportChannel `json:"channels"`
	IncludeRootFields bool                          `json:"include_root_fields"`
}
type UpstreamExportChannel struct {
	Name     string                             `json:"name"`
	Discount *ProviderBillingDiscountProjection `json:"discount,omitempty"`
}

func AuthorizeCustomerExportJob(ctx context.Context, job *CustomerExportJob) error {
	if job.JobType != CustomerExportJobTypeUpstreamDetails {
		return AuthorizeCustomerExport(ctx, job.UserId, job.TargetUserId)
	}
	filters, err := job.DecodeFilters()
	if err != nil {
		return err
	}
	if filters.Upstream == nil || len(filters.Upstream.ChannelIds) == 0 || job.TargetUserId != 0 {
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

func FreezeUpstreamExportScope(ctx context.Context, actorID int, periodStart int64, channelIDs []int, urlKey string) (*UpstreamExportScope, error) {
	var actor User
	if err := DB.WithContext(ctx).Select("id, role, status").First(&actor, actorID).Error; err != nil {
		return nil, err
	}
	if actor.Status != common.UserStatusEnabled || actor.Role < common.RoleAdminUser || len(channelIDs) == 0 {
		return nil, ErrCustomerExportNotFound
	}
	channels, err := getBillingURLChannelsById(channelIDs)
	if err != nil {
		return nil, err
	}
	discounts, err := GetProviderChannelBillingDiscounts(periodStart, channelIDs)
	if err != nil {
		return nil, err
	}
	scope := &UpstreamExportScope{URLKey: urlKey, ChannelIds: channelIDs, Channels: make(map[int]UpstreamExportChannel), IncludeRootFields: actor.Role >= common.RoleRootUser}
	for _, id := range channelIDs {
		item := UpstreamExportChannel{Name: channels[id].Name}
		if record, ok := discounts[id]; ok {
			projection := providerChannelDiscountProjection(record)
			item.Discount = &projection
		}
		scope.Channels[id] = item
	}
	return scope, nil
}

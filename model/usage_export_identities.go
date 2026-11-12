package model

import "context"

// UsageOverviewExportIdentities reads the customer membership and names
// without decoding or aggregating the immutable usage metrics.
func UsageOverviewExportIdentities(ctx context.Context, refs []UsageReportDayReference, search string) ([]UsageCustomerOverviewRow, error) {
	generations := make([]string, 0, len(refs))
	for _, ref := range refs {
		generations = append(generations, ref.Generation)
	}
	var ids []int
	if err := DB.WithContext(ctx).Model(&usageReportDayRow{}).Distinct("user_id").
		Where("generation IN ? AND user_id > 0 AND overview <> ?", generations, "").
		Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	identities, err := loadUsageUserIdentities(ctx, ids)
	if err != nil {
		return nil, err
	}
	rows := make([]UsageCustomerOverviewRow, 0, len(ids))
	for _, id := range ids {
		row := UsageCustomerOverviewRow{UserId: id}
		applyUsageCustomerIdentity(&row, identities)
		if matchesUsageCustomer(row, search) {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

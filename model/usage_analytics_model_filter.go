package model

import "context"

// GetUsageCustomerModelView applies the exact customer model scope before the
// shared finalizer computes key, daily and period totals. Classification and
// historical billing evidence continue to come from the same usage pipeline.
func GetUsageCustomerModelView(ctx context.Context, period UsageAnalyticsPeriod, userID int, modelName string) (UsageCustomerView, error) {
	view := UsageCustomerView{Keys: []UsageCustomerKeyGroup{}, DayTotals: make([]UsageAnalyticsMetrics, len(period.Days))}
	agg, err := runUsageAggregation(ctx, period, usageViewOptions{UserID: userID, WantCustomer: true})
	if err != nil {
		return view, err
	}
	if modelName != "" {
		for key := range agg.customer {
			if key.model != modelName {
				delete(agg.customer, key)
			}
		}
	}
	finalizeUsageCustomerView(&view, agg)
	return view, nil
}

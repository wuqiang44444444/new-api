package model

import "context"

// GetUsageCustomerView 返回单个客户的 API Key → 客户模型用量汇总。
func GetUsageCustomerView(ctx context.Context, period UsageAnalyticsPeriod, userId int) (UsageCustomerView, error) {
	view := UsageCustomerView{Keys: []UsageCustomerKeyGroup{}, DayTotals: make([]UsageAnalyticsMetrics, len(period.Days))}
	agg, err := runUsageAggregation(ctx, period, usageViewOptions{UserID: userId, WantCustomer: true})
	if err != nil {
		return view, err
	}
	finalizeUsageCustomerView(&view, agg)
	return view, nil
}

// GetUsageUpstreamView 返回管理员上游视角的 URL → 模型 → 渠道用量汇总。
func GetUsageUpstreamView(ctx context.Context, period UsageAnalyticsPeriod) (UsageUpstreamView, error) {
	view := UsageUpstreamView{UrlGroups: []UsageUpstreamUrlGroup{}, DayTotals: make([]UsageAnalyticsMetrics, len(period.Days))}
	opts := usageViewOptions{WantUpstream: true}
	agg, err := runUsageAggregation(ctx, period, opts)
	if err != nil {
		return view, err
	}
	if err := finalizeUsageUpstreamView(ctx, &view, agg, UsageUpstreamScope{}); err != nil {
		return view, err
	}
	return view, nil
}

// GetUsageCustomersOverview 返回全客户用量列表与范围总览。
func GetUsageCustomersOverview(ctx context.Context, period UsageAnalyticsPeriod, search string) (UsageCustomersOverview, error) {
	overview := UsageCustomersOverview{Customers: []UsageCustomerOverviewRow{}, DayTotals: make([]UsageAnalyticsMetrics, len(period.Days))}
	agg, err := runUsageAggregation(ctx, period, usageViewOptions{WantOverview: true})
	if err != nil {
		return overview, err
	}
	if err := finalizeUsageCustomersOverview(ctx, &overview, agg, search); err != nil {
		return overview, err
	}
	return overview, nil
}

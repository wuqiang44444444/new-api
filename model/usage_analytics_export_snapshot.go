package model

import "context"

// A non-nil snapshot freezes both configured records and absent/default rows.
// It is stored only on admin usage exports; customer exports never carry it.
type UsageAnalyticsDiscountSnapshot struct {
	Months map[int64]map[int]ProviderChannelBillingDiscount `json:"months"`
}

func FreezeUsageAnalyticsDiscounts(ctx context.Context, period UsageAnalyticsPeriod) (*UsageAnalyticsDiscountSnapshot, error) {
	bundle, err := loadUsageDiscountBundle(ctx, period)
	if err != nil {
		return nil, err
	}
	return &UsageAnalyticsDiscountSnapshot{Months: bundle.byMonth}, nil
}

package model

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

type ProviderDiscountChange struct {
	EffectiveFrom int64           `json:"effective_from"`
	Value         decimal.Decimal `json:"value"`
}

type ProviderDiscountChanges []ProviderDiscountChange

func (ProviderDiscountChanges) GormDataType() string { return "text" }

func (changes ProviderDiscountChanges) Value() (driver.Value, error) {
	if len(changes) == 0 {
		return nil, nil
	}
	data, err := common.Marshal(changes)
	return string(data), err
}

func (changes *ProviderDiscountChanges) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*changes = nil
		return nil
	case string:
		return common.UnmarshalJsonStr(v, changes)
	case []byte:
		return common.Unmarshal(v, changes)
	default:
		return fmt.Errorf("invalid channel discount changes type %T", value)
	}
}

func (changes ProviderDiscountChanges) valueAt(base decimal.Decimal, timestamp int64) decimal.Decimal {
	for _, change := range changes {
		if change.EffectiveFrom > timestamp {
			break
		}
		base = change.Value
	}
	return base
}

func (projection ProviderBillingDiscountProjection) ValueAt(timestamp int64) decimal.Decimal {
	return projection.Changes.valueAt(projection.Value, timestamp)
}

func validateProviderDiscountEffectiveDate(month, date int64) error {
	if date == 0 {
		return nil
	}
	start := time.Unix(month, 0).In(UsageAnalyticsZone())
	day := time.Unix(date, 0).In(UsageAnalyticsZone())
	if month <= 0 || month != usageMonthStart(month) || date < month ||
		!day.Before(start.AddDate(0, 1, 0)) ||
		date != time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location()).Unix() {
		return errors.New("discount effective date must be a day in the selected billing month")
	}
	return nil
}

// A save replaces the coefficient from the chosen day to month end.
func applyProviderDiscountChange(target *ProviderChannelBillingDiscount, base ProviderChannelBillingDiscount, date int64) error {
	target.Changes = nil
	if date <= target.PeriodStart {
		return nil
	}
	if base.PendingReason != "" {
		return errors.New("configure a discount for the entire billing month before adding a dated change")
	}
	value := target.Discount
	target.Discount = base.Discount
	for _, change := range base.Changes {
		if change.EffectiveFrom >= date {
			break
		}
		target.Changes = append(target.Changes, change)
	}
	if !target.Changes.valueAt(target.Discount, date).Equal(value) {
		target.Changes = append(target.Changes, ProviderDiscountChange{EffectiveFrom: date, Value: value})
	}
	return nil
}

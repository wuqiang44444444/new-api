package helper

import (
	"fmt"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

// AttachFrozenExchangeRate resolves the configured CNY/USD rate once and
// attaches it to the billing request input when the expression depends on
// usd_exchange_rate(). Rate-free expressions gain no dependency. Callers
// freeze the same immutable fact into their billing snapshot so settlement
// and recovery never re-read the current setting; a missing or invalid rate
// fails closed before funds are held.
func AttachFrozenExchangeRate(expr string, input *billingexpr.RequestInput, snapshot *billingexpr.BillingSnapshot) error {
	if !billingexpr.UsesExchangeRate(expr) {
		return nil
	}
	if snapshot != nil {
		input.ExchangeRate = snapshot.UsdExchangeRate
		return input.ExchangeRate.Validate()
	}
	if input.ExchangeRate != nil {
		return input.ExchangeRate.Validate()
	}
	rate, err := operation_setting.CurrentUsdExchangeRateContext()
	if err != nil {
		return fmt.Errorf("expression requires a valid USDExchangeRate setting: %w", err)
	}
	input.ExchangeRate = rate
	return nil
}

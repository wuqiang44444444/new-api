package operation_setting

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

// usdRateState is the atomic projection of the persisted USDExchangeRate
// option. One immutable state per write keeps concurrent billing reads
// consistent. The legacy USDExchangeRate float var keeps serving display and
// payment consumers (logger, top-up conversion); billing must use the
// controlled reader below.
type usdRateState struct {
	rate  float64
	valid bool
}

var usdRateStateRef atomic.Value // stores usdRateState

func init() {
	usdRateStateRef.Store(usdRateState{rate: USDExchangeRate, valid: true})
}

// SetUSDExchangeRate parses, validates and stores one option value. It is
// the single write entry reached from model/option.go, covering API saves,
// bulk imports and the startup/periodic database load. An invalid value is
// stored as an invalid state (never the zero value or the last good rate)
// so billing evaluation fails closed instead of guessing.
func SetUSDExchangeRate(value string) error {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		usdRateStateRef.Store(usdRateState{})
		return fmt.Errorf("USDExchangeRate %q is not a number: %w", value, err)
	}
	if err := billingexpr.ValidateExchangeRateValue(parsed); err != nil {
		usdRateStateRef.Store(usdRateState{})
		return err
	}
	USDExchangeRate = parsed
	usdRateStateRef.Store(usdRateState{rate: parsed, valid: true})
	return nil
}

// ValidateUSDExchangeRate checks a candidate option value without storing
// it. Used by the option write path so invalid API writes are rejected
// before persistence.
func ValidateUSDExchangeRate(value string) error {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return fmt.Errorf("USDExchangeRate %q is not a number: %w", value, err)
	}
	return billingexpr.ValidateExchangeRateValue(parsed)
}

// CurrentUsdExchangeRateContext resolves the configured CNY/USD rate into an
// immutable frozen context for one billing run. It errors when the option
// is missing or invalid so expressions calling usd_exchange_rate() fail
// closed at pre-consume instead of billing with any fallback value.
func CurrentUsdExchangeRateContext() (*billingexpr.ExchangeRateContext, error) {
	state := usdRateStateRef.Load().(usdRateState)
	if !state.valid {
		return nil, errors.New("USDExchangeRate option is invalid; fix it before models using usd_exchange_rate() can bill")
	}
	return billingexpr.NewExchangeRateContext(state.rate, time.Now().UTC())
}

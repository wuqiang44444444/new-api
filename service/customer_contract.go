package service

import (
	"errors"
	"fmt"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"strings"
)

var (
	ErrCustomerContractUnavailable = errors.New("customer contract unavailable")
)

// ParseCustomerContractRatio accepts the admin UI's decimal, percentage and
// Chinese-discount notation, then returns the canonical eight-decimal fixed
// point value used by persistence and billing.
func ParseCustomerContractRatio(raw string) (int64, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, fmt.Errorf("contract ratio is required")
	}
	divisor := decimal.NewFromInt(1)
	switch {
	case strings.HasSuffix(value, "%"):
		value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
		divisor = decimal.NewFromInt(100)
	case strings.HasSuffix(value, "折"):
		value = strings.TrimSpace(strings.TrimSuffix(value, "折"))
		divisor = decimal.NewFromInt(10)
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return 0, fmt.Errorf("invalid contract ratio: %w", err)
	}
	parsed = parsed.Div(divisor)
	if !parsed.GreaterThan(decimal.Zero) || parsed.GreaterThan(decimal.NewFromInt(1)) {
		return 0, fmt.Errorf("contract ratio must be greater than zero and no greater than one")
	}
	scaled := parsed.Mul(decimal.NewFromInt(hosttypes.CustomerContractRatioScale))
	if !scaled.Equal(scaled.Truncate(0)) {
		return 0, fmt.Errorf("contract ratio supports at most eight decimal places")
	}
	units := scaled.IntPart()
	if units <= 0 || units > hosttypes.CustomerContractRatioScale {
		return 0, fmt.Errorf("contract ratio is outside the supported range")
	}
	return units, nil
}

func FormatCustomerContractRatio(units int64) (string, error) {
	if units <= 0 || units > hosttypes.CustomerContractRatioScale {
		return "", fmt.Errorf("invalid contract ratio units")
	}
	return decimal.NewFromInt(units).
		Div(decimal.NewFromInt(hosttypes.CustomerContractRatioScale)).
		String(), nil
}

func ApplyCustomerContractRatio(value decimal.Decimal, fact *hosttypes.ContractBillingFact) (decimal.Decimal, error) {
	if fact == nil {
		return value, nil
	}
	ratio := fact.RatioDecimal()
	if ratio.IsZero() {
		return decimal.Zero, fmt.Errorf("invalid customer contract billing fact")
	}
	return value.Mul(ratio), nil
}

package service

import (
	"testing"

	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCustomerContractRatio(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{name: "decimal", input: "0.8", want: 80_000_000},
		{name: "one", input: "1", want: 100_000_000},
		{name: "eight decimals", input: "0.12345678", want: 12_345_678},
		{name: "smallest unit", input: "0.00000001", want: 1},
		{name: "percentage", input: "80%", want: 80_000_000},
		{name: "percentage fractional", input: "12.345678%", want: 12_345_678},
		{name: "chinese discount", input: "8折", want: 80_000_000},
		{name: "chinese fractional discount", input: "6.5折", want: 65_000_000},
		{name: "empty", input: "", wantErr: true},
		{name: "zero", input: "0", wantErr: true},
		{name: "negative", input: "-0.1", wantErr: true},
		{name: "markup", input: "1.00000001", wantErr: true},
		{name: "nine decimals", input: "0.123456789", wantErr: true},
		{name: "too precise percent", input: "0.0000001%", wantErr: true},
		{name: "nan", input: "NaN", wantErr: true},
		{name: "positive infinity", input: "Inf", wantErr: true},
		{name: "invalid", input: "discount", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseCustomerContractRatio(test.input)
			if test.wantErr {
				require.Error(t, err)
				assert.Zero(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
			formatted, err := FormatCustomerContractRatio(got)
			require.NoError(t, err)
			reparsed, err := ParseCustomerContractRatio(formatted)
			require.NoError(t, err)
			assert.Equal(t, got, reparsed)
		})
	}
}

func TestApplyCustomerContractRatioPreservesNativePathAndUsesExactFixedPoint(t *testing.T) {
	value := decimal.RequireFromString("69.6")
	native, err := ApplyCustomerContractRatio(value, nil)
	require.NoError(t, err)
	assert.True(t, value.Equal(native))

	discounted, err := ApplyCustomerContractRatio(decimal.NewFromInt(87), &hosttypes.ContractBillingFact{RatioUnits: 80_000_000})
	require.NoError(t, err)
	assert.True(t, decimal.RequireFromString("69.6").Equal(discounted))

	_, err = ApplyCustomerContractRatio(value, &hosttypes.ContractBillingFact{RatioUnits: 0})
	require.Error(t, err)
	_, err = ApplyCustomerContractRatio(value, &hosttypes.ContractBillingFact{RatioUnits: hosttypes.CustomerContractRatioScale + 1})
	require.Error(t, err)
}

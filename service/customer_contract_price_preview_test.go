package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContractPricePreviewDistinguishesMissingAndZeroPrices(t *testing.T) {
	for _, tc := range []struct {
		name        string
		pricing     model.Pricing
		missing     bool
		mode, final string
	}{
		{name: "absent", missing: true},
		{name: "native fallback", pricing: model.Pricing{ModelRatio: 37.5}, missing: true},
		{name: "configured zero ratio", pricing: model.Pricing{BasisPriceConfigured: true}, mode: "per_token", final: "0"},
		{name: "configured ratio", pricing: model.Pricing{BasisPriceConfigured: true, ModelRatio: 2}, mode: "per_token", final: "1.6"},
		{name: "free per call", pricing: model.Pricing{QuotaType: 1}, mode: "per_call", final: "0"},
		{name: "paid per call", pricing: model.Pricing{QuotaType: 1, ModelPrice: 2}, mode: "per_call", final: "1.6"},
		{name: "expression without ratio", pricing: model.Pricing{BillingMode: "tiered_expr", BillingExpr: `tier("base", p*2)`}, mode: "tiered_expr"},
		{name: "missing expression", pricing: model.Pricing{BillingMode: "tiered_expr"}, missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			price := buildCustomerContractPricePreview(tc.pricing, decimal.RequireFromString("0.8"))
			if tc.missing {
				assert.Nil(t, price)
				return
			}
			require.NotNil(t, price)
			assert.Equal(t, tc.mode, price.BillingMode)
			assert.Equal(t, tc.final, price.CurrentDiscountedPrice)
		})
	}
}

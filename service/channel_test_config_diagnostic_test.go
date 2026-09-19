package service

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiagnoseChannelTestConfigFailureDoesNotInferFromText(t *testing.T) {
	cause := errors.New("model price not configured; fixture-secret; https://example.invalid/?signature=fixture")
	apiErr := types.NewError(cause, types.ErrorCodeModelPriceError)
	diagnostic := DiagnoseChannelTestConfigFailure(cause, apiErr)
	require.NotNil(t, diagnostic)
	assert.Equal(t, "billing_validation_failed", diagnostic.Reason)
	assert.Equal(t, "Billing validation failed. Check the billing model settings.", diagnostic.Summary)
	assert.Empty(t, diagnostic.BillingModel)
	assert.Nil(t, DiagnoseChannelTestConfigFailure(cause, types.NewError(cause, types.ErrorCodeDoRequestFailed)))
	assert.Nil(t, DiagnoseChannelTestConfigFailure(nil, nil))
}

func TestDiagnoseChannelTestConfigFailurePreservesTypedCauseAndSafeSummary(t *testing.T) {
	for _, tc := range []struct{ reason, entry string }{
		{"price_not_configured", "model_pricing"},
		{"image_price_not_configured", "model_pricing"},
		{"billing_expr_missing", "billing_expression"},
		{"billing_expr_failed", "billing_expression"},
		{"billing_usage_unavailable", "billing_expression"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			cause := errors.New("fixture-secret price not configured")
			wrapped := common.NewBillingConfigError(tc.reason, "customer-billing-model", cause)
			apiErr := types.NewError(wrapped, types.ErrorCodeModelPriceError)
			diagnostic := DiagnoseChannelTestConfigFailure(nil, apiErr)
			require.NotNil(t, diagnostic)
			assert.Equal(t, tc.reason, diagnostic.Reason)
			assert.Equal(t, tc.entry, diagnostic.Entry)
			assert.Equal(t, "customer-billing-model", diagnostic.BillingModel)
			assert.NotContains(t, diagnostic.Summary, "fixture-secret")
			assert.Equal(t, cause.Error(), wrapped.Error())
			assert.ErrorIs(t, wrapped, cause)
		})
	}
}

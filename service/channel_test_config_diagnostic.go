package service

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

// Diagnostic text is a fixed translation key, never an excerpt of an error,
// expression, provider response, model mapping or other configuration input.
type ChannelTestConfigDiagnostic struct {
	Reason       string
	Entry        string
	Summary      string
	BillingModel string
}

func DiagnoseChannelTestConfigFailure(localErr error, apiError *types.NewAPIError) *ChannelTestConfigDiagnostic {
	var billing *common.BillingConfigError
	var clamp *common.QuotaClamp
	diagnostic := &ChannelTestConfigDiagnostic{}
	switch {
	case errors.As(localErr, &billing) || (apiError != nil && errors.As(apiError, &billing)):
		diagnostic.Reason, diagnostic.BillingModel = billing.Reason, billing.Model
	case errors.As(localErr, &clamp) || (apiError != nil && errors.As(apiError, &clamp)):
		diagnostic.Reason = "billing_input_invalid"
	case apiError != nil:
		switch apiError.GetErrorCode() {
		case types.ErrorCodeModelPriceError:
			diagnostic.Reason = "billing_validation_failed"
		case types.ErrorCodeChannelModelMappedError:
			diagnostic.Reason = "model_mapping_invalid"
		case types.ErrorCodeChannelParamOverrideInvalid:
			diagnostic.Reason = "parameter_override_invalid"
		case types.ErrorCodeInvalidApiType:
			diagnostic.Reason = "protocol_invalid"
		default:
			return nil
		}
	default:
		return nil
	}
	switch diagnostic.Reason {
	case "price_not_configured":
		diagnostic.Entry, diagnostic.Summary = "model_pricing", "Configure a price for the billing model."
	case "image_price_not_configured":
		diagnostic.Entry, diagnostic.Summary = "model_pricing", "Configure a per-image price or a billing expression."
	case "billing_expr_missing", "billing_expr_required":
		diagnostic.Entry, diagnostic.Summary = "billing_expression", "Configure the required billing expression."
	case "billing_expr_failed":
		diagnostic.Entry, diagnostic.Summary = "billing_expression", "Check the billing expression and its required inputs."
	case "billing_usage_unavailable":
		diagnostic.Entry, diagnostic.Summary = "billing_expression", "The adapter cannot provide the usage required by this expression."
	case "billing_input_invalid":
		diagnostic.Entry, diagnostic.Summary = "model_pricing", "Check billing inputs and quota limits."
	case "model_mapping_invalid":
		diagnostic.Entry, diagnostic.Summary = "channel_edit", "Correct the channel model mapping."
	case "parameter_override_invalid":
		diagnostic.Entry, diagnostic.Summary = "channel_edit", "Correct the channel parameter overrides."
	case "protocol_invalid":
		diagnostic.Entry, diagnostic.Summary = "channel_edit", "Check the channel protocol configuration."
	default:
		diagnostic.Reason = "billing_validation_failed"
		diagnostic.Entry, diagnostic.Summary = "model_pricing", "Billing validation failed. Check the billing model settings."
	}
	return diagnostic
}

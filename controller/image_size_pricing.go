package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ImageSizePricing projects a price grid or builds a draft for the existing
// versioned pricing API. This endpoint never changes a stored price.
func ImageSizePricing(c *gin.Context) {
	var request struct {
		Expression string             `json:"expression"`
		Profile    string             `json:"profile"`
		Prices     map[string]float64 `json:"prices"`
		Migrate    bool               `json:"migrate"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiError(c, err)
		return
	}
	if len(request.Expression) > service.BillingExprPreviewMaxExpressionLength {
		common.ApiErrorMsg(c, "expression exceeds the length limit")
		return
	}
	if request.Profile != "" {
		expression, err := billingexpr.GenerateImageSizePrices(request.Profile, request.Prices)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, gin.H{"expression": expression})
		return
	}
	table := billingexpr.ReadImageSizePrices(request.Expression)
	legacy, warnings := billingexpr.InspectImagePrices(request.Expression)
	if table == nil && request.Migrate {
		table = legacy
	}
	common.ApiSuccess(c, gin.H{"table": table, "profiles": billingexpr.ImageSizePriceProfiles(),
		"importable": legacy != nil, "warnings": warnings})
}

func imagePricingWarnings(expressions map[string]string) map[string][]billingexpr.ImagePriceWarning {
	var result map[string][]billingexpr.ImagePriceWarning
	for name, expression := range expressions {
		if warnings := billingexpr.DiagnoseImagePrices(expression); len(warnings) != 0 {
			if result == nil {
				result = make(map[string][]billingexpr.ImagePriceWarning)
			}
			result[name] = warnings
		}
	}
	return result
}

func writeOptionUpdateSuccess(c *gin.Context, key, raw string) {
	result := gin.H{"success": true, "message": ""}
	if key == "billing_setting.billing_expr" {
		var expressions map[string]string
		if common.UnmarshalJsonStr(raw, &expressions) == nil {
			if warnings := imagePricingWarnings(expressions); len(warnings) > 0 {
				result["pricing_warnings"] = warnings
			}
		}
	}
	c.JSON(http.StatusOK, result)
}

func writeModelPricingUpdateSuccess(c *gin.Context, names []string, changes []model.ModelPricingChange) {
	result := gin.H{"updated_models": names}
	expressions := make(map[string]string)
	for _, change := range changes {
		if expression, ok := change.Pricing["billing_setting.billing_expr"].(string); ok && !change.Reset {
			expressions[change.ModelName] = expression
		}
	}
	if warnings := imagePricingWarnings(expressions); len(warnings) > 0 {
		result["pricing_warnings"] = warnings
	}
	common.ApiSuccess(c, result)
}

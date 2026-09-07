package helper

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// Normalize image-count probes before pre-consume for JSON and multipart alike.
// The expression remains the sole price source; no fee is hardcoded here.
func prepareImageRelayBilling(c *gin.Context, info *relaycommon.RelayInfo) error {
	request, ok := info.Request.(*dto.ImageRequest)
	if !ok || common.GetContextKeyInt(c, constant.ContextKeyChannelType) != constant.ChannelTypeAsyncImage {
		return nil
	}
	mapped := *request
	mappedInfo := *info
	mappedInfo.InitChannelMeta(c)
	if err := ModelMappedHelper(c, &mappedInfo, &mapped); err != nil {
		return err
	}
	contract, apiErr := service.ParseImageRelayContract(c, &mappedInfo, &mapped, mappedInfo.ChannelOtherSettings.ImageUpstreamProtocol)
	if apiErr != nil {
		return apiErr
	}
	if billing_setting.GetBillingMode(info.GetBillingModelName()) == billing_setting.BillingModeTieredExpr {
		expr, _ := billing_setting.GetBillingExpr(info.GetBillingModelName())
		if billingexpr.RequiresUsage(expr) {
			return errors.New("image relay requires per-image pricing; usage-dependent expressions are not supported")
		}
	}
	if billing_setting.GetBillingMode(info.GetBillingModelName()) != billing_setting.BillingModeTieredExpr {
		if _, priced := ratio_setting.GetModelPrice(info.GetBillingModelName(), false); !priced {
			return errors.New("image relay requires a per-image price or billing expression")
		}
	}
	pro := constant.ImageRelayRequiresInputPricing(mappedInfo.ChannelOtherSettings.ImageUpstreamProtocol, mapped.Model)
	if pro && len(contract.Images) > 1 && billing_setting.GetBillingMode(info.GetBillingModelName()) != billing_setting.BillingModeTieredExpr {
		return errors.New("multiple Pro reference images require an input-image billing expression")
	}
	body, err := service.ImageRelayBillingBody(request, len(contract.Images))
	if err != nil {
		return err
	}
	info.BillingRequestInput = &billingexpr.RequestInput{Body: body, Headers: info.RequestHeaders}
	return nil
}

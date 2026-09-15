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
	// InitChannelMeta resets its request model; keep that reset on the pricing copy.
	mappedInfo.Request = &mapped
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
			// 当前图片中转适配尚未支持用量表达式结算；部分协议已返回部分用量，
			// 因此不能将适配缺口描述为所有 Provider 均不提供用量，也不能伪造缺失事实。
			return errors.New("the current image adapter cannot supply the verified usage required by this billing expression")
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

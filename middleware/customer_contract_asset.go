package middleware

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func CustomerContractAssetAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		if service.ActiveCustomerContract(c) == nil {
			c.Next()
			return
		}
		publicModel := strings.TrimSpace(c.Query("model"))
		if c.Request.Method == http.MethodPost || c.Request.Method == http.MethodPatch {
			var request struct {
				Model string `json:"model"`
			}
			if err := common.UnmarshalBodyReusable(c, &request); err != nil {
				c.Next()
				return
			}
			publicModel = strings.TrimSpace(request.Model)
		}
		if _, err := applyCustomerContractRequest(c, publicModel); err != nil {
			abortTokenModelAccess(c, publicModel, "合同不允许此模型 / Model is not allowed by the contract")
			return
		}
		pinID := 0
		if pin, ok, _ := service.GetChannelConstraints(c).ResolvedPin(); ok {
			pinID = pin.ChannelId
		}
		if _, _, err := service.CustomerContractTypedChannel(c, publicModel, constant.ChannelTypeSeedanceLink, pinID); err != nil {
			abortTokenModelAccess(c, publicModel, "合同范围内无可用渠道 / No available channel in this contract")
			return
		}
		c.Next()
	}
}

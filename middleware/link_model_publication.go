package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func ResolveLinkModelPublication(routeFamily model.LinkRouteFamily) gin.HandlerFunc {
	return func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyLinkContractNamespace, model.LinkContractNamespaceDefault)
		common.SetContextKey(c, constant.ContextKeyLinkRouteFamily, string(routeFamily))
		if c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		request, shouldSelectChannel, err := getModelRequest(c)
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusBadRequest, err.Error())
			return
		}
		if !shouldSelectChannel || request == nil || strings.TrimSpace(request.Model) == "" {
			c.Next()
			return
		}
		publication, err := model.GetLinkModelPublication(model.LinkContractNamespaceDefault, routeFamily, request.Model)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.Next()
			return
		}
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "Link customer model publication is temporarily unavailable")
			return
		}
		conflict, err := model.LinkPublicationHasOrdinaryConflict(publication, common.GetContextKeyString(c, constant.ContextKeyUsingGroup))
		if err != nil {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "Link customer model publication could not be verified")
			return
		}
		if conflict {
			abortWithOpenAiMessage(c, http.StatusServiceUnavailable, "Link customer model conflicts with an ordinary channel in the selected routing scope")
			return
		}
		common.SetContextKey(c, constant.ContextKeyLinkModelPublication, *publication)
		common.SetContextKey(c, constant.ContextKeyPublishedLinkContractSKU, publication.LinkSKU)
		common.SetContextKey(c, constant.ContextKeyLinkPublicationVersion, publication.PublicationVersion)
		c.Next()
	}
}

func resolvedLinkModelPublication(c *gin.Context) (model.LinkModelPublication, bool) {
	return common.GetContextKeyType[model.LinkModelPublication](c, constant.ContextKeyLinkModelPublication)
}

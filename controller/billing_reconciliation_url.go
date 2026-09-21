package controller

import (
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// maxBillingURLKeyFilterLength bounds the upstream URL grouping filter accepted
// by the unified upstream reconciliation endpoint.
const maxBillingURLKeyFilterLength = 2048

type upstreamURLGroupNameRequest struct {
	URLKey string `json:"url_key"`
	Name   string `json:"name"`
}

// PutAdminUpstreamURLGroupName stores or clears the admin-defined display name
// of one upstream URL grouping key. The name only replaces the card title of
// the reporting view: it never re-groups channels and never feeds routing,
// billing or settlement. An empty name restores the default safe-URL label;
// the write and its audit row commit in the same transaction.
func PutAdminUpstreamURLGroupName(c *gin.Context) {
	var request upstreamURLGroupNameRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.URLKey == "" ||
		len(request.URLKey) > maxBillingURLKeyFilterLength ||
		utf8.RuneCountInString(strings.TrimSpace(request.Name)) > model.MaxProviderURLGroupNameLength {
		common.ApiErrorMsg(c, "invalid upstream name")
		return
	}
	if err := model.SaveProviderURLGroupName(request.URLKey, request.Name, c.GetInt("id")); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

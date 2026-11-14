package clienterrlog

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const (
	WalletQuotaInsufficient       = "wallet_quota_insufficient"
	SubscriptionQuotaInsufficient = "subscription_quota_insufficient"
	APIKeyQuotaInsufficient       = "api_key_quota_insufficient"
)

// RecordAPIKeyQuotaRejection observes a matched credential rejected before
// auth completes. It deliberately does not mark auth as passed or read bodies.
func RecordAPIKeyQuotaRejection(c *gin.Context) {
	userID := c.GetInt("id")
	module := moduleField(c)
	if userID <= 0 || !persistedModules[module] {
		return
	}
	SubmitBackendEvent(BackendEvent{
		EventType: EventAPIError, Module: module, UserID: userID,
		RequestID: c.GetString(common.RequestIdKey), Method: c.Request.Method,
		Route: routeTemplateField(c), Status: c.Writer.Status(),
		Stage: "billing_preconsume", Reason: APIKeyQuotaInsufficient,
	})
}

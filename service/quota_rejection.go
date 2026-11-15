package service

import (
	"errors"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// quotaRejection preserves the public error message while carrying the
// confirmed failure source to the final diagnostic boundary.
type quotaRejection struct {
	error
	reason string
}

func (err quotaRejection) Unwrap() error { return err.error }

func ObserveQuotaRejection(c *gin.Context, err error) bool {
	var rejected quotaRejection
	var reason string
	switch {
	case errors.As(err, &rejected):
		reason = rejected.reason
	case errors.Is(err, model.ErrTaskAttemptTokenQuota):
		reason = clienterrlog.APIKeyQuotaInsufficient
	case errors.Is(err, model.ErrTaskAttemptWalletFallbackRejected):
		reason = clienterrlog.WalletQuotaInsufficient
	case errors.Is(err, model.ErrTaskAttemptSubscriptionUnavailable):
		reason = clienterrlog.SubscriptionQuotaInsufficient
	case errors.Is(err, model.ErrTaskAttemptInsufficientQuota):
		reason = clienterrlog.WalletQuotaInsufficient
	default:
		return false
	}
	clienterrlog.Attach(c.Request.Context(), clienterrlog.Report{
		Stage: "billing_preconsume", Reason: reason,
	})
	if reason == clienterrlog.WalletQuotaInsufficient {
		enqueueWalletRejectionNotice(c.GetInt("id"))
	}
	return true
}

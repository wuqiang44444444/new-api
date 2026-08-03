package service

import (
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// FinalizeTaskCreateAttemptBillingTransfer mirrors the already committed
// attempt-to-Task transfer into the request-local BillingSession. It performs
// no database mutation; the funds and Task quota were committed atomically by
// model.InsertTaskWithCreateAttempt.
func FinalizeTaskCreateAttemptBillingTransfer(
	info *relaycommon.RelayInfo,
	actualQuota int,
) {
	if info == nil || actualQuota < 0 {
		return
	}
	session, ok := info.Billing.(*BillingSession)
	if !ok {
		return
	}

	session.mu.Lock()
	delta := actualQuota - session.preConsumedQuota
	switch funding := session.funding.(type) {
	case *WalletFunding:
		funding.consumed = actualQuota
	case *SubscriptionFunding:
		funding.amount = int64(actualQuota)
		funding.preConsumed = int64(actualQuota)
		funding.AmountUsedAfter += int64(delta)
	}
	session.preConsumedQuota = actualQuota
	if info.IsPlayground {
		session.tokenConsumed = 0
	} else {
		session.tokenConsumed = actualQuota
	}
	session.extraReserved = 0
	session.fundingSettled = true
	session.settled = true
	session.syncRelayInfo()
	session.mu.Unlock()

	if actualQuota == 0 {
		return
	}
	if info.BillingSource == BillingSourceSubscription {
		checkAndSendSubscriptionQuotaNotify(info)
	} else {
		checkAndSendQuotaNotify(info, delta, actualQuota-delta)
	}
}

package model

import "github.com/QuantumNous/new-api/common"

// Only the documented old note shape is readable as legacy history. Unknown or
// damaged administrative decisions block writes; they are never discarded.
func validBillingSourceAudit(d BillingSourceDecisionRecord, raw string, user int, start int64) bool {
	if d.IssueID == "" || d.Fingerprint == "" {
		return false
	}
	if d.Version == 0 {
		var legacy struct {
			Accept *bool `json:"accept"`
		}
		return common.UnmarshalJsonStr(raw, &legacy) == nil && legacy.Accept != nil
	}
	if d.Version != 2 && d.Version != BillingSourceDecisionVersion {
		return false
	}
	if d.UserID != user || d.PeriodStart != start || d.ActorID <= 0 || len(d.RequestID) < 16 || d.StatementDelta != 0 || d.BalanceDelta != 0 {
		return false
	}
	switch d.Decision {
	case BillingSourceKeep:
		return d.Status == "record_kept" && !d.Blocking && d.Reason == "accept_missing_details"
	case BillingSourceInvestigate:
		return d.Status == "pending_review" && d.Note != "" && (d.Reason == "amount_questioned" || d.Reason == "missing_evidence")
	case BillingSourceWaiver:
		return d.Status == "pending_review" && d.Note != "" && (d.Reason == "customer_agreement" || d.Reason == "service_issue")
	case BillingSourceDuplicate:
		return d.Status == "pending_review" && d.Note != "" && d.Reason == "suspected_duplicate"
	case BillingSourcePeriod:
		return d.Status == "pending_review" && d.Note != "" && d.Reason == "wrong_period"
	case BillingSourceWithdraw:
		return d.Version == BillingSourceDecisionVersion && d.Status == "request_withdrawn" && d.Reason == "request_withdrawn" && d.Note != "" && d.PreviousID > 0
	case BillingSourceReject:
		return d.Version == BillingSourceDecisionVersion && d.Status == "request_rejected" && d.Reason == "request_rejected" && d.Note != "" && d.PreviousID > 0
	}
	return false
}

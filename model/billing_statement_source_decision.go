package model

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Decisions are administrative instructions, never a second financial ledger.
// Adjustment requests remain pending until the authoritative correction runs.
const (
	BillingSourceKeep            = "keep_record"
	BillingSourceInvestigate     = "investigate"
	BillingSourceWaiver          = "request_waiver"
	BillingSourceDuplicate       = "request_duplicate_exclusion"
	BillingSourcePeriod          = "request_period_correction"
	BillingSourceWithdraw        = "withdraw_request"
	BillingSourceReject          = "reject_request"
	BillingSourceDecisionVersion = 3
)

type BillingSourceReviewDecision struct {
	Fingerprint string `json:"fingerprint"`
	IssueID     string `json:"issue_id"`
	Decision    string `json:"decision"`
	Reason      string `json:"reason"`
	Note        string `json:"note"`
	RequestID   string `json:"request_id"`
	PreviousID  int64  `json:"previous_id"`
}

type BillingSourceDecisionRecord struct {
	BillingSourceReviewDecision
	Version        int      `json:"version"`
	ID             int64    `json:"id"`
	ActorID        int      `json:"actor_id"`
	CreatedAt      int64    `json:"created_at"`
	UserID         int      `json:"user_id"`
	PeriodStart    int64    `json:"period_start"`
	Model          string   `json:"model"`
	LogType        int      `json:"log_type"`
	RecordedQuota  int64    `json:"recorded_quota,string"`
	Kind           string   `json:"kind"`
	Blocking       bool     `json:"blocking"`
	TargetQuota    *int64   `json:"target_quota,string,omitempty"`
	EvidenceGaps   []string `json:"evidence_gaps"`
	Status         string   `json:"status"`
	StatementDelta int64    `json:"statement_delta,string"`
	BalanceDelta   int64    `json:"balance_delta,string"`
	CurrentSource  bool     `json:"current_source"`
	Superseded     bool     `json:"superseded"`
}

func loadBillingSourceReviewNotes(ctx context.Context, db *gorm.DB, report *BillingSourceReview) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	budget := billingSourceReviewBudget{remaining: report.historyBudget}
	indices := map[string]int{}
	latestHistory := map[string]int{}
	report.History = []BillingSourceDecisionRecord{}
	report.PendingRequests = []BillingSourceDecisionRecord{}
	report.AuditIssues = []BillingSourceAuditIssue{}
	report.Pending, report.Blockers = 0, 0
	for i := range report.Issues {
		issue := &report.Issues[i]
		indices[issue.ID] = i
		issue.Reviewed, issue.Note, issue.ActorID, issue.ReviewedAt, issue.DecisionID = false, "", 0, 0, 0
	}
	for cursor := int64(0); ; {
		var notes []BillingStatementAudit
		if err := db.WithContext(ctx).Select("id, reason, actor_id, created_at").Where("action = ? AND idempotency_key = ? AND id > ?", "review_source_issue", billingSourceReviewKey(report.UserID, report.Start), cursor).Order("id asc").Limit(100).Find(&notes).Error; err != nil {
			return err
		}
		if len(notes) == 0 {
			break
		}
		for _, note := range notes {
			if err := ctx.Err(); err != nil {
				return err
			}
			// Reserve before decoding: raw page text, decoded strings and the
			// retained record/index all count against the remaining scan budget.
			if err := budget.consume(1024 + 2*len(note.Reason)); err != nil {
				return err
			}
			var d BillingSourceDecisionRecord
			if common.UnmarshalJsonStr(note.Reason, &d) != nil || !validBillingSourceAudit(d, note.Reason, report.UserID, report.Start) {
				report.AuditIssues = append(report.AuditIssues, BillingSourceAuditIssue{ID: note.ID, ActorID: note.ActorId, CreatedAt: note.CreatedAt})
				continue
			}
			d.ID, d.ActorID, d.CreatedAt = note.ID, note.ActorId, note.CreatedAt
			d.CurrentSource = (d.Version == 2 || d.Version == BillingSourceDecisionVersion) && d.Fingerprint == report.Fingerprint
			if d.Version == 0 {
				d.Status = "legacy_note"
			}
			if prior, exists := latestHistory[d.IssueID]; exists {
				report.History[prior].Superseded = true
			}
			latestHistory[d.IssueID] = len(report.History)
			report.History = append(report.History, d)
			i, ok := indices[d.IssueID]
			if !ok {
				continue
			}
			issue := &report.Issues[i]
			// Even a stale decision is the predecessor of its explicit replacement.
			issue.DecisionID = note.ID
			issue.Reviewed, issue.Note, issue.ActorID, issue.ReviewedAt = false, "", 0, 0
			if !d.CurrentSource {
				continue
			}
			issue.Reviewed = d.Decision == BillingSourceKeep && !issue.Blocking
			issue.Note, issue.ActorID, issue.ReviewedAt = d.Note, note.ActorId, note.CreatedAt
		}
		cursor = notes[len(notes)-1].ID
	}
	for _, d := range report.History {
		if !d.Superseded && d.Status == "pending_review" {
			report.PendingRequests = append(report.PendingRequests, d)
			if _, exists := indices[d.IssueID]; !exists {
				report.Pending++
			}
		}
	}
	report.Blockers = len(report.AuditIssues)
	for i := range report.Issues {
		issue := &report.Issues[i]
		// A damaged latest decision must never resurrect an earlier acceptance.
		if len(report.AuditIssues) > 0 {
			issue.Reviewed = false
		}
		if issue.Blocking {
			report.Blockers++
		}
		if !issue.Reviewed {
			report.Pending++
		}
	}
	return ctx.Err()
}

func RecordBillingSourceReviewDecision(ctx context.Context, user int, start, end int64, d BillingSourceReviewDecision, actor int) error {
	d.Note = strings.TrimSpace(d.Note)
	if actor <= 0 || utf8.RuneCountInString(d.Note) > 4000 || d.PreviousID < 0 || len(d.RequestID) < 16 || len(d.RequestID) > 64 {
		return ErrBillingStatementSourceIncomplete
	}
	for _, c := range d.RequestID {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return ErrBillingStatementSourceIncomplete
		}
	}
	validReason := false
	switch d.Decision {
	case BillingSourceKeep:
		validReason = d.Reason == "accept_missing_details"
	case BillingSourceInvestigate:
		validReason = d.Reason == "amount_questioned" || d.Reason == "missing_evidence"
	case BillingSourceWaiver:
		validReason = d.Reason == "customer_agreement" || d.Reason == "service_issue"
	case BillingSourceDuplicate:
		validReason = d.Reason == "suspected_duplicate"
	case BillingSourcePeriod:
		validReason = d.Reason == "wrong_period"
	case BillingSourceWithdraw:
		validReason = d.Reason == "request_withdrawn"
	case BillingSourceReject:
		validReason = d.Reason == "request_rejected"
	}
	if !validReason || d.Decision != BillingSourceKeep && d.Note == "" {
		return ErrBillingStatementSourceIncomplete
	}
	r, err := GetBillingSourceReview(ctx, user, start, end)
	if err != nil {
		return err
	}

	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Same lock order as source verification and formal confirmation.
		if _, err := lockBillingStatementMaintenanceTx(ctx, tx); err != nil {
			return err
		}
		if err := lockBillingSourceUser(tx, user); err != nil {
			return err
		}
		if err := lockBillingSourceReview(ctx, tx, r); err != nil {
			return err
		}
		if err := loadBillingSourceReviewNotes(ctx, tx, r); err != nil {
			return err
		}
		if len(r.AuditIssues) > 0 {
			return ErrBillingStatementSourceIncomplete
		}
		for _, previous := range r.History {
			if previous.RequestID != d.RequestID {
				continue
			}
			if previous.ActorID == actor && previous.BillingSourceReviewDecision == d {
				return nil
			}
			return ErrBillingStatementVersionConflict
		}
		if d.Fingerprint != r.Fingerprint {
			return ErrBillingStatementVersionConflict
		}
		var previous *BillingSourceDecisionRecord
		for i := range r.History {
			if r.History[i].IssueID == d.IssueID && !r.History[i].Superseded {
				previous = &r.History[i]
			}
		}
		previousID := int64(0)
		if previous != nil {
			previousID = previous.ID
		}
		if d.PreviousID != previousID {
			return ErrBillingStatementVersionConflict
		}
		var recorded BillingSourceDecisionRecord
		if d.Decision == BillingSourceWithdraw || d.Decision == BillingSourceReject {
			if previous == nil || previous.Status != "pending_review" {
				return ErrBillingStatementVersionConflict
			}
			// Closing a request retains its original monetary context. It cannot
			// certify current source facts or claim an adjustment was executed.
			recorded = *previous
			recorded.Status = "request_withdrawn"
			if d.Decision == BillingSourceReject {
				recorded.Status = "request_rejected"
			}
		} else {
			var issue *BillingSourceIssue
			for i := range r.Issues {
				if r.Issues[i].ID == d.IssueID {
					issue = &r.Issues[i]
					break
				}
			}
			if issue == nil {
				return ErrBillingStatementVersionConflict
			}
			if d.Decision == BillingSourceKeep && (issue.Blocking || previous != nil && previous.Status == "pending_review") {
				return ErrBillingStatementSourceIncomplete
			}
			if issue.LogType == LogTypeRefund && d.Decision != BillingSourceKeep && d.Decision != BillingSourceInvestigate {
				return ErrBillingStatementSourceIncomplete
			}
			status := "pending_review"
			if d.Decision == BillingSourceKeep {
				status = "record_kept"
			}
			recorded = BillingSourceDecisionRecord{UserID: user, PeriodStart: start, Model: issue.Model, LogType: issue.LogType, RecordedQuota: issue.Quota, Kind: issue.Kind, Blocking: issue.Blocking, TargetQuota: issue.TargetQuota, EvidenceGaps: issue.Reasons, Status: status}
		}
		recorded.BillingSourceReviewDecision = d
		recorded.Version, recorded.ID, recorded.ActorID, recorded.CreatedAt = BillingSourceDecisionVersion, 0, actor, nowSeconds()
		recorded.CurrentSource, recorded.Superseded = false, false
		recorded.StatementDelta, recorded.BalanceDelta = 0, 0
		raw, err := common.Marshal(recorded)
		if err != nil {
			return err
		}
		// Changed decisions revoke prior completeness and unconfirmed drafts, but do
		// not pretend that the underlying log/task evidence changed.
		if err := IncrementBillingStatementRevisionTx(tx, fmt.Sprintf("cm:%d:%d", user, start)); err != nil {
			return err
		}
		return tx.Create(&BillingStatementAudit{Action: "review_source_issue", ActorId: actor, IdempotencyKey: billingSourceReviewKey(user, start), Reason: string(raw), Result: recorded.Status, CreatedAt: recorded.CreatedAt}).Error
	})
}

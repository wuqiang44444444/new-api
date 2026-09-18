package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// This report is evidence for review, never an instruction to debit or repair.
// Only customer-safe identifiers and facts leave this boundary.
type BillingSourceRelatedLog struct {
	ID              int   `json:"id"`
	CreatedAt       int64 `json:"created_at"`
	LogType         int   `json:"log_type"`
	Quota           int64 `json:"quota,string"`
	IdentityMatches bool  `json:"identity_matches"`
}
type BillingSourceIssue struct {
	DecisionID    int64                     `json:"decision_id"`
	ID            string                    `json:"id"`
	Kind          string                    `json:"kind"`
	Blocking      bool                      `json:"blocking"`
	LogID         int                       `json:"log_id"`
	TaskRowID     int64                     `json:"task_row_id"`
	CreatedAt     int64                     `json:"created_at"`
	Model         string                    `json:"model"`
	LogType       int                       `json:"log_type"`
	Quota         int64                     `json:"quota,string"`
	TargetQuota   *int64                    `json:"target_quota,string,omitempty"`
	RelatedLogIDs []int                     `json:"related_log_ids"`
	Reasons       []string                  `json:"reasons"`
	Reviewed      bool                      `json:"reviewed"`
	Note          string                    `json:"note"`
	ActorID       int                       `json:"actor_id"`
	ReviewedAt    int64                     `json:"reviewed_at"`
	RelatedLogs   []BillingSourceRelatedLog `json:"related_logs"`
}
type billingSourceStamp struct {
	Revision   int64 `json:"revision"`
	Generation int64 `json:"generation"`
	Parser     int   `json:"parser"`
}
type BillingSourceAuditIssue struct {
	ID        int64 `json:"id"`
	ActorID   int   `json:"actor_id"`
	CreatedAt int64 `json:"created_at"`
}
type BillingSourceReview struct {
	PendingRequests []BillingSourceDecisionRecord   `json:"pending_requests"`
	AuditIssues     []BillingSourceAuditIssue       `json:"audit_issues"`
	DecisionVersion int                             `json:"decision_version"`
	History         []BillingSourceDecisionRecord   `json:"history"`
	UserID          int                             `json:"user_id"`
	Start           int64                           `json:"start"`
	End             int64                           `json:"end"`
	CapturedAt      int64                           `json:"captured_at"`
	Fingerprint     string                          `json:"fingerprint"`
	Rows            int64                           `json:"rows"`
	NetQuota        int64                           `json:"net_quota,string"`
	Blockers        int                             `json:"blockers"`
	Pending         int                             `json:"pending"`
	Issues          []BillingSourceIssue            `json:"issues"`
	Retention       BillingStatementRetentionStatus `json:"retention_status"`
	stamp           billingSourceStamp
	revisions       map[string]int64
	historyBudget   int
}

func billingSourceReviewStamp(ctx context.Context, db *gorm.DB, user int) (billingSourceStamp, error) {
	s := billingSourceStamp{Parser: BillingStatementParserVersion}
	var m BillingStatementMaintenance
	if err := db.WithContext(ctx).First(&m, billingStatementMaintenanceRowID).Error; err != nil {
		return s, err
	}
	if m.Enabled {
		return s, ErrBillingStatementVersionDisabled
	}
	s.Generation = m.Generation
	var r []BillingStatementRevision
	if err := db.WithContext(ctx).Where("scope = ?", fmt.Sprintf("ev:%d:0", user)).Find(&r).Error; err != nil {
		return s, err
	}
	if len(r) > 0 {
		s.Revision = r[0].Revision
	}
	return s, nil
}

// Reads all history of this user for cross-period task/refund evidence. Page
// boundaries apply only to presentation. Cancellation never yields a report.
func GetBillingSourceReview(ctx context.Context, user int, start, end int64) (*BillingSourceReview, error) {
	return getBillingSourceReview(ctx, user, start, end, 64<<20)
}

func getBillingSourceReview(ctx context.Context, user int, start, end int64, byteBudget int) (*BillingSourceReview, error) {
	budget := billingSourceReviewBudget{remaining: byteBudget}
	if !BillingStatementVersionTopologyOK() {
		return nil, ErrBillingStatementVersionTopology
	}
	if user <= 0 || start != naturalMonthStartAt(start) || end != time.Unix(start, 0).In(time.FixedZone("Asia/Shanghai", 8*3600)).AddDate(0, 1, 0).Unix()-1 {
		return nil, ErrBillingStatementVersionConflict
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	before, err := billingSourceReviewStamp(ctx, DB, user)
	if err != nil {
		return nil, err
	}
	out := &BillingSourceReview{DecisionVersion: BillingSourceDecisionVersion, UserID: user, Start: start, End: end, CapturedAt: nowSeconds(), Issues: []BillingSourceIssue{}, stamp: before, Retention: BillingStatementRetentionUnknown}
	out.revisions = map[string]int64{fmt.Sprintf("cm:%d:%d", user, start): 0, fmt.Sprintf("review-task-month:%d:%d", user, start): 0}
	var retention []BillingStatementRetention
	if err := DB.WithContext(ctx).Where("user_id = ? AND period_start = ?", user, start).Find(&retention).Error; err != nil {
		return nil, err
	}
	if len(retention) > 0 {
		out.Retention = retention[0].Status
	}
	query := LOG_DB.WithContext(ctx).Model(&Log{}).Scopes(customerSettlementLogs).Where("user_id = ? AND type IN ?", user, []int{LogTypeConsume, LogTypeRefund})
	digest := sha256.New()
	write := func(v any) error {
		b, e := common.Marshal(v)
		if e != nil {
			return e
		}
		_, e = digest.Write(b)
		return e
	}
	if err := write([]any{user, start, end, before.Generation, before.Parser}); err != nil {
		return nil, err
	}
	// Keep only task-linked rows for reconciliation, not all text logs.
	linked := map[string][]billingSourceRelatedEvidence{}
	preauth := map[int][]billingSourceRelatedEvidence{}
	for cursor := 0; ; {
		var logs []Log
		if err := query.Session(&gorm.Session{}).Select([]string{"id", "user_id", "created_at", "token_id", "token_name", "channel_id", "model_name", "type", "quota", "prompt_tokens", "completion_tokens", "content", "other", "group"}).Where("id > ?", cursor).Order("id asc").Limit(500).Find(&logs).Error; err != nil {
			return nil, err
		}
		if len(logs) == 0 {
			break
		}
		// Bound retained evidence before parsing or building refund associations.
		// Only this batch's monthly refunds need classification; uniqueness still
		// uses the complete owner/key history in the existing evidence reader.
		batchBytes := 0
		var refunds []billingReconciliationLog
		var refundParsed []parsedBillingReconciliationLog
		for _, l := range logs {
			size := 1024 + len(l.Other) + len(l.Content) + len(l.ModelName) + len(l.TokenName) + len(l.Group)
			if err := budget.consume(size); err != nil {
				return nil, err
			}
			batchBytes += size
			if l.Type == LogTypeRefund && l.CreatedAt >= start && l.CreatedAt <= end {
				fact := billingReconciliationLog{UserId: l.UserId, TokenId: l.TokenId, ChannelId: l.ChannelId, ModelName: l.ModelName, Type: l.Type, Quota: l.Quota, PromptTokens: l.PromptTokens, CompletionTokens: l.CompletionTokens, Other: l.Other}
				refunds = append(refunds, fact)
				refundParsed = append(refundParsed, parseBillingReconciliationLog(fact))
			}
		}
		evidence, err := buildBillingStatementRefundEvidence(ctx, refunds, refundParsed)
		if err != nil {
			return nil, err
		}
		for _, l := range logs {
			if int64(l.Quota) > math.MaxInt32 {
				return nil, fmt.Errorf("invalid recorded quota")
			}
			var relation struct {
				TaskID string `json:"task_id"`
				Admin  struct {
					ID int `json:"original_preauth_log_id"`
				} `json:"admin_info"`
			}
			if common.UnmarshalJsonStr(l.Other, &relation) == nil {
				if relation.TaskID != "" {
					key := fmt.Sprintf("%d:%s", l.TokenId, relation.TaskID)
					if err := budget.consume(1024 + len(l.Other) + len(l.ModelName) + len(key)); err != nil {
						return nil, err
					}
					linked[key] = append(linked[key], billingSourceRelatedEvidence{ID: l.Id, CreatedAt: l.CreatedAt, TokenID: l.TokenId, ChannelID: l.ChannelId, Model: l.ModelName, Type: l.Type, Quota: l.Quota, Other: l.Other})
				}
				if l.Type == LogTypeRefund && relation.Admin.ID > 0 {
					if err := budget.consume(1024 + len(l.Other) + len(l.ModelName)); err != nil {
						return nil, err
					}
					preauth[relation.Admin.ID] = append(preauth[relation.Admin.ID], billingSourceRelatedEvidence{ID: l.Id, CreatedAt: l.CreatedAt, TokenID: l.TokenId, ChannelID: l.ChannelId, Model: l.ModelName, Type: l.Type, Quota: l.Quota, Other: l.Other})
				}
			}
			if l.CreatedAt < start || l.CreatedAt > end {
				continue
			}
			if err := write([]any{l.Id, l.CreatedAt, l.TokenId, l.ChannelId, l.ModelName, l.Type, l.Quota, l.PromptTokens, l.CompletionTokens, l.Other}); err != nil {
				return nil, err
			}
			out.revisions[fmt.Sprintf("refund:%d:%d", user, l.TokenId)] = 0
			if relation.Admin.ID > 0 {
				out.revisions[fmt.Sprintf("log:%d", relation.Admin.ID)] = 0
			}
			if relation.TaskID != "" {
				out.revisions[billingStatementTaskScope(user, l.TokenId, relation.TaskID)] = 0
			}
			if len(out.revisions) > billingSourceReviewMaxDependencies {
				return nil, errBillingSourceReviewBudget
			}
			out.Rows++
			if l.Type == LogTypeRefund {
				out.NetQuota -= int64(l.Quota)
			} else {
				out.NetQuota += int64(l.Quota)
			}
			fact := billingReconciliationLog{UserId: l.UserId, TokenId: l.TokenId, TokenName: l.TokenName, ChannelId: l.ChannelId, ModelName: l.ModelName, Type: l.Type, CreatedAt: l.CreatedAt, PromptTokens: l.PromptTokens, CompletionTokens: l.CompletionTokens, Quota: l.Quota, Other: l.Other, Content: l.Content, GroupName: l.Group}
			parsed := parseBillingReconciliationLog(fact)
			evidence.apply(fact, &parsed)
			reasons := []string{}
			if l.Quota < 0 {
				reasons = append(reasons, "invalid_quota")
			}
			if parsed.billingMode == BillingReconciliationModeUnknown {
				reasons = append(reasons, "unknown_billing_mode")
			}
			if parsed.inputTokensUnavailable {
				reasons = append(reasons, "input_usage_missing")
			}
			if parsed.cacheWriteUnavailable {
				reasons = append(reasons, "cache_usage_missing")
			}
			if parsed.unavailable {
				reasons = append(reasons, "metadata_unreadable")
			}
			if parsed.hasAuxiliaryCharge {
				reasons = append(reasons, "auxiliary_charge")
			}
			if len(reasons) > 0 || l.Quota < 0 {
				if err := budget.consume(1024 + len(parsed.customerModel)); err != nil {
					return nil, err
				}
				out.Issues = append(out.Issues, BillingSourceIssue{ID: fmt.Sprintf("log:%d", l.Id), Kind: "explanation", Blocking: l.Quota < 0, LogID: l.Id, CreatedAt: l.CreatedAt, Model: parsed.customerModel, LogType: l.Type, Quota: int64(l.Quota), Reasons: reasons, RelatedLogIDs: []int{l.Id}})
			}
		}
		cursor = logs[len(logs)-1].Id
		// Page-local evidence has been consumed; only explicit links and output
		// remain retained. History length must not become a billing cutoff.
		budget.remaining += batchBytes
	}
	// A final task target and explicitly related log net are compared only as an
	// investigation signal. Missing identity or manual refunds are never repaired.
	for cursor := int64(0); ; {
		var tasks []Task
		if err := DB.WithContext(ctx).Select("id,created_at,task_id,user_id,app_id,channel_id,quota,properties,private_data,billing_state,status").Where("user_id = ? AND id > ?", user, cursor).Order("id asc").Limit(200).Find(&tasks).Error; err != nil {
			return nil, err
		}
		if len(tasks) == 0 {
			break
		}
		// Count owners only for this page's exact app/task identities. A global
		// GROUP BY previously retained every task identity of the customer.
		identity := DB.Session(&gorm.Session{NewDB: true})
		for _, task := range tasks {
			identity = identity.Or("(app_id = ? AND task_id = ?)", task.AppID, task.TaskID)
		}
		var owners []struct {
			AppID  int
			TaskID string
			Count  int64
		}
		if err := DB.WithContext(ctx).Model(&Task{}).Select("app_id, task_id, COUNT(*) AS count").Where("user_id = ?", user).Where(identity).Group("app_id, task_id").Scan(&owners).Error; err != nil {
			return nil, err
		}
		counts := map[string]int64{}
		for _, owner := range owners {
			counts[fmt.Sprintf("%d:%s", owner.AppID, owner.TaskID)] = owner.Count
		}
		batchBytes := 0
		for _, task := range tasks {
			raw, err := common.Marshal(task.PrivateData)
			if err != nil {
				return nil, err
			}
			size := 1024 + len(raw) + len(task.TaskID) + len(task.Properties.OriginModelName)
			if err := budget.consume(size); err != nil {
				return nil, err
			}
			batchBytes += size
			a := task.PrivateData.AsyncBilling
			if a == nil || a.State != TaskBillingStateSettled || a.TargetQuota == nil {
				continue
			}
			key := fmt.Sprintf("%d:%s", task.PrivateData.TokenId, task.TaskID)
			rows := append([]billingSourceRelatedEvidence(nil), linked[key]...)
			for _, l := range linked[key] {
				rows = append(rows, preauth[l.ID]...)
			}
			relevant := task.CreatedAt >= start && task.CreatedAt <= end
			net := int64(0)
			ids := []int{}
			related := []BillingSourceRelatedLog{}
			seen := map[int]bool{}
			conflict := counts[fmt.Sprintf("%d:%s", task.AppID, task.TaskID)] != 1 || task.TaskID == ""
			for _, l := range rows {
				if seen[l.ID] {
					continue
				}
				seen[l.ID] = true
				if l.CreatedAt >= start && l.CreatedAt <= end {
					relevant = true
				}
				matches := l.TokenID == task.PrivateData.TokenId && l.ChannelID == task.ChannelId && l.Model == task.Properties.OriginModelName
				related = append(related, BillingSourceRelatedLog{ID: l.ID, CreatedAt: l.CreatedAt, LogType: l.Type, Quota: int64(l.Quota), IdentityMatches: matches})
				if !matches {
					conflict = true
					continue
				}
				ids = append(ids, l.ID)
				if l.Type == LogTypeRefund {
					net -= int64(l.Quota)
				} else {
					net += int64(l.Quota)
				}
			}
			if relevant {
				if err := write([]any{task, task.PrivateData}); err != nil {
					return nil, err
				}
				for _, l := range rows {
					if err := write([]any{l.ID, l.Type, l.Quota, l.CreatedAt, l.TokenID, l.ChannelID, l.Model, l.Other}); err != nil {
						return nil, err
					}
				}
				out.revisions[billingStatementTaskScope(user, task.AppID, task.TaskID)] = 0
				out.revisions[billingSourceTaskLogScope(user, task.PrivateData.TokenId, task.TaskID)] = 0
				if len(out.revisions) > billingSourceReviewMaxDependencies {
					return nil, errBillingSourceReviewBudget
				}
				for _, l := range rows {
					out.revisions[fmt.Sprintf("log:%d", l.ID)] = 0
					out.revisions[fmt.Sprintf("refund:%d:%d", user, l.TokenID)] = 0
					if len(out.revisions) > billingSourceReviewMaxDependencies {
						return nil, errBillingSourceReviewBudget
					}
				}
			}
			if !relevant || (net == int64(task.Quota) && !conflict && *a.TargetQuota == task.Quota) {
				continue
			}
			target := int64(task.Quota)
			reasons := []string{"task_log_net_mismatch"}
			if len(ids) == 0 {
				reasons = append(reasons, "no_explicit_task_logs")
			}
			if conflict {
				reasons = append(reasons, "identity_conflict")
			}
			if *a.TargetQuota != task.Quota {
				reasons = append(reasons, "settlement_target_conflict")
			}
			if err := budget.consume(1024 + len(task.Properties.OriginModelName) + 128*len(related)); err != nil {
				return nil, err
			}
			out.Issues = append(out.Issues, BillingSourceIssue{ID: fmt.Sprintf("task:%d", task.ID), Kind: "task_amount", Blocking: true, TaskRowID: task.ID, CreatedAt: task.CreatedAt, Model: task.Properties.OriginModelName, Quota: net, TargetQuota: &target, RelatedLogIDs: ids, RelatedLogs: related, Reasons: reasons})
		}
		cursor = tasks[len(tasks)-1].ID
		budget.remaining += batchBytes
	}
	if err := readBillingSourceReviewRevisions(DB.WithContext(ctx), out.revisions); err != nil {
		return nil, err
	}
	after, err := billingSourceReviewStamp(ctx, DB, user)
	if err != nil {
		return nil, err
	}
	if before != after {
		return nil, ErrBillingStatementVersionConflict
	}
	if err := write(out.Issues); err != nil {
		return nil, err
	}
	out.Fingerprint = fmt.Sprintf("%x", digest.Sum(nil))
	// The same allowance is reused for the replacement history read under lock;
	// it must not reset to a fresh full budget or charge the old history twice.
	out.historyBudget = budget.remaining
	if err := loadBillingSourceReviewNotes(ctx, DB, out); err != nil {
		return nil, err
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// The snapshot stamp is compared under the same per-user registration lock
// used by every tracked log/task writer. No global process cache is authoritative.
func lockBillingSourceReview(ctx context.Context, tx *gorm.DB, r *BillingSourceReview) error {
	m, err := lockBillingStatementMaintenanceTx(ctx, tx)
	if err != nil {
		return err
	}
	if m.Enabled || m.Generation != r.stamp.Generation {
		return ErrBillingStatementVersionConflict
	}
	scope := fmt.Sprintf("ev:%d:0", r.UserID)
	// Ensure and lock without incrementing the revision (review is not a source edit).
	if err := tx.Model(&BillingStatementRevision{}).Where("scope = ?", scope).UpdateColumn("revision", gorm.Expr("revision")).Error; err != nil {
		return err
	}
	var revision BillingStatementRevision
	err = lockForUpdate(tx).Where("scope = ?", scope).First(&revision).Error
	if err != nil {
		return ErrBillingStatementVersionConflict
	}
	current, err := billingSourceReviewStamp(ctx, tx, r.UserID)
	if err != nil {
		return err
	}
	if current != r.stamp {
		return ErrBillingStatementVersionConflict
	}
	return nil
}

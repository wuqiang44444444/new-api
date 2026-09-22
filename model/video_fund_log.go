package model

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"strings"
)

type VideoFundLog struct {
	RefundAmountKnown bool   `json:"refund_amount_known"`
	Kind              string `json:"kind"`
	ID                int64  `json:"id"`
	TaskID            string `json:"task_id"`
	RequestID         string `json:"request_id"`
	UserID            int    `json:"user_id"`
	AppID             int    `json:"app_id"`
	ChannelID         int    `json:"channel_id"`
	Model             string `json:"model"`
	Source            string `json:"source"`
	BusinessStatus    string `json:"business_status"`
	FundState         string `json:"fund_state"`
	Quota             int    `json:"quota"`
	RefundedQuota     int    `json:"refunded_quota"`
	WaivedQuota       int    `json:"waived_quota"`
	CreatedAt         int64  `json:"created_at"`
	DeadlineAt        int64  `json:"deadline_at"`
	RefundedAt        int64  `json:"refunded_at"`
	RetryAt           int64  `json:"retry_at"`
	OperatorID        int    `json:"operator_id"`
	Note              string `json:"note"`
	Failure           string `json:"failure"`
	Delivery          string `json:"delivery"`
	Version           string `json:"version"`
	CanRefund         bool   `json:"can_refund"`
}

type VideoFundFilters struct {
	UserID, AppID, ChannelID int
	TaskID, State            string
	RefundFrom, RefundTo     int64
	Offset, Limit            int
}

// The union is a read projection of the two existing funding owners. The
// atomic transfer removes the attempt from the projection and exposes its Task.
// No JSON SQL, provider identity inference, or new accounting ledger is used.
// The fund_state CASE below must stay equivalent to the Go derivation in
// buildVideoFundLogEntry; TestVideoFundLogProjectionMatchesEntryDerivation
// locks that equivalence.
func videoFundProjection() (string, []any) {
	sql := `SELECT 'attempt' AS kind, id, public_task_id AS task_id, user_id, app_id, channel_id, created_at,
 CASE WHEN billing_hold_state = 'held' THEN held_quota ELSE 0 END AS quota, funds_deadline_at AS deadline_at,
 CASE WHEN billing_hold_state = 'held' AND (status IN ('unknown','upstream_succeeded') OR funds_deadline_at <= ?) THEN 1 ELSE 0 END AS abnormal,
 CASE WHEN COALESCE(refund_failure,'') <> '' OR COALESCE(video_refund_failure,'') <> '' THEN 1 ELSE 0 END AS refund_failed,
 CASE WHEN video_refund_state = 'pending' THEN 'pending' WHEN billing_hold_state = 'released' THEN 'refunded' ELSE 'held' END AS fund_state
 FROM task_create_attempts WHERE client_protocol IN ? AND billing_hold_state IN ('held','released')
 UNION ALL
 SELECT 'task' AS kind, id, task_id, user_id, app_id, channel_id, created_at, quota, 0 AS deadline_at,
 CASE WHEN quota > 0 AND (status = 'FAILURE' OR video_delivery_state IN ('write_failed','result_unavailable') OR billing_state IN ('debt','awaiting_usage')) THEN 1 ELSE 0 END AS abnormal,
 CASE WHEN COALESCE(video_refund_failure,'') <> '' THEN 1 ELSE 0 END AS refund_failed,
 CASE WHEN video_refund_state = 'pending' THEN 'pending' WHEN video_refund_completed_at > 0 THEN 'refunded' WHEN quota = 0 AND EXISTS (SELECT 1 FROM task_billing_deliveries d WHERE d.task_row_id = tasks.id AND d.before_quota > d.after_quota) THEN 'refunded' WHEN quota = 0 THEN 'closed' WHEN billing_state = 'settled' OR status = 'SUCCESS' THEN 'charged' ELSE 'held' END AS fund_state
 FROM tasks WHERE (client_protocol IN ? OR action IN ?) AND platform NOT IN ('azure_batch','suno','mj') AND COALESCE(client_protocol,'') <> 'image_openai_v1'`
	actions := []string{"text_to_video", "image_to_video", "first_tail_to_video", "reference_to_video", "remix", "generate", "textGenerate", "firstTailGenerate", "referenceGenerate", "remixGenerate"}
	return sql, []any{common.GetTimestamp(), videoFundClientProtocols, videoFundClientProtocols, actions}
}

func videoFundLogQuery(f VideoFundFilters) *gorm.DB {
	sql, args := videoFundProjection()
	q := DB.Table("(?) AS video_funds", DB.Raw(sql, args...))
	if f.UserID > 0 {
		q = q.Where("user_id = ?", f.UserID)
	}
	if f.AppID > 0 {
		q = q.Where("app_id = ?", f.AppID)
	}
	if f.ChannelID > 0 {
		q = q.Where("channel_id = ?", f.ChannelID)
	}
	if strings.TrimSpace(f.TaskID) != "" {
		q = q.Where("task_id = ? OR (kind = ? AND id IN (SELECT id FROM task_create_attempts WHERE attempt_id = ?))", strings.TrimSpace(f.TaskID), "attempt", strings.TrimSpace(f.TaskID))
	}
	if f.State == "abnormal" {
		q = q.Where("abnormal = 1 OR refund_failed = 1")
	} else if f.State != "" {
		q = q.Where("fund_state = ?", f.State)
	}
	return q
}

// ListVideoFundLogs pages the projection, then loads the page's rows and
// delivery refund facts in three batched queries. Per-row derivation happens
// once in buildVideoFundLogEntry, so the list never issues per-row queries.
func ListVideoFundLogs(f VideoFundFilters) ([]VideoFundLog, int64, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 20
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	q := videoFundLogQuery(f)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var owners []VideoFundLog
	if err := q.Select("kind, id, created_at").Order("abnormal DESC, refund_failed DESC, created_at DESC, kind, id DESC").Offset(f.Offset).Limit(f.Limit).Scan(&owners).Error; err != nil {
		return nil, 0, err
	}
	attemptIDs := make([]int64, 0, len(owners))
	taskIDs := make([]int64, 0, len(owners))
	for _, o := range owners {
		if o.Kind == "attempt" {
			attemptIDs = append(attemptIDs, o.ID)
		} else if o.Kind == "task" {
			taskIDs = append(taskIDs, o.ID)
		}
	}
	attempts := make(map[int64]*TaskCreateAttempt, len(attemptIDs))
	if len(attemptIDs) > 0 {
		var rows []TaskCreateAttempt
		if err := DB.Where("id IN ?", attemptIDs).Find(&rows).Error; err != nil {
			return nil, 0, err
		}
		for i := range rows {
			attempts[rows[i].ID] = &rows[i]
		}
	}
	tasks := make(map[int64]*Task, len(taskIDs))
	if len(taskIDs) > 0 {
		var rows []Task
		if err := DB.Where("id IN ?", taskIDs).Find(&rows).Error; err != nil {
			return nil, 0, err
		}
		for i := range rows {
			tasks[rows[i].ID] = &rows[i]
		}
	}
	refundSums, refundTimes, err := taskDeliveryRefundFacts(taskIDs)
	if err != nil {
		return nil, 0, err
	}
	out := make([]VideoFundLog, 0, len(owners))
	for _, o := range owners {
		entry, err := buildVideoFundLogEntry(o.Kind, attempts[o.ID], tasks[o.ID], refundSums, refundTimes)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *entry)
	}
	return out, total, nil
}

func GetVideoFundLog(kind string, id int64) (*VideoFundLog, error) {
	refundSums, refundTimes, err := taskDeliveryRefundFacts([]int64{id})
	if err != nil {
		return nil, err
	}
	switch kind {
	case "attempt":
		var a TaskCreateAttempt
		if err := DB.First(&a, id).Error; err != nil {
			return nil, err
		}
		return buildVideoFundLogEntry(kind, &a, nil, refundSums, refundTimes)
	case "task":
		var t Task
		if err := DB.First(&t, id).Error; err != nil {
			return nil, err
		}
		return buildVideoFundLogEntry(kind, nil, &t, refundSums, refundTimes)
	default:
		return nil, fmt.Errorf("invalid video funding owner")
	}
}

// taskDeliveryRefundFacts sums committed non-customer refund deltas and the
// latest refund delta time per task row in one grouped query.
func taskDeliveryRefundFacts(taskIDs []int64) (map[int64]int64, map[int64]int64, error) {
	sums := map[int64]int64{}
	times := map[int64]int64{}
	if len(taskIDs) == 0 {
		return sums, times, nil
	}
	var rows []struct {
		TaskRowID int64
		Amount    int64
		LastAt    int64
	}
	err := DB.Model(&TaskBillingDelivery{}).
		Select("task_row_id, COALESCE(SUM(CASE WHEN before_quota > after_quota AND event <> 'customer_refund' THEN before_quota - after_quota ELSE 0 END), 0) AS amount, COALESCE(MAX(CASE WHEN before_quota > after_quota THEN created_at ELSE 0 END), 0) AS last_at").
		Where("task_row_id IN ?", taskIDs).
		Group("task_row_id").
		Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}
	for _, r := range rows {
		sums[r.TaskRowID] = r.Amount
		if r.Amount > 0 {
			times[r.TaskRowID] = r.LastAt
		}
	}
	return sums, times, nil
}

// buildVideoFundLogEntry is the single Go-side derivation of a fund log row;
// the SQL fund_state CASE in videoFundProjection must stay equivalent to it.
func buildVideoFundLogEntry(kind string, a *TaskCreateAttempt, t *Task, refundSums, refundTimes map[int64]int64) (*VideoFundLog, error) {
	entry := &VideoFundLog{Kind: kind, Delivery: "unverified", RefundAmountKnown: true}
	var refund VideoRefund
	switch kind {
	case "attempt":
		if a == nil {
			return nil, fmt.Errorf("video attempt not found")
		}
		if !IsLinkVideoTaskClientProtocol(a.ClientProtocol) {
			return nil, fmt.Errorf("not a video attempt")
		}
		entry.ID = a.ID
		entry.TaskID, entry.RequestID = a.PublicTaskID, a.AttemptID
		entry.UserID, entry.AppID, entry.ChannelID = a.UserID, a.AppID, a.ChannelID
		entry.Model, entry.Source = a.PublicModel, a.BillingSource
		entry.BusinessStatus = string(a.Status)
		entry.CreatedAt, entry.DeadlineAt, entry.RefundedAt, entry.RetryAt = a.CreatedAt, a.FundsDeadlineAt, a.RefundCompletedAt, a.FundRetryAt
		entry.FundState = string(a.BillingHoldState)
		if a.BillingHoldState == TaskCreateAttemptBillingHeld {
			entry.Quota = a.HeldQuota
		}
		if a.BillingHoldState == TaskCreateAttemptBillingReleased {
			entry.FundState = "refunded"
			if a.ActualRefundQuota != nil {
				entry.RefundedQuota = *a.ActualRefundQuota
			} else {
				entry.Failure = "historical_refund_amount_unverified"
				entry.RefundAmountKnown = false
			}
		}
		if a.RefundFailure != "" {
			entry.Failure = "refund_retry_pending"
		}
		entry.Version = videoAttemptRefundVersion(a)
		entry.CanRefund = a.BillingHoldState == TaskCreateAttemptBillingHeld
		refund = a.VideoRefund
	case "task":
		if t == nil {
			return nil, fmt.Errorf("video task not found")
		}
		if !IsVideoFundTask(t) {
			return nil, fmt.Errorf("not a video task")
		}
		entry.ID = t.ID
		entry.TaskID = t.TaskID
		if t.VideoDeliveryState == "write_failed" {
			entry.Delivery = "write_failed"
		}
		if t.PrivateData.Execution != nil {
			entry.RequestID = t.PrivateData.Execution.RequestID
		}
		entry.UserID, entry.AppID, entry.ChannelID = t.UserId, t.AppID, t.ChannelId
		entry.Model = t.Properties.OriginModelName
		entry.Source = t.PrivateData.BillingSource
		// Legacy native rows predate the persisted funding source; their only
		// possible owner is the wallet, so the display defaults instead of
		// guessing a different account.
		if entry.Source == "" {
			entry.Source = "wallet"
		}
		entry.BusinessStatus = string(t.Status)
		entry.Quota = t.Quota
		entry.CreatedAt = t.CreatedAt
		entry.FundState = "held"
		if t.BillingState == TaskBillingStateSettled || t.Status == TaskStatusSuccess {
			entry.FundState = "charged"
		}
		if t.Quota == 0 {
			entry.FundState = "closed"
		}
		// Refund projections use immutable committed deltas, including normal refunds.
		amount := refundSums[t.ID]
		entry.RefundedQuota = int(amount)
		if t.Quota == 0 && amount > 0 {
			entry.FundState = "refunded"
		}
		if amount > 0 {
			entry.RefundedAt = refundTimes[t.ID]
		}
		if !IsLinkVideoTaskClientProtocol(t.ClientProtocol) && !t.VideoFundingReady {
			entry.RefundAmountKnown = false
		}
		if t.Status == TaskStatusSuccess && t.PrivateData.ResultURL == "" {
			entry.Delivery = "result_unavailable"
		}
		entry.Version = videoTaskRefundVersion(t)
		entry.CanRefund = t.Quota > 0 || (t.PrivateData.AsyncBilling != nil && t.PrivateData.AsyncBilling.State == TaskBillingStateDebt)
		if !IsLinkVideoTaskClientProtocol(t.ClientProtocol) && !t.VideoFundingReady {
			entry.CanRefund = false
			entry.Failure = "funding_evidence_required"
		}
		refund = t.VideoRefund
	default:
		return nil, fmt.Errorf("invalid video funding owner")
	}
	if refund.VideoRefundState != "" {
		entry.FundState = refund.VideoRefundState
		entry.RefundedQuota += refund.VideoRefundQuota
		// Attempt's actual release is already included above.
		if kind == "attempt" {
			entry.RefundedQuota = refund.VideoRefundQuota
		}
		entry.WaivedQuota = refund.VideoRefundWaivedQuota
		entry.OperatorID, entry.Note = refund.VideoRefundOperatorID, refund.VideoRefundNote
		entry.RefundedAt, entry.RetryAt = refund.VideoRefundCompletedAt, refund.VideoRefundRetryAt
		if refund.VideoRefundFailure != "" {
			entry.Failure = refund.VideoRefundFailure
		}
		entry.CanRefund = false
	}
	if entry.Source != "wallet" {
		entry.CanRefund = false
		entry.Failure = "unsupported_funding_source"
	}
	if entry.RetryAt > 0 && entry.RetryAt <= common.GetTimestamp() && entry.Failure == "" {
		entry.Failure = "refund_retry_pending"
	}
	return entry, nil
}

// User usage repair supports the one-off users.used_quota statistical
// correction documented in docs/80-dev/2026-09-23-用户历史用量虚高分析与修复方案.md.
// It only adjusts the cumulative usage projection; wallet balance, request
// count, task funds, provider exposure and business logs are never written.
package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

const (
	// UsageRepairAction is the audit action reserved for this tool. Entries are
	// one-off maintenance evidence; they never become a billing ledger.
	UsageRepairAction = "repair_user_usage"

	// UsageRepairBucketDeliveryMatched counts refund logs whose stable
	// request_id exactly joins a TaskBillingDelivery row. The delivery
	// transaction already applied them to the cumulative statistic, so they
	// are excluded from any correction delta.
	UsageRepairBucketDeliveryMatched = "delivery_matched"
	// UsageRepairBucketPrefixWithoutDelivery counts refund logs shaped like
	// task-billing:<row>:<event> without a matching delivery row. They are
	// itemized for operator verification and are never auto-corrected.
	UsageRepairBucketPrefixWithoutDelivery = "prefix_without_delivery"
	// UsageRepairBucketOtherUnmatched counts every other refund log without a
	// delivery record, e.g. old-version refunds that never decremented the
	// cumulative statistic.
	UsageRepairBucketOtherUnmatched = "other_unmatched"

	// UsageRepairStatusCorrectable means the evidence supports a derived,
	// provable correction delta.
	UsageRepairStatusCorrectable = "correctable"
	// UsageRepairStatusZeroPending means the cumulative value already equals
	// the verified target; no adjustment is pending.
	UsageRepairStatusZeroPending = "zero_pending"
	// UsageRepairStatusDriftAfterRepair means a prior repair exists and the
	// account moved off its recorded target; a new baseline is required.
	UsageRepairStatusDriftAfterRepair = "drift_after_repair"
	// UsageRepairStatusNotCorrectable means the evidence is incomplete or
	// conflicts; writing is refused.
	UsageRepairStatusNotCorrectable = "not_correctable"
)

// UsageRepairRefundItem itemizes one refund log of the prefix bucket so an
// operator can verify the original pre-consumption evidence offline.
type UsageRepairRefundItem struct {
	LogID                int64    `json:"log_id"`
	Quota                int64    `json:"quota"`
	RequestID            string   `json:"request_id"`
	TaskRowID            int64    `json:"task_row_id"`
	TaskExists           bool     `json:"task_exists"`
	TaskDeliveryEvents   []string `json:"task_delivery_events"`
	ManualRefund         bool     `json:"manual_refund"`
	OriginalPreauthLogID int64    `json:"original_preauth_log_id"`
	PreauthExists        bool     `json:"preauth_exists"`
	PreauthQuota         int64    `json:"preauth_quota"`
	Operator             string   `json:"operator"`
}

// UsageRepairBucket is one mutually exclusive refund classification. Bucket
// counts and quota sums must add up to the total refund log population.
type UsageRepairBucket struct {
	Kind  string                  `json:"kind"`
	Count int64                   `json:"count"`
	Quota int64                   `json:"quota"`
	Items []UsageRepairRefundItem `json:"items,omitempty"`
}

// UsageRepairChannelUsage reports the cumulative statistic of one channel
// referenced by the user's logs. Channels mix traffic from many users, so the
// values are informational only and never derived from the user delta.
type UsageRepairChannelUsage struct {
	ChannelID int   `json:"channel_id"`
	UsedQuota int64 `json:"used_quota"`
}

// UsageRepairEvidence is the frozen source-evidence snapshot of one user.
// Every field participates in the fingerprint, so any new consume, refund,
// delivery or manual correction record invalidates a previously built
// correction manifest even when the amounts cancel out.
type UsageRepairEvidence struct {
	NegativeQuotaRows   int64                     `json:"negative_quota_rows,omitempty"`
	Sources             []UsageRepairSourceDigest `json:"sources"`
	PriorOps            []UsageRepairPriorOp      `json:"prior_ops"`
	UserID              int                       `json:"user_id"`
	Username            string                    `json:"username"`
	UsedQuota           int64                     `json:"used_quota"`
	WalletQuota         int64                     `json:"wallet_quota"`
	RequestCount        int64                     `json:"request_count"`
	ConsumeCount        int64                     `json:"consume_count"`
	ConsumeQuota        int64                     `json:"consume_quota"`
	RefundCount         int64                     `json:"refund_count"`
	RefundQuota         int64                     `json:"refund_quota"`
	MaxLogID            int64                     `json:"max_log_id"`
	MinLogCreatedAt     int64                     `json:"min_log_created_at"`
	MaxLogCreatedAt     int64                     `json:"max_log_created_at"`
	DuplicateRequestIDs int64                     `json:"duplicate_request_ids"`
	UndeliveredEvents   int64                     `json:"undelivered_events"`
	DeliveryCount       int64                     `json:"delivery_count"`
	QuotaDataSum        int64                     `json:"quota_data_sum"`
	Buckets             []UsageRepairBucket       `json:"buckets"`
	ChannelUsages       []UsageRepairChannelUsage `json:"channel_usages"`
}

// Fingerprint returns the SHA-256 marker of the evidence snapshot. It is the
// source-identity check enforced again inside the apply transaction.
func (e *UsageRepairEvidence) Fingerprint() (string, error) {
	raw, err := common.Marshal(e)
	if err != nil {
		return "", fmt.Errorf("cannot encode usage repair evidence: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// UsageRepairCandidate is the derived, evidence-proven statistic adjustment.
type UsageRepairCandidate struct {
	ExpectedBefore int64 `json:"expected_before"`
	Delta          int64 `json:"delta"`
	ExpectedAfter  int64 `json:"expected_after"`
}

// UsageRepairAssessment is the read-only verdict of one preview.
type UsageRepairAssessment struct {
	Status    string                `json:"status"`
	Candidate *UsageRepairCandidate `json:"candidate,omitempty"`
	Issues    []string              `json:"issues,omitempty"`
}

// UsageRepairPriorOp is one previously recorded repair operation read back
// from the audit trail. Entries are ordered newest first.
type UsageRepairPriorOp struct {
	EventID       string `json:"event_id"`
	Success       bool   `json:"success"`
	CreatedAt     int64  `json:"created_at"`
	TargetUserID  int    `json:"target_user_id"`
	ExpectedAfter int64  `json:"expected_after"`
	Delta         int64  `json:"delta"`
	RollbackOf    string `json:"rollback_of"`
}

// UsageRepairManifest is the operator-reviewed correction checklist. Apply
// re-verifies every field against a fresh, locked snapshot before writing.
type UsageRepairManifest struct {
	Policy                     string               `json:"policy,omitempty"`
	Review                     *UsageRepairReview   `json:"review"`
	Version                    int                  `json:"version"`
	OperationID                string               `json:"operation_id"`
	UserID                     int                  `json:"user_id"`
	Username                   string               `json:"username"`
	ExpectedBefore             int64                `json:"expected_before"`
	Delta                      int64                `json:"delta"`
	ExpectedAfter              int64                `json:"expected_after"`
	ManualRefundBucketVerified bool                 `json:"manual_refund_bucket_verified"`
	EvidenceFingerprint        string               `json:"evidence_fingerprint"`
	Evidence                   *UsageRepairEvidence `json:"evidence"`
	SourceDBIdentity           string               `json:"source_db_identity"`
	CollectedAtUnix            int64                `json:"collected_at_unix"`
	CollectedAtUTCOffset       string               `json:"collected_at_utc_offset"`
	PreparedByUserID           int                  `json:"prepared_by_user_id"`
	RollbackOf                 string               `json:"rollback_of,omitempty"`
}

type usageRepairSumRow struct {
	C int64
	S int64
}

// CollectUserUsageRepairEvidence freezes the statistical source evidence of
// one user. logDB may be the same handle as mainDB; refund and consume logs
// are read from logDB while user, channel, task, delivery and quota_data
// facts are read from mainDB. The function never writes.
func CollectUserUsageRepairEvidence(mainDB *gorm.DB, logDB *gorm.DB, userID int) (*UsageRepairEvidence, error) {
	e := &UsageRepairEvidence{UserID: userID}
	var userRow struct {
		UsedQuota    int64
		Quota        int64
		RequestCount int64
		Username     string
	}
	if err := mainDB.Table("users").Select("used_quota", "quota", "request_count", "username").
		Where("id = ?", userID).Take(&userRow).Error; err != nil {
		return nil, fmt.Errorf("cannot read user %d: %w", userID, err)
	}
	e.UsedQuota = userRow.UsedQuota
	e.WalletQuota = userRow.Quota
	e.RequestCount = userRow.RequestCount
	e.Username = userRow.Username

	var consumeRow usageRepairSumRow
	if err := logDB.Table("logs").Select("COUNT(*) AS c", "COALESCE(SUM(quota), 0) AS s").
		Where("user_id = ? AND type = ?", userID, LogTypeConsume).
		Take(&consumeRow).Error; err != nil {
		return nil, fmt.Errorf("cannot sum consume logs: %w", err)
	}
	e.ConsumeCount, e.ConsumeQuota = consumeRow.C, consumeRow.S
	if err := logDB.Table("logs").Where("user_id = ? AND type IN ? AND quota < 0", userID, []int{LogTypeConsume, LogTypeRefund}).Count(&e.NegativeQuotaRows).Error; err != nil {
		return nil, fmt.Errorf("cannot validate usage log amounts: %w", err)
	}

	var refundRow usageRepairSumRow
	if err := logDB.Table("logs").Select("COUNT(*) AS c", "COALESCE(SUM(quota), 0) AS s").
		Where("user_id = ? AND type = ?", userID, LogTypeRefund).
		Take(&refundRow).Error; err != nil {
		return nil, fmt.Errorf("cannot sum refund logs: %w", err)
	}
	e.RefundCount, e.RefundQuota = refundRow.C, refundRow.S

	var rangeRow struct {
		Min int64
		Max int64
		ID  int64
	}
	if err := logDB.Table("logs").Select("COALESCE(MIN(created_at), 0) AS min", "COALESCE(MAX(created_at), 0) AS max", "COALESCE(MAX(id), 0) AS id").
		Where("user_id = ?", userID).Take(&rangeRow).Error; err != nil {
		return nil, fmt.Errorf("cannot read log range: %w", err)
	}
	e.MinLogCreatedAt = rangeRow.Min
	e.MaxLogCreatedAt = rangeRow.Max
	e.MaxLogID = rangeRow.ID

	duplicateGroups := logDB.Table("logs").Select("request_id").
		Where("user_id = ? AND type IN ? AND request_id <> ''", userID, []int{LogTypeConsume, LogTypeRefund}).
		Group("request_id").Having("COUNT(*) > 1")
	if err := logDB.Table("(?) AS duplicate_groups", duplicateGroups).Count(&e.DuplicateRequestIDs).Error; err != nil {
		return nil, fmt.Errorf("cannot check duplicate request ids: %w", err)
	}

	if mainDB.Table("quota_data").Migrator().HasTable("quota_data") {
		var quotaRow usageRepairSumRow
		if err := mainDB.Table("quota_data").Select("COALESCE(SUM(quota), 0) AS s").
			Where("user_id = ?", userID).Take(&quotaRow).Error; err != nil {
			return nil, fmt.Errorf("cannot sum quota_data: %w", err)
		}
		e.QuotaDataSum = quotaRow.S
	}
	deliveryKeys, taskEvents, err := collectUsageRepairDeliveries(mainDB, userID)
	if err != nil {
		return nil, err
	}
	e.DeliveryCount = int64(len(deliveryKeys))
	e.UndeliveredEvents, err = countUndeliveredEvents(mainDB, userID)
	if err != nil {
		return nil, err
	}
	var refunds []Log
	if err := logDB.Select("id", "user_id", "quota", "request_id", "other").
		Where("user_id = ? AND type = ?", userID, LogTypeRefund).
		Order("id").Find(&refunds).Error; err != nil {
		return nil, fmt.Errorf("cannot read refund logs: %w", err)
	}
	e.Buckets, err = classifyUsageRepairRefunds(refunds, deliveryKeys, taskEvents, mainDB, logDB)
	if err != nil {
		return nil, err
	}
	e.ChannelUsages, err = collectUsageRepairChannels(mainDB, logDB, userID)
	if err != nil {
		return nil, err
	}
	e.Sources, err = collectUsageRepairSourceDigests(mainDB, logDB, userID)
	if err != nil {
		return nil, err
	}
	e.PriorOps, err = CollectUsageRepairPriorOps(logDB, userID)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// collectUsageRepairDeliveries returns the stable request-id keys of every
// delivery event of the user's tasks, plus per-task delivery event lists.
func collectUsageRepairDeliveries(db *gorm.DB, userID int) (map[string]bool, map[int64][]string, error) {
	if !db.Migrator().HasTable("tasks") || !db.Migrator().HasTable("task_billing_deliveries") {
		return map[string]bool{}, map[int64][]string{}, nil
	}
	var deliveries []deliveryRow
	if err := db.Table("task_billing_deliveries").Select("task_row_id", "event").
		Where("task_row_id IN (?)", db.Table("tasks").Select("id").Where("user_id = ?", userID)).
		Order("id").Find(&deliveries).Error; err != nil {
		return nil, nil, fmt.Errorf("cannot read task billing deliveries: %w", err)
	}
	keys := make(map[string]bool, len(deliveries))
	events := make(map[int64][]string, len(deliveries))
	for _, d := range deliveries {
		keys[usageRepairRequestID(d.TaskRowID, d.Event)] = true
		events[d.TaskRowID] = append(events[d.TaskRowID], d.Event)
	}
	return keys, events, nil
}

// deliveryRow is one delivery event of the user's tasks.
type deliveryRow struct {
	TaskRowID int64
	Event     string
}

func countUndeliveredEvents(db *gorm.DB, userID int) (int64, error) {
	if !db.Migrator().HasTable("tasks") || !db.Migrator().HasTable("task_billing_deliveries") {
		return 0, nil
	}
	var count int64
	if err := db.Table("task_billing_deliveries").
		Where("delivered_at = 0 AND task_row_id IN (?)", db.Table("tasks").Select("id").Where("user_id = ?", userID)).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("cannot count undelivered events: %w", err)
	}
	return count, nil
}

// usageRepairRequestID rebuilds the stable request id of a delivery event.
func usageRepairRequestID(taskRowID int64, event string) string {
	return fmt.Sprintf("task-billing:%d:%s", taskRowID, event)
}

// classifyUsageRepairRefunds sorts refund logs into the three mutually
// exclusive buckets. Bucket totals must equal the full refund population.
func classifyUsageRepairRefunds(refunds []Log, deliveryKeys map[string]bool, taskEvents map[int64][]string, db, logDB *gorm.DB) ([]UsageRepairBucket, error) {
	buckets := map[string]*UsageRepairBucket{
		UsageRepairBucketDeliveryMatched:       {Kind: UsageRepairBucketDeliveryMatched},
		UsageRepairBucketPrefixWithoutDelivery: {Kind: UsageRepairBucketPrefixWithoutDelivery},
		UsageRepairBucketOtherUnmatched:        {Kind: UsageRepairBucketOtherUnmatched},
	}
	for i := range refunds {
		log := &refunds[i]
		quota := int64(log.Quota)
		if quota < 0 {
			return nil, fmt.Errorf("refund %d has negative quota", log.Id)
		}
		for _, bucket := range buckets {
			if bucket.Quota > math.MaxInt64-quota {
				return nil, fmt.Errorf("refund sum overflows int64")
			}
		}
		switch {
		case log.RequestId != "" && deliveryKeys[log.RequestId]:
			buckets[UsageRepairBucketDeliveryMatched].Count++
			buckets[UsageRepairBucketDeliveryMatched].Quota += quota
		case strings.HasPrefix(log.RequestId, "task-billing:"):
			item, err := buildUsageRepairItem(log, taskEvents, db, logDB)
			if err != nil {
				return nil, err
			}
			bucket := buckets[UsageRepairBucketPrefixWithoutDelivery]
			bucket.Count++
			bucket.Quota += quota
			bucket.Items = append(bucket.Items, item)
		default:
			buckets[UsageRepairBucketOtherUnmatched].Count++
			buckets[UsageRepairBucketOtherUnmatched].Quota += quota
			buckets[UsageRepairBucketOtherUnmatched].Items = append(buckets[UsageRepairBucketOtherUnmatched].Items, UsageRepairRefundItem{LogID: int64(log.Id), Quota: quota, RequestID: log.RequestId})
		}
	}
	return []UsageRepairBucket{
		*buckets[UsageRepairBucketDeliveryMatched],
		*buckets[UsageRepairBucketPrefixWithoutDelivery],
		*buckets[UsageRepairBucketOtherUnmatched],
	}, nil
}

// buildUsageRepairItem verifies the offline-verifiable evidence of one
// prefix-bucket refund: its task row, the delivery events already recorded
// for that task, and the referenced original pre-consumption log.
func buildUsageRepairItem(log *Log, taskEvents map[int64][]string, db, logDB *gorm.DB) (UsageRepairRefundItem, error) {
	item := UsageRepairRefundItem{
		LogID:     int64(log.Id),
		Quota:     int64(log.Quota),
		RequestID: log.RequestId,
	}
	if log.Other != "" {
		var other struct {
			Admin struct {
				ManualRefund         bool            `json:"manual_refund"`
				Operator             string          `json:"operator"`
				OriginalPreauthLogID json.RawMessage `json:"original_preauth_log_id"`
			} `json:"admin_info"`
		}
		if err := common.UnmarshalJsonStr(log.Other, &other); err != nil {
			return item, fmt.Errorf("cannot decode refund log %d metadata", log.Id)
		}
		item.ManualRefund = other.Admin.ManualRefund
		item.Operator = other.Admin.Operator
		raw := string(other.Admin.OriginalPreauthLogID)
		if raw != "" && raw != "null" {
			if strings.HasPrefix(raw, "\"") {
				if err := common.Unmarshal(other.Admin.OriginalPreauthLogID, &raw); err != nil {
					return item, fmt.Errorf("invalid preauth reference on refund %d", log.Id)
				}
			}
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || id <= 0 {
				return item, fmt.Errorf("invalid preauth reference on refund %d", log.Id)
			}
			item.OriginalPreauthLogID = id
		}
	}
	if strings.HasPrefix(log.RequestId, "task-billing:") {
		parts := strings.Split(log.RequestId, ":")
		if len(parts) == 3 {
			item.TaskRowID, _ = strconv.ParseInt(parts[1], 10, 64)
		}
	}
	if item.TaskRowID > 0 {
		var taskRow struct {
			ID int64
		}
		err := db.Table("tasks").Select("id").Where("id = ? AND user_id = ?", item.TaskRowID, log.UserId).Take(&taskRow).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return item, err
		}
		item.TaskExists = err == nil
		item.TaskDeliveryEvents = taskEvents[item.TaskRowID]
	}
	if item.OriginalPreauthLogID > 0 {
		var preauth struct {
			Quota int64
		}
		if err := logDB.Table("logs").Select("quota").
			Where("id = ? AND user_id = ? AND type = ?", item.OriginalPreauthLogID, log.UserId, LogTypeConsume).
			Take(&preauth).Error; err == nil {
			item.PreauthExists = true
			item.PreauthQuota = preauth.Quota
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return item, err
		}
	}
	return item, nil
}

// collectUsageRepairChannels lists the cumulative statistics of every channel
// referenced by the user's consume or refund logs. Values are informational:
// channels mix traffic from many users and are never derived from the delta.
func collectUsageRepairChannels(mainDB *gorm.DB, logDB *gorm.DB, userID int) ([]UsageRepairChannelUsage, error) {
	var channelIDs []int
	if err := logDB.Table("logs").Select("DISTINCT channel_id").
		Where("user_id = ? AND type IN ?", userID, []int{LogTypeConsume, LogTypeRefund}).
		Pluck("channel_id", &channelIDs).Error; err != nil {
		return nil, fmt.Errorf("cannot list user channel ids: %w", err)
	}
	if len(channelIDs) == 0 || !mainDB.Migrator().HasTable("channels") {
		return []UsageRepairChannelUsage{}, nil
	}
	var channels []struct {
		ID        int
		UsedQuota int64
	}
	if err := mainDB.Table("channels").Select("id", "used_quota").
		Where("id IN ?", channelIDs).Order("id").Find(&channels).Error; err != nil {
		return nil, fmt.Errorf("cannot read channel usage: %w", err)
	}
	usages := make([]UsageRepairChannelUsage, 0, len(channels))
	for _, c := range channels {
		usages = append(usages, UsageRepairChannelUsage{ChannelID: c.ID, UsedQuota: c.UsedQuota})
	}
	return usages, nil
}

// usageRepairAuditContent is the summary embedded in the audit row Content.
// It carries identifiers and numbers only, never keys or provider payloads.
type usageRepairAuditContent struct {
	Policy                     string                  `json:"policy,omitempty"`
	RollbackBaseline           string                  `json:"rollback_baseline"`
	PriorAuditDigest           UsageRepairSourceDigest `json:"prior_audit_digest"`
	ReviewFingerprint          string                  `json:"review_fingerprint"`
	ReviewedByUserID           int                     `json:"reviewed_by_user_id"`
	OperationID                string                  `json:"operation_id"`
	UserID                     int                     `json:"user_id"`
	Username                   string                  `json:"username"`
	Before                     int64                   `json:"before"`
	Delta                      int64                   `json:"delta"`
	After                      int64                   `json:"after"`
	EvidenceFingerprint        string                  `json:"evidence_fingerprint"`
	ManualRefundBucketVerified bool                    `json:"manual_refund_bucket_verified"`
	RollbackOf                 string                  `json:"rollback_of,omitempty"`
}

// CollectUsageRepairPriorOps reads previously recorded repair operations for
// one target user from the audit trail, newest first.
func CollectUsageRepairPriorOps(db *gorm.DB, userID int) ([]UsageRepairPriorOp, error) {
	if !db.Migrator().HasTable("audit_logs") {
		return []UsageRepairPriorOp{}, nil
	}
	var rows []AuditLog
	if err := db.Where("action = ?", UsageRepairAction).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("cannot read repair audit trail: %w", err)
	}
	ops := make([]UsageRepairPriorOp, 0, len(rows))
	for _, row := range rows {
		var content usageRepairAuditContent
		if err := common.UnmarshalJsonStr(row.Content, &content); err != nil {
			return nil, fmt.Errorf("cannot decode maintenance audit %d", row.Id)
		}
		if content.UserID != userID {
			continue
		}
		ops = append(ops, UsageRepairPriorOp{
			EventID:       row.EventId,
			Success:       row.Success,
			CreatedAt:     row.CreatedAt,
			TargetUserID:  content.UserID,
			ExpectedAfter: content.After,
			Delta:         content.Delta,
			RollbackOf:    content.RollbackOf,
		})
	}
	return ops, nil
}

// DeriveUsageRepairAssessment turns one frozen evidence snapshot into the
// preview verdict. Only independently reviewed missed decrements contribute
// to the adjustment; log net amount is a cross-check, never its authority.
func DeriveUsageRepairAssessment(e *UsageRepairEvidence, ops []UsageRepairPriorOp, review *UsageRepairReview) *UsageRepairAssessment {
	assessment := &UsageRepairAssessment{}
	var bucketCount, bucketQuota int64
	for _, bucket := range e.Buckets {
		if bucket.Count < 0 || bucket.Quota < 0 || bucketCount > math.MaxInt64-bucket.Count || bucketQuota > math.MaxInt64-bucket.Quota {
			assessment.Status = UsageRepairStatusNotCorrectable
			assessment.Issues = append(assessment.Issues, "invalid or overflowing refund totals")
			return assessment
		}
		bucketCount += bucket.Count
		bucketQuota += bucket.Quota
	}
	if bucketCount != e.RefundCount || bucketQuota != e.RefundQuota {
		assessment.Status = UsageRepairStatusNotCorrectable
		assessment.Issues = append(assessment.Issues, "refund bucket totals do not equal the refund log population")
		return assessment
	}
	if e.UndeliveredEvents > 0 {
		assessment.Issues = append(assessment.Issues, fmt.Sprintf("%d undelivered delivery events exist; settle delivery first", e.UndeliveredEvents))
	}
	if e.DuplicateRequestIDs > 0 {
		assessment.Issues = append(assessment.Issues, fmt.Sprintf("%d duplicate request ids exist", e.DuplicateRequestIDs))
	}
	if e.UndeliveredEvents > 0 || e.DuplicateRequestIDs > 0 {
		assessment.Status = UsageRepairStatusNotCorrectable
		return assessment
	}
	if len(ops) > 0 && ops[0].RollbackOf == "" {
		latest := ops[0]
		if !latest.Success {
			assessment.Status = UsageRepairStatusNotCorrectable
			assessment.Issues = append(assessment.Issues, "the latest recorded repair operation was not successful")
			return assessment
		}
		if e.UsedQuota == latest.ExpectedAfter {
			assessment.Status = UsageRepairStatusZeroPending
			return assessment
		}
		assessment.Status = UsageRepairStatusDriftAfterRepair
		assessment.Issues = append(assessment.Issues, fmt.Sprintf("used_quota %d differs from the latest repair target %d; rebuild the baseline", e.UsedQuota, latest.ExpectedAfter))
		return assessment
	}
	delta, err := reviewedUsageRepairDelta(e, review)
	if err != nil {
		assessment.Status = UsageRepairStatusNotCorrectable
		assessment.Issues = append(assessment.Issues, err.Error())
		return assessment
	}
	target := e.UsedQuota + delta // delta is bounded by nonnegative used_quota
	if e.ConsumeQuota < 0 || e.RefundQuota < 0 || target != e.ConsumeQuota-e.RefundQuota {
		assessment.Status = UsageRepairStatusNotCorrectable
		assessment.Issues = append(assessment.Issues, "reviewed adjustment conflicts with log net amount; investigate missing history or other writers")
		return assessment
	}
	if delta == 0 {
		assessment.Status = UsageRepairStatusZeroPending
		return assessment
	}

	assessment.Status = UsageRepairStatusCorrectable
	assessment.Candidate = &UsageRepairCandidate{
		ExpectedBefore: e.UsedQuota,
		Delta:          target - e.UsedQuota,
		ExpectedAfter:  target,
	}
	return assessment
}

// ValidateUsageRepairManifest checks manifest self-consistency before any
// database access: arithmetic, fingerprint binding and the manual-refund
// verification gate.
func ValidateUsageRepairManifest(m *UsageRepairManifest) error {
	if m == nil {
		return fmt.Errorf("manifest is required")
	}
	if m.Version != 2 && m.Version != 3 {
		return fmt.Errorf("unsupported manifest version %d", m.Version)
	}
	if (m.Version == 2 && m.Policy != "") || (m.Version == 3 && (m.Policy != usageRecordedRefundPolicy || m.Review != nil)) {
		return fmt.Errorf("invalid usage repair policy")
	}
	if m.OperationID == "" || len(m.OperationID) > 64 {
		return fmt.Errorf("operation id is required (max 64 chars)")
	}
	if m.UserID <= 0 {
		return fmt.Errorf("manifest user id is required")
	}
	if m.Evidence == nil {
		return fmt.Errorf("manifest evidence snapshot is required")
	}
	if m.Evidence.UserID != m.UserID {
		return fmt.Errorf("manifest user id does not match the evidence snapshot")
	}
	if m.ExpectedBefore < 0 || m.ExpectedAfter < 0 || (m.Delta > 0 && m.ExpectedBefore > math.MaxInt64-m.Delta) || (m.Delta < 0 && m.Delta < -m.ExpectedBefore) {
		return fmt.Errorf("manifest arithmetic is negative or overflows int64")
	}
	if m.ExpectedAfter != m.ExpectedBefore+m.Delta {
		return fmt.Errorf("manifest arithmetic is inconsistent: after != before + delta")
	}
	fingerprint, err := m.Evidence.Fingerprint()
	if err != nil {
		return err
	}
	if fingerprint != m.EvidenceFingerprint {
		return fmt.Errorf("manifest fingerprint does not match the embedded evidence")
	}
	if m.Evidence.UsedQuota != m.ExpectedBefore {
		return fmt.Errorf("manifest expected_before %d does not match the evidence used_quota %d", m.ExpectedBefore, m.Evidence.UsedQuota)
	}
	if m.RollbackOf == "" && m.Version == 3 {
		candidate, err := recordedRefundUsageCandidate(m.Evidence)
		if err != nil || candidate == nil || *candidate != (UsageRepairCandidate{ExpectedBefore: m.ExpectedBefore, Delta: m.Delta, ExpectedAfter: m.ExpectedAfter}) {
			return fmt.Errorf("manifest differs from recorded refund policy: %v", err)
		}
	} else if m.RollbackOf == "" {
		if m.Review == nil || m.Review.SourceDBIdentity != m.SourceDBIdentity {
			return fmt.Errorf("review source identity mismatch")
		}
		assessment := DeriveUsageRepairAssessment(m.Evidence, m.Evidence.PriorOps, m.Review)
		if assessment.Candidate == nil || assessment.Status != UsageRepairStatusCorrectable {
			return fmt.Errorf("manifest has no proven correction: %v", assessment.Issues)
		}
		if m.Delta != assessment.Candidate.Delta || m.ExpectedAfter != assessment.Candidate.ExpectedAfter {
			return fmt.Errorf("manifest target differs from reviewed adjustment")
		}
	} else if m.Delta <= 0 {
		return fmt.Errorf("rollback must reverse a negative correction")
	}
	for _, bucket := range m.Evidence.Buckets {
		if bucket.Kind == UsageRepairBucketPrefixWithoutDelivery && bucket.Count > 0 && m.Version == 2 && !m.ManualRefundBucketVerified {
			return fmt.Errorf("the prefix refund bucket holds %d unverified entries; attest manual_refund_bucket_verified after offline review", bucket.Count)
		}
	}
	return nil
}

// ApplyUserUsageRepair executes the reviewed correction in one main-database
// transaction: it locks the user row, re-derives the evidence snapshot, and
// writes the new cumulative value plus a one-off audit record atomically.
// It returns applied=false when the operation was already applied before.
// The caller must hold a maintenance stop-write window; this function does
// not verify deployment-wide writer quiescence.
func ApplyUserUsageRepair(db *gorm.DB, m *UsageRepairManifest, operatorID int) (bool, error) {
	if err := ValidateUsageRepairManifest(m); err != nil {
		return false, err
	}
	if !db.Migrator().HasTable("logs") || !db.Migrator().HasTable("audit_logs") {
		return false, fmt.Errorf("logs and audit_logs must live in this database; split-database deployments are preview-only")
	}
	var operator struct {
		Username string
		Role     int
		Status   int
	}
	if err := db.Table("users").Select("username", "role", "status").Where("id = ?", operatorID).Take(&operator).Error; err != nil {
		return false, fmt.Errorf("cannot read operator %d: %w", operatorID, err)
	}
	if operator.Role != common.RoleRootUser || operator.Status != common.UserStatusEnabled {
		return false, fmt.Errorf("operator %d is not a root user", operatorID)
	}
	if m.Review != nil {
		var reviewers int64
		if err := db.Table("users").Where("id = ? AND role = ? AND status = ?", m.Review.ReviewedByUserID, common.RoleRootUser, common.UserStatusEnabled).Count(&reviewers).Error; err != nil {
			return false, err
		}
		if reviewers != 1 {
			return false, fmt.Errorf("reviewer is not an active root user")
		}
	}
	return applyUserUsageRepair(db, m, AuditLog{UserId: operatorID, Username: operator.Username, ActorRole: common.RoleRootUser, AuthMethod: "maintenance_tool"})
}

// Both startup and the maintenance CLI use this transaction. Each entry point
// supplies its actual actor; the transaction independently verifies its policy.
func applyUserUsageRepair(db *gorm.DB, m *UsageRepairManifest, actor AuditLog) (bool, error) {
	if err := ValidateUsageRepairManifest(m); err != nil {
		return false, err
	}
	applied := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var userRow struct {
			UsedQuota int64
			Username  string
		}
		if err := lockForUpdate(tx.Table("users").Select("used_quota", "username").Where("id = ?", m.UserID)).Take(&userRow).Error; err != nil {
			return fmt.Errorf("cannot lock user %d: %w", m.UserID, err)
		}
		var prior AuditLog
		priorErr := tx.Where("event_id = ?", m.OperationID).Limit(1).Take(&prior).Error
		if priorErr != nil && !errors.Is(priorErr, gorm.ErrRecordNotFound) {
			return fmt.Errorf("cannot check prior operation: %w", priorErr)
		}
		if priorErr == nil {
			if usageRepairAuditMatches(&prior, m, userRow.UsedQuota) {
				return nil
			}
			return fmt.Errorf("operation %s is already recorded but its audit or account state differs", m.OperationID)
		}
		if userRow.UsedQuota != m.ExpectedBefore {
			return fmt.Errorf("used_quota %d does not match expected_before %d; manifest is stale", userRow.UsedQuota, m.ExpectedBefore)
		}
		evidence, err := CollectUserUsageRepairEvidence(tx, tx, m.UserID)
		if err != nil {
			return err
		}
		fingerprint, err := evidence.Fingerprint()
		if err != nil {
			return err
		}
		if fingerprint != m.EvidenceFingerprint {
			return fmt.Errorf("source evidence changed since the manifest was built; regenerate the manifest")
		}
		if m.RollbackOf == "" && m.Version == 3 {
			candidate, err := recordedRefundUsageCandidate(evidence)
			if err != nil || candidate == nil || candidate.Delta != m.Delta || candidate.ExpectedAfter != m.ExpectedAfter {
				return fmt.Errorf("fresh evidence does not satisfy recorded refund policy: %v", err)
			}
		} else if m.RollbackOf == "" {
			assessment := DeriveUsageRepairAssessment(evidence, evidence.PriorOps, m.Review)
			if assessment.Candidate == nil || assessment.Candidate.Delta != m.Delta || assessment.Candidate.ExpectedAfter != m.ExpectedAfter {
				return fmt.Errorf("fresh evidence does not prove this adjustment")
			}
		} else {
			var original AuditLog
			if err := tx.Where("event_id = ? AND action = ? AND success = ?", m.RollbackOf, UsageRepairAction, true).Take(&original).Error; err != nil {
				return fmt.Errorf("rollback requires a successful original repair")
			}
			var recorded usageRepairAuditContent
			if err := common.UnmarshalJsonStr(original.Content, &recorded); err != nil {
				return err
			}
			if recorded.Policy != m.Policy || recorded.UserID != m.UserID || recorded.RollbackOf != "" || recorded.Delta >= 0 || recorded.Delta == math.MinInt64 || recorded.Delta != -m.Delta || recorded.Before != m.ExpectedAfter || recorded.After != m.ExpectedBefore {
				return fmt.Errorf("rollback differs from original repair")
			}
			if err := verifyUsageRepairRollbackBaseline(tx, evidence, &recorded, original.Id); err != nil {
				return err
			}
			for _, op := range evidence.PriorOps {
				if op.Success && op.RollbackOf == m.RollbackOf {
					return fmt.Errorf("original repair has already been rolled back")
				}
			}
		}
		result := tx.Table("users").Where("id = ? AND used_quota = ?", m.UserID, m.ExpectedBefore).
			UpdateColumn("used_quota", m.ExpectedAfter)
		if result.Error != nil {
			return fmt.Errorf("cannot update used_quota: %w", result.Error)
		}
		if result.RowsAffected != 1 && !(m.Delta == 0 && result.RowsAffected == 0) {
			return fmt.Errorf("used_quota changed while correcting; rolled back")
		}
		content := usageRepairAuditContent{
			Policy:                     m.Policy,
			OperationID:                m.OperationID,
			UserID:                     m.UserID,
			Username:                   m.Username,
			Before:                     m.ExpectedBefore,
			Delta:                      m.Delta,
			After:                      m.ExpectedAfter,
			EvidenceFingerprint:        m.EvidenceFingerprint,
			ManualRefundBucketVerified: m.ManualRefundBucketVerified,
			RollbackOf:                 m.RollbackOf,
		}
		content.RollbackBaseline, err = usageRepairRollbackBaseline(evidence)
		if err != nil {
			return err
		}
		for _, source := range evidence.Sources {
			if source.Table == "audit_logs" {
				content.PriorAuditDigest = source
			}
		}
		if m.Review != nil {
			content.ReviewFingerprint, err = m.Review.Fingerprint()
			if err != nil {
				return err
			}
			content.ReviewedByUserID = m.Review.ReviewedByUserID
		}
		encoded, err := common.Marshal(content)
		if err != nil {
			return fmt.Errorf("cannot encode audit content: %w", err)
		}
		audit := AuditLog{
			EventId:    m.OperationID,
			UserId:     actor.UserId,
			Username:   actor.Username,
			ActorRole:  actor.ActorRole,
			CreatedAt:  time.Now().Unix(),
			Category:   AuditCategoryOperation,
			Action:     UsageRepairAction,
			AuthMethod: actor.AuthMethod,
			Success:    true,
			RequestId:  m.OperationID,
			Content:    string(encoded),
		}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("cannot write the repair audit record; rolled back: %w", err)
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

// UsageRepairAlreadyApplied allows the command to retry a completed operation
// without creating another backup. It never performs a write.
func UsageRepairAlreadyApplied(db *gorm.DB, m *UsageRepairManifest) (bool, error) {
	if err := ValidateUsageRepairManifest(m); err != nil {
		return false, err
	}
	var prior AuditLog
	err := db.Where("event_id = ?", m.OperationID).Take(&prior).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var user User
	if err := db.Select("id", "used_quota").Take(&user, m.UserID).Error; err != nil {
		return false, err
	}
	if !usageRepairAuditMatches(&prior, m, int64(user.UsedQuota)) {
		return false, fmt.Errorf("operation already recorded but its audit or account state differs")
	}
	return true, nil
}

func usageRepairAuditMatches(prior *AuditLog, m *UsageRepairManifest, usedQuota int64) bool {
	var recorded usageRepairAuditContent
	if common.UnmarshalJsonStr(prior.Content, &recorded) != nil {
		return false
	}
	reviewFingerprint := ""
	if m.Review != nil {
		var err error
		reviewFingerprint, err = m.Review.Fingerprint()
		if err != nil {
			return false
		}
	}
	return prior.Action == UsageRepairAction && prior.Success && recorded.OperationID == m.OperationID &&
		recorded.UserID == m.UserID && recorded.Before == m.ExpectedBefore && recorded.Delta == m.Delta &&
		recorded.After == m.ExpectedAfter && recorded.Policy == m.Policy && recorded.RollbackOf == m.RollbackOf &&
		recorded.EvidenceFingerprint == m.EvidenceFingerprint && recorded.ReviewFingerprint == reviewFingerprint && usedQuota == m.ExpectedAfter
}

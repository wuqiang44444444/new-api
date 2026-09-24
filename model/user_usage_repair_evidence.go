package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Only digests leave the process; task snapshots and audit content must never
// be copied into an operator manifest or printed by the maintenance command.
type UsageRepairSourceDigest struct {
	Table  string `json:"table"`
	Rows   int64  `json:"rows"`
	SHA256 string `json:"sha256"`
}

func collectUsageRepairSourceDigests(mainDB, logDB *gorm.DB, userID int) ([]UsageRepairSourceDigest, error) {
	tasks := mainDB.Table("tasks").Select("id").Where("user_id = ?", userID)
	sources := []struct {
		name  string
		db    *gorm.DB
		query *gorm.DB
	}{
		{"logs", logDB, logDB.Table("logs").Where("user_id = ?", userID)},
		{"tasks", mainDB, mainDB.Table("tasks").Where("user_id = ?", userID)},
		{"task_create_attempts", mainDB, mainDB.Table("task_create_attempts").Where("user_id = ?", userID)},
		{"task_billing_deliveries", mainDB, mainDB.Table("task_billing_deliveries").Where("task_row_id IN (?)", tasks)},
		{"quota_data", mainDB, mainDB.Table("quota_data").Where("user_id = ?", userID)},
		// Maintenance targets can be embedded in content, not audit.user_id.
		// Hash the whole audit population rather than guessing target ownership.
		{"audit_logs", logDB, logDB.Table("audit_logs")},
	}
	result := make([]UsageRepairSourceDigest, 0, len(sources))
	for _, source := range sources {
		if !source.db.Migrator().HasTable(source.name) {
			return nil, fmt.Errorf("required evidence table %s is missing", source.name)
		}
		digest, err := usageRepairSourceDigest(source.name, source.query)
		if err != nil {
			return nil, err
		}
		result = append(result, digest)
	}
	return result, nil
}

// A review is offline evidence, not an automatically inferred ledger. Empty
// template fields deliberately prevent generating an executable manifest.
type UsageRepairReview struct {
	UserID                   int                       `json:"user_id"`
	SourceDBIdentity         string                    `json:"source_db_identity"`
	EvidenceFingerprint      string                    `json:"evidence_fingerprint"`
	ReviewedByUserID         int                       `json:"reviewed_by_user_id"`
	LogHistoryComplete       bool                      `json:"log_history_complete"`
	LogHistoryEvidence       string                    `json:"log_history_evidence"`
	PriorAdjustmentsEvidence string                    `json:"prior_adjustments_evidence"`
	Refunds                  []UsageRepairRefundReview `json:"refunds"`
}

type UsageRepairRefundReview struct {
	LogID             int64  `json:"log_id"`
	Quota             int64  `json:"quota"`
	Decision          string `json:"decision"`           // missed_usage_decrement / already_accounted
	EvidenceReference string `json:"evidence_reference"` // writer version, task/attempt, maintenance evidence
}

func NewUsageRepairReview(e *UsageRepairEvidence, source string) (*UsageRepairReview, error) {
	fingerprint, err := e.Fingerprint()
	if err != nil {
		return nil, err
	}
	r := &UsageRepairReview{UserID: e.UserID, SourceDBIdentity: source, EvidenceFingerprint: fingerprint}
	for _, bucket := range e.Buckets {
		if bucket.Kind == UsageRepairBucketDeliveryMatched {
			continue
		}
		for _, item := range bucket.Items {
			r.Refunds = append(r.Refunds, UsageRepairRefundReview{LogID: item.LogID, Quota: item.Quota})
		}
	}
	return r, nil
}

func reviewedUsageRepairDelta(e *UsageRepairEvidence, r *UsageRepairReview) (int64, error) {
	if r == nil || r.ReviewedByUserID <= 0 || !r.LogHistoryComplete ||
		strings.TrimSpace(r.LogHistoryEvidence) == "" || strings.TrimSpace(r.PriorAdjustmentsEvidence) == "" {
		return 0, fmt.Errorf("complete log history and prior adjustment evidence must be reviewed offline")
	}
	fingerprint, err := e.Fingerprint()
	if err != nil {
		return 0, err
	}
	if r.UserID != e.UserID || r.EvidenceFingerprint != fingerprint || strings.TrimSpace(r.SourceDBIdentity) == "" {
		return 0, fmt.Errorf("review does not match the current user and evidence snapshot")
	}
	remaining := make(map[int64]int64)
	for _, bucket := range e.Buckets {
		if bucket.Kind == UsageRepairBucketDeliveryMatched {
			continue
		}
		if int64(len(bucket.Items)) != bucket.Count {
			return 0, fmt.Errorf("refund evidence is not fully itemized")
		}
		for _, item := range bucket.Items {
			if item.ManualRefund && (!item.PreauthExists || item.PreauthQuota < item.Quota) {
				return 0, fmt.Errorf("manual refund %d lacks sufficient original preauth evidence", item.LogID)
			}
			if item.Quota <= 0 {
				return 0, fmt.Errorf("refund %d has invalid quota", item.LogID)
			}
			remaining[item.LogID] = item.Quota
		}
	}
	var amount int64
	for _, item := range r.Refunds {
		quota, ok := remaining[item.LogID]
		if !ok || quota != item.Quota || strings.TrimSpace(item.EvidenceReference) == "" {
			return 0, fmt.Errorf("refund %d is missing evidence, duplicated or inconsistent", item.LogID)
		}
		delete(remaining, item.LogID)
		switch item.Decision {
		case "missed_usage_decrement":
			if amount > math.MaxInt64-quota {
				return 0, fmt.Errorf("reviewed refund sum overflows int64")
			}
			amount += quota
		case "already_accounted":
		default:
			return 0, fmt.Errorf("refund %d has an unresolved decision", item.LogID)
		}
	}
	if len(remaining) != 0 {
		return 0, fmt.Errorf("every unmatched refund requires a reviewed decision")
	}
	if e.UsedQuota < amount {
		return 0, fmt.Errorf("reviewed adjustment would produce negative cumulative usage")
	}
	return -amount, nil
}

func usageRepairSourceDigest(table string, query *gorm.DB) (UsageRepairSourceDigest, error) {
	rows, err := query.Order("id").Rows()
	if err != nil {
		return UsageRepairSourceDigest{}, fmt.Errorf("cannot read evidence table %s", table)
	}
	columns, err := rows.Columns()
	if err != nil {
		rows.Close()
		return UsageRepairSourceDigest{}, err
	}
	hash := sha256.New()
	header, err := common.Marshal(columns)
	if err != nil {
		rows.Close()
		return UsageRepairSourceDigest{}, err
	}
	hash.Write(header)
	digest := UsageRepairSourceDigest{Table: table}
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			rows.Close()
			return UsageRepairSourceDigest{}, err
		}
		// MySQL returns text-protocol numeric cells as []byte but prepared
		// queries as numbers. Hash their scalar value, not driver Go types.
		for i, value := range values {
			switch v := value.(type) {
			case []byte:
				values[i] = string(v)
			case int64, float64, bool:
				values[i] = fmt.Sprint(v)
			}
		}
		encoded, err := common.Marshal(values)
		if err != nil {
			rows.Close()
			return UsageRepairSourceDigest{}, err
		}
		hash.Write([]byte{'\n'})
		hash.Write(encoded)
		digest.Rows++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return UsageRepairSourceDigest{}, err
	}
	digest.SHA256 = hex.EncodeToString(hash.Sum(nil))
	return digest, nil
}

// Audit stores the review hash and reviewer ID; the signed-off manifest keeps
// the full review. This also stays within MySQL TEXT capacity for large users.
func (r *UsageRepairReview) Fingerprint() (string, error) {
	raw, err := common.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

package model

import (
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// The operator's policy is to honor every recorded refund, including erroneous
// manual refunds. This changes the projection, never wallet or task funds.
const usageRecordedRefundPolicy = "recorded-refunds-v1"

type UsageRepairStartupReport struct {
	Corrected           int            `json:"corrected"`
	Unchanged           int            `json:"unchanged"`
	Blocked             map[int]string `json:"blocked"`
	UnsupportedTopology bool           `json:"unsupported_topology"`
}

// A rebuildable startup result, not a monetary fact. Successful migration facts
// live in the main database's audit table; fresh amounts always come from users.
var usageRepairAvailability atomic.Pointer[UsageRepairStartupReport]

// InitUserUsageRepair runs after both databases/audits initialize and before
// HTTP, the system task runner and batch updater start. Deployment is stop/start
// of a single instance; no live/rolling migration is supported by this entry.
func InitUserUsageRepair() error {
	report, err := RepairRecordedUserUsage(DB, LOG_DB)
	if err != nil {
		return err
	}
	usageRepairAvailability.Store(report)
	common.SysLog(fmt.Sprintf("user usage migration: corrected=%d unchanged=%d blocked=%d unsupported_topology=%t", report.Corrected, report.Unchanged, len(report.Blocked), report.UnsupportedTopology))
	for id, reason := range report.Blocked {
		common.SysError(fmt.Sprintf("user usage migration blocked: user_id=%d reason=%s", id, reason))
	}
	return nil
}

// UserUsedQuotaForDisplay preserves the existing numeric field on success and
// withholds only unverified projections. It performs no query or write.
func UserUsedQuotaForDisplay(user *User) *int {
	if user == nil {
		return nil
	}
	if report := usageRepairAvailability.Load(); report != nil {
		if report.UnsupportedTopology {
			return nil
		}
		if _, blocked := report.Blocked[user.Id]; blocked {
			return nil
		}
	}
	value := user.UsedQuota
	return &value
}

// RepairRecordedUserUsage is also the isolated-database deployment test entry.
// It scans in bounded user pages, reuses the offline evidence and transaction,
// and does not need a per-user review file or a maintenance command.
func RepairRecordedUserUsage(db, logDB *gorm.DB) (*UsageRepairStartupReport, error) {
	report := &UsageRepairStartupReport{Blocked: make(map[int]string)}
	if db == nil || logDB == nil {
		return nil, fmt.Errorf("usage migration databases are not initialized")
	}
	if db != logDB || common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		report.UnsupportedTopology = true
		return report, nil
	}
	for _, table := range []string{"users", "logs", "audit_logs"} {
		if !db.Migrator().HasTable(table) {
			return nil, fmt.Errorf("usage migration requires %s", table)
		}
	}
	for cursor := 0; ; {
		var ids []int
		if err := db.Table("users").Where("id > ?", cursor).Order("id").Limit(100).Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			cursor = id
			operationID := fmt.Sprintf("usage-%s:%d", usageRecordedRefundPolicy, id)
			var prior AuditLog
			err := db.Where("event_id = ?", operationID).Take(&prior).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			if err == nil {
				var content usageRepairAuditContent
				if common.UnmarshalJsonStr(prior.Content, &content) != nil || prior.Action != UsageRepairAction || !prior.Success || content.Policy != usageRecordedRefundPolicy || content.OperationID != operationID || content.UserID != id || content.Before < 0 || content.After < 0 || content.Delta > 0 || content.Delta < -content.Before || content.Before+content.Delta != content.After || content.RollbackOf != "" || content.EvidenceFingerprint == "" {
					report.Blocked[id] = "invalid migration audit"
					continue
				}
				ops, err := CollectUsageRepairPriorOps(db, id)
				if err != nil {
					return nil, err
				}
				reversed := false
				for _, op := range ops {
					if op.Success && op.RollbackOf == operationID {
						reversed = true
						break
					}
				}
				if reversed {
					report.Blocked[id] = "migration was rolled back; a new reviewed baseline is required"
					continue
				}
				// Never compare today's balance with the historical after value: legitimate
				// consumption and refunds continue after the one-time correction.
				report.Unchanged++
				continue
			}
			var evidence *UsageRepairEvidence
			err = db.Transaction(func(tx *gorm.DB) error {
				var readErr error
				evidence, readErr = CollectUserUsageRepairEvidence(tx, tx, id)
				return readErr
			})
			if err != nil {
				return nil, fmt.Errorf("cannot collect user %d usage evidence: %w", id, err)
			}
			candidate, err := recordedRefundUsageCandidate(evidence)
			if err != nil {
				report.Blocked[id] = err.Error()
				continue
			}
			fingerprint, err := evidence.Fingerprint()
			if err != nil {
				return nil, err
			}
			manifest := &UsageRepairManifest{
				Version: 3, Policy: usageRecordedRefundPolicy, OperationID: operationID,
				UserID: id, Username: evidence.Username, Evidence: evidence, EvidenceFingerprint: fingerprint,
				ExpectedBefore: candidate.ExpectedBefore, ExpectedAfter: candidate.ExpectedAfter, Delta: candidate.Delta,
				SourceDBIdentity: "startup:" + db.Dialector.Name(), CollectedAtUnix: time.Now().Unix(), CollectedAtUTCOffset: "+00:00",
			}
			if _, err := applyUserUsageRepair(db, manifest, AuditLog{Username: "system", AuthMethod: "startup_migration"}); err != nil {
				return nil, fmt.Errorf("cannot migrate user %d usage: %w", id, err)
			}
			if candidate.Delta == 0 {
				report.Unchanged++
			} else {
				report.Corrected++
			}
		}
	}
	return report, nil
}

// A bounded correction rule, not an assertion that retained logs prove complete
// account history. It recognizes either an already-net projection or the exact
// old defect (all charges less only delivery-owned refunds). Any unexplained
// residual is blocked; missing history must never be replaced by a smaller sum.
func recordedRefundUsageCandidate(e *UsageRepairEvidence) (*UsageRepairCandidate, error) {
	if e == nil || e.UsedQuota < 0 || e.ConsumeQuota < 0 || e.RefundQuota < 0 {
		return nil, fmt.Errorf("negative or missing usage evidence")
	}
	if e.NegativeQuotaRows != 0 {
		return nil, fmt.Errorf("negative recorded charge or refund")
	}
	if e.UndeliveredEvents != 0 {
		return nil, fmt.Errorf("billing deliveries are pending")
	}
	if e.DuplicateRequestIDs != 0 {
		return nil, fmt.Errorf("duplicate billing request IDs")
	}
	var count, quota, matched int64
	for _, bucket := range e.Buckets {
		if bucket.Count < 0 || bucket.Quota < 0 || count > math.MaxInt64-bucket.Count || quota > math.MaxInt64-bucket.Quota {
			return nil, fmt.Errorf("invalid or overflowing refund buckets")
		}
		count += bucket.Count
		quota += bucket.Quota
		switch bucket.Kind {
		case UsageRepairBucketDeliveryMatched:
			matched += bucket.Quota
		case UsageRepairBucketPrefixWithoutDelivery, UsageRepairBucketOtherUnmatched:
			if int64(len(bucket.Items)) != bucket.Count {
				return nil, fmt.Errorf("refund evidence is incomplete")
			}
		default:
			return nil, fmt.Errorf("unknown refund category")
		}
	}
	if count != e.RefundCount || quota != e.RefundQuota {
		return nil, fmt.Errorf("refund buckets do not cover recorded refunds")
	}
	candidate := &UsageRepairCandidate{ExpectedBefore: e.UsedQuota, ExpectedAfter: e.UsedQuota}
	// With no retained refund there is no evidence for this refund correction.
	// Keep the cumulative value even when older consume logs were cleaned up.
	if e.RefundCount == 0 {
		return candidate, nil
	}
	net := e.ConsumeQuota - e.RefundQuota
	if net < 0 {
		return nil, fmt.Errorf("recorded refunds exceed recorded consumption")
	}
	if e.UsedQuota == net {
		return candidate, nil
	}
	if len(e.PriorOps) != 0 {
		return nil, fmt.Errorf("prior maintenance and current totals require reconciliation")
	}
	if e.UsedQuota != e.ConsumeQuota-matched {
		return nil, fmt.Errorf("unexplained usage residual; retained logs cannot replace cumulative history")
	}
	candidate.ExpectedAfter = net
	candidate.Delta = net - e.UsedQuota
	if candidate.Delta >= 0 {
		return nil, fmt.Errorf("no supported missed refund adjustment")
	}
	return candidate, nil
}

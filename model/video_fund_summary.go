package model

import "github.com/QuantumNous/new-api/common"

// Whole-filter aggregates never use the current UI page. Refund dates apply to
// committed refund events, independently of request creation dates.
type VideoFundSummary struct {
	HeldCount     int64 `json:"held_count"`
	HeldQuota     int64 `json:"held_quota"`
	AbnormalCount int64 `json:"abnormal_count"`
	AbnormalQuota int64 `json:"abnormal_quota"`
	OverdueCount  int64 `json:"overdue_count"`
	OverdueQuota  int64 `json:"overdue_quota"`
	FailedCount   int64 `json:"failed_count"`
	FailedQuota   int64 `json:"failed_quota"`
	OldestHeldAt  int64 `json:"oldest_held_at"`
	ReturnedQuota int64 `json:"returned_quota"`
}

func SummarizeVideoFunds(f VideoFundFilters) (*VideoFundSummary, error) {
	s := &VideoFundSummary{}
	err := videoFundLogQuery(f).Select(`
 COALESCE(SUM(CASE WHEN fund_state IN ('held','pending') THEN 1 ELSE 0 END),0) AS held_count,
 COALESCE(SUM(CASE WHEN fund_state IN ('held','pending') THEN quota ELSE 0 END),0) AS held_quota,
 COALESCE(SUM(abnormal),0) AS abnormal_count,
 COALESCE(SUM(CASE WHEN abnormal = 1 THEN quota ELSE 0 END),0) AS abnormal_quota,
 COALESCE(SUM(CASE WHEN kind = 'attempt' AND quota > 0 AND deadline_at > 0 AND deadline_at <= ? THEN 1 ELSE 0 END),0) AS overdue_count,
 COALESCE(SUM(CASE WHEN kind = 'attempt' AND quota > 0 AND deadline_at > 0 AND deadline_at <= ? THEN quota ELSE 0 END),0) AS overdue_quota,
 COALESCE(SUM(refund_failed),0) AS failed_count,
 COALESCE(SUM(CASE WHEN refund_failed = 1 THEN quota ELSE 0 END),0) AS failed_quota,
 COALESCE(MIN(CASE WHEN fund_state IN ('held','pending') THEN created_at ELSE NULL END),0) AS oldest_held_at`, common.GetTimestamp(), common.GetTimestamp()).Scan(s).Error
	if err != nil {
		return nil, err
	}
	events := `SELECT 'attempt' AS kind, id AS owner_id, actual_refund_quota AS returned_quota, refund_completed_at AS refunded_at FROM task_create_attempts WHERE billing_hold_state = 'released' AND actual_refund_quota IS NOT NULL
 UNION ALL SELECT 'task' AS kind, task_row_id AS owner_id, before_quota-after_quota AS returned_quota, created_at AS refunded_at FROM task_billing_deliveries WHERE before_quota > after_quota`
	q := DB.Table("(?) AS refunds", DB.Raw(events)).Joins("JOIN (?) AS owners ON owners.kind = refunds.kind AND owners.id = refunds.owner_id", videoFundLogQuery(f).Select("kind, id"))
	if f.RefundFrom > 0 {
		q = q.Where("refunded_at >= ?", f.RefundFrom)
	}
	if f.RefundTo > 0 {
		q = q.Where("refunded_at <= ?", f.RefundTo)
	}
	if err := q.Select("COALESCE(SUM(returned_quota),0)").Scan(&s.ReturnedQuota).Error; err != nil {
		return nil, err
	}
	return s, nil
}

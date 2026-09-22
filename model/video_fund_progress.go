package model

// VideoFundProgress is an explicit customer whitelist. Internal owner IDs,
// channel/provider facts, notes and refund authorization versions never leave it.
type VideoFundProgress struct {
	TaskID            string `json:"task_id"`
	RequestID         string `json:"request_id"`
	Model             string `json:"model"`
	State             string `json:"fund_state"`
	Quota             int    `json:"quota"`
	RefundedQuota     int    `json:"refunded_quota"`
	RefundAmountKnown bool   `json:"refund_amount_known"`
	CreatedAt         int64  `json:"created_at"`
	DeadlineAt        int64  `json:"deadline_at"`
	RefundedAt        int64  `json:"refunded_at"`
	RetryAt           int64  `json:"retry_at"`
}

func ListVideoFundProgress(userID, appID int, taskID string, offset, limit int) ([]VideoFundProgress, int64, error) {
	// Dashboard credentials represent the user; API-key callers must supply their
	// authenticated application ID at the controller, never a query override.
	if userID <= 0 {
		return nil, 0, ErrVideoRefundConflict
	}
	rows, total, err := ListVideoFundLogs(VideoFundFilters{UserID: userID, AppID: appID, TaskID: taskID, Offset: offset, Limit: limit})
	if err != nil {
		return nil, 0, err
	}
	out := make([]VideoFundProgress, 0, len(rows))
	for _, r := range rows {
		out = append(out, VideoFundProgress{TaskID: r.TaskID, RequestID: r.RequestID, Model: r.Model, State: r.FundState, Quota: r.Quota, RefundedQuota: r.RefundedQuota, RefundAmountKnown: r.RefundAmountKnown, CreatedAt: r.CreatedAt, DeadlineAt: r.DeadlineAt, RefundedAt: r.RefundedAt, RetryAt: r.RetryAt})
	}
	return out, total, nil
}

type VideoFundEvent struct {
	Event       string `json:"event"`
	At          int64  `json:"at"`
	BeforeQuota int    `json:"before_quota"`
	AfterQuota  int    `json:"after_quota"`
}

// Only committed facts are projected. Missing legacy events are not invented.
func VideoFundTimeline(kind string, id int64) ([]VideoFundEvent, error) {
	events := make([]VideoFundEvent, 0)
	if kind == "task" {
		var rows []TaskBillingDelivery
		if err := DB.Where("task_row_id = ?", id).Order("id").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			events = append(events, VideoFundEvent{r.Event, r.CreatedAt, r.BeforeQuota, r.AfterQuota})
		}
	} else {
		var a TaskCreateAttempt
		if err := DB.First(&a, id).Error; err != nil {
			return nil, err
		}
		if a.FundsDeadlineAt > 0 {
			events = append(events, VideoFundEvent{"hold", a.FundsDeadlineAt - TaskCreateFundsGuaranteeSeconds, 0, a.HeldQuota})
		}
		if a.RefundCompletedAt > 0 && a.ActualRefundQuota != nil {
			events = append(events, VideoFundEvent{a.ReleaseReason, a.RefundCompletedAt, *a.ActualRefundQuota, 0})
		}
	}
	return events, nil
}

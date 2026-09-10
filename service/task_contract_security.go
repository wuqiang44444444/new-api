package service

// Local task operations use the upstream session-bound, single-use proof flow.
const (
	VerificationScopeTaskUsageReview    = "task_contract.usage.review"
	VerificationScopeTaskAttemptRecover = "task_contract.attempt.recover"
	VerificationScopeTaskAttemptReject  = "task_contract.attempt.reject"
)

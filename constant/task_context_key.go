package constant

const (
	ContextKeyTaskClientProtocol               ContextKey = "task_client_protocol"
	ContextKeyTaskIdempotencyID                ContextKey = "task_idempotency_id"
	ContextKeyTaskIdempotencyRelease           ContextKey = "task_idempotency_release"
	ContextKeyTaskIdempotencyCompletedNoReplay ContextKey = "task_idempotency_completed_no_replay"
	ContextKeyTaskUpstreamStarted              ContextKey = "task_upstream_started"
	ContextKeyTaskCreateAttemptID              ContextKey = "task_create_attempt_id"
	ContextKeyTaskCreateOutcomeUnknown         ContextKey = "task_create_outcome_unknown"
	ContextKeyTaskPersistenceEnabled           ContextKey = "task_persistence_enabled"
	ContextKeyTaskPromptValidated              ContextKey = "task_prompt_validated"
	ContextKeyTaskDurationValidated            ContextKey = "task_duration_validated"
)

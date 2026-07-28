package constant

const (
	ContextKeyTaskClientProtocol     ContextKey = "task_client_protocol"
	ContextKeyTaskIdempotencyID      ContextKey = "task_idempotency_id"
	ContextKeyTaskIdempotencyRelease ContextKey = "task_idempotency_release"
	ContextKeyTaskUpstreamStarted    ContextKey = "task_upstream_started"
	ContextKeyTaskPersistenceEnabled ContextKey = "task_persistence_enabled"
	ContextKeyTaskPromptValidated    ContextKey = "task_prompt_validated"
	ContextKeyTaskDurationValidated  ContextKey = "task_duration_validated"
)

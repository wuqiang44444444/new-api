package dto

const (
	ImageTaskObject = "image.generation.task"

	ImageTaskStatusQueued     = "queued"
	ImageTaskStatusInProgress = "in_progress"
	ImageTaskStatusCompleted  = "completed"
	ImageTaskStatusFailed     = "failed"
	ImageTaskStatusUnknown    = "unknown"
)

type ImageTaskError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ImageTaskResult struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
}

type ImageTask struct {
	ID          string           `json:"id"`
	Object      string           `json:"object"`
	Status      string           `json:"status"`
	CreatedAt   int64            `json:"created_at"`
	CompletedAt int64            `json:"completed_at,omitempty"`
	Model       string           `json:"model"`
	Result      *ImageTaskResult `json:"result,omitempty"`
	Error       *ImageTaskError  `json:"error,omitempty"`
}

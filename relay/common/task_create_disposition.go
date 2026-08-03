package common

import "github.com/gin-gonic/gin"

type TaskCreateDisposition string

const (
	TaskCreateSafeToRetryBeforeCreate TaskCreateDisposition = "safe_to_retry_before_create"
	TaskCreateTerminalRejection       TaskCreateDisposition = "terminal_rejection"
	TaskCreateOutcomeUnknown          TaskCreateDisposition = "create_outcome_unknown"
)

const taskCreateDispositionContextKey = "task_create_disposition"

func SetTaskCreateDisposition(c *gin.Context, disposition TaskCreateDisposition) {
	if c == nil {
		return
	}
	c.Set(taskCreateDispositionContextKey, disposition)
}

func GetTaskCreateDisposition(c *gin.Context) TaskCreateDisposition {
	if c == nil {
		return ""
	}
	value, exists := c.Get(taskCreateDispositionContextKey)
	if !exists {
		return ""
	}
	disposition, _ := value.(TaskCreateDisposition)
	return disposition
}

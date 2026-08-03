package common

import "github.com/gin-gonic/gin"

const upstreamTaskTraceContextKey = "upstream_task_trace"

// UpstreamTaskTrace records the minimum durable correlation data for a
// blocking relay that internally creates and polls an upstream task.
type UpstreamTaskTrace struct {
	TaskID                  string
	CreateRequestID         string
	LastPollRequestID       string
	PollAttempts            int
	PollElapsedMilliseconds int64
}

func SetUpstreamTaskTrace(c *gin.Context, trace *UpstreamTaskTrace) {
	if c == nil || trace == nil {
		return
	}
	c.Set(upstreamTaskTraceContextKey, trace)
}

func GetUpstreamTaskTrace(c *gin.Context) (*UpstreamTaskTrace, bool) {
	if c == nil {
		return nil, false
	}
	value, exists := c.Get(upstreamTaskTraceContextKey)
	if !exists {
		return nil, false
	}
	trace, ok := value.(*UpstreamTaskTrace)
	return trace, ok && trace != nil
}

func (t *UpstreamTaskTrace) AuditMap() map[string]interface{} {
	if t == nil || t.TaskID == "" {
		return nil
	}
	audit := map[string]interface{}{
		"task_id":         t.TaskID,
		"poll_attempts":   t.PollAttempts,
		"poll_elapsed_ms": t.PollElapsedMilliseconds,
	}
	if t.CreateRequestID != "" {
		audit["create_request_id"] = t.CreateRequestID
	}
	if t.LastPollRequestID != "" {
		audit["last_poll_request_id"] = t.LastPollRequestID
	}
	return audit
}

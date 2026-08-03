package channel

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/model"
)

type TaskLifecycleCapabilities struct {
	SupportsContent         bool
	SupportsRemix           bool
	SupportsCancelQueued    bool
	SupportsDeleteTerminal  bool
	SupportsAssetReferences bool
}

type TaskLifecycleAdaptor interface {
	TaskLifecycleCapabilities(task *model.Task) TaskLifecycleCapabilities
	CancelQueuedTask(ctx context.Context, task *model.Task, channel *model.Channel) error
	DeleteTerminalTask(ctx context.Context, task *model.Task, channel *model.Channel) error
}

type TaskLifecycleError struct {
	StatusCode int
	Message    string
}

func (e *TaskLifecycleError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("upstream task lifecycle request returned HTTP %d", e.StatusCode)
	}
	return e.Message
}

func IsDefinitiveTaskLifecycleRejection(err error) bool {
	lifecycleErr, ok := err.(*TaskLifecycleError)
	if !ok || lifecycleErr.StatusCode < 400 || lifecycleErr.StatusCode >= 500 {
		return false
	}
	return lifecycleErr.StatusCode != 408 &&
		lifecycleErr.StatusCode != 425 &&
		lifecycleErr.StatusCode != 429
}

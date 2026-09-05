package common

import "fmt"

// UpstreamTaskNotFound reports a provider business response that definitively
// states the polled task does not exist. Unlike UpstreamContractViolation it is
// a trusted terminal observation, so the polling loop may retire the task
// instead of holding it for reconciliation.
type UpstreamTaskNotFound struct {
	ProviderCode int
}

func (e *UpstreamTaskNotFound) Error() string {
	return fmt.Sprintf("upstream task not found (provider code %d)", e.ProviderCode)
}

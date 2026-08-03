package common

import "fmt"

type UpstreamContractViolation struct {
	Reason string
}

func (e *UpstreamContractViolation) Error() string {
	if e == nil || e.Reason == "" {
		return "upstream_contract_violation"
	}
	return fmt.Sprintf("upstream_contract_violation: %s", e.Reason)
}

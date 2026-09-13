package billingexpr

import "fmt"

// ValidateAsyncDeterminism applies only to callers that freeze neither headers
// nor pricing time. It does not change native synchronous or frozen-time runs.
func ValidateAsyncDeterminism(expression string) error {
	for name := range UsedVars(expression) {
		switch name {
		case "header", "hour", "minute", "weekday", "month", "day":
			return fmt.Errorf("task expression must not use non-deterministic function %q: header/time are not frozen across pre-consume and settlement", name)
		}
	}
	return nil
}

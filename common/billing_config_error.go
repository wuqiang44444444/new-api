package common

// BillingConfigError adds a stable diagnostic to the existing error without
// changing its public text or unwrap chain. Only the validator knows the reason.
type BillingConfigError struct {
	Reason string
	Model  string
	Cause  error
}

func (e *BillingConfigError) Error() string { return e.Cause.Error() }
func (e *BillingConfigError) Unwrap() error { return e.Cause }

func NewBillingConfigError(reason, model string, cause error) error {
	return &BillingConfigError{Reason: reason, Model: model, Cause: cause}
}

package model

// HasTaskUsageBilling reads the native expression's frozen unit contract.
// A model name or current channel configuration cannot change that contract.
func (t *Task) HasTaskUsageBilling() bool {
	context := t.PrivateData.BillingContext
	return context != nil && context.TieredSnapshot != nil && context.TieredSnapshot.TaskUsageBilling
}

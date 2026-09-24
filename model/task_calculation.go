package model

import "fmt"

// Validate only events that declare the recording contract. Old accepted
// targets remain recoverable without manufacturing missing historical evidence.
func validateTaskCalculation(task *Task, async *TaskAsyncBillingContext, target int) error {
	if async == nil || async.CalculationVersion == 0 {
		return nil
	}
	if async.CalculationSource == "initial" {
		bc := task.PrivateData.BillingContext
		if bc == nil || bc.CalculationVersion == 0 {
			return nil
		}
		if bc.InitialCalculation == nil || bc.InitialCalculation.Quota != target {
			return fmt.Errorf("initial billing calculation missing or inconsistent")
		}
		return nil
	}
	if async.Calculation == nil || async.Calculation.Quota != target {
		return fmt.Errorf("billing calculation missing or inconsistent")
	}
	return nil
}

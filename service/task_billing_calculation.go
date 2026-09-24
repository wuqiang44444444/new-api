package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
)

// setTaskCalculation attaches the process at the same boundary as its target.
func setTaskCalculation(task *model.Task, c *billingexpr.Calculation, source string) {
	if task.PrivateData.AsyncBilling != nil {
		task.PrivateData.AsyncBilling.CalculationVersion = 1
		task.PrivateData.AsyncBilling.Calculation = c
		task.PrivateData.AsyncBilling.CalculationSource = source
	} else {
		if source == "initial" {
			return
		}
		if task.PrivateData.BillingContext == nil {
			task.PrivateData.BillingContext = &model.TaskBillingContext{}
		}
		task.PrivateData.BillingContext.SettlementCalculationVersion = 1
		task.PrivateData.BillingContext.SettlementCalculation = c
	}
}

func retainTaskInitialCalculation(task *model.Task) {
	// A reference avoids copying a large expression trace. A legacy initial event
	// remains legacy even when its terminal decision is recorded by new code.
	setTaskCalculation(task, nil, "initial")
}

func zeroTaskCalculation(task *model.Task, operation string) {
	c := billingexpr.NewCalculation()
	c.Add(operation, "quota", 0, task.Quota)
	setTaskCalculation(task, c.Finish(0), operation)
}

// Read-only selection of accepted evidence for logs and delayed delivery.
func taskAcceptedCalculation(task *model.Task) *billingexpr.Calculation {
	if async := task.PrivateData.AsyncBilling; async != nil && async.CalculationSource != "initial" && async.Calculation != nil {
		return async.Calculation
	}
	if bc := task.PrivateData.BillingContext; bc != nil {
		if bc.SettlementCalculation != nil {
			return bc.SettlementCalculation
		}
		return bc.InitialCalculation
	}
	return nil
}

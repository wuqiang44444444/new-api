package model

func ApplyTaskBillingTargetWithExposure(task *Task, targetQuota int, exposure *ProviderCostExposure) (bool, int, error) {
	return applyTaskBillingTarget(task, targetQuota, exposure)
}

package service

import "github.com/QuantumNous/new-api/model"

func appendTaskStatementOriginalQuota(other *model.LogOther, task *model.Task, event model.TaskBillingDelivery, logType int) {
	if original := model.TaskStatementOriginalQuota(task, event, logType); original != nil && *original >= 0 {
		other.SetAdmin(model.StatementOriginalQuotaLogKey, *original)
	}
}

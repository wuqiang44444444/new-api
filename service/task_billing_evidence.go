package service

import "github.com/QuantumNous/new-api/model"

func appendProviderBillingEvidence(other *model.LogOther, task *model.Task) {
	if other == nil || task == nil || task.PrivateData.AsyncBilling == nil {
		return
	}
	evidence := task.PrivateData.AsyncBilling.ProviderBillingEvidence
	if evidence == nil {
		return
	}
	other.SetAdmin("provider_billing_evidence", *evidence)
}

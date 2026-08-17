package service

import "github.com/QuantumNous/new-api/model"

func appendProviderBillingEvidence(other map[string]interface{}, task *model.Task) {
	if other == nil || task == nil || task.PrivateData.AsyncBilling == nil {
		return
	}
	evidence := task.PrivateData.AsyncBilling.ProviderBillingEvidence
	if evidence == nil {
		return
	}
	adminInfo, _ := other["admin_info"].(map[string]interface{})
	if adminInfo == nil {
		adminInfo = make(map[string]interface{})
		other["admin_info"] = adminInfo
	}
	adminInfo["provider_billing_evidence"] = *evidence
}

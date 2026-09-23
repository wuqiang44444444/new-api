package controller

import "github.com/QuantumNous/new-api/model"

// The template endpoints are admin-only. Keep source diagnostics out of the
// shared model's JSON (including snapshots/audits and customer projections).
type customerContractTemplateAdminView struct {
	*model.ContractTemplateSnapshot
	Rules []customerContractTemplateAdminRuleView `json:"rules"`
}

type customerContractTemplateAdminRuleView struct {
	model.ContractEntityRule
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

func buildCustomerContractTemplateAdminView(snapshot *model.ContractTemplateSnapshot) customerContractTemplateAdminView {
	rules := make([]customerContractTemplateAdminRuleView, 0, len(snapshot.Rules))
	for _, rule := range snapshot.Rules {
		rules = append(rules, customerContractTemplateAdminRuleView{
			ContractEntityRule: rule, UnavailableReason: rule.UnavailableCategory,
		})
	}
	return customerContractTemplateAdminView{ContractTemplateSnapshot: snapshot, Rules: rules}
}

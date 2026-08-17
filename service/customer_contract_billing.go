package service

import hosttypes "github.com/QuantumNous/new-api/types"

func appendCustomerContractBillingInfo(other map[string]interface{}, fact *hosttypes.ContractBillingFact) {
	if other == nil || fact == nil {
		return
	}
	other["contract_version"] = fact.ContractVersion
	other["contract_discount"] = fact.RatioString()
}

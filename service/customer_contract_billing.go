package service

import (
	"github.com/QuantumNous/new-api/model"
	hosttypes "github.com/QuantumNous/new-api/types"
)

func appendCustomerContractBillingInfo(other *model.LogOther, fact *hosttypes.ContractBillingFact) {
	if other == nil || fact == nil {
		return
	}
	other.SetPublic("contract_version", fact.ContractVersion)
	other.SetPublic("contract_discount", fact.RatioString())
}

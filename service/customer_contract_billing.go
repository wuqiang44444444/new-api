package service

import (
	"github.com/QuantumNous/new-api/model"
	hosttypes "github.com/QuantumNous/new-api/types"
)

func appendCustomerContractBillingInfo(other *model.LogOther, fact *hosttypes.ContractBillingFact) {
	if other == nil {
		return
	}
	if fact == nil {
		// 结算时合同解析已完成但未命中折扣：显式记录“明确未适用”，与
		// 历史行的“未记录”区分（方案 1.2），不用缺失值构造生效合同。
		other.SetPublic("contract_applicable", false)
		return
	}
	other.SetPublic("contract_id", fact.ContractId)
	other.SetPublic("contract_version", fact.ContractVersion)
	other.SetPublic("contract_discount", fact.RatioString())
	if fact.ContractName != "" {
		other.SetPublic("contract_name", fact.ContractName)
	}
}

package model

const RequireAPIKeyContractOption = "customer_contract.require_for_api_keys"

func APIKeyContractRequired() bool {
	return CurrentRequestPolicy().Options[RequireAPIKeyContractOption] != "false"
}

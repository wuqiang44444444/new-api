package constant

const (
	// ContextKeyTokenContractId carries the API key's bound customer contract
	// entity id (0 = unbound). Contract authorization is key-scoped; users
	// without a bound key never enter the contract path.
	ContextKeyTokenContractId ContextKey = "token_contract_id"
	// ContextKeyContractFact carries the frozen ContractBillingFact resolved
	// for the current request.
	ContextKeyContractFact ContextKey = "contract_billing_fact"
	// ContextKeyAuthVersion carries the owner's authentication version written
	// by UserBase.WriteContext. Contract resolution uses it to fence the
	// per-contract snapshot cache.
	ContextKeyAuthVersion ContextKey = "auth_version"
)

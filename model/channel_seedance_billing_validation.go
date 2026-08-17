package model

// GetEnabledSeedanceChannelsForBillingValidation returns the same
// management-approved channels that can provide task billing probes. Callers
// must match customer models exactly; model names do not imply a protocol.
func GetEnabledSeedanceChannelsForBillingValidation() ([]Channel, error) {
	return enabledSeedanceChannels(DB)
}

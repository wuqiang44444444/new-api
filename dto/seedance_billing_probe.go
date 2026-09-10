package dto

// SeedanceBillingProbeValidationExtraFields returns fields that exist only for the
// selected protocol. Common task fields are owned by the billing validator;
// protocol metadata is shared by model pricing validation so a Feicai-only
// field cannot be accepted for other Seedance channels.
func SeedanceBillingProbeValidationExtraFields(protocol VideoUpstreamProtocol) map[string]any {
	if protocol == VideoUpstreamProtocolFunCloudModelArkV3 {
		return map[string]any{"billing_mode": "per-second"}
	}
	if protocol.TransportProfile() != VideoUpstreamProfileThirdPartyFeicaiVideos {
		return nil
	}
	return map[string]any{
		"ratio":           "16:9",
		"size_multiplier": 1.0,
		"billing_mode":    "per-second",
	}
}

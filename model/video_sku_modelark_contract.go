package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

// validateModelArkContractScalars owns scalar rules shared by published
// ModelArk SKUs. Per-SKU duration, resolution, ratio and unsupported-field
// rules remain on VideoSKUCapability.
func validateModelArkContractScalars(request *dto.ModelArkVideoCreateRequest) error {
	if request.ServiceTier != nil {
		serviceTier := strings.TrimSpace(*request.ServiceTier)
		if serviceTier != "default" && serviceTier != "flex" {
			return fmt.Errorf("service_tier is not supported")
		}
	}
	if request.ExecutionExpiresAfter != nil &&
		(*request.ExecutionExpiresAfter < 3600 || *request.ExecutionExpiresAfter > 259200) {
		return fmt.Errorf("execution_expires_after must be between 3600 and 259200")
	}
	if request.Seed != nil && (*request.Seed < -1 || int64(*request.Seed) > int64(^uint32(0))) {
		return fmt.Errorf("seed must be between -1 and 4294967295")
	}
	if request.SafetyIdentifier != nil && len(strings.TrimSpace(*request.SafetyIdentifier)) > 64 {
		return fmt.Errorf("safety_identifier must not exceed 64 characters")
	}
	return nil
}

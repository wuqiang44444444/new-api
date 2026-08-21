package seedance

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

const modelSeedance20CMCC = "doubao-seedance-2.0"

var cmccSeedance20Resolutions = stringSet("480p", "720p", "1080p")
var cmccSeedance20Ratios = stringSet("16:9", "9:16", "1:1")

// validateCMCCProviderModelRequest keeps Mobile Cloud's independently verified
// contract out of the shared third-party model-spec implementation.
func validateCMCCProviderModelRequest(model string, request *dto.ModelArkVideoCreateRequest) error {
	if strings.TrimSpace(model) != modelSeedance20CMCC {
		return fmt.Errorf("the selected customer model is not supported by its configured video adapter")
	}
	if request == nil {
		return fmt.Errorf("ModelArk request is required")
	}
	if request.CallbackURL != nil || request.OutputFormat != nil || request.ServiceTier != nil ||
		request.ReturnLastFrame != nil || request.ExecutionExpiresAfter != nil || request.Draft != nil ||
		request.Tools != nil || request.SafetyIdentifier != nil || request.Priority != nil ||
		request.Frames != nil || request.Seed != nil || request.CameraFixed != nil {
		return fmt.Errorf("request contains a parameter unsupported by the selected customer model")
	}

	duration := 5
	if request.Duration != nil {
		duration = *request.Duration
	}
	if duration < 4 || duration > 15 {
		return fmt.Errorf("duration must be between 4 and 15 for the selected customer model")
	}

	resolution := "720p"
	if request.Resolution != nil {
		resolution = strings.TrimSpace(*request.Resolution)
		if resolution == "" {
			return fmt.Errorf("resolution must not be empty")
		}
	}
	if _, allowed := cmccSeedance20Resolutions[resolution]; !allowed {
		return fmt.Errorf("resolution %q is not supported by the selected customer model", resolution)
	}

	if request.Ratio != nil {
		ratio := strings.TrimSpace(*request.Ratio)
		if ratio == "" {
			return fmt.Errorf("ratio must not be empty")
		}
		if _, allowed := cmccSeedance20Ratios[ratio]; !allowed {
			return fmt.Errorf("ratio %q is not supported by the selected customer model", ratio)
		}
	}
	return nil
}

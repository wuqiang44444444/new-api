package seedance

import (
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"slices"
	"strings"
)

func validateSynlinkRequest(providerModel string, request *dto.ModelArkVideoCreateRequest) error {
	if !slices.Contains(kitdto.SynlinkVideoModels(), providerModel) || request == nil {
		return fmt.Errorf("the selected model is not registered for this video protocol")
	}
	// Only the documented create fields are published for this new protocol.
	// Callback delivery is not yet verified; forwarding Provider identities to
	// the caller would violate the existing northbound task contract.
	if request.CallbackURL != nil || request.ServiceTier != nil || request.ExecutionExpiresAfter != nil || request.Draft != nil || request.Tools != nil || request.SafetyIdentifier != nil || request.Priority != nil || request.Frames != nil || request.Seed != nil || request.CameraFixed != nil || request.OutputFormat != nil {
		return fmt.Errorf("request contains a parameter not published for this video protocol")
	}
	if request.Duration != nil && (*request.Duration < 1 || *request.Duration > relaycommon.MaxTaskDurationSeconds) {
		return fmt.Errorf("duration must be between 1 and %d; automatic duration is not verified", relaycommon.MaxTaskDurationSeconds)
	}
	if request.Resolution != nil && !slices.Contains([]string{"480p", "720p", "1080p", "4k", "4K"}, *request.Resolution) {
		return fmt.Errorf("resolution is outside the published video request format")
	}
	if request.Ratio != nil {
		if _, ok := modelArkRatios[*request.Ratio]; !ok {
			return fmt.Errorf("ratio is outside the published video request format")
		}
	}
	return validateSynlinkMedia(request)
}

func buildSynlinkRequest(c *gin.Context, body *requestPayload) ([]byte, error) {
	payload := *body
	// Pin northbound defaults so the frozen billing probe matches the wire.
	if payload.Duration == nil {
		value := kitdto.IntValue(5)
		payload.Duration = &value
	}
	if payload.Resolution == "" {
		payload.Resolution = "720p"
	}
	if payload.GenerateAudio == nil {
		value := kitdto.BoolValue(false)
		payload.GenerateAudio = &value
	}
	var err error
	payload.Content, err = resolveHostedImageContent(c, payload.Content)
	if err != nil {
		return nil, err
	}
	for _, item := range payload.Content {
		for _, media := range []*MediaURL{item.ImageURL, item.VideoURL, item.AudioURL} {
			if media != nil && strings.HasPrefix(strings.TrimSpace(media.URL), "asset://") {
				return nil, fmt.Errorf("unresolved asset reference cannot be sent to this video protocol")
			}
		}
	}
	return common.Marshal(&payload)
}

package model

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/dto"
)

func (capability VideoSKUCapability) ValidateContractRequest(request dto.VideoContractRequest) error {
	if capability.Version == "" || capability.ContentHash == "" ||
		videoSKUCapabilityHash(capability) != capability.ContentHash {
		return fmt.Errorf("video SKU capability snapshot is invalid")
	}
	if capability.ContractID != string(request.ContractID) {
		return fmt.Errorf("video SKU contract does not match the request contract")
	}
	switch request.ContractID {
	case dto.VideoContractModelArkV3:
		return capability.ValidateModelArkRequest(request.ModelArk)
	case dto.VideoContractKlingV1:
		return capability.validateKlingRequest(request.Kling)
	case dto.VideoContractJimeng:
		return capability.validateJimengRequest(request.Jimeng)
	default:
		return fmt.Errorf("video contract is not registered")
	}
}

func (capability VideoSKUCapability) validateKlingRequest(request *dto.KlingVideoCreateRequest) error {
	if request == nil || strings.TrimSpace(videoString(request.ModelName)) != capability.PublicModel {
		return fmt.Errorf("video SKU capability does not match request model")
	}
	if strings.TrimSpace(videoString(request.Prompt)) == "" {
		return fmt.Errorf("prompt is required")
	}
	if mode := strings.TrimSpace(videoString(request.Mode)); mode != "" && !slices.Contains(capability.Modes, mode) {
		return fmt.Errorf("mode is not supported by this model")
	}
	if durationValue := strings.TrimSpace(videoString(request.Duration)); durationValue != "" {
		duration, err := strconv.Atoi(durationValue)
		if err != nil || !slices.Contains(capability.DurationValues, duration) {
			return fmt.Errorf("duration is not supported by this model")
		}
	}
	if ratio := strings.TrimSpace(videoString(request.AspectRatio)); ratio != "" &&
		!slices.Contains(capability.Ratios, ratio) {
		return fmt.Errorf("aspect_ratio is not supported by this model")
	}
	if request.CfgScale != nil && capability.HasCFGScaleRange &&
		(*request.CfgScale < capability.MinCFGScale || *request.CfgScale > capability.MaxCFGScale) {
		return fmt.Errorf("cfg_scale must be between %g and %g", capability.MinCFGScale, capability.MaxCFGScale)
	}
	imageCount := 0
	for _, value := range []*string{request.Image, request.ImageTail} {
		if strings.TrimSpace(videoString(value)) != "" {
			imageCount++
		}
	}
	if capability.MaxImages > 0 && imageCount > capability.MaxImages {
		return fmt.Errorf("image input exceeds the maximum of %d", capability.MaxImages)
	}
	if strings.TrimSpace(videoString(request.ImageTail)) != "" &&
		strings.TrimSpace(videoString(request.Image)) == "" {
		return fmt.Errorf("image_tail requires image")
	}
	return nil
}

func (capability VideoSKUCapability) validateJimengRequest(request *dto.JimengVideoCreateRequest) error {
	if request == nil || strings.TrimSpace(request.ReqKey) != capability.PublicModel {
		return fmt.Errorf("video SKU capability does not match request model")
	}
	return nil
}

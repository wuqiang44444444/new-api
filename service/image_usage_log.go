package service

import (
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func appendImageUsageForLog(other *model.LogOther, usage *dto.Usage) {
	if usage != nil && usage.InputImages != nil {
		other.SetPublic("input_images", *usage.InputImages)
	}
}

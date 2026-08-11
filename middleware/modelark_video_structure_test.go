package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validModelArkVideoRequest() modelArkVideoCreateRequest {
	return modelArkVideoCreateRequest{
		Model: "seedance-customer-model",
		Content: []dto.ModelArkVideoContent{{
			Type: "text",
			Text: common.GetPointer("generate a short video"),
		}},
	}
}

func TestValidateModelArkVideoCreateRequestUsesOfficialStructuralBounds(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*modelArkVideoCreateRequest)
	}{
		{"zero duration", func(req *modelArkVideoCreateRequest) { req.Duration = common.GetPointer(0) }},
		{"oversized duration", func(req *modelArkVideoCreateRequest) { req.Duration = common.GetPointer(3601) }},
		{"invalid frames", func(req *modelArkVideoCreateRequest) { req.Frames = common.GetPointer(30) }},
		{"invalid expiry", func(req *modelArkVideoCreateRequest) { req.ExecutionExpiresAfter = common.GetPointer(3599) }},
		{"invalid priority", func(req *modelArkVideoCreateRequest) { req.Priority = common.GetPointer(10) }},
		{"invalid seed", func(req *modelArkVideoCreateRequest) { req.Seed = common.GetPointer(-2) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := validModelArkVideoRequest()
			test.mutate(&request)
			_, err := validateModelArkVideoCreateRequest(request)
			require.Error(t, err)
		})
	}
}

func TestValidateModelArkVideoCreateRequestAcceptsIntelligentDurationAndOfficialFrames(t *testing.T) {
	request := validModelArkVideoRequest()
	request.Duration = common.GetPointer(-1)
	request.Frames = common.GetPointer(289)
	request.ExecutionExpiresAfter = common.GetPointer(259200)
	request.Priority = common.GetPointer(9)
	request.Seed = common.GetPointer(1<<31 - 1)

	prompt, err := validateModelArkVideoCreateRequest(request)
	require.NoError(t, err)
	assert.Equal(t, "generate a short video", prompt)
}

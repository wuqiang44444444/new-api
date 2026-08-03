package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestModelArkCapabilityOwnsSharedScalarValidation(t *testing.T) {
	capability, ok := ResolveVideoSKUCapability(VideoSKUSeedanceBytePlus)
	require.True(t, ok)
	tests := []struct {
		name    string
		mutate  func(*dto.ModelArkVideoCreateRequest)
		message string
	}{
		{
			name: "service tier",
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.ServiceTier = common.GetPointer("priority")
			},
			message: "service_tier",
		},
		{
			name: "execution expiry",
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.ExecutionExpiresAfter = common.GetPointer(3599)
			},
			message: "execution_expires_after",
		},
		{
			name: "seed",
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Seed = common.GetPointer(-2)
			},
			message: "seed",
		},
		{
			name: "safety identifier",
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.SafetyIdentifier = common.GetPointer(strings.Repeat("a", 65))
			},
			message: "safety_identifier",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := &dto.ModelArkVideoCreateRequest{
				Model: capability.PublicModel,
				Content: []dto.ModelArkVideoContent{
					{Type: "text", Text: common.GetPointer("move")},
				},
			}
			test.mutate(request)

			require.ErrorContains(t, capability.ValidateModelArkRequest(request), test.message)
		})
	}
}

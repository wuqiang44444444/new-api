package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestFixedSeedanceSKURejectsEndUserSubject(t *testing.T) {
	capability, ok := ResolveVideoSKUCapability(VideoSKUSeedance20Standard720P)
	require.True(t, ok)
	err := capability.ValidateModelArkRequest(&dto.ModelArkVideoCreateRequest{
		Model:          capability.PublicModel,
		EndUserSubject: common.GetPointer("customer-42"),
		Content: []dto.ModelArkVideoContent{
			{Type: "text", Text: common.GetPointer("make a video")},
		},
	})
	require.ErrorContains(t, err, "end_user_subject")
}

package service

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestAssetBindingResponseCanonicalizesLegacyProviderTarget(t *testing.T) {
	response := AssetBindingResponse(
		&model.Asset{PublicID: "ast_public"},
		&model.AssetBinding{
			PublicID:      "asb_public",
			BindingTarget: dto.AssetTargetJoyCreatorLegacy,
		},
	)

	assert.Equal(t, dto.AssetTargetManagementLibrary, response.Target)
}

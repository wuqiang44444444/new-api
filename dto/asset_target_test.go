package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeAssetTargetKeepsProviderNamesOutOfPublicContract(t *testing.T) {
	for _, input := range []string{AssetTargetManagementLibrary, AssetTargetJoyCreatorLegacy} {
		target, ok := NormalizeAssetTarget(input)
		assert.True(t, ok)
		assert.Equal(t, AssetTargetManagementLibrary, target)
		assert.Equal(t, AssetTargetManagementLibrary, PublicAssetTarget(input))
	}
	_, ok := NormalizeAssetTarget("ark_assets")
	assert.False(t, ok)
	assert.Empty(t, PublicAssetTarget("ark_assets"))
}

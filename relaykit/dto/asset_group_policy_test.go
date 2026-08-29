package dto

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneralAssetGroupPolicyRegistry(t *testing.T) {
	tests := []struct {
		protocol AssetUpstreamProtocol
		want     GeneralAssetGroupPolicy
	}{
		{AssetUpstreamProtocolNone, GeneralAssetGroupPolicyNone},
		{AssetUpstreamProtocolVolcengineAction, GeneralAssetGroupPolicyDefaultFallback},
		{AssetUpstreamProtocolBytePlusAction, GeneralAssetGroupPolicyDefaultFallback},
		{AssetUpstreamProtocolArkAssetsV1, GeneralAssetGroupPolicyDefaultFallback},
		{AssetUpstreamProtocolTokenSaveAssetsV1, GeneralAssetGroupPolicyDefaultFallback},
		{AssetUpstreamProtocolMoxingJoyCreatorV1, GeneralAssetGroupPolicyDefaultFallback},
		{AssetUpstreamProtocolMoxingVolcAssetsV1, GeneralAssetGroupPolicyDefaultFallback},
		{AssetUpstreamProtocolFunCloudMaterial, GeneralAssetGroupPolicyDefaultFallback},
		{AssetUpstreamProtocolCMCCAICCV2, GeneralAssetGroupPolicyDefaultFallback},
	}

	for _, test := range tests {
		t.Run(string(test.protocol), func(t *testing.T) {
			assert.True(t, test.protocol.IsValid())
			assert.Equal(t, test.want, test.protocol.GeneralAssetGroupPolicy())
		})
	}

	assert.False(t, AssetUpstreamProtocol("unknown").IsValid())
	assert.Empty(t, AssetUpstreamProtocol("unknown").GeneralAssetGroupPolicy())
}

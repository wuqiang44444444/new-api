package service

import (
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	"github.com/stretchr/testify/assert"
)

func TestAssetResponseReturnsOpaqueProviderIDAndReference(t *testing.T) {
	response := assetResponse("customer-model", "provider-resource-id", assetadapter.AssetResult{
		ResourceID: "provider-resource-id", ReferenceValue: "provider-reference-id", Status: "active",
	})

	assert.Equal(t, "asset", response.Object)
	assert.Equal(t, "provider-resource-id", response.ID)
	assert.Equal(t, "customer-model", response.Model)
	assert.Equal(t, "asset://provider-reference-id", response.Reference)
	assert.Equal(t, "ready", response.Status)
	assert.NotContains(t, fmt.Sprintf("%+v", response), "volcengine")
}

func TestValidateAssetLookupRequiresCallerModelAndOpaqueID(t *testing.T) {
	_, _, err := validateAssetLookup("", "provider-id")
	assert.ErrorIs(t, err, ErrInvalidAssetRequest)

	modelName, resourceID, err := validateAssetLookup(" customer-model ", " provider-id ")
	assert.NoError(t, err)
	assert.Equal(t, "customer-model", modelName)
	assert.Equal(t, "provider-id", resourceID)

	modelName, resourceID, err = validateAssetLookup("customer-model", "opaque/id?owned-by=provider")
	assert.NoError(t, err)
	assert.Equal(t, "customer-model", modelName)
	assert.Equal(t, "opaque/id?owned-by=provider", resourceID)
}

func TestUnsupportedAdapterOperationHasExplicitPublicSentinel(t *testing.T) {
	assert.ErrorIs(t, normalizeAssetAdapterError(assetadapter.ErrAssetOperationUnsupported), ErrUnsupportedAssetOperation)
}

func TestSeedanceAssetAdapterDistinguishesUnsupportedFromInvalidProtocol(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeSeedanceLink, Key: "unused"}
	channel.SetOtherSettings(dto.ChannelOtherSettings{AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
	_, err := seedanceAssetAdapter(channel)
	assert.ErrorIs(t, err, ErrAssetLibraryUnsupported)

	channel.SetOtherSettings(dto.ChannelOtherSettings{AssetUpstreamProtocol: dto.AssetUpstreamProtocol("unknown_asset_protocol")})
	_, err = seedanceAssetAdapter(channel)
	assert.ErrorIs(t, err, ErrAssetLibraryUnavailable)
	assert.NotErrorIs(t, err, ErrAssetLibraryUnsupported)
}

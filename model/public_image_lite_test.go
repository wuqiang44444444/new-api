package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicGeminiLiteUsesMappedModelForSizeConstraints(t *testing.T) {
	resetPricingEndpointTestTables(t)
	channel := &Channel{Id: 480, Type: constant.ChannelTypeVertexAi, Key: "fixture", Status: common.ChannelStatusEnabled,
		Name: "fixture", Group: "default", Models: "customer-image", ModelMapping: common.GetPointer(`{"customer-image":"gemini-3.1-flash-lite-image"}`)}
	require.NoError(t, DB.Create(channel).Error)
	require.NoError(t, DB.Create(&Ability{Group: "default", Model: "customer-image", ChannelId: 480, Enabled: true}).Error)
	for _, groups := range [][]string{{"default"}, nil} {
		apis, err := GetPublicMediaModelAPIs([]string{"customer-image"}, groups)
		require.NoError(t, err)
		require.NotNil(t, apis["customer-image"])
		require.NotNil(t, apis["customer-image"].Image)
		for _, parameter := range apis["customer-image"].Image.Creation.Parameters {
			if parameter.Name == "size" {
				assert.Nil(t, parameter.SizeConstraints)
				assert.Equal(t, []string{"auto", "1024x1024"}, parameter.Enum)
			}
		}
		encoded, err := common.Marshal(apis)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), "gemini-3.1-flash-lite-image")
	}
}

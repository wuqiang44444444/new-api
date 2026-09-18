package publicmodel_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/geminiimage"
	"github.com/QuantumNous/new-api/pkg/publicmodel"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLitePublishedSizesMatchRuntime(t *testing.T) {
	api := publicmodel.GeminiImageAPI("customer-image", "gemini-3.1-flash-lite-image", 24)
	for _, operation := range []dto.PublicImageCreation{api.Image.Creation, *api.Image.Edit} {
		assert.Equal(t, "customer-image", operation.Model)
		var size dto.PublicAPIParameter
		for _, p := range operation.Parameters {
			if p.Name == "size" {
				size = p
			}
		}
		assert.Nil(t, size.SizeConstraints)
		assert.Equal(t, "auto", size.DefaultValue)
		assert.Equal(t, []string{"auto", "1024x1024"}, size.Enum)
		for _, value := range size.Enum {
			_, _, err := geminiimage.Resolve("gemini-3.1-flash-lite-image", value)
			require.NoError(t, err)
		}
		_, _, err := geminiimage.Resolve("gemini-3.1-flash-lite-image", "1699x1699")
		require.Error(t, err)
	}
	encoded, err := common.Marshal(api)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), `"size_constraints"`)
	assert.NotContains(t, string(encoded), "gemini-3.1-flash-lite-image")
	encoded, err = common.Marshal(dto.PublicAPIParameter{Name: "prompt", Type: "string"})
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "size_constraints")
}

func TestLiteInlineLimitMatchesChannel(t *testing.T) {
	for _, channelType := range []int{24, 41} {
		api := publicmodel.GeminiImageAPI("customer-image", "gemini-3.1-flash-lite-image", channelType)
		for _, p := range api.Image.Edit.Parameters {
			if p.Name != "image" && p.Name != "images" {
				continue
			}
			if channelType == 41 {
				require.NotNil(t, p.MaxDecodedBytes)
				assert.Equal(t, 7000000, *p.MaxDecodedBytes)
			} else {
				assert.Nil(t, p.MaxDecodedBytes)
			}
		}
	}
}

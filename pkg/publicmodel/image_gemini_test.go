package publicmodel

import (
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func geminiParameterByName(t *testing.T, parameters []dto.PublicAPIParameter, name string) *dto.PublicAPIParameter {
	t.Helper()
	for i := range parameters {
		if parameters[i].Name == name {
			return &parameters[i]
		}
	}
	return nil
}

func TestGeminiImageAPIContractShape(t *testing.T) {
	api := GeminiImageAPI("nano-banana-2-gemini")
	require.NotNil(t, api)
	require.NotNil(t, api.Image)
	require.Len(t, api.Image.Operations, 2)

	creation := api.Image.Creation
	assert.Equal(t, "/v1/images/generations", creation.Path)
	assert.False(t, creation.AdditionalProperties)
	prompt := geminiParameterByName(t, creation.Parameters, "prompt")
	require.NotNil(t, prompt)
	require.NotNil(t, prompt.MaxLength)
	assert.Equal(t, 20000, *prompt.MaxLength)
	n := geminiParameterByName(t, creation.Parameters, "n")
	require.NotNil(t, n)
	assert.Equal(t, 1, n.FixedValue)
	responseFormat := geminiParameterByName(t, creation.Parameters, "response_format")
	require.NotNil(t, responseFormat)
	assert.Equal(t, "b64_json", responseFormat.DefaultValue)
	for _, unsupported := range []string{"quality", "style", "background", "moderation", "output_format", "mask", "partial_images"} {
		assert.Nil(t, geminiParameterByName(t, creation.Parameters, unsupported), "%s must not be published", unsupported)
	}

	edit := api.Image.Edit
	require.NotNil(t, edit)
	assert.Equal(t, "/v1/images/edits", edit.Path)
	assert.Equal(t, "application/json", edit.ContentType)
	assert.NotContains(t, edit.RequiredFields, "image")
	assert.Equal(t, []string{"image", "images"}, edit.RequiredOneOf)
	images := geminiParameterByName(t, edit.Parameters, "images")
	require.NotNil(t, images)
	assert.Equal(t, "array", images.Type)
	assert.Equal(t, "string", images.ItemType)
	require.NotNil(t, images.MinItems)
	require.NotNil(t, images.MaxItems)
	assert.Equal(t, 1, *images.MinItems)
	assert.Equal(t, 14, *images.MaxItems)
}

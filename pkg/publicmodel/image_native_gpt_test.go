package publicmodel

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeGPTImageJSONEditMetadata(t *testing.T) {
	for _, model := range []string{"gpt-image-2", "gpt-image-2-2026-04-21", "gpt-image-2.5-flare", "gpt-image-2.5-flare-2026-09-08", "gpt-image-2.5-sunburst", "gpt-image-2.5-sunburst-2026-09-08"} {
		t.Run(model, func(t *testing.T) {
			api := NativeAsyncImageAPI("customer-picture", model)
			require.NotNil(t, api.Image.Edit)
			edit := api.Image.Edit
			assert.Equal(t, "application/json", edit.ContentType)
			assert.Equal(t, []string{"model", "prompt", "images"}, edit.RequiredFields)
			assert.Empty(t, edit.RequiredOneOf)
			parameters := make(map[string]dto.PublicAPIParameter)
			for _, parameter := range edit.Parameters {
				parameters[parameter.Name] = parameter
			}
			assert.NotContains(t, parameters, "image")
			assert.Contains(t, parameters, "response_format")
			assert.Equal(t, "object", parameters["images"].ItemType)
			assert.Equal(t, 16, *parameters["images"].MaxItems)
			assert.Equal(t, []string{"image_url", "file_id"}, parameters["images"].RequiredOneOf)
			assert.Equal(t, "string", parameters["images[].image_url"].Type)
			assert.Equal(t, "string", parameters["images[].file_id"].Type)
			assert.Equal(t, "object", parameters["mask"].Type)
			assert.Equal(t, []string{"image_url", "file_id"}, parameters["mask"].RequiredOneOf)
			assert.Equal(t, 10, *parameters["n"].Maximum)
			assert.Equal(t, 32000, *parameters["prompt"].MaxLength)
			assert.Empty(t, parameters["size"].Enum)
			assert.Equal(t, "auto", parameters["size"].DefaultValue)
			encoded, err := common.Marshal(api)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), model)
			assert.Contains(t, string(encoded), `"required_one_of":["image_url","file_id"]`)
			assert.Equal(t, "customer-picture", edit.Model)
		})
	}
}

func TestNativeImageQualityProfilesAndUnknownEdits(t *testing.T) {
	for _, tc := range []struct {
		model   string
		quality []string
	}{
		{"gpt-image-2", []string{"low", "medium", "high", "auto"}},
		{"gpt-image-2.5-flare", []string{"low", "medium", "high", "xhigh", "max", "auto"}},
		{"gpt-image-2.5-sunburst-2026-09-08", []string{"low", "medium", "high", "xhigh", "max", "auto"}},
	} {
		t.Run(tc.model, func(t *testing.T) {
			api := NativeAsyncImageAPI("customer-picture", tc.model)
			for _, contract := range []dto.PublicImageCreation{api.Image.Creation, *api.Image.Edit} {
				var quality *dto.PublicAPIParameter
				for i := range contract.Parameters {
					if contract.Parameters[i].Name == "quality" {
						quality = &contract.Parameters[i]
					}
				}
				require.NotNil(t, quality)
				assert.Equal(t, tc.quality, quality.Enum)
			}
		})
	}
	for _, model := range []string{"dall-e-3", "unknown-image", "gpt-image-2.5", "gpt-image-2.5-flare-unverified"} {
		api := NativeAsyncImageAPI("customer-picture", model)
		assert.Nil(t, api.Image.Edit, model)
		for _, operation := range api.Image.Operations {
			assert.NotEqual(t, "edit_image", operation.Operation, model)
		}
	}
	assert.Equal(t, "multipart/form-data", NativeAsyncImageAPI("customer-picture", "dall-e-2").Image.Edit.ContentType)
}

func TestNativeImageMetadataDoesNotMergeDifferentEditInputs(t *testing.T) {
	left := NativeAsyncImageAPI("customer-picture", "gpt-image-2")
	right := NativeAsyncImageAPI("customer-picture", "gpt-image-2")
	right.Image.Edit.ContentType = "multipart/form-data"
	api := CommonNativeImageAPI(left, right)
	assert.Nil(t, api.Image.Edit)
	require.NotNil(t, api.Image.Async)
	for _, operation := range api.Image.Operations {
		assert.NotEqual(t, "edit_image", operation.Operation)
	}
}

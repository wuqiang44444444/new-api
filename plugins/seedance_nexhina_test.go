package plugins

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNexhinaSeedanceMediaReferences(t *testing.T) {
	plugin, _, err := jsplugin.CompileSeedanceExtension(SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)

	for _, ref := range []string{"https://example.com/image.jpg?signature=opaque", "asset://provider-image"} {
		content := []any{map[string]any{"type": "image_url", "image_url": map[string]any{"url": ref}, "role": "reference_image"}}
		out, err := plugin.Engine.CallPathWithAdmissionTimeout(t.Context(), 0, "seedance", []string{"nexhina_modelark_v3", "buildCreate"}, map[string]any{
			"request": map[string]any{"content": content, "duration": 4}, "providerModel": "doubao-seedance-2-0-mini-260615",
		})
		require.NoError(t, err)
		got := out.(map[string]any)
		assert.Equal(t, "/seedance/api/v3/contents/generations/tasks", got["createPath"])
		assert.Equal(t, "/seedance/api/v3/contents/generations/tasks/{task_id}", got["queryPath"])
		assert.Equal(t, content, got["body"].(map[string]any)["content"])
	}
}

func TestNexhinaSeedanceAssetAction(t *testing.T) {
	plugin, _, err := jsplugin.CompileSeedanceExtension(SeedanceSource(), jsplugin.Options{}, jsplugin.SeedanceHostContract())
	require.NoError(t, err)
	hook := func(name string, input map[string]any) map[string]any {
		t.Helper()
		out, err := plugin.Engine.CallPathWithAdmissionTimeout(context.Background(), 0, "seedanceAssets", []string{"nexhina_ark_assets_v1", name}, input)
		require.NoError(t, err)
		result, ok := out.(map[string]any)
		require.True(t, ok)
		return result
	}
	out := hook("buildRequest", map[string]any{"operation": "create_asset", "request": map[string]any{
		"GroupResourceID": "ag-example", "URL": "https://example.com/image.jpg", "Name": "test", "MediaType": "image",
	}})
	assert.Equal(t, "/api/ark/v1/?Action=CreateAsset&Version=2024-01-01", out["path"])
	assert.Equal(t, map[string]any{"ProjectName": "default", "GroupId": "ag-example", "Url": "https://example.com/image.jpg", "Name": "test", "AssetType": "Image"}, out["body"])

	out = hook("parseResponse", map[string]any{"operation": "get_asset", "status": 200, "body": `{"ResponseMetadata":{"Error":{"Code":"NotFound.asset_id"}}}`})
	assert.Equal(t, map[string]any{"notFound": true}, out)

	out = hook("parseResponse", map[string]any{"operation": "get_asset", "status": 200, "body": `{"Result":{"Id":"asset-example","Status":"Active"}}`})
	assert.Equal(t, "asset-example", out["result"].(map[string]any)["ReferenceValue"])
}

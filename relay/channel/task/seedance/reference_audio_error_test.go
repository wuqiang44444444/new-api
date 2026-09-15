package seedance

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunCloudReferenceMediaFailurePreservesProviderReason(t *testing.T) {
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	require.NoError(t, err)
	result, err := plugin.Engine.CallPathWithAdmissionTimeout(context.Background(), seedanceExtensionPollAdmissionTimeout, "seedance", []string{"funcloud_modelark_v3", "parseTaskObservation"}, map[string]any{
		"taskId": "fixture-task", "body": `{"id":"fixture-task","status":"failed","error":{"code":"NO_ASSET_ID","message":"asset service no asset id returned"}}`,
	})
	require.NoError(t, err)
	raw, err := decodeOfficialPluginObservation(result, "fixture-task", plugin.Meta.APIVersion, dto.VideoUpstreamProtocolFunCloudModelArkV3)
	require.NoError(t, err)
	raw, err = validatePluginProviderObservation(raw, dto.VideoUpstreamProtocolFunCloudModelArkV3, "", "")
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, common.Unmarshal(raw, &body))
	assert.Equal(t, "failed", body["status"])
	assert.Equal(t, map[string]any{"code": "NO_ASSET_ID", "message": "asset service no asset id returned"}, body["error"])
}

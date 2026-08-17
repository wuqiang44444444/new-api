package main

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerificationModelSpecsCoverEveryCurrentFeicaiModel(t *testing.T) {
	const referenceImage = "https://example.com/reference.png"
	specs, err := verificationModelSpecs("vip-key", "value-key", referenceImage)
	require.NoError(t, err)

	expected := map[string]struct {
		credential string
		duration   int
		needsImage bool
	}{
		feicai.ProviderModelSeedance20Mini720P:      {credential: "vip", duration: 4},
		feicai.ProviderModelSeedance20SD2720P:       {credential: "value", duration: 11, needsImage: true},
		feicai.ProviderModelSeedance20Fast720P:      {credential: "vip", duration: 4},
		feicai.ProviderModelSeedance20Value720P:     {credential: "value", duration: 4},
		feicai.ProviderModelSeedance20Standard720P:  {credential: "vip", duration: 4},
		feicai.ProviderModelSeedance20Value1080P:    {credential: "value", duration: 4},
		feicai.ProviderModelSeedance20Standard1080P: {credential: "vip", duration: 4},
		feicai.ProviderModelSeedance20Value4K:       {credential: "value", duration: 4},
		feicai.ProviderModelSeedance20Standard4K:    {credential: "vip", duration: 4},
		feicai.ProviderModelSeedance20ProPI720P:     {credential: "value", duration: 15},
	}

	require.Len(t, specs, len(expected))
	assert.InDelta(t, estimatedFullMatrixSpendCNY, estimatedSpendCNY(specs), 0.001)
	seen := make(map[string]struct{}, len(specs))
	for _, spec := range specs {
		want, ok := expected[spec.ProviderModel]
		require.True(t, ok, spec.ProviderModel)
		_, duplicate := seen[spec.ProviderModel]
		assert.False(t, duplicate, spec.ProviderModel)
		seen[spec.ProviderModel] = struct{}{}
		assert.Equal(t, want.credential, spec.CredentialName, spec.ProviderModel)
		assert.Equal(t, want.duration, spec.Duration, spec.ProviderModel)
		if want.needsImage {
			assert.Equal(t, referenceImage, spec.ReferenceImage, spec.ProviderModel)
		} else {
			assert.Empty(t, spec.ReferenceImage, spec.ProviderModel)
		}
	}
}

func TestValidatedBaseURLRequiresExplicitOptInForHTTP(t *testing.T) {
	_, err := validatedBaseURL("http://43.161.200.208", false)
	require.ErrorContains(t, err, "allow-insecure-http")

	parsed, err := validatedBaseURL("http://43.161.200.208", true)
	require.NoError(t, err)
	assert.Equal(t, "http://43.161.200.208", parsed.String())

	parsed, err = validatedBaseURL("https://provider.example", false)
	require.NoError(t, err)
	assert.Equal(t, "https://provider.example", parsed.String())
}

func TestSelectVerificationModelSpecsUsesExactUniqueModels(t *testing.T) {
	specs, err := verificationModelSpecs("vip-key", "value-key", "https://example.com/reference.png")
	require.NoError(t, err)

	selected, err := selectVerificationModelSpecs(
		specs,
		"seedance-2.0-vip-720p-fast-azhw,seedance-2.0-933-4k-azhw",
	)
	require.NoError(t, err)
	require.Len(t, selected, 2)
	assert.Equal(t, "seedance-2.0-vip-720p-fast-azhw", selected[0].ProviderModel)
	assert.Equal(t, "seedance-2.0-933-4k-azhw", selected[1].ProviderModel)
	assert.InDelta(t, 16.72, estimatedSpendCNY(selected), 0.001)

	_, err = selectVerificationModelSpecs(specs, "unknown")
	require.ErrorContains(t, err, "unknown provider model")
	_, err = selectVerificationModelSpecs(specs, "seedance-2.0-vip-720p-fast-azhw,seedance-2.0-vip-720p-fast-azhw")
	require.ErrorContains(t, err, "duplicate provider model")
}

func TestVerificationReportRedactsProviderTaskAndContentIdentities(t *testing.T) {
	result := modelResult{
		ProviderModel:  "provider-model",
		ProviderID:     "private-provider-task-id",
		ObservedTaskID: "private-observed-task-id",
		VideoURL:       "https://provider.example/private-content-url",
		Passed:         true,
	}

	payload, err := common.Marshal(result)
	require.NoError(t, err)
	serialized := string(payload)

	assert.Contains(t, serialized, "provider-model")
	assert.NotContains(t, serialized, "private-provider-task-id")
	assert.NotContains(t, serialized, "private-observed-task-id")
	assert.NotContains(t, serialized, "private-content-url")
	assert.NotContains(t, serialized, "task_list")
	assert.NotContains(t, serialized, "task_quota")
}

func TestProviderErrorCodeDoesNotEchoRawProviderResponse(t *testing.T) {
	assert.Equal(t, "invalid_request", providerErrorCode([]byte(`{"error":{"code":"invalid_request","message":"private detail"}}`)))
	assert.Equal(t, "provider_error_response", providerErrorCode([]byte("private unstructured response")))
	assert.False(t, strings.Contains(providerErrorCode([]byte("private unstructured response")), "private"))
}

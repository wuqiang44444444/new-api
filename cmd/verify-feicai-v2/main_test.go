package main

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerificationModelSpecsCoverEveryFeicaiV2Model(t *testing.T) {
	const referenceImage = "https://example.com/reference.png"
	specs := verificationModelSpecs("vip-key", "value-key", referenceImage)

	expected := map[string]struct {
		credential string
		duration   int
		needsImage bool
	}{
		"seedance-2.0-vip-720p-mini-azhw-feicai": {credential: "vip", duration: 4},
		"seedance2.0-sd2-feicai":                 {credential: "value", duration: 11, needsImage: true},
		"seedance-2.0-vip-720p-fast-azhw-feicai": {credential: "vip", duration: 4},
		"seedance-2.0-933-720p-azhw-feicai":      {credential: "value", duration: 4},
		"seedance-2.0-vip-720p-azhw-feicai":      {credential: "vip", duration: 4},
		"seedance-2.0-933-1080p-azhw-feicai":     {credential: "value", duration: 4},
		"seedance-2.0-vip-1080p-azhw-feicai":     {credential: "vip", duration: 4},
		"seedance-2.0-933-4k-azhw-feicai":        {credential: "value", duration: 4},
		"seedance-2.0-vip-4k-azhw-feicai":        {credential: "vip", duration: 4},
		"seedance-933-pro-pi-feicai":             {credential: "value", duration: 15},
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

func TestVerificationModelSpecsCoverEveryFeicaiV3Model(t *testing.T) {
	specs := verificationModelSpecsForVersion("v3", "shared-key", "shared-key", "https://example.com/reference.png")

	require.Len(t, specs, 10)
	assert.Equal(t, "seedance-2.0-vip-720p-mini-azhw", specs[0].ProviderModel)
	assert.Equal(t, "seedance2.0-sd2", specs[1].ProviderModel)
	assert.Equal(t, "seedance-933-pro-pi", specs[9].ProviderModel)
	assert.Equal(t, []float64{2.88, 6.60, 3.52, 3.08, 3.64, 7.26, 8.58, 17.60, 20.80, 11.55}, []float64{
		specs[0].EstimatedCNY,
		specs[1].EstimatedCNY,
		specs[2].EstimatedCNY,
		specs[3].EstimatedCNY,
		specs[4].EstimatedCNY,
		specs[5].EstimatedCNY,
		specs[6].EstimatedCNY,
		specs[7].EstimatedCNY,
		specs[8].EstimatedCNY,
		specs[9].EstimatedCNY,
	})
	for _, spec := range specs {
		assert.NotContains(t, spec.ProviderModel, "-feicai")
		assert.Equal(t, "shared-key", spec.Credential)
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
	specs := verificationModelSpecs("vip-key", "value-key", "https://example.com/reference.png")

	selected, err := selectVerificationModelSpecs(
		specs,
		"seedance-2.0-vip-720p-fast-azhw-feicai,seedance-2.0-933-4k-azhw-feicai",
	)
	require.NoError(t, err)
	require.Len(t, selected, 2)
	assert.Equal(t, "seedance-2.0-vip-720p-fast-azhw-feicai", selected[0].ProviderModel)
	assert.Equal(t, "seedance-2.0-933-4k-azhw-feicai", selected[1].ProviderModel)
	assert.InDelta(t, 16.72, estimatedSpendCNY(selected), 0.001)

	_, err = selectVerificationModelSpecs(specs, "unknown")
	require.ErrorContains(t, err, "unknown provider model")
	_, err = selectVerificationModelSpecs(specs, "seedance-2.0-vip-720p-fast-azhw-feicai,seedance-2.0-vip-720p-fast-azhw-feicai")
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
}

func TestProviderErrorCodeDoesNotEchoRawProviderResponse(t *testing.T) {
	assert.Equal(t, "invalid_request", providerErrorCode([]byte(`{"error":{"code":"invalid_request","message":"private detail"}}`)))
	assert.Equal(t, "provider_error_response", providerErrorCode([]byte("private unstructured response")))
	assert.False(t, strings.Contains(providerErrorCode([]byte("private unstructured response")), "private"))
}

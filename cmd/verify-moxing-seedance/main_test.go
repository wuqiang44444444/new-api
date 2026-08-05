package main

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEstimatedSpendUsesMaximumDurationForAutomaticMode(t *testing.T) {
	automatic := estimatedSpendUpperCNY(moxingOverseaModel, -1, "720p", 7)
	explicitMaximum := estimatedSpendUpperCNY(moxingOverseaModel, 15, "720p", 7)
	assert.Equal(t, explicitMaximum, automatic)
	assert.Greater(t, automatic, estimatedSpendUpperCNY(moxingOverseaModel, 4, "720p", 7))
	assert.Greater(t, automatic, estimatedSpendUpperCNY(moxingOverseaModel, 15, "480p", 7))

	tokenSaveAutomatic := estimatedSpendUpperCNY(tokenSaveDoubaoModel, -1, "1080p", 7)
	assert.Equal(t, estimatedSpendUpperCNY(tokenSaveDoubaoModel, 15, "1080p", 7), tokenSaveAutomatic)
	assert.Greater(t, tokenSaveAutomatic, estimatedSpendUpperCNY(tokenSaveDoubaoModel, 4, "480p", 7))
}

func TestModelSpecificResolutionContracts(t *testing.T) {
	require.NoError(t, validateRequestDimensions(moxingOverseaModel, 4, "720p", "16:9"))
	require.Error(t, validateRequestDimensions(moxingOverseaModel, 4, "1080p", "16:9"))
	require.NoError(t, validateRequestDimensions(tokenSaveDoubaoModel, 4, "1080p", "16:9"))
}

func TestResultURLAcceptsDocumentedStringAndObjectShapes(t *testing.T) {
	for _, value := range []any{
		"https://cdn.example/video.mp4",
		`{"url":"https://cdn.example/video.mp4"}`,
		map[string]any{"primary_url": "https://cdn.example/video.mp4"},
		map[string]any{"urls": []any{"https://cdn.example/video.mp4"}},
	} {
		result, err := resultURL(value)
		require.NoError(t, err)
		assert.Equal(t, "https://cdn.example/video.mp4", result)
	}
	_, err := resultURL(map[string]any{"url": "http://cdn.example/video.mp4"})
	require.Error(t, err)
}

func TestVerificationReportRedactsProviderIdentities(t *testing.T) {
	report := verificationReport{
		ProviderTaskID: "private-provider-task-id",
		ResultURL:      "https://provider.example/private-signed-url",
		Passed:         true,
	}
	payload, err := common.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(payload), "private-provider-task-id")
	assert.NotContains(t, string(payload), "private-signed-url")
}

func TestConsumeTaskResponseRequiresStableTaskIdentity(t *testing.T) {
	report := verificationReport{ProviderTaskID: "expected"}
	_, err := consumeTaskResponse(
		[]byte(`{"task_id":"different","status":"running"}`),
		&report,
	)
	require.ErrorContains(t, err, "identity")
}

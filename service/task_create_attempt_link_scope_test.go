package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskAttemptLinkImplementationIsolatedFromNativeVideo(t *testing.T) {
	ref := dto.LinkImplementationRef{
		ID: model.LinkImplementationMoxingSeedanceMedia, Version: model.LinkImplementationVersionV1,
	}

	native, err := resolveTaskAttemptLinkImplementation("sora-2", ref)
	require.NoError(t, err)
	assert.Empty(t, native.ID)
	assert.Empty(t, native.Version)
	assert.Empty(t, native.ContentHash)

	link, err := resolveTaskAttemptLinkImplementation(model.VideoSKUSeedance20Oversea, ref)
	require.NoError(t, err)
	assert.Equal(t, model.LinkImplementationMoxingSeedanceMedia, link.ID)
	assert.NotEmpty(t, link.ContentHash)

	_, err = resolveTaskAttemptLinkImplementation(model.VideoSKUSeedance20Oversea, dto.LinkImplementationRef{})
	require.ErrorContains(t, err, "not registered")
}

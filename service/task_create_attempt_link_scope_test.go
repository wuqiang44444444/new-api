package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskAttemptLinkImplementationRequiresRegisteredSKUAndImplementation(t *testing.T) {
	ref := dto.LinkImplementationRef{
		ID: model.LinkImplementationMoxingSeedanceMedia, Version: model.LinkImplementationVersionV1,
	}

	link, err := resolveTaskAttemptLinkImplementation(model.VideoSKUSeedance20Oversea, ref)
	require.NoError(t, err)
	assert.Equal(t, model.LinkImplementationMoxingSeedanceMedia, link.ID)
	assert.NotEmpty(t, link.ContentHash)

	_, err = resolveTaskAttemptLinkImplementation(model.VideoSKUSeedance20Oversea, dto.LinkImplementationRef{})
	require.ErrorContains(t, err, "not registered")
}

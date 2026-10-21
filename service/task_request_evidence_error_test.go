package service

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDurableSendRejectionDoesNotExposePrivateCause(t *testing.T) {
	failure := &TaskRequestEvidenceError{Cause: errors.New("https://provider.test/task?sig=private")}
	require.ErrorIs(t, failure, ErrTaskRequestEvidenceUnavailable)
	assert.NotContains(t, failure.Error(), "private")
	projection, ok := ClassifyTaskRequestEvidenceRejection(failure)
	require.True(t, ok)
	assert.Equal(t, 503, projection.Status)
	assert.NotContains(t, projection.Message, "private")
	assert.NotContains(t, projection.Diagnostic, "private")
}

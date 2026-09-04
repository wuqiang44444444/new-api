package seedance

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTaskResultRejectsUnknownStatus(t *testing.T) {
	_, err := (&TaskAdaptor{}).ParseTaskResult(nil, nil, []byte(`{"id":"task","status":"provider_private_state"}`))
	require.Error(t, err)
}

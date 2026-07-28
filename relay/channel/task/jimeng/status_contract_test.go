package jimeng

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTaskResultUsesBoundedOfficialStatuses(t *testing.T) {
	generating, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"code":10000,"data":{"status":"generating"}}`))
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusInProgress, generating.Status)

	_, err = (&TaskAdaptor{}).ParseTaskResult([]byte(`{"code":10000,"data":{"status":"provider_private_state"}}`))
	require.Error(t, err)
}

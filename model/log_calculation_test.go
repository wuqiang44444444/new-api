package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogListsOmitCalculationButPreserveStoredEvidence(t *testing.T) {
	stored := `{"task_id":"task-1","billing_calculation":{"version":1,"quota":80},"group_ratio":1}`
	for _, format := range []func([]*Log){
		func(logs []*Log) { formatUserLogs(logs, 0) }, FormatAdminLogs, FormatRootLogs,
	} {
		logs := []*Log{{Other: stored}}
		format(logs)
		var other map[string]any
		require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &other))
		assert.NotContains(t, other, "billing_calculation")
		assert.Equal(t, "task-1", other["task_id"])
		assert.Equal(t, float64(1), other["group_ratio"])
	}
}

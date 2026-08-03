package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelInfoScanAcceptsSQLiteJSONStorageClasses(t *testing.T) {
	expected := ChannelInfo{
		IsMultiKey:           true,
		MultiKeySize:         2,
		MultiKeyStatusList:   map[int]int{1: 2},
		MultiKeyPollingIndex: 1,
		MultiKeyMode:         constant.MultiKeyModePolling,
	}
	encoded, err := common.Marshal(expected)
	require.NoError(t, err)

	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "blob", value: encoded},
		{name: "text", value: string(encoded)},
		{name: "null", value: nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			actual := expected

			require.NoError(t, actual.Scan(test.value))
			if test.value == nil {
				assert.Equal(t, ChannelInfo{}, actual)
				return
			}
			assert.Equal(t, expected, actual)
		})
	}
}

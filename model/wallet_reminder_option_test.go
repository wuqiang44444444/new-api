package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWalletReminderThresholdRejectsInvalidValuesWithoutReplacingSavedValue(t *testing.T) {
	db := setupErrorEventTestDB(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	oldOptions, oldThreshold := common.OptionMap, common.QuotaRemindThreshold
	common.OptionMap = map[string]string{}
	t.Cleanup(func() { common.OptionMap, common.QuotaRemindThreshold = oldOptions, oldThreshold })
	const key = "QuotaRemindThreshold"
	require.NoError(t, UpdateOption(key, "10000"))
	for _, bad := range []string{"", "1e4", "1.5", "-1", "9007199254740992"} {
		t.Run(bad, func(t *testing.T) {
			require.Error(t, UpdateOption(key, bad))
			var stored Option
			require.NoError(t, db.Where(&Option{Key: key}).First(&stored).Error)
			assert.Equal(t, "10000", stored.Value)
			assert.Equal(t, 10000, common.QuotaRemindThreshold)
		})
	}
	require.Error(t, updateOptionMap(key, "1e4"), "an invalid existing option must leave the last valid value active")
	assert.Equal(t, 10000, common.QuotaRemindThreshold)
	require.NoError(t, UpdateOption(key, "0"))
	assert.Zero(t, common.QuotaRemindThreshold)
}

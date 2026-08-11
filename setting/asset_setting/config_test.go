package asset_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCurrentUsesSimpleBoundedAssetSettings(t *testing.T) {
	original := businessSetting
	t.Cleanup(func() { businessSetting = original })

	businessSetting.Enabled = false
	businessSetting.MaxAssetsPerUser = 0
	businessSetting.RemoteURLMaxLength = 100000

	config := Current()
	assert.False(t, config.Enabled)
	assert.Equal(t, int64(1000), config.MaxAssetsPerUser)
	assert.Equal(t, 65536, config.RemoteURLMaxLength)
}

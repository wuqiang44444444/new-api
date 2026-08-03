package asset_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCurrentUsesDynamicBusinessSettingsAndBoundedOperationalValues(t *testing.T) {
	original := businessSetting
	t.Cleanup(func() { businessSetting = original })
	t.Setenv("REAL_PERSON_ASSET_ENABLED", "invalid")

	businessSetting.Enabled = true
	businessSetting.RealPersonEnabled = true
	businessSetting.JobMaxAttempts = 1000
	businessSetting.PollIntervalSeconds = 0

	config := Current()
	assert.True(t, config.Enabled)
	assert.True(t, config.RealPersonEnabled)
	assert.Equal(t, 100, config.JobMaxAttempts)
	assert.Equal(t, int64(5), config.PollIntervalSeconds)
}

func TestOrdinaryAssetLibraryDefaultsEnabled(t *testing.T) {
	assert.True(t, businessSetting.Enabled)
}

func TestOrdinaryAssetLibraryUsesDynamicSetting(t *testing.T) {
	original := businessSetting
	t.Cleanup(func() { businessSetting = original })
	businessSetting.Enabled = false

	assert.False(t, Current().Enabled)
}

func TestRealPersonEnvironmentKillSwitchOverridesDynamicEnablement(t *testing.T) {
	original := businessSetting
	t.Cleanup(func() { businessSetting = original })
	businessSetting.Enabled = true
	businessSetting.RealPersonEnabled = true
	t.Setenv("REAL_PERSON_ASSET_ENABLED", "false")

	config := Current()
	assert.True(t, config.Enabled)
	assert.False(t, config.RealPersonEnabled)
}

func TestVerificationReadyRequiresHTTPSAndPersistentSecret(t *testing.T) {
	t.Setenv("SESSION_SECRET", "stable-test-session-secret")
	config := Config{
		Enabled: true, RealPersonEnabled: true,
		PublicBaseURL: "https://api.example",
	}
	assert.True(t, config.VerificationReady())
	config.PublicBaseURL = "http://api.example"
	assert.False(t, config.VerificationReady())
	config.PublicBaseURL = "https://api.example"
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("CRYPTO_SECRET", "")
	assert.False(t, config.VerificationReady())
}

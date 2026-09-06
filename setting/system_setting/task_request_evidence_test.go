package system_setting

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEvidenceInvalidConfigStaysEnabledAndRejects(t *testing.T) {
	for _, value := range []string{"0", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv(TaskRequestEvidenceEnabledEnv, "true")
			t.Setenv(TaskRequestEvidenceWriteTimeoutSecondsEnv, value)
			config := LoadTaskRequestEvidenceConfig()
			assert.True(t, config.Enabled)
			require.Error(t, ValidateTaskRequestEvidenceConfig(config))
		})
	}
}

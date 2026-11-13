package system_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceConfigDefaultOff(t *testing.T) {
	t.Setenv(TaskRequestEvidenceEnabledEnv, "")
	t.Setenv(TaskRequestEvidenceRetentionDaysEnv, "")
	config := LoadTaskRequestEvidenceConfig()
	assert.False(t, config.Enabled)
	assert.Equal(t, 2, config.RetentionDays)
	require.NoError(t, ValidateTaskRequestEvidenceConfig(config))
}

func TestEvidenceConfigExplicitEnable(t *testing.T) {
	t.Setenv(TaskRequestEvidenceEnabledEnv, "true")
	t.Setenv(TaskRequestEvidenceRetentionDaysEnv, "7")
	config := LoadTaskRequestEvidenceConfig()
	assert.True(t, config.Enabled)
	assert.Equal(t, 7, config.RetentionDays)
	assert.True(t, config.RetentionFromEnv)
	require.NoError(t, ValidateTaskRequestEvidenceConfig(config))
}

func TestEvidenceConfigInvalidEnabledValue(t *testing.T) {
	t.Setenv(TaskRequestEvidenceEnabledEnv, "yes")
	config := LoadTaskRequestEvidenceConfig()
	// 非法启用值不被当成开启，但保留加载期错误事实供诊断。
	assert.False(t, config.Enabled)
	assert.NotEmpty(t, config.InvalidReason)
}

func TestEvidenceConfigRetentionRange(t *testing.T) {
	for _, raw := range []string{"0", "8", "-1", "1.5", "invalid"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv(TaskRequestEvidenceEnabledEnv, "true")
			t.Setenv(TaskRequestEvidenceRetentionDaysEnv, raw)
			config := LoadTaskRequestEvidenceConfig()
			require.NoError(t, ValidateTaskRequestEvidenceConfig(config))
			assert.Equal(t, 2, config.RetentionDays)
			assert.False(t, config.RetentionFromEnv)
		})
	}
}

func TestEvidenceConfigDisabledIgnoresInvalidRetention(t *testing.T) {
	t.Setenv(TaskRequestEvidenceEnabledEnv, "false")
	t.Setenv(TaskRequestEvidenceRetentionDaysEnv, "99")
	config := LoadTaskRequestEvidenceConfig()
	assert.False(t, config.Enabled)
	assert.Equal(t, 2, config.RetentionDays)
	require.NoError(t, ValidateTaskRequestEvidenceConfig(config))
}

package system_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvidenceConfigDefaultOff(t *testing.T) {
	t.Setenv(TaskRequestEvidenceEnabledEnv, "")
	config := LoadTaskRequestEvidenceConfig()
	assert.False(t, config.Enabled)
	require.NoError(t, ValidateTaskRequestEvidenceConfig(config))
}

func TestEvidenceConfigExplicitEnable(t *testing.T) {
	t.Setenv(TaskRequestEvidenceEnabledEnv, "true")
	config := LoadTaskRequestEvidenceConfig()
	assert.True(t, config.Enabled)
	require.NoError(t, ValidateTaskRequestEvidenceConfig(config))
}

func TestEvidenceConfigInvalidEnabledValue(t *testing.T) {
	t.Setenv(TaskRequestEvidenceEnabledEnv, "yes")
	config := LoadTaskRequestEvidenceConfig()
	// 非法启用值不被当成开启，但保留加载期错误事实供诊断。
	assert.False(t, config.Enabled)
	assert.NotEmpty(t, config.InvalidReason)
}

package billingexpr

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRequiresUsage(t *testing.T) {
	for name, value := range compileEnvPrototypeV1 {
		if _, numeric := value.(float64); numeric {
			assert.True(t, RequiresUsage(name+" * 2"), name)
		}
	}
	for _, expression := range []string{`u("output_tokens")`, `u(param("key"))`} {
		assert.True(t, RequiresUsage(expression), expression)
	}
	for _, expression := range []string{`tier("base", 5)`, `max(param("input_image_count") - 1, 0)`, `hour("UTC") > 12 ? 1 : 2`} {
		assert.False(t, RequiresUsage(expression), expression)
	}
}

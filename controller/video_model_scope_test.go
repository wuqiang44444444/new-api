package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNativeOpenAIVideoModelsRemainDiscoverable(t *testing.T) {
	for _, modelName := range []string{"sora-2", "sora-2-pro"} {
		_, exists := openAIModelsMap[modelName]
		assert.True(t, exists, modelName)
	}
}

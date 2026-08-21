package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestStrictImageAPITypesDisablePassThrough(t *testing.T) {
	assert.True(t, isStrictImageAPIType(constant.APITypeAsyncImage))
	assert.True(t, isStrictImageAPIType(constant.APITypeMoxingImage))
	assert.False(t, isStrictImageAPIType(constant.APITypeOpenAI))
}

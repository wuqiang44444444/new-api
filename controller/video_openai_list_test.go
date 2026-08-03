package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIVideoEmptyListKeepsNullableCursorFields(t *testing.T) {
	body, err := common.Marshal(openAIVideoListResponse{
		Object: "list",
		Data:   []*dto.OpenAIVideo{},
	})
	require.NoError(t, err)

	var response map[string]any
	require.NoError(t, common.Unmarshal(body, &response))
	firstID, hasFirstID := response["first_id"]
	lastID, hasLastID := response["last_id"]
	assert.True(t, hasFirstID)
	assert.Nil(t, firstID)
	assert.True(t, hasLastID)
	assert.Nil(t, lastID)
}

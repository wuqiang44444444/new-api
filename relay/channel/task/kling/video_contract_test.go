package kling

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKlingContractPayloadPreservesOfficialFieldsAndModelMapping(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractKlingV1,
		Kling: &dto.KlingVideoCreateRequest{
			ModelName: common.GetPointer("public-kling"),
			Prompt:    common.GetPointer("move"),
			CfgScale:  common.GetPointer(0.0),
		},
	})

	payload, typed, err := klingContractPayload(context, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "upstream-kling"},
	})

	require.NoError(t, err)
	assert.True(t, typed)
	assert.Equal(t, "upstream-kling", payload.ModelName)
	assert.Equal(t, "move", payload.Prompt)
	assert.Equal(t, 0.0, payload.CfgScale)
}

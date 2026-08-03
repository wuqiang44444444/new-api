package jimeng

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

func TestJimengContractPayloadPreservesExplicitSeedAndMapsModel(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractJimeng,
		Jimeng: &dto.JimengVideoCreateRequest{
			ReqKey: "public-jimeng",
			Prompt: common.GetPointer("move"),
			Seed:   common.GetPointer(int64(0)),
		},
	})

	payload, typed, err := jimengContractPayload(context, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "jimeng_v30"},
	})

	require.NoError(t, err)
	assert.True(t, typed)
	assert.Equal(t, "jimeng_t2v_v30", payload.ReqKey)
	assert.Equal(t, int64(0), payload.Seed)
	assert.Equal(t, "move", payload.Prompt)
}

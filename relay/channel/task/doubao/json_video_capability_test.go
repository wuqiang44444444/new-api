package doubao

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONVideoCreateRequestUsesMappedModelFromTypedContract(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model:   model.VideoSKUSeedance20Standard720P,
			Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("hello")}},
		},
	})
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Standard720P)
	require.True(t, ok)
	common.SetContextKey(c, constant.ContextKeyResolvedVideoSKUCapability, capability)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "private-upstream-model",
			IsModelMapped:     true,
		},
	}

	body, handled, err := buildJSONVideoMediaArraysCreateRequest(
		c,
		info,
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
	)

	require.NoError(t, err)
	assert.True(t, handled)
	assert.JSONEq(t, `{
		"model":"private-upstream-model",
		"prompt":"hello",
		"duration":4,
		"size":"1280x720"
	}`, string(body))
}

func TestJSONVideoCreateRequestRejectsGenericBodyFallback(t *testing.T) {
	body, err := convertVideoCreateRequest(
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		[]byte(`{"model":"private-upstream-model"}`),
	)

	require.ErrorContains(t, err, "typed capability path")
	assert.Nil(t, body)
}

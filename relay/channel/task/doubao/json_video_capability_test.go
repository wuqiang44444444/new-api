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

func TestJSONVideoCreateRequestFailsClosedWithoutVerifiedMappedModelSize(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	duration, resolution, ratio := 4, "1080p", "16:9"
	relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model: model.VideoSKUSeedance20Value1080P, Duration: &duration, Resolution: &resolution, Ratio: &ratio,
			Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("hello")}},
		},
	})
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Value1080P)
	require.True(t, ok)
	capability.Ratios = []string{"16:9"}
	common.SetContextKey(c, constant.ContextKeyResolvedVideoSKUCapability, capability)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: model.FeicaiProviderModelSeedance20Value1080P,
			IsModelMapped:     true,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				LinkImplementation: dto.LinkImplementationRef{
					ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV2,
				},
			},
		},
	}

	body, handled, err := buildJSONVideoMediaArraysCreateRequest(
		c,
		info,
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
	)

	require.ErrorContains(t, err, "no verified provider size")
	assert.True(t, handled)
	assert.Nil(t, body)
}

func TestJSONVideoCreateRequestRejectsGenericBodyFallback(t *testing.T) {
	body, err := convertVideoCreateRequest(
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		[]byte(`{"model":"private-upstream-model"}`),
	)

	require.ErrorContains(t, err, "typed capability path")
	assert.Nil(t, body)
}

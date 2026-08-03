package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaykitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdapterPreflightRevalidatesFrozenVideoSKUCapability(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUKlingV1)
	require.True(t, ok)
	common.SetContextKey(context, constant.ContextKeyResolvedVideoSKUCapability, capability)
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractKlingV1,
		Kling: &dto.KlingVideoCreateRequest{
			ModelName: common.GetPointer(model.VideoSKUKlingV1),
			Prompt:    common.GetPointer("move"),
			Duration:  common.GetPointer("5"),
		},
	})
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType: constant.ChannelTypeKling,
		ChannelOtherSettings: relaykitdto.ChannelOtherSettings{
			VideoUpstreamProfile: relaykitdto.VideoUpstreamProfileOfficial,
			LinkImplementation: relaykitdto.LinkImplementationRef{
				ID: model.LinkImplementationKlingVideos, Version: model.LinkImplementationVersionV1,
			},
		},
	}}

	require.Nil(t, ValidateFrozenVideoSKUCapability(context, info))

	info.ChannelType = constant.ChannelTypeJimeng
	require.NotNil(t, ValidateFrozenVideoSKUCapability(context, info))
}

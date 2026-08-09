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

func TestAdapterPreflightRejectsFeicaiModelWithoutVerifiedSize(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUSeedance20Value1080P)
	require.True(t, ok)
	common.SetContextKey(context, constant.ContextKeyResolvedVideoSKUCapability, capability)
	common.SetContextKey(
		context,
		constant.ContextKeyChannelModelMapping,
		`{"seedance-2.0-value-1080p":"seedance-2.0-933-1080p-azhw-feicai"}`,
	)
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model:      model.VideoSKUSeedance20Value1080P,
			Content:    []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
			Duration:   common.GetPointer(4),
			Resolution: common.GetPointer("1080p"),
			Ratio:      common.GetPointer("16:9"),
		},
	})
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelType: constant.ChannelTypeDoubaoVideo,
		ChannelOtherSettings: relaykitdto.ChannelOtherSettings{
			VideoUpstreamProfile:           relaykitdto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
			VideoUpstreamCreatePath:        "/v1/videos",
			VideoUpstreamQueryPathTemplate: "/v1/videos/{task_id}",
			AssetUpstreamProfile:           relaykitdto.AssetUpstreamProfileNone,
			LinkImplementation: relaykitdto.LinkImplementationRef{
				ID: model.LinkImplementationFeicaiSeedanceVideos, Version: model.LinkImplementationVersionV2,
			},
		},
	}}

	require.Equal(t, "unsupported_parameter", ValidateFrozenVideoSKUCapability(context, info).Code)

	common.SetContextKey(
		context,
		constant.ContextKeyChannelModelMapping,
		`{"seedance-2.0-value-1080p":"seedance-2.0-933-1080p-azhw-unknown"}`,
	)
	require.NotNil(t, ValidateFrozenVideoSKUCapability(context, info))
}

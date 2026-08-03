package controller

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

func TestPublishedVideoTaskFreezesResolvedSKUCapability(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	capability, ok := model.ResolveVideoSKUCapability(model.VideoSKUKlingV1)
	require.True(t, ok)
	common.SetContextKey(context, constant.ContextKeyResolvedVideoSKUCapability, capability)
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractKlingV1,
		Kling: &dto.KlingVideoCreateRequest{
			ModelName: common.GetPointer(model.VideoSKUKlingV1),
			Prompt:    common.GetPointer("move"),
		},
	})
	task := &model.Task{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{ClientProtocol: model.TaskClientProtocolKlingV1},
		ChannelMeta:   &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeKling},
	}

	attachTaskProtocolSnapshot(context, task, info)

	assert.Equal(t, capability.Version, task.PrivateData.SKUCapabilityVersion)
	assert.Equal(t, capability.ContentHash, task.PrivateData.SKUCapabilityHash)
	assert.Equal(t, capability.Lifecycle, task.PrivateData.SKULifecycle)
	assert.Equal(t, string(dto.VideoContractKlingV1), task.PrivateData.NorthboundContractID)
}

func TestPublishedOmniVideoTaskFreezesAdapterV2(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk:   &dto.ModelArkVideoCreateRequest{Model: model.VideoSKUSeedance20Standard720P},
	})
	task := &model.Task{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{ClientProtocol: model.TaskClientProtocolModelArkV3},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeDoubaoVideo,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProfile: dto.VideoUpstreamProfileThirdPartyJSONVideoOmniReference,
			},
		},
	}

	attachTaskProtocolSnapshot(context, task, info)

	assert.Equal(t,
		"54:third_party_json_video_omni_reference:v2",
		task.PrivateData.SouthboundAdapterVersion,
	)
}

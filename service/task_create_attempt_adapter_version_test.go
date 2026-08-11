package service

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTaskAttemptRecoveryTemplateUsesSameURLMediaArraysAdapter(t *testing.T) {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	task := &model.Task{}
	info := &relaycommon.RelayInfo{
		TaskRelayInfo: &relaycommon.TaskRelayInfo{ClientProtocol: model.TaskClientProtocolModelArkV3},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType: constant.ChannelTypeSeedanceLink,
			ChannelOtherSettings: dto.ChannelOtherSettings{
				VideoUpstreamProtocol: dto.VideoUpstreamProtocolURLMediaArraysV1,
			},
		},
	}

	stageTaskProtocolSnapshot(context, task, info)

	assert.Equal(t,
		"61:third_party_json_video_media_arrays:v2",
		task.PrivateData.SouthboundAdapterVersion,
	)
}

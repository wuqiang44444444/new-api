package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestValidateChannelVideoSettingsRejectsRetiredOpenAIVideoModels(t *testing.T) {
	tests := []Channel{
		{Type: constant.ChannelTypeSora, Models: "sora-2", Status: common.ChannelStatusEnabled},
		{Type: constant.ChannelTypeOpenAI, Models: "gpt-4o,sora-2-pro", Status: common.ChannelStatusEnabled},
	}
	for index := range tests {
		require.Error(t, validateChannelVideoSettings(&tests[index], &dto.ChannelOtherSettings{}))
	}
	require.NoError(t, validateChannelVideoSettings(
		&Channel{Type: constant.ChannelTypeKling, Models: "kling-v2-master"},
		&dto.ChannelOtherSettings{},
	))
	require.NoError(t, validateChannelVideoSettings(
		&Channel{Type: constant.ChannelTypeSora, Models: "sora-2", Status: common.ChannelStatusManuallyDisabled},
		&dto.ChannelOtherSettings{},
	))
}

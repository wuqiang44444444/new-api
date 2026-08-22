package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func imageRelayChannel(protocol dto.ImageUpstreamProtocol, models, mapping string) *Channel {
	channel := &Channel{
		Type:         constant.ChannelTypeAsyncImage,
		Models:       models,
		ModelMapping: common.GetPointer(mapping),
		BaseURL:      common.GetPointer("https://provider.example"),
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{ImageUpstreamProtocol: protocol})
	return channel
}

func TestImageRelayChannelSettingsUseAdministratorModelMapping(t *testing.T) {
	channel := imageRelayChannel(
		dto.ImageUpstreamProtocolMoxingImagesV1,
		"lite-customer,pro-customer",
		`{"lite-customer":"doubao-seedream-5-0-260128","pro-customer":"alias-pro","alias-pro":"doubao-seedream-5-0-pro-260628","unrelated":"free"}`,
	)
	require.NoError(t, channel.ValidateSettings())

	direct := imageRelayChannel(
		dto.ImageUpstreamProtocolFunCloudAIGCV2,
		constant.FunCloudImageProviderModelNanoBanana2,
		"{}",
	)
	require.NoError(t, direct.ValidateSettings())
}

func TestImageRelayChannelSettingsRejectProtocolMismatchAndUnsafeOverrides(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Channel)
		message string
	}{
		{
			name: "missing protocol",
			mutate: func(channel *Channel) {
				channel.SetOtherSettings(dto.ChannelOtherSettings{})
			},
			message: "unsupported image upstream protocol",
		},
		{
			name: "model belongs to other protocol",
			mutate: func(channel *Channel) {
				channel.ModelMapping = common.GetPointer(`{"customer":"nano-banana-2"}`)
			},
			message: "unsupported by image protocol",
		},
		{
			name: "body pass through",
			mutate: func(channel *Channel) {
				channel.SetSetting(dto.ChannelSettings{PassThroughBodyEnabled: true})
			},
			message: "do not allow request body pass-through",
		},
		{
			name: "parameter override",
			mutate: func(channel *Channel) {
				channel.ParamOverride = common.GetPointer(`{"size":"4K"}`)
			},
			message: "do not allow parameter overrides",
		},
		{
			name: "missing explicit base URL",
			mutate: func(channel *Channel) {
				channel.BaseURL = nil
			},
			message: "explicit base URL",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := imageRelayChannel(
				dto.ImageUpstreamProtocolMoxingImagesV1,
				"customer",
				`{"customer":"doubao-seedream-5-0-260128"}`,
			)
			test.mutate(channel)
			err := channel.ValidateSettings()
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.message)
		})
	}
}

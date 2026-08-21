package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validMoxingImageChannel() *Channel {
	return &Channel{
		Type:         constant.ChannelTypeMoxingImage,
		Models:       "seedream-5-moxing",
		ModelMapping: common.GetPointer(`{"seedream-5-moxing":"doubao-seedream-5-0-260128"}`),
	}
}

func TestMoxingImageChannelSettingsAcceptPublishedMappings(t *testing.T) {
	tests := []struct {
		name          string
		customerModel string
		providerModel string
	}{
		{name: "lite", customerModel: "seedream-5-moxing", providerModel: constant.MoxingImageProviderModelSeedream5Lite},
		{name: "pro", customerModel: "seedream-5-pro-moxing", providerModel: constant.MoxingImageProviderModelSeedream5Pro},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := validMoxingImageChannel()
			channel.Models = test.customerModel
			channel.ModelMapping = common.GetPointer(`{"` + test.customerModel + `":"` + test.providerModel + `"}`)
			require.NoError(t, channel.ValidateSettings())
		})
	}
}

func TestMoxingImageChannelSettingsAcceptsAllPublishedModelsInOneChannel(t *testing.T) {
	channel := validMoxingImageChannel()
	channel.Models = "seedream-5-moxing,seedream-5-pro-moxing"
	channel.ModelMapping = common.GetPointer(`{
		"seedream-5-moxing":"doubao-seedream-5-0-260128",
		"seedream-5-pro-moxing":"doubao-seedream-5-0-pro-260628"
	}`)

	require.NoError(t, channel.ValidateSettings())
}

func TestMoxingImageChannelSettingsUseAdministratorModelMappingSemantics(t *testing.T) {
	tests := []struct {
		name         string
		models       string
		modelMapping *string
	}{
		{
			name:         "provider models need no mapping",
			models:       "doubao-seedream-5-0-260128,doubao-seedream-5-0-pro-260628",
			modelMapping: nil,
		},
		{
			name:   "administrator mapping may contain an intermediate alias",
			models: "customer-lite",
			modelMapping: common.GetPointer(`{
				"customer-lite":"provider-lite-alias",
				"provider-lite-alias":"doubao-seedream-5-0-260128"
			}`),
		},
		{
			name:   "administrator mapping may contain unrelated entries",
			models: "customer-lite",
			modelMapping: common.GetPointer(`{
				"customer-lite":"doubao-seedream-5-0-260128",
				"unused-administrator-alias":"another-model"
			}`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := validMoxingImageChannel()
			channel.Models = test.models
			channel.ModelMapping = test.modelMapping

			require.NoError(t, channel.ValidateSettings())
		})
	}
}

func TestMoxingImageChannelSettingsRejectUnsafeConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Channel)
		wantErr string
	}{
		{
			name: "customer model without a resolvable provider model",
			mutate: func(channel *Channel) {
				channel.Models = "administrator-customer-model"
				channel.ModelMapping = nil
			},
			wantErr: "resolves to unsupported",
		},
		{
			name: "duplicate customer model",
			mutate: func(channel *Channel) {
				channel.Models = "seedream-5-moxing,seedream-5-moxing"
				channel.ModelMapping = common.GetPointer(`{"seedream-5-moxing":"doubao-seedream-5-0-260128"}`)
			},
			wantErr: "duplicated",
		},
		{
			name: "customer model with surrounding whitespace",
			mutate: func(channel *Channel) {
				channel.Models = "seedream-5-moxing, seedream-5-pro-moxing"
				channel.ModelMapping = common.GetPointer(`{
					"seedream-5-moxing":"doubao-seedream-5-0-260128",
					"seedream-5-pro-moxing":"doubao-seedream-5-0-pro-260628"
				}`)
			},
			wantErr: "surrounding whitespace",
		},
		{
			name: "unknown provider model",
			mutate: func(channel *Channel) {
				channel.ModelMapping = common.GetPointer(`{"seedream-5-moxing":"doubao-seedream-unknown"}`)
			},
			wantErr: "resolves to unsupported",
		},
		{
			name: "mapping cycle",
			mutate: func(channel *Channel) {
				channel.ModelMapping = common.GetPointer(`{
					"seedream-5-moxing":"alias",
					"alias":"seedream-5-moxing"
				}`)
			},
			wantErr: "model mapping contains a cycle",
		},
		{
			name: "request pass-through",
			mutate: func(channel *Channel) {
				channel.SetSetting(dto.ChannelSettings{PassThroughBodyEnabled: true})
			},
			wantErr: "pass-through",
		},
		{
			name: "parameter override",
			mutate: func(channel *Channel) {
				channel.ParamOverride = common.GetPointer(`{"size":"4K"}`)
			},
			wantErr: "parameter overrides",
		},
		{
			name: "advanced custom route",
			mutate: func(channel *Channel) {
				channel.SetOtherSettings(dto.ChannelOtherSettings{
					AdvancedCustom: &dto.AdvancedCustomConfig{
						Routes: []dto.AdvancedCustomRoute{{
							IncomingPath: "/v1/images/generations",
							UpstreamPath: "/v1/images/generations",
							Converter:    "none",
						}},
					},
				})
			},
			wantErr: "advanced custom routes",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := validMoxingImageChannel()
			test.mutate(channel)

			err := channel.ValidateSettings()
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

package service

import (
	"context"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/doubao/assets"
)

func CheckAssetChannelConnectivity(ctx context.Context, channel *model.Channel) error {
	if channel == nil {
		return newChannelConnectivityError(
			ChannelConnectivityAssetProxyInvalidConfig,
			"asset upstream configuration is invalid",
			nil,
		)
	}
	profile := channel.GetOtherSettings().AssetUpstreamProfile
	if !profile.IsRoutable() {
		return newChannelConnectivityError(
			ChannelConnectivityAssetProxyInvalidConfig,
			"channel does not use a routable asset upstream",
			nil,
		)
	}
	key, _, err := singleChannelCredential(channel)
	if err != nil {
		if profile != dto.AssetUpstreamProfileOfficial {
			return newChannelConnectivityError(
				ChannelConnectivityAssetProxyNotConfigured,
				"asset upstream credentials are not configured",
				err,
			)
		}
		return newChannelConnectivityError(
			ChannelConnectivityAssetNotConfigured,
			"official asset Action credentials are not configured",
			err,
		)
	}
	return checkAssetChannelConnectivity(ctx, channel, profile, key)
}

func CheckProposedOfficialAssetCredentialConnectivity(
	ctx context.Context,
	channel *model.Channel,
	input *dto.ChannelAssetCredentialInput,
) error {
	if channel == nil || channel.GetOtherSettings().AssetUpstreamProfile != dto.AssetUpstreamProfileOfficial {
		return newChannelConnectivityError(
			ChannelConnectivityAssetInvalidConfig,
			"channel does not use official Action Assets",
			nil,
		)
	}
	credential, err := model.NormalizeChannelAssetCredential(input)
	if err != nil {
		return newChannelConnectivityError(
			ChannelConnectivityAssetNotConfigured,
			"official asset Action credentials are not configured",
			err,
		)
	}
	if credential == nil {
		return newChannelConnectivityError(
			ChannelConnectivityAssetNotConfigured,
			"official asset Action credentials are not configured",
			nil,
		)
	}
	key := credential.AccessKeyID + "|" + credential.SecretAccessKey
	return checkAssetChannelConnectivity(ctx, channel, dto.AssetUpstreamProfileOfficial, key)
}

func checkAssetChannelConnectivity(ctx context.Context, channel *model.Channel, profile dto.AssetUpstreamProfile, key string) error {
	adapter, err := assetAdapterForChannel(channel, profile, key)
	if err != nil {
		if profile != dto.AssetUpstreamProfileOfficial {
			return newChannelConnectivityError(
				ChannelConnectivityAssetProxyInvalidConfig,
				"asset upstream configuration is invalid",
				err,
			)
		}
		return newChannelConnectivityError(
			ChannelConnectivityAssetInvalidConfig,
			"official asset Action configuration is invalid",
			err,
		)
	}
	connectivityAdapter, ok := adapter.(assetadapter.ConnectivityAdapter)
	if !ok {
		if profile != dto.AssetUpstreamProfileOfficial {
			return newChannelConnectivityError(
				ChannelConnectivityAssetProxyInvalidConfig,
				"asset upstream does not support a read-only connectivity test",
				nil,
			)
		}
		return newChannelConnectivityError(
			ChannelConnectivityAssetInvalidConfig,
			"official asset Action does not support a read-only connectivity test",
			nil,
		)
	}
	if err := connectivityAdapter.CheckConnectivity(ctx); err != nil {
		if profile != dto.AssetUpstreamProfileOfficial {
			code := ChannelConnectivityAssetProxyUnavailable
			message := "asset upstream is unavailable"
			if assetadapter.IsDefinitiveUpstreamRejection(err) {
				code = ChannelConnectivityAssetProxyRejected
				message = "asset upstream rejected the request"
			}
			return newChannelConnectivityError(code, message, err)
		}
		code := ChannelConnectivityAssetUnavailable
		message := "official asset Action is unavailable"
		if assetadapter.IsDefinitiveUpstreamRejection(err) {
			code = ChannelConnectivityAssetRejected
			message = "official asset Action rejected the request"
		}
		return newChannelConnectivityError(code, message, err)
	}
	return nil
}

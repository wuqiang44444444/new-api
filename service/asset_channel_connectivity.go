package service

import (
	"errors"

	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
)

func assetChannelConfigurationError(err error) error {
	if errors.Is(err, ErrAssetLibraryUnsupported) {
		return newChannelConnectivityError(
			ChannelConnectivityAssetNotConfigured,
			"asset action is not configured",
			err,
		)
	}
	return newChannelConnectivityError(
		ChannelConnectivityAssetInvalidConfig,
		"asset action configuration is invalid",
		err,
	)
}

func assetChannelUpstreamError(err error) error {
	if assetadapter.IsDefinitiveUpstreamRejection(err) {
		return newChannelConnectivityError(
			ChannelConnectivityAssetRejected,
			"asset upstream rejected the request",
			err,
		)
	}
	return newChannelConnectivityError(
		ChannelConnectivityAssetUnavailable,
		"asset upstream is unavailable",
		err,
	)
}

package service

import "errors"

const (
	ChannelConnectivityAssetNotConfigured      = "asset_action_not_configured"
	ChannelConnectivityAssetInvalidConfig      = "asset_action_invalid_configuration"
	ChannelConnectivityAssetRejected           = "asset_action_upstream_rejected"
	ChannelConnectivityAssetUnavailable        = "asset_action_upstream_unavailable"
	ChannelConnectivityAssetProxyNotConfigured = "asset_upstream_not_configured"
	ChannelConnectivityAssetProxyInvalidConfig = "asset_upstream_invalid_configuration"
	ChannelConnectivityAssetProxyRejected      = "asset_upstream_rejected"
	ChannelConnectivityAssetProxyUnavailable   = "asset_upstream_unavailable"
	ChannelConnectivityVideoNotConfigured      = "video_api_not_configured"
	ChannelConnectivityVideoInvalidConfig      = "video_api_invalid_configuration"
	ChannelConnectivityVideoRejected           = "video_api_upstream_rejected"
	ChannelConnectivityVideoUnavailable        = "video_api_upstream_unavailable"
)

type channelConnectivityError struct {
	code    string
	message string
	cause   error
}

func (e *channelConnectivityError) Error() string {
	return e.message
}

func (e *channelConnectivityError) Unwrap() error {
	return e.cause
}

func newChannelConnectivityError(code, message string, cause error) error {
	return &channelConnectivityError{code: code, message: message, cause: cause}
}

func ChannelConnectivityErrorCode(err error) string {
	var connectivityErr *channelConnectivityError
	if errors.As(err, &connectivityErr) {
		return connectivityErr.code
	}
	return ""
}

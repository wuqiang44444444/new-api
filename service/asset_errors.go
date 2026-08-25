package service

import "errors"

var (
	ErrInvalidAssetRequest       = errors.New("asset request is invalid")
	ErrAssetNotFound             = errors.New("asset not found")
	ErrUnsupportedAssetType      = errors.New("unsupported asset type for upstream")
	ErrUnsupportedAssetOperation = errors.New("asset operation is not supported by this model")
	ErrAssetModelNotFound        = errors.New("asset model was not found")
	ErrAssetLibraryUnsupported   = errors.New("asset library is not supported by this model")
	ErrAssetLibraryUnavailable   = errors.New("asset library is unavailable")
	ErrAssetURLRequired          = errors.New("remote assets require an HTTPS URL")
	ErrUnsafeAssetURL            = errors.New("asset URL is unsafe")
	ErrAssetURLTTLInsufficient   = errors.New("asset URL TTL is insufficient")
	ErrAssetUpstreamError        = errors.New("asset upstream rejected the request")
	ErrAssetUpstreamUnavailable  = errors.New("asset upstream is unavailable")
)

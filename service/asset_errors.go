package service

import "errors"

var (
	ErrInvalidAssetRequest        = errors.New("asset request is invalid")
	ErrAssetNotFound              = errors.New("asset not found")
	ErrAssetNotReady              = errors.New("asset is not ready")
	ErrAssetChannelMismatch       = errors.New("asset belongs to a different Seedance channel")
	ErrAssetScopeConflict         = errors.New("asset belongs to a different Provider account scope")
	ErrAssetReferenceUnresolvable = errors.New("asset reference cannot be resolved by the selected channel")
	ErrUnsupportedAssetType       = errors.New("unsupported asset type for upstream")
	ErrAssetLibraryUnavailable    = errors.New("asset library is unavailable")
	ErrAssetURLRequired           = errors.New("remote assets require an HTTPS URL")
	ErrUnsafeAssetURL             = errors.New("asset URL is unsafe")
	ErrAssetURLTTLInsufficient    = errors.New("asset URL TTL is insufficient")
	ErrAssetUpstreamError         = errors.New("asset upstream rejected the request")
	ErrAssetUpstreamUnavailable   = errors.New("asset upstream is unavailable")
)

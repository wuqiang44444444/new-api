package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/model"
)

// AssetClientErrorDiagnostic maps asset contract errors to controlled stage and
// reason codes for the unified 4xx diagnostic event. An empty stage means the
// error settles as a 5xx and is not covered by this event type.
func AssetClientErrorDiagnostic(err error) (stage, reason string) {
	switch {
	case errors.Is(err, ErrInvalidAssetRequest):
		return "request_validation", "invalid_request"
	case errors.Is(err, ErrAssetURLRequired):
		return "source_validation", "source_url_required"
	case errors.Is(err, ErrAssetURLTTLInsufficient):
		return "source_validation", "url_ttl_insufficient"
	case errors.Is(err, ErrUnsafeAssetURL):
		return "source_validation", "unsafe_url"
	case errors.Is(err, ErrReservedAssetGroupName):
		return "request_validation", "reserved_asset_group_name"
	case errors.Is(err, ErrAssetModelNotFound):
		return "model_resolution", "model_not_found"
	case errors.Is(err, ErrAssetNotFound):
		return "upstream_operation", "resource_not_found"
	case errors.Is(err, ErrUnsupportedAssetType):
		return "capability_check", "unsupported_asset_type"
	case errors.Is(err, ErrUnsupportedAssetOperation), errors.Is(err, ErrAssetLibraryUnsupported):
		return "capability_check", "unsupported_asset_operation"
	case errors.Is(err, ErrDefaultAssetGroupNotConfigured):
		return "group_resolution", "default_asset_group_not_configured"
	default:
		return "", ""
	}
}

// AttachAssetClientError merges safe diagnostics for err into the request-scoped
// unified event. Callers may pass known context (customer model, channel,
// whitelisted detail); stage/reason default to the controlled error mapping.
// It is a safe no-op when ctx carries no event carrier.
func AttachAssetClientError(ctx context.Context, err error, report clienterrlog.Report) {
	stage, reason := AssetClientErrorDiagnostic(err)
	if stage == "" {
		return
	}
	if report.Stage == "" {
		report.Stage = stage
	}
	if report.Reason == "" {
		report.Reason = reason
	}
	clienterrlog.Attach(ctx, report)
}

// safeAssetEnumDetail limits enum-ish detail fields to validated values; invalid
// input is recorded only as the controlled category "invalid", never as-is.
func safeAssetEnumDetail(value string, valid bool) string {
	if value == "" {
		return ""
	}
	if !valid {
		return "invalid"
	}
	return value
}

// assetKindMediaDetail builds whitelisted detail fields from validated asset
// kind and media type; invalid input is recorded as the controlled "invalid"
// category instead of the raw value.
func assetKindMediaDetail(kind, mediaType string) map[string]string {
	detail := map[string]string{}
	if kind != "" {
		detail["asset_kind"] = safeAssetEnumDetail(kind, model.ValidateAssetKind(kind))
	}
	if mediaType != "" {
		detail["media_type"] = safeAssetEnumDetail(mediaType, model.ValidateAssetMediaType(mediaType))
	}
	return detail
}

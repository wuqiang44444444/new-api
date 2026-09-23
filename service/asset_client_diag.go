package service

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/model"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
)

// AssetClientErrorDiagnostic maps asset contract errors to controlled stage and
// reason codes for the unified diagnostic event. 4xx branches keep their
// original mapping; upstream availability failures get structured 5xx reasons
// so persisted events no longer settle with empty stage/reason.
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
	case errors.Is(err, ErrAssetUpstreamUnavailable):
		return "channel_resolution", "asset_upstream_unavailable"
	case errors.Is(err, ErrAssetLibraryUnavailable):
		return "capability_check", "asset_library_unavailable"
	case errors.Is(err, ErrAssetUpstreamError):
		return "upstream_operation", "asset_upstream_error"
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

// assetUpstreamReasonFromClass maps an adapter diagnostic class to a controlled
// event reason; empty input falls back to the generic upstream error reason.
func assetUpstreamReasonFromClass(class string) string {
	switch class {
	case assetadapter.AssetClassTimeout:
		return "upstream_timeout"
	case assetadapter.AssetClassConnect:
		return "upstream_connect_failed"
	case assetadapter.AssetClassReset:
		return "upstream_connection_reset"
	case assetadapter.AssetClassUpstreamHTTP:
		return "upstream_http_error"
	case assetadapter.AssetClassApplicationError:
		return "upstream_business_error"
	case assetadapter.AssetClassInvalidResponse:
		return "upstream_invalid_response"
	default:
		return "upstream_transport_error"
	}
}

// attachAssetUpstreamDiag merges one controlled asset upstream observation into
// the request-scoped unified event; it never changes the business response.
func attachAssetUpstreamDiag(ctx context.Context, stage, reason string, detail map[string]string) {
	clienterrlog.Attach(ctx, clienterrlog.Report{Stage: stage, Reason: reason, Detail: detail})
}

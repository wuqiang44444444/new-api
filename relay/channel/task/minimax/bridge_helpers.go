package minimax

import (
	"context"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"math"
	"reflect"
	"strings"

	taskdto "github.com/QuantumNous/new-api/dto"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

// hookRoot is the export root shared by typed extension artifacts. The
// minimax artifact uses the same root name because it compiles under the
// shared typed-extension contract; the protocol path separates it from
// Seedance conversions.
const hookRoot = "seedance"

// hookErrorMessage sanitizes engine hook failures: the sanitized message
// reaches client errors, the full engine error stays in logs only.
func hookErrorMessage(err error) string {
	var hookErr *pluginruntime.HookError
	if errors.As(err, &hookErr) {
		return hookErr.Message
	}
	return err.Error()
}

// requestContextFrom returns the request context for plugin hook calls,
// tolerating synthetic gin contexts without a request.
func requestContextFrom(requestContext *gin.Context) context.Context {
	if requestContext == nil || requestContext.Request == nil {
		return context.Background()
	}
	return requestContext.Request.Context()
}

// providerModelFromRelayInfo mirrors the typed-channel provider model
// resolution: the mapped upstream model wins; otherwise the request model.
func providerModelFromRelayInfo(info *relaycommon.RelayInfo, requestModel string) string {
	if info != nil && strings.TrimSpace(info.UpstreamModelName) != "" {
		return strings.TrimSpace(info.UpstreamModelName)
	}
	return strings.TrimSpace(requestModel)
}

// pluginStringField reads a required non-empty string field within bounds.
func pluginStringField(object map[string]any, name string, maxBytes int) (string, error) {
	value, exists := object[name]
	if !exists || value == nil {
		return "", fmt.Errorf("minimax plugin field %q is required", name)
	}
	text, ok := value.(string)
	if !ok || text == "" || len(text) > maxBytes {
		return "", fmt.Errorf("minimax plugin field %q is invalid", name)
	}
	return text, nil
}

// pluginNumberField normalizes a numeric hook output. Sobek exports JS
// integers as int64 and JS floats as float64.
func pluginNumberField(value any) (float64, bool) {
	switch number := value.(type) {
	case int64:
		return float64(number), true
	case int:
		return float64(number), true
	case float64:
		return number, true
	default:
		return 0, false
	}
}

// pluginDurationField re-validates the duration billing multiplier in Go:
// the plugin enforces declared ranges, but the host independently caps
// duration before it can reach quota calculation.
func pluginDurationField(parameters map[string]any) (int, error) {
	value, exists := parameters["duration"]
	if !exists || value == nil {
		return 0, fmt.Errorf("minimax plugin field %q is required", "duration")
	}
	number, ok := pluginNumberField(value)
	if !ok || number != math.Trunc(number) {
		return 0, fmt.Errorf("minimax plugin field %q must be an integer", "duration")
	}
	if number <= 0 || number > relaycommon.MaxTaskDurationSeconds {
		return 0, fmt.Errorf("minimax plugin field %q exceeds the host duration bound", "duration")
	}
	return int(number), nil
}

// effectiveRequestDuration resolves the duration the request is priced
// against: the explicit request value or the declared default from the
// pinned metadata. It is the host-side authority for the billing multiplier.
func effectiveRequestDuration(contract *taskdto.ModelArkVideoCreateRequest, metadata *pluginruntime.SeedanceVideoModelMetadata) (int, error) {
	if contract.Duration != nil {
		if *contract.Duration <= 0 || *contract.Duration > relaycommon.MaxTaskDurationSeconds {
			return 0, relaycommon.NewVideoContractError("invalid_video_parameter", "duration is out of range")
		}
		return *contract.Duration, nil
	}
	if metadata != nil && metadata.DefaultDuration > 0 {
		return metadata.DefaultDuration, nil
	}
	return 0, relaycommon.NewVideoContractError("invalid_video_parameter", "duration is required for this model")
}

// enumValuePreserved verifies the explicit client value or the declared
// first-value default, allowing only case conversion. Spelling conversion stays plugin-owned; the host
// verifies scope preservation, not JD spellings.
func enumValuePreserved(metadata *pluginruntime.SeedanceVideoModelMetadata, explicit *string, wire string, extract func(*pluginruntime.SeedanceVideoModelMetadata) []string) bool {
	wire = strings.TrimSpace(wire)
	if explicit != nil && strings.TrimSpace(*explicit) != "" {
		return strings.EqualFold(strings.TrimSpace(*explicit), wire)
	}
	if metadata == nil {
		return false
	}
	values := extract(metadata)
	return len(values) > 0 && strings.EqualFold(values[0], wire)
}

// validateWireContent enforces lossless conversion of the entire prepared
// standard content array, including order, roles and media URLs.
func validateWireContent(value any, contract *taskdto.ModelArkVideoCreateRequest) error {
	encoded, err := common.Marshal(contract.Content)
	if err != nil {
		return err
	}
	var expected any
	if err := common.Unmarshal(encoded, &expected); err != nil {
		return err
	}
	if !reflect.DeepEqual(value, expected) {
		return fmt.Errorf("minimax plugin content does not preserve the request")
	}
	return nil
}

// failInvalidParameter converts declared-scope violations into the northbound
// invalid_request contract error.
func failInvalidParameter(message string) error {
	return relaycommon.NewVideoContractError("invalid_request", message)
}

// declaredProbeEnum resolves a probe enum fact: the explicit request value or
// first declared enum value, which is the MiniMax artifact default. This also
// preserves the original single-value artifact contract.
func declaredProbeEnum(metadata *pluginruntime.SeedanceVideoModelMetadata, explicit *string, extract func(*pluginruntime.SeedanceVideoModelMetadata) []string) string {
	if explicit != nil && strings.TrimSpace(*explicit) != "" {
		return strings.TrimSpace(*explicit)
	}
	if metadata == nil {
		return ""
	}
	values := extract(metadata)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

// joinProviderURL joins the configured absolute base URL with a
// code-registered path.
func joinProviderURL(baseURL, path string) string {
	return strings.TrimRight(baseURL, "/") + path
}

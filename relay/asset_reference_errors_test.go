package relay

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAssetResolveTaskError 锁定 resolveAssetReferencesForAttempt 错误哨兵到文档 §14 HTTP 合同的映射，
// 并验证 service 层 %w 包装能被 errors.Is 正确穿透识别。
func TestAssetResolveTaskError(t *testing.T) {
	cases := []struct {
		name     string
		sentinel error
		code     string
		status   int
	}{
		{"invalid reference", service.ErrInvalidAssetRequest, "invalid_asset_reference", http.StatusBadRequest},
		{"library unavailable", service.ErrAssetLibraryUnavailable, "asset_upstream_unavailable", http.StatusServiceUnavailable},
		{"asset not found", service.ErrAssetNotFound, "asset_not_found", http.StatusNotFound},
		{"asset not ready", service.ErrAssetNotReady, "asset_not_ready", http.StatusConflict},
		{"channel mismatch", service.ErrAssetChannelMismatch, "asset_channel_mismatch", http.StatusConflict},
		{"scope conflict", service.ErrAssetScopeConflict, "asset_scope_conflict", http.StatusConflict},
		{"reference unresolvable", service.ErrAssetReferenceUnresolvable, "asset_reference_unresolvable", http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("%w: detail for logging", tc.sentinel)
			require.True(t, errors.Is(wrapped, tc.sentinel), "sentinel must survive wrapping")
			code, status := assetResolveTaskError(wrapped)
			assert.Equal(t, tc.code, code)
			assert.Equal(t, tc.status, status)
		})
	}

	t.Run("unknown error falls back to 500", func(t *testing.T) {
		code, status := assetResolveTaskError(errors.New("db connection lost"))
		assert.Equal(t, "asset_resolve_failed", code)
		assert.Equal(t, http.StatusInternalServerError, status)
	})
}

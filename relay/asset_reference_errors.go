package relay

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/service"
)

// assetResolveTaskError 将 resolveAssetReferencesForAttempt 返回的错误映射到素材方案文档 §14 的对外错误码与 HTTP 状态。
// resolver 用 fmt.Errorf("%w: ...", service 哨兵) 包装原始细节，这里用 errors.Is 识别；未被识别的错误（如底层 DB 故障）
// 视为内部错误返回 500，不伪装成素材竞态。映射独立成文件，避免扩写 asset_reference_resolver.go 的现有逻辑。
func assetResolveTaskError(err error) (code string, status int) {
	switch {
	case errors.Is(err, service.ErrAssetLibraryUnavailable):
		return "asset_upstream_unavailable", http.StatusServiceUnavailable
	case errors.Is(err, service.ErrAssetNotFound):
		return "asset_not_found", http.StatusNotFound
	case errors.Is(err, service.ErrAssetNotReady):
		return "asset_not_ready", http.StatusConflict
	case errors.Is(err, service.ErrRealPersonAuthorizationNotReady):
		return "real_person_authorization_not_ready", http.StatusConflict
	case errors.Is(err, service.ErrAssetCredentialChanged):
		return "asset_credential_changed", http.StatusConflict
	case errors.Is(err, service.ErrAssetBindingRequired):
		return "asset_binding_required", http.StatusConflict
	default:
		return "asset_resolve_failed", http.StatusInternalServerError
	}
}

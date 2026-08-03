package service

import "errors"

// 素材创建和引用路径的错误哨兵。service 用 fmt.Errorf("%w: ...", 哨兵)
// 包装内部细节，controller/relay 据此返回稳定的外部错误合同。
var (
	// 400 invalid_request：素材创建请求的基础字段或幂等键不合法。
	ErrInvalidAssetRequest = errors.New("asset request is invalid")
	// 400 invalid_asset_binding_request：model/target 缺失或同时提供、target 形式不合法、真人素材 model 与授权记录不一致。
	ErrAssetBindingInvalidRequest = errors.New("asset binding request is invalid")
	// 404 asset_not_found：素材不存在或不属于当前用户。binding 创建路径由 controller 提前拦截；此哨兵供改写层（resolveAssetReferencesForAttempt）在素材于首次校验后被删除时使用。
	ErrAssetNotFound = errors.New("asset not found")
	// 409 asset_not_ready：素材尚未完成上传/导入/扫描。
	ErrAssetNotReady = errors.New("asset is not ready")
	// 409 asset_binding_required：当前模型/目标没有可用的单 Key 上游渠道。
	ErrAssetBindingRequired       = errors.New("no compatible asset binding is available")
	ErrAssetReferenceUnresolvable = errors.New("asset reference has no compatible resolution path")
	ErrAssetSourceExpired         = errors.New("asset source has expired or expires too soon")
	// 409 asset_credential_changed：渠道 Key 指纹已变化。
	ErrAssetCredentialChanged = errors.New("asset channel credential has changed")
	// 409 real_person_authorization_not_ready：真人授权缺失、未同意或未完成 H5。
	ErrRealPersonAuthorizationNotReady = errors.New("real-person authorization is not ready")
	// 403 real_person_verification_rejected：真人核验已被 Provider 明确拒绝。
	ErrRealPersonVerificationRejected = errors.New("real-person verification was rejected")
	// 422 unsupported_asset_binding_target：目标未启用，或该素材不允许绑定到 JoyCreator。
	ErrUnsupportedAssetBindingTarget = errors.New("unsupported asset binding target")
	// 422 unsupported_asset_type：目标上游协议不支持该 kind/media 组合。
	ErrUnsupportedAssetType = errors.New("unsupported asset type for upstream")
	// 503 asset_upstream_unavailable：素材库或真人能力开关关闭。
	ErrAssetLibraryUnavailable  = errors.New("asset library is unavailable")
	ErrAssetURLRequired         = errors.New("remote assets require an HTTPS URL")
	ErrUnsafeAssetURL           = errors.New("asset URL is unsafe")
	ErrAssetURLTTLInsufficient  = errors.New("asset URL TTL is insufficient")
	ErrAssetUpstreamError       = errors.New("asset upstream rejected the request")
	ErrAssetUpstreamUnavailable = errors.New("asset upstream is unavailable")
	ErrIdempotencyConflict      = errors.New("idempotency key conflicts with an existing request")
)

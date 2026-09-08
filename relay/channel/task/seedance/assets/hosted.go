package assets

import "errors"

// ErrAssetResourceNotFound 标记托管素材实现内部资源不存在或归属不符。
// 北向按既有 404 asset_not_found 语义返回，不泄漏其它用户的素材存在性。
var ErrAssetResourceNotFound = errors.New("asset resource was not found")

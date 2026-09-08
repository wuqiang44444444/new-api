package assets

import (
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// CMCC reports missing assets using generic HTTP/application codes. Match only
// the single-asset operations and complete error tuples verified against it.
func cmccAssetHTTPError(method, path string, status int, body []byte) error {
	resourceID, singleAsset := strings.CutPrefix(path, "/asset/")
	if singleAsset && resourceID != "" && !strings.Contains(resourceID, "/") {
		var envelope cmccEnvelope
		if common.Unmarshal(body, &envelope) == nil && envelope.State == "ERROR" {
			missingGet := method == http.MethodGet && status == http.StatusInternalServerError &&
				envelope.ErrorCode == "C500999" && envelope.ErrorMessage == "NotFound.asset_id"
			missingDelete := method == http.MethodDelete && status == http.StatusBadRequest &&
				envelope.ErrorCode == "C400999" && envelope.ErrorMessage == "素材不存在或无权限访问"
			if missingGet || missingDelete {
				return ErrAssetResourceNotFound
			}
		}
	}
	return &upstreamHTTPError{StatusCode: status}
}

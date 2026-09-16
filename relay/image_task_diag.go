package relay

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/service"
)

// 后台图片任务执行诊断（方案 §5.4 F2/F3）：在原生执行边界把一次失败的
// 上游交互约简为最小、受限的事实证据——上游 HTTP 状态、白名单响应头中的
// 请求关联 ID、既有违规扣费政策固定标记的命中布尔。错误正文只做有界读取
// 后即弃，不持久化、不记录原始 message、prompt、媒体、认证头或签名 URL；
// Provider 任意 code/type 不作为可信分类。

// nativeImageRejectionBodyBudget bounds the one-shot rejection body read.
const nativeImageRejectionBodyBudget = 32 << 10

// nativeImageRequestIDHeaders is the whitelist of provider request-correlation
// response headers. Only these headers are read, sanitized and persisted.
var nativeImageRequestIDHeaders = []string{"X-Request-Id", "X-Ms-Request-Id", "Apim-Request-Id"}

type nativeImageRejectionEvidence struct {
	StatusCode        int
	ProviderRequestID string
	ViolationMarker   bool
}

// inspectNativeImageRejection reads the rejection body once and reduces it to
// stable facts. A read failure never changes the outcome classification.
func inspectNativeImageRejection(resp *http.Response) nativeImageRejectionEvidence {
	evidence := nativeImageRejectionEvidence{StatusCode: resp.StatusCode, ProviderRequestID: nativeImageRequestID(resp)}
	body, err := io.ReadAll(io.LimitReader(resp.Body, nativeImageRejectionBodyBudget+1))
	if err != nil || len(body) > nativeImageRejectionBodyBudget {
		return evidence
	}
	evidence.ViolationMarker = service.ImageTaskViolationFeeApplies(body, resp.StatusCode)
	return evidence
}

// nativeImageRequestID also correlates successful generation responses whose
// result parsing or delivery subsequently fails; it never consumes the body.
func nativeImageRequestID(resp *http.Response) string {
	for _, name := range nativeImageRequestIDHeaders {
		if value := clienterrlog.SanitizeLogValue(resp.Header.Get(name), 64); value != "" {
			return value
		}
	}
	return ""
}

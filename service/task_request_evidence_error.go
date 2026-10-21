package service

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// Only durable send authorization can reject a request at this boundary.
// Optional evidence storage never produces a business error.
var (
	ErrTaskRequestEvidenceStoreBusy    = errors.New("evidence store busy")
	ErrTaskRequestEvidenceWriteTimeout = errors.New("evidence write timeout")
)

type TaskRequestEvidenceError struct{ Cause error }

func (e *TaskRequestEvidenceError) Error() string {
	return "task sending permission could not be verified"
}
func (e *TaskRequestEvidenceError) Unwrap() []error {
	if e.Cause == nil {
		return []error{ErrTaskRequestEvidenceUnavailable}
	}
	return []error{ErrTaskRequestEvidenceUnavailable, e.Cause}
}

// TaskRequestEvidenceRejection 是证据拒绝的唯一公开投影：稳定 HTTP 状态、
// 公开 code 与固定客户文案。Diagnostic 是净化后的运维诊断。
type TaskRequestEvidenceRejection struct {
	Status     int
	Code       string
	Message    string
	Diagnostic string
}

// ClassifyTaskRequestEvidenceRejection 把结构化证据错误映射为公开拒绝投影。
// 这里只处理独立的 durable sending 资格拒绝，不处理可丢弃的取证故障。
func ClassifyTaskRequestEvidenceRejection(err error) (TaskRequestEvidenceRejection, bool) {
	var evidenceErr *TaskRequestEvidenceError
	if err == nil || !errors.As(err, &evidenceErr) {
		return TaskRequestEvidenceRejection{}, false
	}
	return evidenceRejection(evidenceErr), true
}

func evidenceRejection(e *TaskRequestEvidenceError) TaskRequestEvidenceRejection {
	diagnostic := "category=send_unverifiable stage=send_verify"
	if e.Cause != nil {
		diagnostic += " cause=" + common.SanitizeTaskDiagnostic(e.Cause.Error())
	}
	return TaskRequestEvidenceRejection{Status: http.StatusServiceUnavailable, Code: "evidence_unavailable", Message: "The request outcome could not be verified; check the task status before any retry", Diagnostic: diagnostic}
}

// LogTaskRequestEvidenceRejection 在公开响应或发送拒绝边界输出一次带平台
// 请求 ID 的净化诊断。
func LogTaskRequestEvidenceRejection(c *gin.Context, err error) {
	rejection, ok := ClassifyTaskRequestEvidenceRejection(err)
	if !ok {
		return
	}
	requestID := ""
	if c != nil {
		requestID = c.GetString(common.RequestIdKey)
	}
	logger.LogWarn(c, fmt.Sprintf("request evidence rejected: request_id=%s %s", requestID, rejection.Diagnostic))
}

// TaskErrorForEvidenceRejection 把证据拒绝边界错误包装为 TaskError；err
// 保存在 Error 字段，协议呈现层经 errors.As 取回结构化错误后仍走同一分类
// 映射。非证据错误按内部错误兜底，不外泄诊断。
func TaskErrorForEvidenceRejection(err error) *dto.TaskError {
	if err == nil {
		return nil
	}
	status, code, message := http.StatusInternalServerError, "internal_error", "The request could not be processed"
	if rejection, ok := ClassifyTaskRequestEvidenceRejection(err); ok {
		status, code, message = rejection.Status, rejection.Code, rejection.Message
	}
	return &dto.TaskError{
		Code:       code,
		Message:    message,
		StatusCode: status,
		LocalError: true,
		Error:      err,
	}
}

// APIErrorForTaskRequestEvidenceRejection 把证据拒绝边界错误投影为通用
// NewAPIError（音频等 OpenAI 风格入口）；响应只携带分类文案，cause 链保留
// 给日志与身份识别。
func APIErrorForTaskRequestEvidenceRejection(err error) *types.NewAPIError {
	status, code, message := http.StatusInternalServerError, "internal_error", "The request could not be processed"
	if rejection, ok := ClassifyTaskRequestEvidenceRejection(err); ok {
		status, code, message = rejection.Status, rejection.Code, rejection.Message
	}
	apiErr := types.NewErrorWithStatusCode(errors.New(message), types.ErrorCode(code), status, types.ErrOptionWithSkipRetry())
	apiErr.Err = err
	return apiErr
}

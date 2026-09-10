package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
)

// 证据采集失败的统一结构化错误与唯一公开分类映射。结构化错误只承载证据
// 子系统内部的故障事实（类别、阶段、正文来源、原因链）；客户可见的状态码、
// 公开 code 与固定文案只能来自 ClassifyTaskRequestEvidenceRejection 这一处
// 映射。内部 cause 只进入净化后的运维诊断，不直接作为客户文案。

// TaskRequestEvidenceCategory 标识证据采集失败发生的最近边界。
type TaskRequestEvidenceCategory string

const (
	TaskRequestEvidenceCategoryConfiguration   TaskRequestEvidenceCategory = "configuration"
	TaskRequestEvidenceCategoryBodyRead        TaskRequestEvidenceCategory = "body_read"
	TaskRequestEvidenceCategoryNorthCapacity   TaskRequestEvidenceCategory = "north_capacity"
	TaskRequestEvidenceCategoryStructuredParse TaskRequestEvidenceCategory = "structured_parse"
	TaskRequestEvidenceCategoryEventDatabase   TaskRequestEvidenceCategory = "event_database"
	TaskRequestEvidenceCategoryObjectStorage   TaskRequestEvidenceCategory = "object_storage"
	TaskRequestEvidenceCategoryStoreBusy       TaskRequestEvidenceCategory = "store_busy"
	TaskRequestEvidenceCategoryWriteTimeout    TaskRequestEvidenceCategory = "write_timeout"
	TaskRequestEvidenceCategoryProcessing      TaskRequestEvidenceCategory = "processing"
	// TaskRequestEvidenceCategorySendUnverifiable 表示发送前资格复核失败
	//（恢复接管、unknown 或状态读取失败）；资金处理保持既有 unknown 边界。
	TaskRequestEvidenceCategorySendUnverifiable TaskRequestEvidenceCategory = "send_unverifiable"
	TaskRequestEvidenceCategoryUnavailable      TaskRequestEvidenceCategory = "unavailable"
)

// 证据故障的发生阶段；与类别一起进入净化诊断。
const (
	TaskRequestEvidenceStageConfiguration = "configuration"
	TaskRequestEvidenceStageBodyCache     = "body_cache"
	TaskRequestEvidenceStageNorthCapacity = "north_capacity"
	TaskRequestEvidenceStageBodySnapshot  = "body_snapshot"
	TaskRequestEvidenceStageIndexCreate   = "index_create"
	TaskRequestEvidenceStageEventReserve  = "event_reserve"
	TaskRequestEvidenceStageRedaction     = "redaction"
	TaskRequestEvidenceStageObjectWrite   = "object_write"
	TaskRequestEvidenceStageEventCommit   = "event_commit"
	TaskRequestEvidenceStageSendVerify    = "send_verify"
)

// TaskRequestEvidenceBodySource 标识失败正文的归属；客户违例只能由北向
// 原始正文加明确合同证明，南向与响应正文永不归责客户。
type TaskRequestEvidenceBodySource string

const (
	TaskRequestEvidenceSourceNorth    TaskRequestEvidenceBodySource = "north"
	TaskRequestEvidenceSourceSouth    TaskRequestEvidenceBodySource = "south"
	TaskRequestEvidenceSourceResponse TaskRequestEvidenceBodySource = "response"
	TaskRequestEvidenceSourceNone     TaskRequestEvidenceBodySource = "none"
)

// 对象写入繁忙与超时按错误类型识别，不靠匹配错误文本。
var (
	ErrTaskRequestEvidenceStoreBusy    = errors.New("evidence store busy")
	ErrTaskRequestEvidenceWriteTimeout = errors.New("evidence write timeout")
)

// evidenceJSONParseError 在解析现场保留带类型的原始 cause；是否归属客户
// 由 TaskRequestEvidenceError 的来源与 JSON 合同标记决定，不由本类型决定。
type evidenceJSONParseError struct {
	cause error
}

func (e *evidenceJSONParseError) Error() string { return "structured evidence could not be parsed" }
func (e *evidenceJSONParseError) Unwrap() error { return e.cause }

// TaskRequestEvidenceError 是证据拒绝边界的结构化错误。Error() 只输出类别、
// 阶段等非敏感事实；原始 cause 只能经 Unwrap 链或净化诊断获取，避免默认
// 格式化把内部细节带进响应。Identity 保持既有
// ErrTaskRequestEvidenceUnavailable / ErrTaskRequestEvidenceBodyTooLarge
// 的错误身份，不因改造破坏不可重试识别与资金释放路径。
type TaskRequestEvidenceError struct {
	Category TaskRequestEvidenceCategory
	Stage    string
	Source   TaskRequestEvidenceBodySource
	// JSONContract 表示该正文的请求合同要求 JSON（由 Content-Type 明确
	// 声明）；嗅探 `{`/`[` 不能证明错误归属客户。
	JSONContract bool
	// Observed/Limit 仅容量错误填充，允许出现在诊断中。
	Observed int64
	Limit    int64
	// Identity 是既有拒绝边界哨兵；Cause 是主因原始 cause；Secondary 是
	// 次因原始 cause（如主因之后的事件完成更新失败）。公开类别只由
	// Category/Stage/Source 决定，不遍历 cause 链猜测。
	Identity  error
	Cause     error
	Secondary error
}

func (e *TaskRequestEvidenceError) Error() string {
	return fmt.Sprintf("task request evidence unavailable: category=%s stage=%s source=%s",
		e.Category, e.Stage, e.Source)
}

func (e *TaskRequestEvidenceError) Unwrap() []error {
	var chain []error
	for _, err := range []error{e.Identity, e.Cause, e.Secondary} {
		if err != nil {
			chain = append(chain, err)
		}
	}
	return chain
}

// evidenceUnavailableFailure 构造保持 ErrTaskRequestEvidenceUnavailable
// 身份的证据错误。
func evidenceUnavailableFailure(category TaskRequestEvidenceCategory, stage string, source TaskRequestEvidenceBodySource) *TaskRequestEvidenceError {
	return &TaskRequestEvidenceError{
		Category: category,
		Stage:    stage,
		Source:   source,
		Identity: ErrTaskRequestEvidenceUnavailable,
	}
}

// evidenceTooLargeFailure 构造容量超限错误；北向保持正文超限错误身份，
// 南向与响应容量不足按平台处理故障分类，不归责客户。
func evidenceTooLargeFailure(stage string, source TaskRequestEvidenceBodySource, observed, limit int64) *TaskRequestEvidenceError {
	category := TaskRequestEvidenceCategoryNorthCapacity
	if source != TaskRequestEvidenceSourceNorth {
		category = TaskRequestEvidenceCategoryProcessing
	}
	return &TaskRequestEvidenceError{
		Category: category,
		Stage:    stage,
		Source:   source,
		Observed: observed,
		Limit:    limit,
		Identity: ErrTaskRequestEvidenceBodyTooLarge,
	}
}

// evidenceStoreWriteFailure 按错误类型区分对象写入繁忙、超时与存储故障。
func evidenceStoreWriteFailure(putErr error, source TaskRequestEvidenceBodySource) *TaskRequestEvidenceError {
	category := TaskRequestEvidenceCategoryObjectStorage
	switch {
	case errors.Is(putErr, ErrTaskRequestEvidenceStoreBusy):
		category = TaskRequestEvidenceCategoryStoreBusy
	case errors.Is(putErr, ErrTaskRequestEvidenceWriteTimeout):
		category = TaskRequestEvidenceCategoryWriteTimeout
	}
	return evidenceUnavailableFailure(category, TaskRequestEvidenceStageObjectWrite, source).WithCause(putErr)
}

// evidenceRedactionFailure 把脱敏失败分类为结构化解析或平台处理故障；
// 两者的原始 cause 一律保留。归属判定由分类映射结合正文来源完成。
func evidenceRedactionFailure(redactErr error, source TaskRequestEvidenceBodySource, jsonContract bool) *TaskRequestEvidenceError {
	var parseErr *evidenceJSONParseError
	category := TaskRequestEvidenceCategoryProcessing
	if errors.As(redactErr, &parseErr) {
		category = TaskRequestEvidenceCategoryStructuredParse
	}
	return evidenceUnavailableFailure(category, TaskRequestEvidenceStageRedaction, source).
		WithCause(redactErr).WithJSONContract(jsonContract)
}

func (e *TaskRequestEvidenceError) WithCause(cause error) *TaskRequestEvidenceError {
	e.Cause = cause
	return e
}

func (e *TaskRequestEvidenceError) WithJSONContract(value bool) *TaskRequestEvidenceError {
	e.JSONContract = value
	return e
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
// 公开类别只由显式的 Category/Stage/Source 与已证明的客户合同违例事实决定；
// 非证据错误返回 ok=false，调用方保持既有错误处理。
func ClassifyTaskRequestEvidenceRejection(err error) (TaskRequestEvidenceRejection, bool) {
	var evidenceErr *TaskRequestEvidenceError
	if err == nil || !errors.As(err, &evidenceErr) {
		return TaskRequestEvidenceRejection{}, false
	}
	return evidenceRejection(evidenceErr), true
}

func evidenceRejection(e *TaskRequestEvidenceError) TaskRequestEvidenceRejection {
	diagnostic := evidenceDiagnostic(e)
	switch e.Category {
	case TaskRequestEvidenceCategoryConfiguration:
		return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_configuration_error",
			"The service failed to prepare request recording due to an invalid configuration; contact the administrator", diagnostic)
	case TaskRequestEvidenceCategoryBodyRead:
		// 客户正文未完整接收须与本地缓存故障区分，不能一律归责客户端。
		if common.IsRequestBodyTooLargeError(e.Cause) {
			return evidenceRejectionFields(http.StatusRequestEntityTooLarge, "request_body_too_large",
				"The request body exceeds the service limit and was not accepted", diagnostic)
		}
		if evidenceIsClientBodyStreamError(e.Cause) {
			return evidenceRejectionFields(http.StatusBadRequest, "invalid_request",
				"The request body was not fully received", diagnostic)
		}
		return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_storage_unavailable",
			"The service could not read the request for recording; try again later", diagnostic)
	case TaskRequestEvidenceCategoryNorthCapacity:
		if e.Source != TaskRequestEvidenceSourceNorth {
			return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_processing_failed",
				"The service could not process the request record", diagnostic)
		}
		return evidenceRejectionFields(http.StatusRequestEntityTooLarge, "request_body_too_large",
			fmt.Sprintf("The request body exceeds the %d byte recording limit; reduce the body or inline media size. The request was not accepted", e.Limit), diagnostic)
	case TaskRequestEvidenceCategoryStructuredParse:
		// 仅当请求合同要求 JSON、处理对象是客户原始北向正文且类型化 cause
		// 证明语法非法时，才归责客户；其余保持平台处理故障。
		if e.Source == TaskRequestEvidenceSourceNorth && e.JSONContract && evidenceCauseIsJSONSyntax(e.Cause) {
			return evidenceRejectionFields(http.StatusBadRequest, "invalid_request",
				"The request body is not valid JSON", diagnostic)
		}
		return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_processing_failed",
			"The service could not process the request record", diagnostic)
	case TaskRequestEvidenceCategoryEventDatabase:
		return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_database_unavailable",
			"The service could not save the request record; try again later", diagnostic)
	case TaskRequestEvidenceCategoryObjectStorage:
		return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_storage_unavailable",
			"Request recording storage is unavailable; try again later", diagnostic)
	case TaskRequestEvidenceCategoryStoreBusy:
		return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_store_busy",
			"The service is temporarily busy and did not accept the request; try again later", diagnostic)
	case TaskRequestEvidenceCategoryWriteTimeout:
		return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_write_timeout",
			"Saving the request record timed out; the request was not accepted", diagnostic)
	case TaskRequestEvidenceCategoryProcessing:
		return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_processing_failed",
			"The service could not process the request record", diagnostic)
	case TaskRequestEvidenceCategorySendUnverifiable:
		return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_unavailable",
			"The request outcome could not be verified; check the task status before any retry", diagnostic)
	default:
		return evidenceRejectionFields(http.StatusServiceUnavailable, "evidence_unavailable",
			"Request evidence is unavailable; contact support with the request ID", diagnostic)
	}
}

func evidenceRejectionFields(status int, code, message, diagnostic string) TaskRequestEvidenceRejection {
	return TaskRequestEvidenceRejection{Status: status, Code: code, Message: message, Diagnostic: diagnostic}
}

// evidenceCauseIsJSONSyntax 只在类型化 cause 证明 JSON 语法非法时成立；
// 有效的 JSON（如数值溢出导致的类型错误）不算语法错误。
func evidenceCauseIsJSONSyntax(cause error) bool {
	var syntaxErr *json.SyntaxError
	return errors.As(cause, &syntaxErr)
}

// evidenceIsClientBodyStreamError 只识别读取客户请求流时证明正文未完整
// 接收的错误；本地缓存故障不在此列。
func evidenceIsClientBodyStreamError(cause error) bool {
	return errors.Is(cause, io.ErrUnexpectedEOF) || errors.Is(cause, context.Canceled)
}

// evidenceDiagnostic 输出一次净化诊断：类别、阶段、来源与净化后的主因、
// 次因。容量错误附带观测字节数与上限；不包含正文与完整配置。
func evidenceDiagnostic(e *TaskRequestEvidenceError) string {
	diagnostic := fmt.Sprintf("category=%s stage=%s source=%s", e.Category, e.Stage, e.Source)
	if e.Limit > 0 {
		diagnostic += fmt.Sprintf(" observed_bytes=%d limit=%d", e.Observed, e.Limit)
	}
	if e.Cause != nil {
		diagnostic += " cause=" + common.SanitizeTaskDiagnostic(e.Cause.Error())
	}
	if e.Secondary != nil {
		diagnostic += " secondary=" + common.SanitizeTaskDiagnostic(e.Secondary.Error())
	}
	return diagnostic
}

// LogTaskRequestEvidenceRejection 在公开响应或发送拒绝边界输出一次带平台
// 请求 ID 的净化诊断；双重故障同事件分别记录主因与次因。
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

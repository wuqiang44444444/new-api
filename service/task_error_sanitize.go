package service

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func SanitizeTaskErrorText(err error) string { return sanitizeTaskErrorText(err) }

func sanitizeTaskErrorText(err error) string {
	if err == nil || err.Error() == "" {
		return "task request failed"
	}
	return common.PublicTaskErrorMessage(err.Error())
}

// taskErrorDetails preserves structured business details without HTTP diagnostic prefixes.
func taskErrorDetails(err error, fallbackCode string, status int, originModel, upstreamModel string) (string, string) {
	var detailed interface{ TaskErrorDetails() (string, string) }
	if errors.As(err, &detailed) {
		code, message := detailed.TaskErrorDetails()
		code = common.PublicTaskErrorCode(code)
		if code == "" {
			code = "upstream_rejected"
			if status >= http.StatusInternalServerError {
				code = "upstream_unavailable"
			}
		}
		return code, common.PublicTaskErrorMessageForModel(message, originModel, upstreamModel)
	}
	if status >= http.StatusInternalServerError {
		return "upstream_unavailable", "Video service is temporarily unavailable"
	}
	if err == nil || err.Error() == "" {
		return fallbackCode, "task request failed"
	}
	return fallbackCode, common.PublicTaskErrorMessageForModel(err.Error(), originModel, upstreamModel)
}

// TaskErrorWrapperForModel projects a structured rejection using the resolved
// request identities. Classification is shared with the native task wrapper.
func TaskErrorWrapperForModel(err error, code string, status int, originModel, upstreamModel string) *dto.TaskError {
	code, message := taskErrorDetails(err, code, status, originModel, upstreamModel)
	return &dto.TaskError{Code: code, Message: message, StatusCode: status, Error: errors.New(message)}
}

package service

import (
	"errors"
	"net/http"

	"github.com/QuantumNous/new-api/common"
)

func SanitizeTaskErrorText(err error) string { return sanitizeTaskErrorText(err) }

func sanitizeTaskErrorText(err error) string {
	if err == nil || err.Error() == "" {
		return "task request failed"
	}
	return common.PublicTaskErrorMessage(err.Error())
}

// taskErrorDetails preserves structured business details without HTTP diagnostic prefixes.
func taskErrorDetails(err error, fallbackCode string, status int) (string, string) {
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
		return code, common.PublicTaskErrorMessage(message)
	}
	if status >= http.StatusInternalServerError {
		return "upstream_unavailable", "Video service is temporarily unavailable"
	}
	return fallbackCode, sanitizeTaskErrorText(err)
}

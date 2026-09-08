package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// PublicFailReason also protects historical rows written before error messages
// were normalized. Do not use it to decide task state, billing or retries.
func (t *Task) PublicFailReason() string {
	if t.Status == TaskStatusSuccess {
		return ""
	}
	return t.PublicVideoErrorMessage(t.FailReason)
}

// PublicVideoErrorMessage normalizes an accepted video error or a historical
// projection without changing the task or its status/CAS semantics.
func (t *Task) PublicVideoErrorMessage(message string) string {
	return common.PublicTaskErrorMessageForModel(message, t.Properties.OriginModelName, t.Properties.UpstreamModelName)
}

// PublicVideoResultURL exposes results only for successful tasks, including
// legacy rows whose result was stored in FailReason.
func (t *Task) PublicVideoResultURL() string {
	if t.Status != TaskStatusSuccess {
		return ""
	}
	return t.GetResultURL()
}

// PublicVideoFailure uses FailReason as the message authority. A normalized
// response can supply a code only when it describes that same failed result.
func (t *Task) PublicVideoFailure() *dto.ModelArkVideoTaskError {
	result := &dto.ModelArkVideoTaskError{Code: "generation_failed", Message: t.PublicFailReason()}
	if result.Message == "" {
		result.Message = "Video generation failed"
	}
	var response struct {
		Status string `json:"status"`
		Error  struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if t.Status == TaskStatusFailure && common.Unmarshal(t.Data, &response) == nil && response.Status == "failed" && response.Error.Message != "" {
		if t.PublicVideoErrorMessage(response.Error.Message) == result.Message {
			if code := common.PublicTaskErrorCode(response.Error.Code); code != "" {
				result.Code = code
			}
		}
	}
	return result
}

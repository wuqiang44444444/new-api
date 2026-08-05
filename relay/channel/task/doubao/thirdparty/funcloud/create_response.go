package funcloud

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/QuantumNous/new-api/common"
)

type CreateError struct {
	Code int
}

func (e *CreateError) Error() string {
	return fmt.Sprintf("FunCloud video create returned application code %d", e.Code)
}

func CreateResponse(body []byte) ([]byte, error) {
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			TaskID string `json:"taskId"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := common.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("invalid FunCloud create response")
	}
	taskID := strings.TrimSpace(envelope.Data.TaskID)
	if envelope.Code != 0 {
		if taskID != "" {
			return nil, fmt.Errorf("FunCloud create response contains both an error and a task id")
		}
		// The published FunCloud documents do not bind any application code to
		// an exact HTTP status, create endpoint, and "task was not created"
		// guarantee. The shared durable-attempt path therefore treats every
		// non-zero code observed after sending as an unknown create outcome.
		return nil, &CreateError{Code: envelope.Code}
	}
	if !validTaskID(taskID) {
		return nil, fmt.Errorf("FunCloud create response contains an invalid task id")
	}
	if strings.ToLower(strings.TrimSpace(envelope.Data.Status)) != "processing" {
		return nil, fmt.Errorf("FunCloud create response contains an unsupported status")
	}
	return common.Marshal(struct {
		ID string `json:"id"`
	}{ID: taskID})
}

func validTaskID(value string) bool {
	if value == "" || len(value) > 191 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

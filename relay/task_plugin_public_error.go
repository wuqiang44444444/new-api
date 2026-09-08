package relay

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

// SetTaskFailure receives only a persisted business failure, never hook errors.
func (m *PluginResponsesMachine) SetTaskFailure(code, message string) {
	code = common.PublicTaskErrorCode(code)
	if code == "" {
		code = "generation_failed"
	}
	m.taskFailure = &dto.PluginResponsesError{Code: code, Message: common.PublicTaskErrorMessage(message)}
}

func (m *PluginResponsesMachine) publicTaskFailure() *dto.PluginResponsesError {
	if m.taskFailure != nil {
		return m.taskFailure
	}
	return &dto.PluginResponsesError{Code: "server_error", Message: "The task failed."}
}

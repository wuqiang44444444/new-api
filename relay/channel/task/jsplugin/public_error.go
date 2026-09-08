package jsplugin

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
)

// The plugin owns its response schema. Preserve its business error without
// assuming that its stored payload has the ModelArk status/error shape.
func publicPluginVideoError(task *model.Task, rendered, host *kitdto.OpenAIVideoError) *kitdto.OpenAIVideoError {
	if rendered == nil {
		return host
	}
	result := *rendered
	result.Code = common.PublicTaskErrorCode(result.Code)
	if result.Code == "" {
		result.Code = host.Code
	}
	result.Message = task.PublicVideoErrorMessage(result.Message)
	if result.Message == "" {
		result.Message = host.Message
	}
	return &result
}

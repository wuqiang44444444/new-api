package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/gin-gonic/gin"
)

func setTaskPluginFailure(machine *relay.PluginResponsesMachine, task *model.Task) {
	if task.Status == model.TaskStatusFailure {
		failure := task.PublicVideoFailure()
		machine.SetTaskFailure(failure.Code, failure.Message)
	}
}

func respondPublicPluginSubmissionError(c *gin.Context, taskErr *dto.TaskError) {
	if taskErr == nil {
		taskErr = &dto.TaskError{StatusCode: http.StatusInternalServerError, LocalError: true}
	}
	input := *taskErr
	if input.StatusCode < 400 || input.StatusCode > 599 {
		input.StatusCode = http.StatusInternalServerError
	}
	status, code, _, message := taskProtocolErrorFields(&input)
	if input.LocalError && status == http.StatusBadRequest {
		code = "invalid_request_error"
	}
	respondPluginProtocolError(c, status, code, message)
}

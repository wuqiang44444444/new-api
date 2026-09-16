package controller

import (
	"github.com/QuantumNous/new-api/clienterrlog"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

func appendTaskImageDiagnostics(item *dto.TaskDto, task *model.Task, viewerRole int) {
	data := task.PrivateData.ImageTask
	if !model.IsImageTask(task) || data == nil || viewerRole < common.RoleAdminUser {
		return
	}
	if data.FailureStatus != 0 || data.ViolationMarker {
		if item.AdminInfo == nil {
			item.AdminInfo = &dto.TaskAdminInfo{}
		}
		item.AdminInfo.ImageExecution = &dto.TaskImageDiagnostics{
			UpstreamStatus: data.FailureStatus, ViolationMarker: data.ViolationMarker,
		}
	}
	if viewerRole >= common.RoleRootUser && data.ProviderRequestID != "" {
		if item.RootInfo == nil {
			item.RootInfo = &dto.TaskRootInfo{}
		}
		item.RootInfo.UpstreamRequestID = clienterrlog.SanitizeLogValue(data.ProviderRequestID, 64)
	}
}

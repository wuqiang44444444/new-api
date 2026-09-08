package relay

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// projectLinkTaskPublicFields is the dashboard/generic DTO boundary. Provider
// payloads are not public task data; media delivery uses the artifact endpoints.
func projectLinkTaskPublicFields(task *model.Task, result *dto.TaskDto) *dto.TaskDto {
	if !model.IsLinkVideoTaskClientProtocol(task.ClientProtocol) {
		return result
	}
	result.Properties = struct {
		OriginModelName string `json:"origin_model_name,omitempty"`
	}{OriginModelName: task.Properties.OriginModelName}
	result.Data = nil
	result.ChannelId = 0
	result.ResultURL = ""
	return result
}

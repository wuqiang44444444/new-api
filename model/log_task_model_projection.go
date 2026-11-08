package model

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
)

// projectTaskLogModelMapping keeps the existing administrator response shape
// and removes legacy public mappings using the log's frozen route or plugin.
func projectTaskLogModelMapping(values map[string]json.RawMessage, visibility logOtherVisibility) bool {
	if visibility != logOtherVisibilityUser {
		if len(values["task_id"]) == 0 || len(values["upstream_model_name"]) != 0 {
			return false
		}
		var admin struct {
			UpstreamModel json.RawMessage `json:"upstream_model_name"`
		}
		if common.Unmarshal(values[logOtherAdminInfoKey], &admin) != nil || len(admin.UpstreamModel) == 0 {
			return false
		}
		values["upstream_model_name"] = admin.UpstreamModel
		values["is_model_mapped"] = json.RawMessage("true")
		return true
	}

	if len(values["upstream_model_name"]) == 0 && len(values["is_model_mapped"]) == 0 {
		return false
	}
	endpoint, _ := common.GetDefaultEndpointInfo(constant.EndpointTypeModelArkVideo)
	var path string
	_ = common.Unmarshal(values["request_path"], &path)
	if path != endpoint.Path {
		// Settlement rows may omit the request path but retain plugin provenance.
		var admin struct {
			TaskPlugin struct {
				Key string `json:"key"`
			} `json:"task_plugin"`
		}
		if common.Unmarshal(values[logOtherAdminInfoKey], &admin) != nil ||
			(admin.TaskPlugin.Key != jsplugin.SeedancePluginKey && admin.TaskPlugin.Key != jsplugin.MinimaxPluginKey) {
			return false
		}
	}
	delete(values, "upstream_model_name")
	delete(values, "is_model_mapped")
	return true
}

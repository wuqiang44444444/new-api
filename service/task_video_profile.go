package service

import (
	"fmt"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

func taskVideoProviderChannel(task *model.Task, current *model.Channel) (*model.Channel, error) {
	if model.TaskUsesFrozenVideoConnection(task) {
		channel, ok := model.FrozenVideoTaskChannel(task)
		if !ok {
			return nil, fmt.Errorf("frozen video connection is unavailable")
		}
		return channel, nil
	}
	if current == nil {
		return nil, fmt.Errorf("provider channel is unavailable")
	}
	return current, nil
}

// taskVideoUpstreamProfile returns the Seedance transport frozen at creation.
// Native video channels use the official profile and do not read local profile
// configuration.
func taskVideoUpstreamProfile(task *model.Task, channel *model.Channel) dto.VideoUpstreamProfile {
	if task != nil && task.PrivateData.VideoUpstreamProfile != "" {
		return task.PrivateData.VideoUpstreamProfile
	}
	return dto.VideoUpstreamProfileOfficial
}

// taskVideoUpstreamQueryBaseURL returns the upstream root address used to poll
// a task. The creation-time snapshot wins (方案 §7) so an admin editing the
// channel cannot reroute an in-flight task; historical tasks fall back to the
// channel's current base url, which itself falls back to the type default.
func taskVideoUpstreamQueryBaseURL(task *model.Task, channel *model.Channel) string {
	if task != nil && task.PrivateData.VideoUpstreamQueryBaseURL != "" {
		return task.PrivateData.VideoUpstreamQueryBaseURL
	}
	if channel == nil {
		return ""
	}
	return channel.GetBaseURL()
}

// taskVideoUpstreamQueryPath returns the query path frozen for a Seedance task.
// Native official video tasks use their built-in path.
func taskVideoUpstreamQueryPath(task *model.Task, channel *model.Channel) string {
	if task != nil && task.PrivateData.VideoUpstreamQueryPathTemplate != "" {
		return task.PrivateData.VideoUpstreamQueryPathTemplate
	}
	return ""
}

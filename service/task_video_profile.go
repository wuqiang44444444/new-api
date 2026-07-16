package service

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
)

// taskVideoUpstreamProfile returns the protocol frozen when the task was
// created. Historical tasks without a snapshot fall back to the channel's
// current profile so their pre-upgrade behavior remains queryable.
func taskVideoUpstreamProfile(task *model.Task, channel *model.Channel) dto.VideoUpstreamProfile {
	if task != nil && task.PrivateData.VideoUpstreamProfile != "" {
		return task.PrivateData.VideoUpstreamProfile
	}
	if channel == nil {
		return dto.VideoUpstreamProfileOfficial
	}
	profile := channel.GetOtherSettings().VideoUpstreamProfile
	if profile == "" {
		return dto.VideoUpstreamProfileOfficial
	}
	return profile
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

// taskVideoUpstreamQueryPath returns the query path template used to poll a
// task. The creation-time snapshot wins (方案 §7); historical tasks fall back
// to the channel's current template. Empty for the official profile, whose
// polling path is built-in.
func taskVideoUpstreamQueryPath(task *model.Task, channel *model.Channel) string {
	if task != nil && task.PrivateData.VideoUpstreamQueryPathTemplate != "" {
		return task.PrivateData.VideoUpstreamQueryPathTemplate
	}
	if channel == nil {
		return ""
	}
	return channel.GetOtherSettings().VideoUpstreamQueryPathTemplate
}

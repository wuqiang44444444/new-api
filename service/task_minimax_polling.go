package service

import (
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

// upstreamQueryScheduleInterval is the JD Cloud task API background query
// cadence recommended by the provider documentation. It is a per-protocol
// scheduling policy only: it never applies to other protocols and never
// blocks the on-demand content download path.
const upstreamQueryScheduleInterval = 15 * time.Minute

// taskPollingDeferredByUpstreamSchedule reports whether the task's protocol
// schedule defers this round. Only typed platforms with a registered cadence
// participate; everything else stays always due.
func taskPollingDeferredByUpstreamSchedule(task *model.Task, now time.Time) bool {
	if task == nil || task.Platform != constant.TaskPlatform(model.MiniMaxLinkTaskPlatform()) {
		return false
	}
	return task.PrivateData.VideoUpstreamNextQueryAt > now.Unix()
}

// scheduleTaskPollingAfterUpstreamQuery records that a provider query is
// about to run, deferring the next background round. The field persists with
// the observation write; when a round fails before persistence the next
// query simply runs sooner.
func scheduleTaskPollingAfterUpstreamQuery(task *model.Task, now time.Time) {
	if task == nil || task.Platform != constant.TaskPlatform(model.MiniMaxLinkTaskPlatform()) {
		return
	}
	task.PrivateData.VideoUpstreamNextQueryAt = now.Add(upstreamQueryScheduleInterval).Unix()
}

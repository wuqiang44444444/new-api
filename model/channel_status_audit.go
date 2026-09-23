package model

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

// Channel status change audit (docs/80-dev/2026-09-22 DB error analysis, P1-2).
// Reuses the existing audit_logs infrastructure as immutable events: one row per
// committed channel-level status transition, with before/after, auto/manual
// source, controlled trigger and actor. Audit storage is best effort: absence of
// a row is not proof that no transition occurred.

// ChannelStatusAuditSource labels who committed the transition. All current
// callers of UpdateChannelStatus are automated paths; manual operations carry
// an actor through the WithActor entry points.
const (
	ChannelStatusSourceAuto        = "auto"
	ChannelStatusSourceManual      = "manual"
	ChannelStatusSourceManualBatch = "manual_batch"
	ChannelStatusSourceManualTag   = "manual_tag"
)

const channelStatusAuditAction = "channel_status_change"

// ChannelStatusAudit contains frozen observation metadata, never provider text.
// Optional metadata preserves existing non-request callers without inventing IDs.
type ChannelStatusAudit struct {
	RequestID string
	Trigger   string
}

// recordChannelStatusTransition writes one audit row for an already committed
// channel status change. Callers must invoke it only after the status write
// succeeded and only when beforeStatus != afterStatus; it must never turn a
// call attempt into a claimed transition.
func recordChannelStatusTransition(channelID int, beforeStatus, afterStatus int, source string, actorID int, observations ...ChannelStatusAudit) {
	if beforeStatus == afterStatus {
		return
	}
	observation := ChannelStatusAudit{}
	if len(observations) > 0 {
		observation = observations[0]
	}
	reason := "manual_status_change"
	if source == ChannelStatusSourceAuto {
		reason = "automatic_status_change"
	}
	switch observation.Trigger {
	case "relay_error", "channel_test_failed", "response_time_exceeded", "channel_test_recovered":
		reason = observation.Trigger
	}
	before := channelStatusAuditName(beforeStatus)
	after := channelStatusAuditName(afterStatus)
	entry := AuditLog{
		UserId:    actorID,
		RequestId: observation.RequestID,
		Category:  AuditCategoryOperation,
		Action:    channelStatusAuditAction,
		ActorRole: 0,
		Content:   "channel #" + strconv.Itoa(channelID) + " status changed: " + before + " -> " + after + " (" + source + ")",
		Success:   true,
		Other: AuditOther{Op: &AuditOperation{
			Action: channelStatusAuditAction,
			Params: AuditFields{
				"channel_id": channelID,
				"before":     before,
				"after":      after,
				"source":     source,
				"reason":     reason,
				"actor_id":   actorID,
			},
		}},
	}
	if actorID > 0 {
		entry.ActorRole = lookupChannelAuditActorRole(actorID)
	}
	RecordAuditLog(nil, entry)
}

// lookupChannelAuditActorRole resolves the actor role for manual transitions;
// unknown roles stay root-only visible per the existing audit rule.
func lookupChannelAuditActorRole(actorID int) int {
	if actorID <= 0 {
		return 0
	}
	var role int
	err := DB.Model(&User{}).Select("role").Where("id = ?", actorID).Scan(&role).Error
	if err != nil {
		return 0
	}
	switch role {
	case common.RoleCommonUser, common.RoleAdminUser, common.RoleRootUser:
		return role
	default:
		return 0
	}
}

// channelStatusAuditName maps a channel status constant to a controlled label;
// unknown values stay queryable with their numeric form.
func channelStatusAuditName(status int) string {
	switch status {
	case common.ChannelStatusEnabled:
		return "enabled"
	case common.ChannelStatusManuallyDisabled:
		return "manually_disabled"
	case common.ChannelStatusAutoDisabled:
		return "auto_disabled"
	case common.ChannelStatusUnknown:
		return "unknown"
	default:
		return "unknown:" + strconv.Itoa(status)
	}
}

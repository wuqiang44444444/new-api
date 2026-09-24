package model

import (
	"strconv"

	"gorm.io/gorm"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
)

// SeedancePluginTaskRef / SeedancePluginAttemptRef identify execution
// dependencies on a seedance-link plugin version. They are admin-visible
// identifiers only and never carry credentials or task payloads.
type SeedancePluginTaskRef struct {
	Id     int64  `json:"id"`
	TaskId string `json:"task_id"`
	Status string `json:"status"`
}

type SeedancePluginAttemptRef struct {
	Id            int64  `json:"id"`
	Status        string `json:"status"`
	UpstreamTask  string `json:"upstream_task_id,omitempty"`
	UpstreamProto string `json:"upstream_protocol"`
}

// GetSeedancePluginExecutionUsage lists exact frozen execution references. An
// empty version includes every version. The frozen identity is authoritative;
// current protocol registration cannot remove a historical execution dependency.
func GetSeedancePluginExecutionUsage(key, version string) ([]SeedancePluginTaskRef, []SeedancePluginAttemptRef, error) {
	return seedancePluginExecutionUsage(DB, key, version)
}

func seedancePluginExecutionUsage(db *gorm.DB, key, version string) ([]SeedancePluginTaskRef, []SeedancePluginAttemptRef, error) {
	if key == "" {
		return nil, nil, nil
	}
	// Read attempts FIRST: transfer creates the Task and completes the attempt in
	// one transaction. Even under READ COMMITTED the reference cannot fall between
	// these two reads. Admission is blocked by the version lock during deletion.
	attemptRefs := make([]SeedancePluginAttemptRef, 0)
	query := db.Model(&TaskCreateAttempt{}).
		Select("id", "status", "upstream_task_id", "upstream_protocol", "recovery_snapshot", "frozen_connection_snapshot").
		Where("status IN ?", []TaskCreateAttemptStatus{TaskCreateAttemptPrepared, TaskCreateAttemptSending, TaskCreateAttemptUnknown, TaskCreateAttemptUpstreamSucceeded})
	var attempts []TaskCreateAttempt
	if err := query.Find(&attempts).Error; err != nil {
		return nil, nil, err
	}
	for i := range attempts {
		if seedanceAttemptMatchesPin(attempts[i].RecoverySnapshot, attempts[i].FrozenConnectionSnapshot, key, version) {
			attemptRefs = append(attemptRefs, SeedancePluginAttemptRef{Id: attempts[i].ID, Status: string(attempts[i].Status), UpstreamTask: attempts[i].UpstreamTaskID, UpstreamProto: attempts[i].UpstreamProtocol})
		}
	}
	taskQuery := db.Model(&Task{}).
		Select("id", "task_id", "status", "private_data").
		Where("(status NOT IN ? OR billing_state IN ? OR (status = ? AND client_deleted_at = 0))",
			TerminalTaskStatuses(),
			[]TaskBillingState{TaskBillingStatePending, TaskBillingStateFailed, TaskBillingStateDebt, TaskBillingStateAwaitingUsage}, TaskStatusSuccess)
	// The typed extension key owns its channel type's platform identity, so
	// each key only scans its own task rows.
	platforms := typedExtensionPlatformsForKey(key)
	if len(platforms) > 0 {
		taskQuery = taskQuery.Where("platform IN ?", platforms)
	}
	var tasks []Task
	if err := taskQuery.Find(&tasks).Error; err != nil {
		return nil, nil, err
	}
	// Non-success terminal tasks no longer poll. Successful visible tasks still
	// refresh through the frozen plugin on ModelArk GET, even after settlement.
	taskRefs := make([]SeedancePluginTaskRef, 0)
	for i := range tasks {
		snapshot := tasks[i].PrivateData.Execution
		if snapshot == nil || snapshot.TaskPlugin == nil || snapshot.TaskPlugin.Key != key {
			continue
		}
		if version != "" && snapshot.TaskPlugin.Version != version {
			continue
		}
		taskRefs = append(taskRefs, SeedancePluginTaskRef{Id: tasks[i].ID, TaskId: tasks[i].TaskID, Status: string(tasks[i].Status)})
	}
	return taskRefs, attemptRefs, nil
}

// seedanceFrozenConnectionPin mirrors the plugin identity fields of the
// service-side frozen connection JSON (model cannot import service).
type seedanceFrozenConnectionPin struct {
	PluginKey     string `json:"plugin_key,omitempty"`
	PluginVersion string `json:"plugin_version,omitempty"`
}

func seedanceAttemptMatchesPin(recoveryRaw, frozenRaw []byte, key, version string) bool {
	if len(recoveryRaw) > 0 {
		var snapshot taskAttemptRecoverySnapshot
		if err := common.Unmarshal(recoveryRaw, &snapshot); err == nil {
			if execution := snapshot.PrivateData.Execution; execution != nil && execution.TaskPlugin != nil &&
				execution.TaskPlugin.Key == key &&
				(version == "" || execution.TaskPlugin.Version == version) {
				return true
			}
		}
	}
	if len(frozenRaw) > 0 {
		var frozen seedanceFrozenConnectionPin
		if err := common.Unmarshal(frozenRaw, &frozen); err == nil &&
			frozen.PluginKey == key && (version == "" || frozen.PluginVersion == version) {
			return true
		}
	}
	return false
}

// SeedancePluginExecutionUsageApplies reports whether the plugin key is a
// reserved typed-extension key whose versions carry execution dependencies
// (attempt locking and deletion protection).
func SeedancePluginExecutionUsageApplies(key string) bool {
	return key != "" && len(typedExtensionPlatformsForKey(key)) > 0
}

// seedanceExtensionPluginKey is mirrored from the relay layer to avoid a
// reverse dependency; the value is a stable reserved contract constant.
const seedanceExtensionPluginKey = "seedance-link"

// typedExtensionPlatformsForKey maps a reserved extension key to the task
// platform identities (numeric channel-type strings) it can be frozen on.
func typedExtensionPlatformsForKey(key string) []string {
	switch key {
	case seedanceExtensionPluginKey:
		return []string{strconv.Itoa(constant.ChannelTypeSeedanceLink)}
	case "minimax-link":
		return []string{strconv.Itoa(constant.ChannelTypeMiniMaxLink)}
	default:
		return nil
	}
}

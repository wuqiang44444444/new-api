package system_setting

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

const TaskRequestEvidenceRetentionOptionKey = "task_request_evidence.retention_days"
const defaultTaskRequestEvidenceRetention = 2
const maxTaskRequestEvidenceRetention = 30

type TaskRequestEvidenceRetentionPolicy struct {
	Days   int    `json:"days"`
	Source string `json:"source"`
}

func ParseTaskRequestEvidenceRetentionDays(value string) (int, error) {
	days, err := strconv.Atoi(value)
	if err != nil || days < 1 || days > maxTaskRequestEvidenceRetention {
		return 0, common.NewMessage("Request evidence retention must be an integer from 1 to 30 days")
	}
	return days, nil
}

// GetTaskRequestEvidenceRetentionPolicy uses the synchronized DB option when valid.
func GetTaskRequestEvidenceRetentionPolicy() TaskRequestEvidenceRetentionPolicy {
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap[TaskRequestEvidenceRetentionOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if value != "" {
		if days, err := ParseTaskRequestEvidenceRetentionDays(value); err == nil {
			return TaskRequestEvidenceRetentionPolicy{Days: days, Source: "database"}
		}
	}
	return TaskRequestEvidenceRetentionPolicy{Days: defaultTaskRequestEvidenceRetention, Source: "default"}
}

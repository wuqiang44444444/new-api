package system_setting

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
)

const TaskRequestEvidenceRetentionOptionKey = "task_request_evidence.retention_days"
const maxTaskRequestEvidenceRetention = 7

type TaskRequestEvidenceRetentionPolicy struct {
	Days   int    `json:"days"`
	Source string `json:"source"`
}

func ParseTaskRequestEvidenceRetentionDays(value string) (int, error) {
	days, err := strconv.Atoi(value)
	if err != nil || days < 1 || days > maxTaskRequestEvidenceRetention {
		return 0, common.NewMessage("Request evidence retention must be an integer from 1 to 7 days")
	}
	return days, nil
}

// GetTaskRequestEvidenceRetentionPolicy reads the process's synchronized DB option
// before consulting the deployment fallback. Reads never query the database.
func GetTaskRequestEvidenceRetentionPolicy() TaskRequestEvidenceRetentionPolicy {
	common.OptionMapRWMutex.RLock()
	value := common.OptionMap[TaskRequestEvidenceRetentionOptionKey]
	common.OptionMapRWMutex.RUnlock()
	if value != "" {
		if days, err := ParseTaskRequestEvidenceRetentionDays(value); err == nil {
			return TaskRequestEvidenceRetentionPolicy{Days: days, Source: "database"}
		}
	}
	cfg := GetTaskRequestEvidenceConfig()
	if cfg.RetentionDays >= 1 && cfg.RetentionDays <= maxTaskRequestEvidenceRetention {
		source := "default"
		if cfg.RetentionFromEnv {
			source = "environment"
		}
		return TaskRequestEvidenceRetentionPolicy{Days: cfg.RetentionDays, Source: source}
	}
	return TaskRequestEvidenceRetentionPolicy{Days: defaultTaskRequestEvidenceRetention, Source: "default"}
}

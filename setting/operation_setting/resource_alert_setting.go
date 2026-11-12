package operation_setting

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

const ResourceAlertPrefix = "resource_alert_setting."

type ResourceAlertThresholds struct {
	CPU    int
	Memory int
	Disk   int
}

func GetResourceAlertThresholds() ResourceAlertThresholds {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	var thresholds ResourceAlertThresholds
	for key, threshold := range map[string]*int{
		"cpu": &thresholds.CPU, "memory": &thresholds.Memory, "disk": &thresholds.Disk,
	} {
		*threshold = 85
		if value, ok := common.OptionMap[ResourceAlertPrefix+key]; ok {
			if n, err := strconv.Atoi(common.Interface2String(value)); err == nil && n >= 1 && n <= 100 {
				*threshold = n
			}
		}
	}
	return thresholds
}

func ValidateResourceAlertOption(key, value string) error {
	metric := strings.TrimPrefix(key, ResourceAlertPrefix)
	n, err := strconv.Atoi(value)
	if (metric != "cpu" && metric != "memory" && metric != "disk") || err != nil || n < 1 || n > 100 {
		return errors.New("Resource alert threshold must be between 1 and 100 percent")
	}
	return nil
}

package provider_exposure_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const defaultMonitoredImplementations = "byteplus.seedance-ark/v1,moxing.seedance-media-task/v2,tokensave.seedance-media-task/v1,tokensave.seedance-media-task/v2,feicai.seedance-videos/v2,feicai.seedance-videos/v3,funcloud.seedance-json/v1,moxing.images.media-task/v1,qihang.images.openai-compatible/v1,kling.videos-official/v1,jimeng.videos-official/v1"

// PolicySetting controls the provider-cost exposure circuit breaker. Thresholds
// set to zero are disabled. The default intentionally pages and pauses the
// affected public SKU on the first exposure for the initial JSON video profile.
type PolicySetting struct {
	Enabled                       bool    `json:"enabled"`
	MonitoredImplementations      string  `json:"monitored_implementations"`
	WindowSeconds                 int64   `json:"window_seconds"`
	WarningCount                  int64   `json:"warning_count"`
	PagingCount                   int64   `json:"paging_count"`
	AutoDisableCount              int64   `json:"auto_disable_count"`
	WarningQuota                  int64   `json:"warning_quota"`
	PagingQuota                   int64   `json:"paging_quota"`
	AutoDisableQuota              int64   `json:"auto_disable_quota"`
	WarningRatePerHour            float64 `json:"warning_rate_per_hour"`
	PagingRatePerHour             float64 `json:"paging_rate_per_hour"`
	AutoDisableRatePerHour        float64 `json:"auto_disable_rate_per_hour"`
	WarningConversionRatio        float64 `json:"warning_conversion_ratio"`
	PagingConversionRatio         float64 `json:"paging_conversion_ratio"`
	AutoDisableConversionRatio    float64 `json:"auto_disable_conversion_ratio"`
	WarningOldestAgeSeconds       int64   `json:"warning_oldest_age_seconds"`
	PagingOldestAgeSeconds        int64   `json:"paging_oldest_age_seconds"`
	AutoDisableOldestAgeSeconds   int64   `json:"auto_disable_oldest_age_seconds"`
	AutoDisablePublicModelEnabled bool    `json:"auto_disable_public_model_enabled"`
}

var policySetting = PolicySetting{
	Enabled:                       true,
	MonitoredImplementations:      defaultMonitoredImplementations,
	WindowSeconds:                 3600,
	WarningCount:                  1,
	PagingCount:                   1,
	AutoDisableCount:              1,
	AutoDisablePublicModelEnabled: true,
}

func init() {
	config.GlobalConfig.Register("provider_exposure_setting", &policySetting)
}

func GetSetting() *PolicySetting {
	return &policySetting
}

func Current() PolicySetting {
	current := policySetting
	if current.WindowSeconds < 60 {
		current.WindowSeconds = 60
	} else if current.WindowSeconds > 30*24*60*60 {
		current.WindowSeconds = 30 * 24 * 60 * 60
	}
	current.WarningConversionRatio = boundedRatio(current.WarningConversionRatio)
	current.PagingConversionRatio = boundedRatio(current.PagingConversionRatio)
	current.AutoDisableConversionRatio = boundedRatio(current.AutoDisableConversionRatio)
	return current
}

func (setting PolicySetting) MonitorsImplementation(id, version string) bool {
	identity := strings.TrimSpace(id) + "/" + strings.TrimSpace(version)
	if identity == "/" {
		return false
	}
	for _, candidate := range strings.Split(setting.MonitoredImplementations, ",") {
		if strings.TrimSpace(candidate) == identity {
			return true
		}
	}
	return false
}

func (setting PolicySetting) ActiveForImplementation(id, version string) bool {
	return setting.Enabled && setting.MonitorsImplementation(id, version)
}

func boundedRatio(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

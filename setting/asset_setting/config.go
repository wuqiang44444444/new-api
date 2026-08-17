package asset_setting

import settingconfig "github.com/QuantumNous/new-api/setting/config"

type BusinessSetting struct {
	Enabled            bool  `json:"enabled"`
	RemoteURLMaxLength int64 `json:"remote_url_max_length"`
}

var businessSetting = BusinessSetting{
	Enabled:            true,
	RemoteURLMaxLength: 8192,
}

func init() {
	settingconfig.GlobalConfig.Register("asset_setting", &businessSetting)
}

type Config struct {
	Enabled            bool
	RemoteURLMaxLength int
}

func Current() Config {
	return Config{
		Enabled:            businessSetting.Enabled,
		RemoteURLMaxLength: int(boundedPositive(businessSetting.RemoteURLMaxLength, 8192, 65536)),
	}
}

func positiveOr(value, fallback int64) int64 {
	if value <= 0 {
		return fallback
	}
	return value
}

func boundedPositive(value, fallback, maximum int64) int64 {
	value = positiveOr(value, fallback)
	if value > maximum {
		return maximum
	}
	return value
}

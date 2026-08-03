package asset_setting

import (
	"net/url"
	"os"
	"strconv"
	"strings"

	settingconfig "github.com/QuantumNous/new-api/setting/config"
)

type BusinessSetting struct {
	Enabled                     bool  `json:"enabled"`
	RealPersonEnabled           bool  `json:"real_person_enabled"`
	JobMaxAttempts              int64 `json:"job_max_attempts"`
	VerificationPollMaxAttempts int64 `json:"verification_poll_max_attempts"`
	JobLeaseSeconds             int64 `json:"job_lease_seconds"`
	PollIntervalSeconds         int64 `json:"poll_interval_seconds"`
	MaxAssetsPerUser            int64 `json:"max_assets_per_user"`
	RemoteURLMaxLength          int64 `json:"remote_url_max_length"`
	CreateUnknownTTLSeconds     int64 `json:"create_unknown_ttl_seconds"`
}

var businessSetting = BusinessSetting{
	Enabled:                     true,
	JobMaxAttempts:              12,
	VerificationPollMaxAttempts: 360,
	JobLeaseSeconds:             60,
	PollIntervalSeconds:         5,
	MaxAssetsPerUser:            1000,
	RemoteURLMaxLength:          8192,
	CreateUnknownTTLSeconds:     5 * 60,
}

func init() {
	settingconfig.GlobalConfig.Register("asset_setting", &businessSetting)
}

func GetBusinessSetting() *BusinessSetting {
	return &businessSetting
}

type Config struct {
	Enabled                     bool
	RealPersonEnabled           bool
	PublicBaseURL               string
	H5AllowedHosts              []string
	JobMaxAttempts              int
	VerificationPollMaxAttempts int
	JobLeaseSeconds             int64
	PollIntervalSeconds         int64
	MaxAssetsPerUser            int64
	RemoteURLMaxLength          int
	CreateUnknownTTLSeconds     int64
}

func Current() Config {
	business := GetBusinessSetting()
	return Config{
		Enabled:                     business.Enabled,
		RealPersonEnabled:           boolEnvOr("REAL_PERSON_ASSET_ENABLED", business.RealPersonEnabled),
		PublicBaseURL:               strings.TrimRight(os.Getenv("ASSET_PUBLIC_BASE_URL"), "/"),
		H5AllowedHosts:              csvEnv("ASSET_H5_ALLOWED_HOSTS"),
		JobMaxAttempts:              int(boundedPositive(business.JobMaxAttempts, 12, 100)),
		VerificationPollMaxAttempts: int(boundedPositive(business.VerificationPollMaxAttempts, 360, 10000)),
		JobLeaseSeconds:             boundedPositive(business.JobLeaseSeconds, 60, 3600),
		PollIntervalSeconds:         boundedPositive(business.PollIntervalSeconds, 5, 300),
		MaxAssetsPerUser:            positiveOr(business.MaxAssetsPerUser, 1000),
		RemoteURLMaxLength:          int(boundedPositive(business.RemoteURLMaxLength, 8192, 65536)),
		CreateUnknownTTLSeconds:     boundedPositive(business.CreateUnknownTTLSeconds, 5*60, 24*60*60),
	}
}

func (c Config) VerificationReady() bool {
	if !c.Enabled || !c.RealPersonEnabled || !persistentCryptoSecretConfigured() {
		return false
	}
	publicURL, err := url.Parse(c.PublicBaseURL)
	return err == nil && publicURL.Scheme == "https" && publicURL.Host != ""
}

func persistentCryptoSecretConfigured() bool {
	for _, key := range []string{"CRYPTO_SECRET", "SESSION_SECRET"} {
		value := strings.TrimSpace(os.Getenv(key))
		if value != "" && value != "random_string" {
			return true
		}
	}
	return false
}

func boolEnvOr(key string, fallback bool) bool {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return v
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

func csvEnv(key string) []string {
	values := strings.Split(os.Getenv(key), ",")
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.ToLower(strings.TrimSpace(value)); value != "" {
			result = append(result, value)
		}
	}
	return result
}

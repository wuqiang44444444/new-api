package setting

import (
	"os"
	"regexp"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
)

const ArkCacheBaseURL = "https://ark.cn-beijing.volces.com"

const ArkPrefixCacheModelVersion = "doubao-seed-2-1-lite-260915"
const ArkSessionPlatformStorageMeter = "session-platform-retention-v1"

var arkCacheEndpoint = regexp.MustCompile(`^ep-[a-zA-Z0-9-]{1,96}$`)
var arkCacheMeterVersion = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)

// ArkPrefixCachePilot is an explicitly provisioned pilot binding. Credentials
// remain on the dedicated channel, outside the admission configuration.
type ArkPrefixCachePilot struct {
	Mode                string `json:"mode"`
	UserID              int    `json:"user_id"`
	TokenID             int    `json:"app_id"`
	Model               string `json:"model"`
	ChannelID           int    `json:"channel_id"`
	Endpoint            string `json:"endpoint"`
	ModelVersion        string `json:"model_version"`
	MaxInputBytes       int    `json:"max_input_bytes"`
	MaxActive           int    `json:"max_active"`
	MaxHourlyWrites     int    `json:"max_hourly_writes"`
	MaxTTLSeconds       int64  `json:"max_ttl_seconds"`
	Price               string `json:"usd_per_million_token_hour"`
	MeterVersion        string `json:"verified_meter_version"`
	MaxStoredTokens     int    `json:"max_storage_tokens"`
	MaxHeld             int    `json:"max_held_quota"`
	FundsGrace          int64  `json:"funds_grace_seconds"`
	ReuseStorage        string `json:"verified_reuse_storage"`
	LocalSessionStorage bool   `json:"local_session_storage"`
	NativeChannel       bool   `json:"-"`
}

var arkCachePilotOption = sync.OnceValue(func() string { return os.Getenv("ARK_PREFIX_CACHE_PILOT") })

var loadArkPrefixCachePilot = sync.OnceValue(func() *ArkPrefixCachePilot {
	raw := arkCachePilotOption()
	if raw == "" {
		return nil
	}
	var config ArkPrefixCachePilot
	if common.UnmarshalJsonStr(raw, &config) != nil || !config.Valid() {
		common.SysError("invalid Ark prefix cache pilot configuration")
		return nil
	}
	return &config
})

func ArkCachePilot() *ArkPrefixCachePilot { return loadArkPrefixCachePilot() }

func ArkDefaultSessionEnabled() bool { return arkCachePilotOption() == "" }

func (c ArkPrefixCachePilot) ChargesSessionStorage() bool {
	return c.Mode == "charge" && c.MeterVersion == ArkSessionPlatformStorageMeter
}

func (c ArkPrefixCachePilot) AllowsApplication(userID, tokenID int) bool {
	return userID > 0 && tokenID > 0 &&
		(c.UserID == userID && c.TokenID == tokenID || c.ChargesSessionStorage() && c.UserID == 0 && c.TokenID == 0)
}

func (c ArkPrefixCachePilot) Valid() bool {
	chargeSession := c.ChargesSessionStorage()
	validTarget := arkCacheEndpoint.MatchString(c.Endpoint) || ((c.Mode != "charge" || chargeSession) && c.Endpoint == ArkPrefixCacheModelVersion)
	validApplication := c.UserID > 0 && c.TokenID > 0 || chargeSession && c.UserID == 0 && c.TokenID == 0
	if !validApplication || c.ChannelID <= 0 || c.Model == "" || len(c.Model) > 191 ||
		!validTarget || c.ModelVersion != ArkPrefixCacheModelVersion ||
		c.MaxInputBytes <= 0 || c.MaxInputBytes > 1<<20 || c.MaxActive <= 0 || c.MaxActive > 1000 ||
		c.MaxHourlyWrites <= 0 || c.MaxHourlyWrites > 10000 ||
		c.MaxTTLSeconds <= 0 || c.MaxTTLSeconds > 7*24*3600 {
		return false
	}
	if chargeSession {
		return !c.LocalSessionStorage && c.MaxHeld > 0 && c.Price == ""
	}
	if c.Mode == "observe" || c.Mode == "disabled" {
		if !c.LocalSessionStorage {
			return true
		}
		if c.Mode != "observe" || c.MaxHeld <= 0 || !ArkCacheIsolatedLocalDatabase() {
			return false
		}
	} else if c.Mode != "charge" || c.LocalSessionStorage || !arkCacheMeterVersion.MatchString(c.MeterVersion) ||
		(c.ReuseStorage != "no_write" && c.ReuseStorage != "child_resource") ||
		c.MaxStoredTokens < 256 || c.MaxStoredTokens > 1024*1024 || c.MaxHeld <= 0 ||
		c.FundsGrace <= 0 || c.FundsGrace > 30*24*3600 {
		return false
	}
	price, err := decimal.NewFromString(c.Price)
	return err == nil && len(c.Price) <= 32 && price.IsPositive() && price.Exponent() >= -12 && price.Exponent() <= 6
}

func ArkCacheRequestBaseURL() string {
	if ArkCacheIsolatedLocalDatabase() {
		return "http://127.0.0.1:38570"
	}
	return ArkCacheBaseURL
}

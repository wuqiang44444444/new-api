//go:build !yuan_gateway_local_dev

package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArkCacheDeployedBuildKeepsObservationAndRejectsLocalMeter(t *testing.T) {
	pilot := ArkPrefixCachePilot{
		Mode: "observe", UserID: 1, TokenID: 2, ChannelID: 267, Model: "seed-lite",
		Endpoint: ArkPrefixCacheModelVersion, ModelVersion: ArkPrefixCacheModelVersion,
		MaxInputBytes: 8192, MaxActive: 8, MaxHourlyWrites: 8, MaxTTLSeconds: 1800,
	}
	require.True(t, pilot.Valid())
	pilot.LocalSessionStorage, pilot.Price, pilot.MaxHeld = true, "0.002328767123", 100
	assert.False(t, pilot.Valid())
	assert.Equal(t, ArkCacheBaseURL, ArkCacheRequestBaseURL())
}

func TestArkCacheDeployedBuildAllowsProductStorageMeter(t *testing.T) {
	pilot := ArkPrefixCachePilot{
		Mode: "charge", MeterVersion: ArkSessionPlatformStorageMeter, ChannelID: 267, Model: "seed-lite",
		Endpoint: ArkPrefixCacheModelVersion, ModelVersion: ArkPrefixCacheModelVersion,
		MaxInputBytes: 8192, MaxActive: 8, MaxHourlyWrites: 8, MaxTTLSeconds: 1800, MaxHeld: 1000,
	}
	require.True(t, pilot.Valid())
	assert.True(t, pilot.AllowsApplication(3, 5))
	assert.True(t, pilot.AllowsApplication(4, 5))
	assert.False(t, pilot.AllowsApplication(3, 0))
	pilot.Price = "0.01"
	assert.False(t, pilot.Valid())
}

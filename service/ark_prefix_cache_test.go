package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArkCacheCreateRejectionRequiresRecognizedAdmissionEvidence(t *testing.T) {
	for _, tt := range []struct {
		status int
		code   string
		known  bool
	}{
		{http.StatusBadRequest, "InvalidParameter", true},
		{http.StatusTooManyRequests, "RateLimitExceeded.EndpointRPMExceeded", true},
		{http.StatusTooManyRequests, "", false},
		{http.StatusUnprocessableEntity, "unconfirmed", false},
		{http.StatusBadGateway, "InvalidParameter", false},
		{0, "", false},
	} {
		assert.Equal(t, tt.known, ArkCacheCreateRejected(ArkCacheReply{ErrorCode: tt.code}, tt.status),
			"status=%d code=%q", tt.status, tt.code)
	}
}

func TestArkPrefixCacheRequiresDedicatedEndpointForCharge(t *testing.T) {
	pilot := ArkPrefixCachePilot{Mode: "observe", UserID: 1, TokenID: 2, ChannelID: 3, Model: "seed-lite-pilot",
		Endpoint: ArkPrefixCacheModelVersion, ModelVersion: ArkPrefixCacheModelVersion, MaxInputBytes: 16384,
		MaxActive: 4, MaxHourlyWrites: 10, MaxTTLSeconds: 600, Price: "1", MeterVersion: "verified-1",
		MaxStoredTokens: 1024, MaxHeld: 1000, FundsGrace: 3600, ReuseStorage: "no_write"}
	assert.True(t, pilot.Valid())
	pilot.Mode = "charge"
	assert.False(t, pilot.Valid())
	pilot.Endpoint = "ep-dedicated"
	assert.True(t, pilot.Valid())
}

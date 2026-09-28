package controller

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViduRequestLimitsAreCheckedBeforeHoldAndProviderPost(t *testing.T) {
	for _, tc := range []struct {
		name, provider, extra string
		accepted              bool
	}{
		{"defaults", "viduq3-drama-std", ``, true},
		{"expiry-minimum", "viduq3-drama-std", `,"execution_expires_after":3600`, true},
		{"expiry-maximum", "viduq3-drama-std", `,"execution_expires_after":259200`, true},
		{"expiry-below-minimum", "viduq3-drama-std", `,"execution_expires_after":3599`, false},
		{"expiry-above-maximum", "viduq3-drama-std", `,"execution_expires_after":259201`, false},
		{"4K-conversion", "viduq3-drama-std", `,"resolution":"4K"`, true},
		{"q31-duration", "viduq3.1-drama-std", `,"duration":30`, true},
		{"fast-4k", "viduq3-drama-fast", `,"resolution":"4k"`, false},
		{"q3-duration", "viduq3-drama-std", `,"duration":30`, false},
		{"too-short", "viduq3-drama-mini", `,"duration":3`, false},
		{"ratio", "viduq3-drama-std", `,"ratio":"bad"`, false},
		{"empty-resolution", "viduq3-drama-std", `,"resolution":""`, false},
		{"unpublished-seed", "viduq3-drama-std", `,"seed":0`, false},
		{"unpublished-camera", "viduq3-drama-std", `,"camera_fixed":false`, false},
		{"unpublished-draft", "viduq3-drama-std", `,"draft":false`, false},
		{"tool", "viduq3-drama-std", `,"tools":[{"type":"other"}]`, false},
		{"safety-id", "viduq3-drama-std", `,"safety_identifier":"` + strings.Repeat("x", 65) + `"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fx := newSeedanceFundsFixture(t)
			var channel model.Channel
			require.NoError(t, fx.db.First(&channel, fx.channelID).Error)
			mapping, err := common.Marshal(map[string]string{"customer-video": tc.provider})
			require.NoError(t, err)
			channel.ModelMapping = common.GetPointer(string(mapping))
			channel.SetOtherSettings(dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3, AssetUpstreamProtocol: dto.AssetUpstreamProtocolNone})
			require.NoError(t, fx.db.Save(&channel).Error)
			response := fx.submitCreateBody(`{"model":"customer-video","content":[{"type":"text","text":"fixture"}]` + tc.extra + `}`)
			if tc.accepted {
				decodeSeedanceFundsCreateID(t, response)
				assert.EqualValues(t, 1, fx.createCalls.Load())
				assert.Contains(t, string(fx.createRequestBody()), `"generate_audio":false`)
				assert.Contains(t, string(fx.createRequestBody()), `"ratio":"adaptive"`)
				if tc.name == "4K-conversion" {
					assert.Contains(t, string(fx.createRequestBody()), `"resolution":"4k"`)
				}
				return
			}
			assert.Contains(t, response, `"error"`)
			assert.Zero(t, fx.createCalls.Load())
			assert.Equal(t, seedanceFundsInitialQuota, fx.userQuota())
			var attempts int64
			require.NoError(t, fx.db.Model(&model.TaskCreateAttempt{}).Count(&attempts).Error)
			assert.Zero(t, attempts)
		})
	}
}

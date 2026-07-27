package doubao

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyVideoServiceTierPolicy(t *testing.T) {
	tests := []struct {
		name           string
		profile        dto.VideoUpstreamProfile
		allow          bool
		value          any
		wantPresent    bool
		wantValue      string
		wantErrorCode  string
		wantLocalError bool
	}{
		{
			name:        "official default is omitted when channel does not opt in",
			profile:     dto.VideoUpstreamProfileOfficial,
			value:       "default",
			wantPresent: false,
		},
		{
			name:        "nil is treated as omitted",
			profile:     dto.VideoUpstreamProfileOfficial,
			value:       nil,
			wantPresent: false,
		},
		{
			name:           "official flex is rejected when channel does not opt in",
			profile:        dto.VideoUpstreamProfileOfficial,
			value:          "flex",
			wantPresent:    true,
			wantValue:      "flex",
			wantErrorCode:  "unsupported_parameter",
			wantLocalError: true,
		},
		{
			name:        "official flex is preserved when channel opts in",
			profile:     dto.VideoUpstreamProfileOfficial,
			allow:       true,
			value:       " flex ",
			wantPresent: true,
			wantValue:   "flex",
		},
		{
			name:        "reverse proxy tier is preserved when channel opts in",
			profile:     dto.VideoUpstreamProfileThirdPartyReverseProxy,
			allow:       true,
			value:       "default",
			wantPresent: true,
			wantValue:   "default",
		},
		{
			name:        "relay default is omitted even when channel opts in",
			profile:     dto.VideoUpstreamProfileThirdPartyRelay,
			allow:       true,
			value:       "default",
			wantPresent: false,
		},
		{
			name:           "relay flex is rejected because relay contract has no tier field",
			profile:        dto.VideoUpstreamProfileThirdPartyRelay,
			allow:          true,
			value:          "flex",
			wantPresent:    true,
			wantValue:      "flex",
			wantErrorCode:  "unsupported_parameter",
			wantLocalError: true,
		},
		{
			name:           "non-string tier is rejected",
			profile:        dto.VideoUpstreamProfileOfficial,
			value:          1,
			wantPresent:    true,
			wantErrorCode:  "invalid_request",
			wantLocalError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := map[string]any{"service_tier": test.value}
			context := probeContext(relaycommon.TaskSubmitReq{Metadata: metadata})
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelOtherSettings: dto.ChannelOtherSettings{AllowServiceTier: test.allow},
				},
			}

			taskErr := applyVideoServiceTierPolicy(context, info, test.profile)

			if test.wantErrorCode == "" {
				require.Nil(t, taskErr)
			} else {
				require.NotNil(t, taskErr)
				assert.Equal(t, test.wantErrorCode, taskErr.Code)
				assert.Equal(t, test.wantLocalError, taskErr.LocalError)
			}
			value, present := metadata["service_tier"]
			assert.Equal(t, test.wantPresent, present)
			if test.wantValue != "" {
				assert.Equal(t, test.wantValue, value)
			}
		})
	}
}

func TestValidateRequestAppliesVideoServiceTierPolicyBeforeSubmission(t *testing.T) {
	tests := []struct {
		name          string
		tier          string
		wantErrorCode string
		wantPresent   bool
	}{
		{name: "default is normalized to omission", tier: "default"},
		{name: "flex is rejected locally", tier: "flex", wantErrorCode: "unsupported_parameter", wantPresent: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest(
				http.MethodPost,
				"/v1/video/generations",
				strings.NewReader(`{"model":"seedance-byteplus","prompt":"一只白猫晒太阳","metadata":{"service_tier":"`+test.tier+`"}}`),
			)
			context.Request.Header.Set("Content-Type", "application/json")
			defer common.CleanupBodyStorage(context)

			info := &relaycommon.RelayInfo{
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelOtherSettings: dto.ChannelOtherSettings{
						VideoUpstreamProfile: dto.VideoUpstreamProfileOfficial,
					},
				},
			}
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)

			taskErr := adaptor.ValidateRequestAndSetAction(context, info)

			if test.wantErrorCode == "" {
				require.Nil(t, taskErr)
			} else {
				require.NotNil(t, taskErr)
				assert.Equal(t, test.wantErrorCode, taskErr.Code)
				assert.True(t, taskErr.LocalError)
			}
			stored, err := relaycommon.GetTaskRequest(context)
			require.NoError(t, err)
			_, present := stored.Metadata["service_tier"]
			assert.Equal(t, test.wantPresent, present)
		})
	}
}

func TestVideoServiceTierNormalizationPreservesExplicitFalseFields(t *testing.T) {
	metadata := map[string]any{
		"service_tier":   "default",
		"generate_audio": false,
		"watermark":      false,
	}
	context := probeContext(relaycommon.TaskSubmitReq{
		Model:    "seedance-byteplus",
		Prompt:   "一只白猫晒太阳",
		Metadata: metadata,
	})
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelOtherSettings: dto.ChannelOtherSettings{},
		},
	}
	adaptor := &TaskAdaptor{profile: dto.VideoUpstreamProfileOfficial}

	require.Nil(t, applyVideoServiceTierPolicy(context, info, adaptor.profile))
	body, err := adaptor.BuildRequestBody(context, info)
	require.NoError(t, err)
	encoded, err := io.ReadAll(body)
	require.NoError(t, err)

	var upstream map[string]any
	require.NoError(t, common.Unmarshal(encoded, &upstream))
	assert.NotContains(t, upstream, "service_tier")
	assert.Equal(t, false, upstream["generate_audio"])
	assert.Equal(t, false, upstream["watermark"])
}

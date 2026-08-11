package seedance

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelArkServiceTierIsPreservedOrRejectedWithoutSilentOmission(t *testing.T) {
	tests := []struct {
		name      string
		profile   dto.VideoUpstreamProfile
		allow     bool
		tier      string
		wantError bool
		wantValue string
	}{
		{name: "default is rejected when the channel cannot send it", profile: dto.VideoUpstreamProfileOfficial, tier: "default", wantError: true, wantValue: "default"},
		{name: "default is rejected by relay even when enabled", profile: dto.VideoUpstreamProfileThirdPartyRelay, allow: true, tier: "default", wantError: true, wantValue: "default"},
		{name: "default is preserved for an equivalent channel", profile: dto.VideoUpstreamProfileOfficial, allow: true, tier: " default ", wantValue: "default"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := probeContext(relaycommon.TaskSubmitReq{})
			relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
				ContractID: dto.VideoContractModelArkV3,
				ModelArk: &dto.ModelArkVideoCreateRequest{
					Model: "seedance-byteplus", ServiceTier: common.GetPointer(test.tier),
				},
			})
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelOtherSettings: dto.ChannelOtherSettings{AllowServiceTier: test.allow},
			}}

			taskErr := applyVideoServiceTierPolicy(context, info, test.profile)
			if test.wantError {
				require.NotNil(t, taskErr)
				assert.Equal(t, "unsupported_parameter", taskErr.Code)
			} else {
				require.Nil(t, taskErr)
			}
			contract, ok := relaycommon.GetVideoContractRequest(context)
			require.True(t, ok)
			require.NotNil(t, contract.ModelArk.ServiceTier)
			assert.Equal(t, test.wantValue, *contract.ModelArk.ServiceTier)
		})
	}
}

func TestValidateRequestRejectsUnsupportedServiceTierBeforeSubmission(t *testing.T) {
	tests := []struct {
		name          string
		tier          string
		wantErrorCode string
		wantPresent   bool
	}{
		{name: "default is rejected locally", tier: "default", wantErrorCode: "unsupported_parameter", wantPresent: true},
		{name: "flex is rejected locally", tier: "flex", wantErrorCode: "unsupported_parameter", wantPresent: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := probeContext(relaycommon.TaskSubmitReq{Model: "seedance-byteplus"})
			context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", strings.NewReader(`{"model":"seedance-byteplus"}`))
			context.Request.Header.Set("Content-Type", "application/json")
			context.Set(string(constant.ContextKeyTaskPromptValidated), true)
			contract, ok := relaycommon.GetVideoContractRequest(context)
			require.True(t, ok)
			contract.ModelArk.ServiceTier = common.GetPointer(test.tier)
			relaycommon.SetVideoContractRequest(context, contract)

			info := &relaycommon.RelayInfo{
				TaskRelayInfo: &relaycommon.TaskRelayInfo{},
				ChannelMeta: &relaycommon.ChannelMeta{
					ChannelOtherSettings: dto.ChannelOtherSettings{
						VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3BytePlus,
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
			stored, ok := relaycommon.GetVideoContractRequest(context)
			require.True(t, ok)
			assert.Equal(t, test.wantPresent, stored.ModelArk.ServiceTier != nil)
		})
	}
}

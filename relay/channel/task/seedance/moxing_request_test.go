package seedance

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoxingStandardFieldsSurviveMappedSubmission(t *testing.T) {
	for _, model := range dto.MoxingVideoModelContracts {
		t.Run(model.ProviderModel, func(t *testing.T) {
			request := &dto.ModelArkVideoCreateRequest{
				Model: "customer-model", Content: contentItems(textItem("generate")),
				Duration: common.GetPointer(5), OutputFormat: common.GetPointer("mp4"),
				CameraFixed: common.GetPointer(false), Seed: common.GetPointer(0),
				ReturnLastFrame: common.GetPointer(false), Priority: common.GetPointer(0),
				CallbackURL: common.GetPointer(""), SafetyIdentifier: common.GetPointer(""),
				ExecutionExpiresAfter: common.GetPointer(3600), Draft: common.GetPointer(false),
				Tools:         common.GetPointer([]dto.ModelArkVideoTool{}),
				GenerateAudio: common.GetPointer(false), Watermark: common.GetPointer(false),
				ServiceTier: common.GetPointer("default"),
			}
			context := moxingTestContext(t, request)
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: model.ProviderModel, IsModelMapped: true,
			}}
			info.ChannelOtherSettings.AllowServiceTier = true
			require.Nil(t, applyVideoServiceTierPolicy(context, info, moxingAdaptor().profile))
			require.Nil(t, moxingAdaptor().ValidateMappedRequest(context, info))
			payload := buildMoxingOutboundBody(t, model.ProviderModel, request)
			encoded, err := common.Marshal(request)
			require.NoError(t, err)
			var expected map[string]any
			require.NoError(t, common.Unmarshal(encoded, &expected))
			expected["model"] = model.ProviderModel
			assert.Equal(t, expected, payload, "all explicit fields must reach the provider unchanged")
			assert.Equal(t, "customer-model", request.Model, "southbound mapping must not alter the client fact")
		})
	}
}

func TestMoxingLengthDefaultAndBillingAgree(t *testing.T) {
	for _, model := range dto.MoxingVideoModelContracts {
		for _, scenario := range []struct {
			name        string
			duration    *int
			frames      *int
			wantSeconds int
		}{
			{name: "omitted", wantSeconds: 5},
			{name: "explicit automatic", duration: common.GetPointer(-1), wantSeconds: model.IntelligentDurationSeconds},
			{name: "frames only", frames: common.GetPointer(121), wantSeconds: 6},
			{name: "frames precede duration", duration: common.GetPointer(5), frames: common.GetPointer(121), wantSeconds: 6},
		} {
			t.Run(model.ProviderModel+"/"+scenario.name, func(t *testing.T) {
				request := &dto.ModelArkVideoCreateRequest{Model: "customer", Content: contentItems(textItem("generate")), Duration: scenario.duration, Frames: scenario.frames}
				require.NoError(t, validateProviderModelRequest(dto.VideoUpstreamProtocolMoxingModelArkV1, model.ProviderModel, request))
				payload := buildMoxingOutboundBody(t, model.ProviderModel, request)
				if scenario.duration != nil {
					assert.Equal(t, float64(*scenario.duration), payload["duration"])
				} else if scenario.frames != nil {
					assert.NotContains(t, payload, "duration")
				} else {
					assert.Equal(t, float64(5), payload["duration"])
				}
				if scenario.frames != nil {
					assert.Equal(t, float64(*scenario.frames), payload["frames"])
				}
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model.ProviderModel}}
				probe, err := moxingAdaptor().BuildTaskBillingProbe(moxingTestContext(t, request), info)
				require.NoError(t, err)
				assert.Equal(t, scenario.wantSeconds, probe["duration_seconds"])
				assert.Equal(t, scenario.duration, request.Duration)
			})
		}
	}
}

func TestRetiredMoxingCannotBuildNewSubmission(t *testing.T) {
	adaptor := &TaskAdaptor{protocol: dto.VideoUpstreamProtocolMoxingMediaTaskV1, profile: dto.VideoUpstreamProfileThirdPartyRelay}
	_, err := adaptor.BuildRequestBody(moxingTestContext(t, providerTestRequest()), &relaycommon.RelayInfo{})
	require.ErrorContains(t, err, "retired")
}

func TestMoxingMissingDurationContractCannotBuildSubmission(t *testing.T) {
	request := &dto.ModelArkVideoCreateRequest{Model: "customer-model", Content: contentItems(textItem("generate"))}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		UpstreamModelName: "unregistered-provider-model", IsModelMapped: true,
	}}
	reader, err := moxingAdaptor().BuildRequestBody(moxingTestContext(t, request), info)
	require.Error(t, err)
	assert.Nil(t, reader, "never emit a request that silently selects the provider's default duration")
	contractErr, ok := relaycommon.AsVideoContractError(err)
	require.True(t, ok)
	assert.Equal(t, "invalid_video_parameter", contractErr.Code)
	assert.NotContains(t, err.Error(), info.UpstreamModelName)
	assert.Nil(t, request.Duration, "preserve the omitted client fact even on error")
}

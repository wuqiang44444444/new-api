package seedance

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func probeContext(request relaycommon.TaskSubmitReq) *gin.Context {
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	context.Set("task_request", request)
	payload := &dto.ModelArkVideoCreateRequest{Model: request.Model}
	if encoded, err := common.Marshal(request.Metadata); err == nil {
		_ = common.Unmarshal(encoded, payload)
	}
	if seconds, err := strconv.Atoi(request.Seconds); err == nil && seconds != 0 {
		payload.Duration = &seconds
	}
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk:   payload,
	})
	return context
}

func TestBuildTaskBillingProbeNormalizesTrustedFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name          string
		metadata      map[string]any
		seconds       string
		resolution    string
		hasVideo      bool
		duration      int
		generateAudio bool
		inputMode     string
		controlMode   string
	}{
		{
			name:       "defaults to 720p and no video",
			metadata:   map[string]any{},
			resolution: "720p", duration: 5,
			inputMode: "text", controlMode: "none",
		},
		{
			name: "normalizes case and reads non-empty video URL",
			metadata: map[string]any{
				"resolution":     " 1080P ",
				"generate_audio": true,
				"content": []any{map[string]any{
					"type":      "video_url",
					"video_url": map[string]any{"url": "https://example.com/input.mp4"},
				}},
			},
			seconds: "5", resolution: "1080p", hasVideo: true, duration: 5, generateAudio: true, inputMode: "text", controlMode: "none",
		},
		{
			name: "empty video URL cannot select video tier",
			metadata: map[string]any{
				"resolution": "4K",
				"content": []any{map[string]any{
					"type":      "video_url",
					"video_url": map[string]any{"url": "   "},
				}},
			},
			resolution: "4k", duration: 5, inputMode: "text", controlMode: "none",
		},
		{
			name: "maps reference-image pricing dimensions",
			metadata: map[string]any{
				"content": []any{map[string]any{
					"type": "image_url", "role": "reference_image",
					"image_url": map[string]any{"url": "asset://reference-1"},
				}},
			},
			resolution: "720p", duration: 5, inputMode: "multi_image", controlMode: "reference",
		},
		{
			name:       "pre-consumes intelligent duration at provider maximum",
			metadata:   map[string]any{"duration": float64(-1)},
			resolution: "720p", duration: modelArkIntelligentDurationBillingSeconds,
			inputMode: "text", controlMode: "none",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			probe, err := (&TaskAdaptor{}).BuildTaskBillingProbe(probeContext(relaycommon.TaskSubmitReq{
				Metadata: test.metadata,
				Seconds:  test.seconds,
			}), &relaycommon.RelayInfo{})

			require.NoError(t, err)
			assert.Equal(t, test.resolution, probe["resolution"])
			assert.Equal(t, test.hasVideo, probe["has_video_input"])
			assert.Equal(t, test.duration, probe["duration_seconds"])
			assert.Equal(t, test.generateAudio, probe["generate_audio"])
			assert.Equal(t, test.inputMode, probe["input_mode"])
			assert.Equal(t, test.controlMode, probe["control_mode"])
		})
	}
}

func TestBuildTaskBillingProbeRejectsInvalidResolutionAndDuration(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		seconds  string
	}{
		{name: "invalid resolution", metadata: map[string]any{"resolution": "2k"}},
		{name: "duration over shared limit", metadata: map[string]any{}, seconds: "86401"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := (&TaskAdaptor{}).BuildTaskBillingProbe(probeContext(relaycommon.TaskSubmitReq{
				Metadata: test.metadata,
				Seconds:  test.seconds,
			}), &relaycommon.RelayInfo{})
			require.Error(t, err)
		})
	}
}

func TestBuildTaskBillingProbeUsesFramesForSafePreConsume(t *testing.T) {
	context := probeContext(relaycommon.TaskSubmitReq{})
	frames := 289
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model:   "seedance-customer-model",
			Frames:  &frames,
			Content: []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
		},
	})

	probe, err := (&TaskAdaptor{}).BuildTaskBillingProbe(context, &relaycommon.RelayInfo{})
	require.NoError(t, err)
	assert.Equal(t, 13, probe["duration_seconds"])
}

func TestBuildTaskBillingProbeRejectsUnknownFeicaiModelBeforeHold(t *testing.T) {
	context := probeContext(relaycommon.TaskSubmitReq{})
	duration, resolution, ratio := 4, "720p", "9:16"
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model:      "customer-seedance-model",
			Duration:   &duration,
			Resolution: &resolution,
			Ratio:      &ratio,
			Content:    []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
		},
	})
	_, err := (&TaskAdaptor{profile: dto.VideoUpstreamProfileThirdPartyFeicaiVideos}).BuildTaskBillingProbe(
		context,
		&relaycommon.RelayInfo{
			OriginModelName: "customer-seedance-model",
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "unverified-provider-model",
			},
		},
	)
	require.ErrorContains(t, err, "selected customer model is not supported")
	assert.NotContains(t, err.Error(), "unverified-provider-model")
}

func TestBuildTaskBillingProbeRequiresSelectedFeicaiChannelMeta(t *testing.T) {
	context := probeContext(relaycommon.TaskSubmitReq{})
	duration, resolution, ratio := 4, "1080p", "16:9"
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model:      "customer-seedance-1080p",
			Duration:   &duration,
			Resolution: &resolution,
			Ratio:      &ratio,
			Content:    []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
		},
	})

	_, err := (&TaskAdaptor{profile: dto.VideoUpstreamProfileThirdPartyFeicaiVideos}).BuildTaskBillingProbe(
		context,
		&relaycommon.RelayInfo{OriginModelName: "customer-seedance-1080p"},
	)
	require.ErrorContains(t, err, "billing capability is unavailable")
}

func TestBuildTaskBillingProbeUsesPerSecondModeAndActualRatioForFeicaiProPI(t *testing.T) {
	context := probeContext(relaycommon.TaskSubmitReq{})
	duration, resolution, ratio := 15, "720p", "21:9"
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{
			Model:      "seedance-2.0-pro-pi-720p",
			Duration:   &duration,
			Resolution: &resolution,
			Ratio:      &ratio,
			Content:    []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
		},
	})
	probe, err := (&TaskAdaptor{profile: dto.VideoUpstreamProfileThirdPartyFeicaiVideos}).BuildTaskBillingProbe(
		context,
		&relaycommon.RelayInfo{
			OriginModelName: "seedance-2.0-pro-pi-720p",
			ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: feicai.ProviderModelSeedance20ProPI720P,
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, "per-second", probe["billing_mode"])
	assert.Equal(t, "21:9", probe["ratio"])
	assert.Equal(t, 1.0, probe["size_multiplier"])
	assert.NotContains(t, probe, "size")
	assert.NotContains(t, probe, "billing_size_class")
}

func TestBuildTaskBillingProbeKeepsAllFeicaiRatiosPriceNeutral(t *testing.T) {
	for _, ratio := range []string{"21:9", "16:9", "4:3", "1:1", "3:4", "9:16"} {
		t.Run(ratio, func(t *testing.T) {
			context := probeContext(relaycommon.TaskSubmitReq{})
			duration, resolution := 4, "720p"
			relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
				ContractID: dto.VideoContractModelArkV3,
				ModelArk: &dto.ModelArkVideoCreateRequest{
					Model:      "administrator-defined-client-name",
					Duration:   &duration,
					Resolution: &resolution,
					Ratio:      &ratio,
					Content:    []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("move")}},
				},
			})

			probe, err := (&TaskAdaptor{profile: dto.VideoUpstreamProfileThirdPartyFeicaiVideos}).BuildTaskBillingProbe(
				context,
				&relaycommon.RelayInfo{
					OriginModelName: "administrator-defined-client-name",
					ChannelMeta: &relaycommon.ChannelMeta{
						UpstreamModelName: feicai.ProviderModelSeedance20Mini720P,
					},
				},
			)
			require.NoError(t, err)
			assert.Equal(t, ratio, probe["ratio"])
			assert.Equal(t, 1.0, probe["size_multiplier"])
			assert.NotContains(t, probe, "size")
		})
	}
}

func TestOfficialPriceTableRemainsIsolatedFromExternalModels(t *testing.T) {
	ratio, configured := GetVideoInputRatio("doubao-seedance-2-0-260128", "1080p", true)
	require.True(t, configured)
	assert.InDelta(t, 31.0/46.0, ratio, 0.0000001)

	_, configured = GetVideoInputRatio("seedance-2-0-oversea-key", "1080p", true)
	assert.False(t, configured)
}

func TestTokenSaveDoubaoRelayDoesNotApplyOfficialTokenPriceRatio(t *testing.T) {
	adaptor := &TaskAdaptor{profile: dto.VideoUpstreamProfileThirdPartyRelay}
	info := &relaycommon.RelayInfo{OriginModelName: "doubao-seedance-2-0-260128"}

	ratios := adaptor.EstimateBilling(probeContext(relaycommon.TaskSubmitReq{
		Metadata: map[string]any{"resolution": "1080p"},
	}), info)

	assert.Nil(t, ratios)
}

func TestMoxing25BillingProbeUsesThirtySecondAudioDefault(t *testing.T) {
	context := probeContext(relaycommon.TaskSubmitReq{Metadata: map[string]any{"duration": float64(-1)}})
	probe, err := (&TaskAdaptor{protocol: dto.VideoUpstreamProtocolMoxingModelArkV1}).BuildTaskBillingProbe(
		context,
		&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: modelSeedance25}},
	)

	require.NoError(t, err)
	assert.Equal(t, 30, probe["duration_seconds"])
	assert.Equal(t, true, probe["generate_audio"])
}

package seedance

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func providerTestRequest() *dto.ModelArkVideoCreateRequest {
	return &dto.ModelArkVideoCreateRequest{
		Model:      modelSeedance20,
		Content:    []dto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("generate")}},
		Duration:   common.GetPointer(5),
		Resolution: common.GetPointer("720p"),
	}
}

func TestProviderModelSpecsEnforcePerModelContracts(t *testing.T) {
	tests := []struct {
		name     string
		protocol kitdto.VideoUpstreamProtocol
		model    string
		mutate   func(*dto.ModelArkVideoCreateRequest)
		wantErr  bool
	}{
		{
			name: "TokenSave 2.0 accepts 1080p", protocol: kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1, model: modelSeedance20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Resolution = common.GetPointer("1080p") },
		},
		{
			name: "TokenSave retains its callback boundary", protocol: kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1, model: modelSeedance20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.CallbackURL = common.GetPointer("https://example.com/callback")
			}, wantErr: true,
		},
		{
			name: "TokenSave 2.0 accepts video input before submission", protocol: kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1, model: modelSeedance20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Content = []dto.ModelArkVideoContent{{
					Type: "video_url", Role: common.GetPointer("reference_video"),
					VideoURL: &dto.VideoMediaURL{URL: "https://example.com/video.mp4"},
				}}
			},
		},
		{
			name: "TokenSave 2.0 accepts audio input before submission", protocol: kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1, model: modelSeedance20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Content = []dto.ModelArkVideoContent{{
					Type: "audio_url", Role: common.GetPointer("reference_audio"),
					AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"},
				}}
			},
		},
		{
			name: "Moxing 2.0 accepts documented 1080p", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: "doubao-seedance-2-0-260128-0818",
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Resolution = common.GetPointer("1080p") },
		},
		{
			name: "Moxing 2.0 accepts seed", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: "doubao-seedance-2-0-260128-0818",
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Seed = common.GetPointer(24) },
		},
		{
			name: "Moxing 2.0 accepts explicit false camera fixed", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: "doubao-seedance-2-0-260128-0818",
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.CameraFixed = common.GetPointer(false) },
		},
		{
			name: "Moxing 0818 does not inherit the old audio pairing rule", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: "doubao-seedance-2-0-260128-0818",
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Content = []dto.ModelArkVideoContent{{Type: "audio_url", Role: common.GetPointer("reference_audio"), AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"}}}
			},
		},
		{
			name: "Moxing 2.0 accepts audio paired with a reference video", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: "doubao-seedance-2-0-260128-0818",
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Content = []dto.ModelArkVideoContent{
					{Type: "video_url", Role: common.GetPointer("reference_video"), VideoURL: &dto.VideoMediaURL{URL: "https://example.com/video.mp4"}},
					{Type: "audio_url", Role: common.GetPointer("reference_audio"), AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"}},
				}
			},
		},
		{
			name: "Moxing Fast rejects duration 16", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance20Fast,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Duration = common.GetPointer(16) }, wantErr: true,
		},
		{
			name: "Moxing Fast accepts seed", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance20Fast,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Seed = common.GetPointer(24) },
		},
		{
			name: "Moxing Mini accepts audio only", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance20Mini,
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Content = []dto.ModelArkVideoContent{{Type: "audio_url", Role: common.GetPointer("reference_audio"), AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"}}}
			},
		},
		{
			name: "Moxing 2.5 accepts audio only and mov", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance25,
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Duration = common.GetPointer(30)
				request.OutputFormat = common.GetPointer("mov")
				request.Content = []dto.ModelArkVideoContent{{Type: "audio_url", Role: common.GetPointer("reference_audio"), AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"}}}
			},
		},
		{
			name: "Moxing 2.5 accepts seed", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance25,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Seed = common.GetPointer(24) },
		},
		{
			name: "Moxing 2.5 accepts explicit false camera fixed", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance25,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.CameraFixed = common.GetPointer(false) },
		},
		{
			name: "Moxing Fast rejects unpublished output format", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance20Fast,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.OutputFormat = common.GetPointer("mp4") }, wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := providerTestRequest()
			test.mutate(request)
			err := validateProviderModelRequest(test.protocol, test.model, request)
			if test.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestProviderModelValidationErrorsDoNotExposeProviderIdentity(t *testing.T) {
	providerModel := "private-provider-model"
	protocol := kitdto.VideoUpstreamProtocolFunCloudModelArkV3
	err := validateProviderModelRequest(protocol, providerModel, providerTestRequest())

	require.Error(t, err)
	assert.NotContains(t, err.Error(), providerModel)
	assert.NotContains(t, err.Error(), string(protocol))
}

func TestMoxingModelArkBillingDefaultsFollowModelMaximum(t *testing.T) {
	duration, generateAudio, ok := providerBillingDefaults(kitdto.VideoUpstreamProtocolMoxingModelArkV1, modelSeedance25)
	require.True(t, ok)
	assert.Equal(t, 30, duration)
	assert.True(t, generateAudio)

	duration, generateAudio, ok = providerBillingDefaults(kitdto.VideoUpstreamProtocolMoxingModelArkV1, modelSeedance20Mini)
	require.True(t, ok)
	assert.Equal(t, 15, duration)
	assert.True(t, generateAudio)
}

func TestMoxingModelArkRequestPreservesTypedOutputFormat(t *testing.T) {
	context := probeContext(relaycommon.TaskSubmitReq{
		Model: "doubao-seedance-2-5-260628-moxing",
		Metadata: map[string]any{
			"content":  []any{map[string]any{"type": "text", "text": "extend"}},
			"duration": 30, "resolution": "720p", "output_format": "mov",
		},
	})
	pinSeedanceExtensionForTest(t, context)
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		UpstreamModelName: modelSeedance25,
		IsModelMapped:     true,
	}}
	reader, err := (&TaskAdaptor{
		protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, baseURL: "https://provider.example",
		profile: kitdto.VideoUpstreamProfileThirdPartyMoxingModelArk,
	}).BuildRequestBody(context, info)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.DecodeJson(reader, &payload))
	assert.Equal(t, modelSeedance25, payload["model"])
	assert.Equal(t, "mov", payload["output_format"])
	assert.Equal(t, true, payload["generate_audio"])
}

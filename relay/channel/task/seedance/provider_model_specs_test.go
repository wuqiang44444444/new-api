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
			name: "TokenSave 2.0 rejects video input before submission", protocol: kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1, model: modelSeedance20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Content = []dto.ModelArkVideoContent{{
					Type: "video_url", Role: common.GetPointer("reference_video"),
					VideoURL: &dto.VideoMediaURL{URL: "https://example.com/video.mp4"},
				}}
			}, wantErr: true,
		},
		{
			name: "TokenSave 2.0 rejects audio input before submission", protocol: kitdto.VideoUpstreamProtocolTokenSaveMediaTaskV1, model: modelSeedance20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Content = []dto.ModelArkVideoContent{{
					Type: "audio_url", Role: common.GetPointer("reference_audio"),
					AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"},
				}}
			}, wantErr: true,
		},
		{
			name: "Moxing 2.0 rejects 1080p", protocol: kitdto.VideoUpstreamProtocolMoxingMediaTaskV1, model: modelSeedance20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Resolution = common.GetPointer("1080p") }, wantErr: true,
		},
		{
			name: "Moxing 2.0 rejects seed", protocol: kitdto.VideoUpstreamProtocolMoxingMediaTaskV1, model: modelSeedance20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Seed = common.GetPointer(24) }, wantErr: true,
		},
		{
			name: "Moxing 2.0 rejects explicit false camera fixed", protocol: kitdto.VideoUpstreamProtocolMoxingMediaTaskV1, model: modelSeedance20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.CameraFixed = common.GetPointer(false) }, wantErr: true,
		},
		{
			name: "Moxing Fast rejects duration 16", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance20Fast,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Duration = common.GetPointer(16) }, wantErr: true,
		},
		{
			name: "Moxing Fast rejects seed", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance20Fast,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Seed = common.GetPointer(24) }, wantErr: true,
		},
		{
			name: "Moxing Mini rejects audio only", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance20Mini,
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Content = []dto.ModelArkVideoContent{{Type: "audio_url", Role: common.GetPointer("reference_audio"), AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"}}}
			}, wantErr: true,
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
			name: "Moxing 2.5 rejects explicit false camera fixed", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance25,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.CameraFixed = common.GetPointer(false) }, wantErr: true,
		},
		{
			name: "Moxing Fast rejects output format", protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, model: modelSeedance20Fast,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.OutputFormat = common.GetPointer("mp4") }, wantErr: true,
		},
		{
			name: "FunCloud standard accepts 1080p", protocol: kitdto.VideoUpstreamProtocolFunCloudSeedance, model: modelFunCloud20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Resolution = common.GetPointer("1080p") },
		},
		{
			name: "FunCloud fast rejects 1080p", protocol: kitdto.VideoUpstreamProtocolFunCloudSeedance, model: modelFunCloud20Fast,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Resolution = common.GetPointer("1080p") }, wantErr: true,
		},
		{
			name: "FunCloud mini follows fast limits", protocol: kitdto.VideoUpstreamProtocolFunCloudSeedance, model: modelFunCloud20Mini,
			mutate: func(request *dto.ModelArkVideoCreateRequest) {
				request.Content = []dto.ModelArkVideoContent{
					{Type: "image_url", ImageURL: &dto.VideoMediaURL{URL: "https://example.com/1.png"}},
					{Type: "image_url", ImageURL: &dto.VideoMediaURL{URL: "https://example.com/2.png"}},
					{Type: "image_url", ImageURL: &dto.VideoMediaURL{URL: "https://example.com/3.png"}},
				}
			},
		},
		{
			name: "FunCloud 2.5 accepts documented maximum duration", protocol: kitdto.VideoUpstreamProtocolFunCloudSeedance, model: modelFunCloud25,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Duration = common.GetPointer(30) },
		},
		{
			name: "FunCloud standard rejects intelligent duration", protocol: kitdto.VideoUpstreamProtocolFunCloudSeedance, model: modelFunCloud20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Duration = common.GetPointer(-1) }, wantErr: true,
		},
		{
			name: "FunCloud fast rejects intelligent duration", protocol: kitdto.VideoUpstreamProtocolFunCloudSeedance, model: modelFunCloud20Fast,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Duration = common.GetPointer(-1) }, wantErr: true,
		},
		{
			name: "FunCloud mini rejects intelligent duration", protocol: kitdto.VideoUpstreamProtocolFunCloudSeedance, model: modelFunCloud20Mini,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Duration = common.GetPointer(-1) }, wantErr: true,
		},
		{
			name: "FunCloud 2.5 accepts intelligent duration", protocol: kitdto.VideoUpstreamProtocolFunCloudSeedance, model: modelFunCloud25,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Duration = common.GetPointer(-1) },
		},
		{
			name: "FunCloud rejects unsupported provider private fields", protocol: kitdto.VideoUpstreamProtocolFunCloudSeedance, model: modelFunCloud20,
			mutate: func(request *dto.ModelArkVideoCreateRequest) { request.Priority = common.GetPointer(1) }, wantErr: true,
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
	protocol := kitdto.VideoUpstreamProtocolFunCloudSeedance
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
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		UpstreamModelName: modelSeedance25,
		IsModelMapped:     true,
	}}
	reader, err := (&TaskAdaptor{
		protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1,
		profile:  kitdto.VideoUpstreamProfileThirdPartyMoxingModelArk,
	}).BuildRequestBody(context, info)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, common.DecodeJson(reader, &payload))
	assert.Equal(t, modelSeedance25, payload["model"])
	assert.Equal(t, "mov", payload["output_format"])
	assert.Equal(t, true, payload["generate_audio"])
}

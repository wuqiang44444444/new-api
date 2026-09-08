package seedance

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/publicmodel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunCloudModelArkMappedRequestAndWireContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, providerModel := range kitdto.FunCloudModelArkModels() {
		for _, count := range []int{4, 9, 10} {
			t.Run(fmt.Sprintf("%s/%d", providerModel, count), func(t *testing.T) {
				request := providerTestRequest()
				request.Model = "isolated-customer"
				request.Duration = common.GetPointer(4)
				request.GenerateAudio = common.GetPointer(false)
				request.Seed = common.GetPointer(0)
				urls := make([]string, count)
				for i := range urls {
					urls[i] = fmt.Sprintf("https://example.com/reference-%d.jpg", i)
					if i == 0 {
						urls[i] = "asset://opaque-existing-asset"
					}
					request.Content = append(request.Content, dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: urls[i]}})
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
				relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: request})
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, IsModelMapped: true, UpstreamModelName: providerModel, ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3}}}
				adaptor := &TaskAdaptor{}
				adaptor.Init(info)
				if count == 10 {
					require.NotNil(t, adaptor.ValidateMappedRequest(c, info))
					return
				}
				require.Nil(t, adaptor.ValidateMappedRequest(c, info))
				body, err := adaptor.BuildRequestBody(c, info)
				require.NoError(t, err)
				data, err := io.ReadAll(body)
				require.NoError(t, err)
				var actual map[string]any
				require.NoError(t, common.Unmarshal(data, &actual))
				assert.Equal(t, providerModel, actual["model"])
				assert.Equal(t, false, actual["generate_audio"])
				assert.Equal(t, true, actual["real_person_mode"])
				assert.Equal(t, float64(0), actual["seed"])
				assert.Equal(t, float64(4), actual["duration"])
				content := actual["content"].([]any)
				require.Len(t, content, count+1)
				for i, url := range urls {
					item := content[i+1].(map[string]any)
					assert.Equal(t, url, item["image_url"].(map[string]any)["url"])
					assert.Equal(t, "reference_image", item["role"])
				}
				assert.Equal(t, "isolated-customer", request.Model)
				probe, err := adaptor.BuildTaskBillingProbe(c, info)
				require.NoError(t, err)
				assert.Equal(t, 4, probe["duration_seconds"])
				assert.Equal(t, false, probe["generate_audio"])
				assert.Equal(t, "per-second", probe["billing_mode"])
				api, ok := publicmodel.VideoAPI(request.Model, dto.VideoUpstreamProtocolFunCloudModelArkV3, providerModel, false)
				require.True(t, ok)
				assert.Equal(t, 9, api.Creation.ContentTypes[1].MaxItems)
				for _, op := range api.Operations {
					if op.Operation == "delete_video" {
						assert.False(t, op.Supported)
					}
				}
			})
		}
	}
}

func TestFunCloudModelArkDefaultsAndReferenceVideo(t *testing.T) {
	payload := &requestPayload{Model: "seedance-2-5", Content: []ContentItem{{Type: "video_url", Role: "reference_video", VideoURL: &MediaURL{URL: "asset://original-video"}}}}
	raw, err := buildFunCloudModelArkRequest(nil, payload)
	require.NoError(t, err)
	var actual map[string]any
	require.NoError(t, common.Unmarshal(raw, &actual))
	assert.Equal(t, float64(5), actual["duration"])
	assert.Equal(t, "reference", actual["omni_reference_task_type"])
	assert.Equal(t, true, actual["real_person_mode"])
	assert.Nil(t, payload.Duration)
	assert.Equal(t, "", payload.Resolution)
}

func TestFunCloudModelArkModelBounds(t *testing.T) {
	for _, name := range kitdto.FunCloudModelArkModels() {
		spec, ok := kitdto.FunCloudModelArkSpec(name)
		require.True(t, ok)
		for _, duration := range []int{3, 4, spec.MaxDuration, spec.MaxDuration + 1, -1} {
			req := providerTestRequest()
			req.Duration = &duration
			err := validateProviderModelRequest(dto.VideoUpstreamProtocolFunCloudModelArkV3, name, req)
			if duration == -1 && spec.IntelligentDuration || duration >= 4 && duration <= spec.MaxDuration {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		}
		req := providerTestRequest()
		req.Resolution = common.GetPointer("1080p")
		err := validateProviderModelRequest(dto.VideoUpstreamProtocolFunCloudModelArkV3, name, req)
		if name == "seedance-2-5" {
			assert.NoError(t, err)
		} else {
			assert.Error(t, err)
		}
	}
}

func TestFunCloudModelArkFrozenVersionAndLifecycle(t *testing.T) {
	profile := dto.VideoUpstreamProfileThirdPartyFunCloudModelArkV3
	protocol := dto.VideoUpstreamProtocolFunCloudModelArkV3
	assert.Equal(t, profile, protocol.TransportProfile())
	create, query := protocol.TransportPaths("seedance-2-0")
	assert.Equal(t, "/api/v3/contents/generations/tasks", create)
	assert.Equal(t, create+"/{task_id}", query)
	frozen := relaycommon.CurrentVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile)
	_, err := relaycommon.ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, frozen)
	require.NoError(t, err)
	_, err = relaycommon.ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, dto.VideoUpstreamProfileThirdPartyFunCloudSeedance, frozen)
	require.Error(t, err)
	_, err = relaycommon.ResolveVideoSouthboundAdapterVersion(constant.ChannelTypeSeedanceLink, profile, "")
	require.Error(t, err)
	caps := (&TaskAdaptor{}).TaskLifecycleCapabilities(&model.Task{PrivateData: model.TaskPrivateData{VideoUpstreamProfile: profile}})
	assert.True(t, caps.SupportsContent)
	assert.False(t, caps.SupportsCancelQueued)
	assert.False(t, caps.SupportsDeleteTerminal)
}

func TestFunCloudModelArkReferenceMediaLimits(t *testing.T) {
	for _, name := range kitdto.FunCloudModelArkModels() {
		for _, kind := range []string{"video_url", "audio_url"} {
			for _, count := range []int{3, 4} {
				req := providerTestRequest()
				req.Content = append(req.Content, dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: "asset://image"}})
				for i := 0; i < count; i++ {
					item := dto.ModelArkVideoContent{Type: kind}
					if kind == "video_url" {
						item.Role = common.GetPointer("reference_video")
						item.VideoURL = &dto.VideoMediaURL{URL: "https://example.com/video.mp4"}
					} else {
						item.Role = common.GetPointer("reference_audio")
						item.AudioURL = &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"}
					}
					req.Content = append(req.Content, item)
				}
				err := validateProviderModelRequest(dto.VideoUpstreamProtocolFunCloudModelArkV3, name, req)
				if count == 3 {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			}
		}
	}
}

package seedance

import (
	"io"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenSaveMixedReferencesPreserveFrameRoles(t *testing.T) {
	for _, tt := range []struct {
		name                             string
		first, last, image, video, audio bool
	}{
		{name: "first with audio", first: true, audio: true},
		{name: "first with video", first: true, video: true},
		{name: "both frames with audio", first: true, last: true, audio: true},
		{name: "both frames with video", first: true, last: true, video: true},
		{name: "both frames with reference image", first: true, last: true, image: true},
		{name: "all references", first: true, last: true, image: true, video: true, audio: true},
		{name: "both frames", first: true, last: true},
		{name: "last frame only", last: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := providerTestRequest()
			for _, image := range []struct {
				present   bool
				role, url string
			}{
				{tt.first, "first_frame", "https://media.example/first.png"},
				{tt.last, "last_frame", "https://media.example/last.png"},
				{tt.image, "reference_image", "https://media.example/reference.png"},
			} {
				if image.present {
					req.Content = append(req.Content, dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer(image.role), ImageURL: &dto.VideoMediaURL{URL: image.url}})
				}
			}
			if tt.video {
				req.Content = append(req.Content, dto.ModelArkVideoContent{Type: "video_url", Role: common.GetPointer("reference_video"), VideoURL: &dto.VideoMediaURL{URL: "https://media.example/ref.mp4"}})
			}
			if tt.audio {
				req.Content = append(req.Content, dto.ModelArkVideoContent{Type: "audio_url", Role: common.GetPointer("reference_audio"), AudioURL: &dto.VideoMediaURL{URL: "https://media.example/ref.mp3"}})
			}
			c := seedancePluginTestContext(t)
			pinSeedanceExtensionForTest(t, c)
			relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: req})
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://provider.example", ChannelType: constant.ChannelTypeSeedanceLink, IsModelMapped: true, UpstreamModelName: modelSeedance20, ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolTokenSaveMediaTaskV1}}}
			pinSeedanceExtensionForTest(t, c)
			a := &TaskAdaptor{}
			a.Init(info)
			require.Nil(t, a.ValidateMappedRequest(c, info))
			reader, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			raw, err := io.ReadAll(reader)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, common.Unmarshal(raw, &wire))
			if tt.first {
				assert.Equal(t, "https://media.example/first.png", wire["image"])
			} else {
				assert.NotContains(t, wire, "image")
			}
			if tt.last {
				assert.Equal(t, "https://media.example/last.png", wire["end_image"])
			} else {
				assert.NotContains(t, wire, "end_image")
			}
			if tt.image {
				assert.Equal(t, []any{"https://media.example/reference.png"}, wire["reference_images"])
			} else {
				assert.NotContains(t, wire, "reference_images")
			}
			if tt.video {
				assert.Equal(t, []any{"https://media.example/ref.mp4"}, wire["reference_videos"])
			} else {
				assert.NotContains(t, wire, "reference_videos")
			}
			if tt.audio {
				assert.Equal(t, []any{"https://media.example/ref.mp3"}, wire["reference_audios"])
			} else {
				assert.NotContains(t, wire, "reference_audios")
			}
			inputMode, controlMode := "single_image", "end_frame"
			if tt.image || tt.video || tt.audio {
				inputMode, controlMode = "multi_image", "reference"
			}
			assert.Equal(t, inputMode, wire["input_mode"])
			assert.Equal(t, controlMode, wire["control_mode"])
			probe, err := a.BuildTaskBillingProbe(c, info)
			require.NoError(t, err)
			assert.Equal(t, inputMode, probe["input_mode"])
			assert.Equal(t, controlMode, probe["control_mode"])
			assert.Equal(t, tt.video, probe["has_video_input"])
		})
	}
}

func TestFunCloudReferenceAudioWithPriorityAndLastFrame(t *testing.T) {
	for _, providerModel := range kitdto.FunCloudModelArkModels() {
		for _, enabled := range []bool{false, true} {
			t.Run(providerModel+"/"+map[bool]string{false: "explicit zero", true: "enabled"}[enabled], func(t *testing.T) {
				req := providerTestRequest()
				req.Model = "customer-review"
				priority := 0
				if enabled {
					priority = 9
				}
				req.Priority = &priority
				req.ReturnLastFrame = &enabled
				req.Content = append(req.Content, dto.ModelArkVideoContent{Type: "audio_url", Role: common.GetPointer("reference_audio"), AudioURL: &dto.VideoMediaURL{URL: "https://media.example/ref.mp3"}})
				c := seedancePluginTestContext(t)
				relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: req})
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: "https://provider.example", ChannelType: constant.ChannelTypeSeedanceLink, IsModelMapped: true, UpstreamModelName: providerModel, ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3}}}
				pinSeedanceExtensionForTest(t, c)
				a := &TaskAdaptor{}
				a.Init(info)
				require.Nil(t, a.ValidateMappedRequest(c, info))
				reader, err := a.BuildRequestBody(c, info)
				require.NoError(t, err)
				raw, err := io.ReadAll(reader)
				require.NoError(t, err)
				var wire map[string]any
				require.NoError(t, common.Unmarshal(raw, &wire))
				assert.Equal(t, float64(priority), wire["priority"])
				assert.Equal(t, enabled, wire["return_last_frame"])
				content := wire["content"].([]any)
				require.Len(t, content, 2)
				assert.Equal(t, "https://media.example/ref.mp3", content[1].(map[string]any)["audio_url"].(map[string]any)["url"])
				probe, err := a.BuildTaskBillingProbe(c, info)
				require.NoError(t, err)
				assert.Equal(t, 5, probe["duration_seconds"])
				assert.Equal(t, false, probe["has_video_input"])
				api, ok := publishedVideoFixture(req.Model, dto.VideoUpstreamProtocolFunCloudModelArkV3, providerModel, false)
				require.True(t, ok)
				names := map[string]int{}
				for _, p := range api.Creation.Parameters {
					names[p.Name]++
				}
				assert.Equal(t, 1, names["priority"])
				assert.Equal(t, 1, names["return_last_frame"])
			})
		}
	}
}

func TestFunCloudLastFrameSurvivesPublicProjection(t *testing.T) {
	raw, err := thirdparty.FunCloudModelArkTaskResponse([]byte(`{"id":"provider-task","status":"succeeded","content":{"video_url":"https://media.example/video.mp4","last_frame_url":"https://media.example/last.png"}}`), "provider-task")
	require.NoError(t, err)
	var normalized map[string]any
	require.NoError(t, common.Unmarshal(raw, &normalized))
	assert.Equal(t, "https://media.example/last.png", normalized["content"].(map[string]any)["last_frame_url"])
	task := &model.Task{TaskID: "public-task", Status: model.TaskStatusSuccess, Data: raw}
	projected := task.ToModelArkVideoTask()
	assert.Equal(t, "/v1/videos/public-task/content?part=last_frame", projected.Content.LastFrameURL)
}

func TestFunCloudLastFrameRejectsUnsafeResultURL(t *testing.T) {
	for _, url := range []string{"http://media.example/last.png", "https:///last.png", "file:///tmp/last.png"} {
		body, err := common.Marshal(map[string]any{"id": "provider-task", "status": "succeeded", "content": map[string]string{"video_url": "https://media.example/video.mp4", "last_frame_url": url}})
		require.NoError(t, err)
		_, err = thirdparty.FunCloudModelArkTaskResponse(body, "provider-task")
		require.Error(t, err)
	}
}

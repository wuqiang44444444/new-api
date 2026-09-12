package seedance

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func moxingTestContext(t *testing.T, request *dto.ModelArkVideoCreateRequest) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	pinSeedanceExtensionForTest(t, context)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/v3/contents/generations/tasks", nil)
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk:   request,
	})
	return context
}

func moxingAdaptor() *TaskAdaptor {
	return &TaskAdaptor{
		protocol: kitdto.VideoUpstreamProtocolMoxingModelArkV1, baseURL: "https://provider.example",
		profile: kitdto.VideoUpstreamProfileThirdPartyMoxingModelArk,
	}
}

// buildMoxingOutboundBody captures the actual southbound request body the
// unified adapter sends for one mapped provider model.
func buildMoxingOutboundBody(t *testing.T, providerModel string, request *dto.ModelArkVideoCreateRequest) map[string]any {
	t.Helper()
	context := moxingTestContext(t, request)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: providerModel,
			IsModelMapped:     true,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}
	reader, err := moxingAdaptor().BuildRequestBody(context, info)
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	return payload
}

func contentItems(items ...dto.ModelArkVideoContent) []dto.ModelArkVideoContent {
	return items
}

func textItem(text string) dto.ModelArkVideoContent {
	return dto.ModelArkVideoContent{Type: "text", Text: common.GetPointer(text)}
}

func imageReference(url string) dto.ModelArkVideoContent {
	return dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &dto.VideoMediaURL{URL: url}}
}

func videoReference(url string) dto.ModelArkVideoContent {
	return dto.ModelArkVideoContent{Type: "video_url", Role: common.GetPointer("reference_video"), VideoURL: &dto.VideoMediaURL{URL: url}}
}

func audioReference(url string) dto.ModelArkVideoContent {
	return dto.ModelArkVideoContent{Type: "audio_url", Role: common.GetPointer("reference_audio"), AudioURL: &dto.VideoMediaURL{URL: url}}
}

func firstFrameImage(url string) dto.ModelArkVideoContent {
	return dto.ModelArkVideoContent{Type: "image_url", Role: common.GetPointer("first_frame"), ImageURL: &dto.VideoMediaURL{URL: url}}
}

func TestMoxingUnifiedAdapterSendsOfficialMultimodalBody(t *testing.T) {
	models := []string{"doubao-seedance-2-0-260128-0818", modelSeedance20Fast, modelSeedance20Mini, modelSeedance25}
	scenarios := []struct {
		name    string
		content []dto.ModelArkVideoContent
	}{
		{name: "text only", content: contentItems(textItem("two puppies playing"))},
		{name: "single first frame image", content: contentItems(textItem("move"), firstFrameImage("https://example.com/first.png"))},
		{name: "multiple reference images", content: contentItems(textItem("style"), imageReference("asset://ref-img-1"), imageReference("asset://ref-img-2"))},
		{name: "video reference without image", content: contentItems(textItem("extend the motion"), videoReference("asset://ref-video-1"))},
		{name: "image and video references", content: contentItems(textItem("mix"), imageReference("asset://ref-img-1"), videoReference("asset://ref-video-1"))},
		{name: "image and audio references", content: contentItems(textItem("dub"), imageReference("asset://ref-img-1"), audioReference("asset://ref-audio-1"))},
		{name: "video and audio references", content: contentItems(textItem("replace"), videoReference("asset://ref-video-1"), audioReference("asset://ref-audio-1"))},
	}

	for _, model := range models {
		for _, scenario := range scenarios {
			t.Run(model+"/"+scenario.name, func(t *testing.T) {
				request := &dto.ModelArkVideoCreateRequest{
					Model:      "customer-" + model,
					Content:    scenario.content,
					Duration:   common.GetPointer(5),
					Resolution: common.GetPointer("720p"),
				}
				require.NoError(t, validateProviderModelRequest(kitdto.VideoUpstreamProtocolMoxingModelArkV1, model, request))

				payload := buildMoxingOutboundBody(t, model, request)
				assert.Equal(t, model, payload["model"], "customer model must not leak upstream")
				content, ok := payload["content"].([]any)
				require.True(t, ok, "content must be sent as the official multimodal array")
				require.Len(t, content, len(scenario.content))
				for i, item := range scenario.content {
					sent := content[i].(map[string]any)
					assert.Equal(t, item.Type, sent["type"])
					if item.Text != nil {
						assert.Equal(t, *item.Text, sent["text"])
					}
					if item.Role != nil {
						assert.Equal(t, *item.Role, sent["role"])
					}
					switch item.Type {
					case "image_url":
						assert.Equal(t, item.ImageURL.URL, sent["image_url"].(map[string]any)["url"])
					case "video_url":
						assert.Equal(t, item.VideoURL.URL, sent["video_url"].(map[string]any)["url"])
					case "audio_url":
						assert.Equal(t, item.AudioURL.URL, sent["audio_url"].(map[string]any)["url"])
					}
				}
				assert.NotContains(t, payload, "input_mode", "scene fields stay billing dimensions only")
				assert.NotContains(t, payload, "control_mode")
				assert.NotContains(t, payload, "capability")
			})
		}
	}
}

func TestMoxingUnifiedAdapterAllowsDocumentedAudioOnlyInput(t *testing.T) {
	for _, model := range []string{"doubao-seedance-2-0-260128-0818", modelSeedance20Fast, modelSeedance20Mini, modelSeedance25} {
		t.Run(model, func(t *testing.T) {
			request := &dto.ModelArkVideoCreateRequest{
				Model:      "customer-" + model,
				Content:    contentItems(textItem("sing along"), audioReference("asset://ref-audio-1")),
				Resolution: common.GetPointer("720p"),
			}
			require.NoError(t, validateProviderModelRequest(kitdto.VideoUpstreamProtocolMoxingModelArkV1, model, request))

			payload := buildMoxingOutboundBody(t, model, request)
			content := payload["content"].([]any)
			require.Len(t, content, 2)
			assert.Equal(t, "asset://ref-audio-1", content[1].(map[string]any)["audio_url"].(map[string]any)["url"])
		})
	}
}

// Reproduces the reported incident end to end: a customer model mapped to the
// Fast provider model submitting text plus two audio references without any
// image or video. The request must pass mapped validation and both opaque
// audio references must reach the provider body unchanged.
func TestMoxingFastCustomerModelAcceptsTwoAudioReferencesThroughMappedValidation(t *testing.T) {
	request := &dto.ModelArkVideoCreateRequest{
		Model:      "seedance-2-0-fast-m",
		Content:    contentItems(textItem("两位将军对话"), audioReference("asset://ref-audio-1"), audioReference("asset://ref-audio-2")),
		Resolution: common.GetPointer("720p"),
	}
	context := moxingTestContext(t, request)
	adaptor := moxingAdaptor()
	info := &relaycommon.RelayInfo{
		ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: modelSeedance20Fast, IsModelMapped: true},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{},
	}

	taskErr := adaptor.ValidateMappedRequest(context, info)
	require.Nil(t, taskErr, "audio-only references must not fail mapped validation for the Fast model")

	payload := buildMoxingOutboundBody(t, modelSeedance20Fast, request)
	assert.Equal(t, modelSeedance20Fast, payload["model"], "the customer model name must not leak upstream")
	content, ok := payload["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 3)
	first := content[1].(map[string]any)
	assert.Equal(t, "audio_url", first["type"])
	assert.Equal(t, "reference_audio", first["role"])
	assert.Equal(t, "asset://ref-audio-1", first["audio_url"].(map[string]any)["url"])
	second := content[2].(map[string]any)
	assert.Equal(t, "asset://ref-audio-2", second["audio_url"].(map[string]any)["url"])
	assert.Equal(t, float64(5), payload["duration"], "the omitted duration is fulfilled as the published 5s default")
	assert.Equal(t, true, payload["generate_audio"], "Fast fills its documented output-sound default when the client omits the switch")
}

func TestMoxingUnifiedAdapterRemovesUnprovenFastMiniQuantityCaps(t *testing.T) {
	request := &dto.ModelArkVideoCreateRequest{
		Model:      "customer-fast",
		Content:    contentItems(textItem("many references")),
		Duration:   common.GetPointer(5),
		Resolution: common.GetPointer("720p"),
	}
	for i := 0; i < 12; i++ {
		request.Content = append(request.Content, imageReference("asset://img-"+strconv.Itoa(i)))
	}
	request.Content = append(request.Content, videoReference("asset://video-1"), videoReference("asset://video-2"),
		videoReference("asset://video-3"), videoReference("asset://video-4"))
	for i := 0; i < 5; i++ {
		request.Content = append(request.Content, audioReference("asset://audio-"+strconv.Itoa(i)))
	}
	require.NoError(t, validateProviderModelRequest(kitdto.VideoUpstreamProtocolMoxingModelArkV1, modelSeedance20Fast, request))

	payload := buildMoxingOutboundBody(t, modelSeedance20Fast, request)
	content := payload["content"].([]any)
	assert.Len(t, content, len(request.Content), "every reference must reach the provider body")
}

func TestMoxingUnifiedAdapterKeepsDocumented25QuantityBounds(t *testing.T) {
	build := func(images, videos, audios int) *dto.ModelArkVideoCreateRequest {
		request := &dto.ModelArkVideoCreateRequest{
			Model:      "customer-2-5",
			Content:    contentItems(textItem("references")),
			Duration:   common.GetPointer(5),
			Resolution: common.GetPointer("720p"),
		}
		for i := 0; i < images; i++ {
			request.Content = append(request.Content, imageReference("asset://img"))
		}
		for i := 0; i < videos; i++ {
			request.Content = append(request.Content, videoReference("asset://video"))
		}
		for i := 0; i < audios; i++ {
			request.Content = append(request.Content, audioReference("asset://audio"))
		}
		return request
	}

	require.NoError(t, validateProviderModelRequest(kitdto.VideoUpstreamProtocolMoxingModelArkV1, modelSeedance25, build(30, 10, 10)))
	require.Error(t, validateProviderModelRequest(kitdto.VideoUpstreamProtocolMoxingModelArkV1, modelSeedance25, build(31, 0, 0)))
	require.Error(t, validateProviderModelRequest(kitdto.VideoUpstreamProtocolMoxingModelArkV1, modelSeedance25, build(0, 11, 0)))
	require.Error(t, validateProviderModelRequest(kitdto.VideoUpstreamProtocolMoxingModelArkV1, modelSeedance25, build(0, 0, 11)))
	require.Error(t, validateProviderModelRequest(kitdto.VideoUpstreamProtocolMoxingModelArkV1, modelSeedance25, build(30, 10, 11)), "total media stays capped at 50")
}

func TestMoxingUnifiedAdapterFulfillsDefaultDurationSouthbound(t *testing.T) {
	request := &dto.ModelArkVideoCreateRequest{
		Model:      "customer-fast",
		Content:    contentItems(textItem("no duration given")),
		Resolution: common.GetPointer("720p"),
	}
	payload := buildMoxingOutboundBody(t, modelSeedance20Fast, request)
	assert.Equal(t, float64(5), payload["duration"], "an omitted duration must be fulfilled as the published 5s default")
	assert.NotContains(t, payload, "watermark", "absent optional switches stay absent")
}

func TestMoxingUnifiedAdapterKeepsExplicitDurationsAndIntelligentBudget(t *testing.T) {
	tests := []struct {
		name      string
		model     string
		duration  *int
		wantBody  any
		wantProbe int
	}{
		{name: "explicit five seconds", model: modelSeedance20Fast, duration: common.GetPointer(5), wantBody: float64(5), wantProbe: 5},
		{name: "explicit -1 fast budgets 15", model: modelSeedance20Fast, duration: common.GetPointer(-1), wantBody: float64(-1), wantProbe: 15},
		{name: "explicit -1 2.5 budgets 30", model: modelSeedance25, duration: common.GetPointer(-1), wantBody: float64(-1), wantProbe: 30},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := &dto.ModelArkVideoCreateRequest{
				Model:      "customer-" + test.model,
				Content:    contentItems(textItem("generate")),
				Duration:   test.duration,
				Resolution: common.GetPointer("720p"),
			}
			require.NoError(t, validateProviderModelRequest(kitdto.VideoUpstreamProtocolMoxingModelArkV1, test.model, request))

			payload := buildMoxingOutboundBody(t, test.model, request)
			assert.Equal(t, test.wantBody, payload["duration"], "an explicit duration, including -1, must be sent unchanged")

			probe, err := moxingAdaptor().BuildTaskBillingProbe(moxingTestContext(t, request), &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: test.model, IsModelMapped: true},
			})
			require.NoError(t, err)
			assert.Equal(t, test.wantProbe, probe["duration_seconds"], "the budget upper bound is estimation only")
		})
	}
}

func TestMoxingUnifiedAdapterRecordsMixedMediaFacts(t *testing.T) {
	makeContent := func(items ...ContentItem) []ContentItem { return items }
	tests := []struct {
		name          string
		content       []ContentItem
		wantInputMode string
		wantControl   string
	}{
		{
			name:          "video reference only",
			content:       makeContent(ContentItem{Type: "text", Text: "extend"}, ContentItem{Type: "video_url", VideoURL: &MediaURL{URL: "asset://v"}}),
			wantInputMode: "multi_modal", wantControl: "reference",
		},
		{
			name:          "audio only reference",
			content:       makeContent(ContentItem{Type: "text", Text: "sing"}, ContentItem{Type: "audio_url", AudioURL: &MediaURL{URL: "asset://a"}}),
			wantInputMode: "multi_modal", wantControl: "reference",
		},
		{
			name:          "image plus audio",
			content:       makeContent(ContentItem{Type: "image_url", Role: "reference_image", ImageURL: &MediaURL{URL: "asset://i"}}, ContentItem{Type: "audio_url", AudioURL: &MediaURL{URL: "asset://a"}}),
			wantInputMode: "multi_modal", wantControl: "reference",
		},
		{
			name:          "text only",
			content:       makeContent(ContentItem{Type: "text", Text: "plain"}),
			wantInputMode: "text", wantControl: "none",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			payload := &requestPayload{Content: test.content}
			inputMode, controlMode := relayBillingModes(payload)
			assert.Equal(t, test.wantInputMode, inputMode)
			assert.Equal(t, test.wantControl, controlMode)
		})
	}
}

func TestMoxingUnifiedModelsPublishAllFourInputTypes(t *testing.T) {
	for _, contract := range kitdto.MoxingVideoModelContracts {
		t.Run(contract.ProviderModel, func(t *testing.T) {
			api, ok := publishedVideoFixture("customer-model", kitdto.VideoUpstreamProtocolMoxingModelArkV1, contract.ProviderModel, false)
			require.True(t, ok)
			types := make(map[string]int)
			for index, contentType := range api.Creation.ContentTypes {
				types[contentType.Type] = index
			}
			assert.Contains(t, types, "text", "text input must be published")
			assert.Contains(t, types, "image_url", "image input must be published")
			assert.Contains(t, types, "video_url", "video input must be published")
			assert.Contains(t, types, "audio_url", "audio input must be published")
			assert.Equal(t, contract.MaxImages, api.Creation.ContentTypes[types["image_url"]].MaxItems, "catalog caps must come from the shared registry")
			assert.Equal(t, contract.MaxVideos, api.Creation.ContentTypes[types["video_url"]].MaxItems)
			assert.Equal(t, contract.MaxAudios, api.Creation.ContentTypes[types["audio_url"]].MaxItems)
			parameters := make(map[string]kitdto.PublicAPIParameter)
			for _, parameter := range api.Creation.Parameters {
				assert.NotContains(t, parameters, parameter.Name, "each field must have one public definition")
				parameters[parameter.Name] = parameter
			}
			for _, name := range []string{"camera_fixed", "seed", "return_last_frame", "priority", "callback_url", "execution_expires_after", "draft", "tools", "safety_identifier", "frames"} {
				assert.Contains(t, parameters, name, "accepted standard fields must be discoverable")
			}
			_, hasOutputFormat := parameters["output_format"]
			assert.Equal(t, !contract.OmitOutputFormat, hasOutputFormat)
			assert.NotContains(t, parameters, "service_tier", "the existing channel setting still governs service tier")
			assert.False(t, api.Creation.AdditionalProperties, "provider-private fields remain outside the contract")
		})
	}
}

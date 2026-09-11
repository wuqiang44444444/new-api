package seedance

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoxing0818AudioDefaultMatchesSubmissionAndBilling(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		audio *bool
		want  bool
	}{
		{name: "omitted defaults true", want: true},
		{name: "explicit false", audio: common.GetPointer(false), want: false},
		{name: "explicit true", audio: common.GetPointer(true), want: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			request := &dto.ModelArkVideoCreateRequest{
				Model: "customer-video", Content: contentItems(textItem("generate"), audioReference("asset://ref-audio")),
				GenerateAudio: scenario.audio, Watermark: common.GetPointer(false),
			}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				UpstreamModelName: "doubao-seedance-2-0-260128-0818", IsModelMapped: true,
				ChannelBaseUrl: "https://provider.example.com",
			}}
			info.ChannelOtherSettings.VideoUpstreamProtocol = dto.VideoUpstreamProtocolMoxingModelArkV1
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)
			context := moxingTestContext(t, request)
			require.Nil(t, adaptor.ValidateMappedRequest(context, info))
			url, err := adaptor.BuildRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://provider.example.com/v1/media/generations", url)
			assert.Equal(t, "/v1/media/tasks/{task_id}", info.ChannelOtherSettings.VideoUpstreamQueryPathTemplate)
			body := buildMoxingOutboundBody(t, info.UpstreamModelName, request)
			assert.Equal(t, "doubao-seedance-2-0-260128-0818", body["model"])
			assert.Equal(t, scenario.want, body["generate_audio"])
			assert.Equal(t, false, body["watermark"])
			assert.Equal(t, float64(5), body["duration"])
			probe, err := adaptor.BuildTaskBillingProbe(context, info)
			require.NoError(t, err)
			assert.Equal(t, scenario.want, probe["generate_audio"])
			assert.Equal(t, 5, probe["duration_seconds"])
			assert.Equal(t, false, probe["has_video_input"])
			assert.Equal(t, scenario.audio, request.GenerateAudio)
		})
	}
}

func TestMoxingOldAndUnknownModelsHaveNoNewRequestOrCatalogEntry(t *testing.T) {
	for _, model := range []string{"doubao-seedance-2-0-260128", "doubao-seedance-2-0-unregistered"} {
		t.Run(model, func(t *testing.T) {
			request := &dto.ModelArkVideoCreateRequest{Model: "customer-video", Content: contentItems(textItem("generate"))}
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: model, IsModelMapped: true}}
			require.NotNil(t, moxingAdaptor().ValidateMappedRequest(moxingTestContext(t, request), info))
			_, ok := publishedVideoFixture("customer-video", dto.VideoUpstreamProtocolMoxingModelArkV1, model, false)
			assert.False(t, ok)
		})
	}
}

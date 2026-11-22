package seedance

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestOfficialPreConsumeEstimatesOnlyKnownFixedOutputWithoutVideoInput(t *testing.T) {
	for _, tt := range []struct {
		name              string
		protocol          dto.VideoUpstreamProtocol
		model, resolution string
		duration          *int
		hasVideo, draft   bool
		frames            *int
		want              int
	}{
		{name: "short 720p", duration: common.GetPointer(5), want: 135_851},
		{name: "4K", resolution: "4k", duration: common.GetPointer(5), want: 1_215_095},
		{name: "2.5 long output", model: modelSeedance25, duration: common.GetPointer(30), want: 815_105},
		{name: "reference video", duration: common.GetPointer(5), hasVideo: true},
		{name: "draft", duration: common.GetPointer(5), draft: true},
		{name: "automatic duration", duration: common.GetPointer(-1)},
		{name: "duration omitted"},
		{name: "frame count", duration: common.GetPointer(5), frames: common.GetPointer(121)},
		{name: "unverified deployment", model: "ep-reviewed-deployment", duration: common.GetPointer(5)},
		{name: "unsupported protocol", protocol: dto.VideoUpstreamProtocolSynlinkVideoV1, duration: common.GetPointer(5)},
		{name: "outside the model duration range", duration: common.GetPointer(30)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			resolution := common.GetStringIfEmpty(tt.resolution, "720p")
			relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{ModelArk: &dto.ModelArkVideoCreateRequest{
				Duration: tt.duration, Resolution: &resolution, Frames: tt.frames,
			}})
			protocol := tt.protocol
			if protocol == "" {
				protocol = dto.VideoUpstreamProtocolModelArkV3Volcengine
			}
			adaptor := TaskAdaptor{protocol: protocol}
			info := &relaycommon.RelayInfo{
				ChannelMeta:   &relaycommon.ChannelMeta{UpstreamModelName: common.GetStringIfEmpty(tt.model, modelSeedance20)},
				TaskRelayInfo: &relaycommon.TaskRelayInfo{VideoDraftRequested: tt.draft},
			}
			probe := map[string]any{"resolution": resolution, "has_video_input": tt.hasVideo}
			assert.Equal(t, tt.want, adaptor.EstimateTaskPreConsumeTokens(context, info, probe))
		})
	}
}

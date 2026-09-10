package seedance

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These are wire-contract tests, not claims that a provider will accept or use
// a particular media payload. No media is downloaded or generation purchased.
func TestReferenceMediaRepresentationsReachProviderBody(t *testing.T) {
	for _, protocol := range []dto.VideoUpstreamProtocol{dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, dto.VideoUpstreamProtocolSynlinkVideoV1, dto.VideoUpstreamProtocolFunCloudModelArkV3, dto.VideoUpstreamProtocolFeicaiVideosV1} {
		for _, kind := range []string{"audio_url", "video_url"} {
			for _, scheme := range []string{"http", "data"} {
				t.Run(string(protocol)+"/"+kind+"/"+scheme, func(t *testing.T) {
					ref := "http://media.example.com/reference.mp4"
					if scheme == "data" {
						ref = "data:video/mp4;base64,dmlkZW8="
					}
					role, field := "reference_video", "reference_videos"
					item := dto.ModelArkVideoContent{Type: kind, VideoURL: &dto.VideoMediaURL{URL: ref}}
					if kind == "audio_url" {
						ref = "http://media.example.com/reference.mp3"
						if scheme == "data" {
							ref = "data:audio/mpeg;base64,YXVkaW8="
						}
						role, field = "reference_audio", "reference_audios"
						item.VideoURL = nil
						item.AudioURL = &dto.VideoMediaURL{URL: ref}
					}
					item.Role = common.GetPointer(role)
					req := providerTestRequest()
					req.Model = "customer-reference"
					req.Content = append(req.Content, item)
					req.GenerateAudio = common.GetPointer(false)
					req.Watermark = common.GetPointer(false)
					providerModel := modelSeedance20
					switch protocol {
					case dto.VideoUpstreamProtocolSynlinkVideoV1:
						providerModel = kitdto.SynlinkVideoModels()[0]
					case dto.VideoUpstreamProtocolFunCloudModelArkV3:
						providerModel = "seedance-2-0-mini"
					case dto.VideoUpstreamProtocolFeicaiVideosV1:
						providerModel = feicai.ProviderModelSeedance20ProPI720P
						req.Duration = common.GetPointer(15)
						req.Ratio = common.GetPointer("16:9")
						req.GenerateAudio, req.Watermark = nil, nil
					}
					c := seedancePluginTestContext(t)
					if protocol == dto.VideoUpstreamProtocolFeicaiVideosV1 {
						pinSeedanceExtensionForTest(t, c)
					}
					relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: req})
					info := &relaycommon.RelayInfo{OriginModelName: req.Model, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, IsModelMapped: true, UpstreamModelName: providerModel, ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: protocol}}}
					adaptor := &TaskAdaptor{}
					adaptor.Init(info)
					require.Nil(t, adaptor.ValidateMappedRequest(c, info))
					reader, err := adaptor.BuildRequestBody(c, info)
					require.NoError(t, err)
					raw, err := io.ReadAll(reader)
					require.NoError(t, err)
					var wire map[string]any
					require.NoError(t, common.Unmarshal(raw, &wire))
					switch protocol {
					case dto.VideoUpstreamProtocolTokenSaveMediaTaskV1:
						assert.Equal(t, []any{ref}, wire[field])
						assert.Equal(t, "multi_image", wire["input_mode"])
						assert.Equal(t, "reference", wire["control_mode"])
						assert.Equal(t, false, wire["with_audio"])
						assert.Equal(t, false, wire["watermark"])
						probe, err := adaptor.BuildTaskBillingProbe(c, info)
						require.NoError(t, err)
						assert.Equal(t, "multi_image", probe["input_mode"])
						assert.Equal(t, "reference", probe["control_mode"])
						assert.Equal(t, kind == "video_url", probe["has_video_input"])
					case dto.VideoUpstreamProtocolFeicaiVideosV1:
						key := "videos"
						if kind == "audio_url" {
							key = "audios"
						}
						assert.Equal(t, []any{ref}, wire[key])
						// Also keep the Go adapter consistent with the active embedded plugin.
						goBody, err := feicai.CreateRequest(req, providerModel)
						require.NoError(t, err)
						assert.JSONEq(t, string(goBody), string(raw))
					default:
						content, ok := wire["content"].([]any)
						require.True(t, ok)
						require.Len(t, content, 2)
						media := content[1].(map[string]any)
						assert.Equal(t, role, media["role"])
						assert.Equal(t, ref, media[kind].(map[string]any)["url"])
						assert.Equal(t, false, wire["generate_audio"])
						assert.Equal(t, false, wire["watermark"])
					}
				})
			}
		}
	}
}

func TestRetiredFunCloudV2CannotSubmitOrBuildCreate(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: providerTestRequest()})
	adaptor := &TaskAdaptor{protocol: dto.VideoUpstreamProtocolFunCloudSeedance, profile: dto.VideoUpstreamProfileThirdPartyFunCloudSeedance}
	taskErr := adaptor.ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{})
	require.NotNil(t, taskErr)
	assert.Equal(t, "invalid_video_protocol", taskErr.Code)
	_, err := adaptor.BuildRequestBody(c, &relaycommon.RelayInfo{})
	require.ErrorContains(t, err, "retired")
}

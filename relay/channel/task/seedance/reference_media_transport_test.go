package seedance

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	kitdto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These are wire-contract tests, not claims that a provider will accept or use
// a particular media payload. No media is downloaded or generation purchased.
func TestReferenceMediaRepresentationsReachProviderBody(t *testing.T) {
	for _, protocol := range []dto.VideoUpstreamProtocol{dto.VideoUpstreamProtocolTokenSaveMediaTaskV1, dto.VideoUpstreamProtocolSynlinkVideoV1, dto.VideoUpstreamProtocolFunCloudModelArkV3, dto.VideoUpstreamProtocolFeicaiVideosV1, dto.VideoUpstreamProtocolMoxingModelArkV1, dto.VideoUpstreamProtocolModelArkV3CMCC, dto.VideoUpstreamProtocolModelArkV3Volcengine, dto.VideoUpstreamProtocolModelArkV3BytePlus, dto.VideoUpstreamProtocolArkMediaV1} {
		for _, kind := range []string{"audio_url", "video_url"} {
			for _, scheme := range []string{"http", "https", "data", "base64", "file"} {
				if kind != "audio_url" && (scheme == "base64" || scheme == "file") {
					continue
				}
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
						if scheme == "base64" {
							ref = "YXVkaW8="
						}
						if scheme == "file" {
							ref = "file://audio"
						}
						role, field = "reference_audio", "reference_audios"
						item.VideoURL = nil
						item.AudioURL = &dto.VideoMediaURL{URL: ref}
					}
					if scheme == "https" {
						ref = strings.Replace(ref, "http://", "https://", 1)
						if kind == "audio_url" {
							item.AudioURL.URL = ref
						} else {
							item.VideoURL.URL = ref
						}
					}
					item.Role = common.GetPointer(role)
					req := providerTestRequest()
					req.Model = "customer-reference"
					req.Content = append(req.Content, item)
					req.GenerateAudio = common.GetPointer(false)
					req.Watermark = common.GetPointer(false)
					providerModel := modelSeedance20
					switch protocol {
					case dto.VideoUpstreamProtocolMoxingModelArkV1:
						providerModel = "doubao-seedance-2-0-260128-0818"
					case dto.VideoUpstreamProtocolModelArkV3CMCC:
						providerModel = "doubao-seedance-2.0"
						req.Ratio = common.GetPointer("16:9")
					case dto.VideoUpstreamProtocolModelArkV3Volcengine, dto.VideoUpstreamProtocolModelArkV3BytePlus, dto.VideoUpstreamProtocolArkMediaV1:
						providerModel = "ep-audio-test"
					case dto.VideoUpstreamProtocolSynlinkVideoV1:
						providerModel = kitdto.SynlinkVideoModels()[0]
						if kind == "audio_url" {
							// 2.5 supports audio-only references. Keep this test focused
							// on transporting each representation, not invalid model input.
							providerModel = "doubao-seedance-2-5-260628"
						}
					case dto.VideoUpstreamProtocolFunCloudModelArkV3:
						providerModel = "seedance-2-0-mini"
					case dto.VideoUpstreamProtocolFeicaiVideosV1:
						providerModel = feicai.ProviderModelSeedance20ProPI720P
						req.Duration = common.GetPointer(15)
						req.Ratio = common.GetPointer("16:9")
						req.GenerateAudio, req.Watermark = nil, nil
					}
					c := seedancePluginTestContext(t)
					if SeedanceExtensionProtocolMigrated(protocol) {
						pinSeedanceExtensionForTest(t, c)
					}
					relaycommon.SetVideoContractRequest(c, dto.VideoContractRequest{ContractID: dto.VideoContractModelArkV3, ModelArk: req})
					info := &relaycommon.RelayInfo{UserId: 7, OriginModelName: req.Model, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeSeedanceLink, ChannelBaseUrl: "https://provider.example", IsModelMapped: true, UpstreamModelName: providerModel, ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: protocol}}}
					if kind == "audio_url" && (scheme == "data" || scheme == "base64" || scheme == "file") {
						store := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							assert.Equal(t, http.MethodPut, r.Method)
							data, err := io.ReadAll(r.Body)
							assert.NoError(t, err)
							assert.Equal(t, []byte("audio"), data)
							assert.Equal(t, "audio/mpeg", r.Header.Get("Content-Type"))
							w.WriteHeader(200)
						}))
						t.Cleanup(store.Close)
						previous := http.DefaultTransport
						http.DefaultTransport = store.Client().Transport
						t.Cleanup(func() { http.DefaultTransport = previous })
						config, err := common.Marshal(system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: store.URL, Bucket: "audio", AccountName: "fixture", Credential: "fixture", Region: "us-east-1", Revision: t.Name()})
						require.NoError(t, err)
						model.NotifyObjectStorageSettingUpdate(string(config))
						t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
						data, err := base64.StdEncoding.DecodeString("YXVkaW8=")
						require.NoError(t, err)
						service.SetVideoReferenceAudioInputs(c, map[int]service.VideoReferenceAudioInput{len(req.Content) - 1: {Source: ref, Data: data, MimeType: "audio/mpeg"}})
					}
					adaptor := &TaskAdaptor{}
					adaptor.Init(info)
					require.Nil(t, adaptor.ValidateMappedRequest(c, info))
					if kind == "audio_url" && (scheme == "data" || scheme == "base64" || scheme == "file") {
						ref = req.Content[len(req.Content)-1].AudioURL.URL
						snapshot := &model.Task{}
						service.StageVideoReferenceAudioSnapshot(c, snapshot)
						require.Len(t, snapshot.PrivateData.ReferenceAudio, 1)
						assert.True(t, strings.HasPrefix(ref, "https://"))
						assert.Contains(t, ref, "/audio/"+snapshot.PrivateData.ReferenceAudio[0].ObjectKey+"?")
					}

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
						assert.Equal(t, kind, media["type"])
						assert.NotContains(t, media, "image_url")
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

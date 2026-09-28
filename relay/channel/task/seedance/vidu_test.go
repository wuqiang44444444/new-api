package seedance

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/plugins"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestViduObservationNormalizesDocumentedAndObservedFields(t *testing.T) {
	for _, numeric := range []bool{false, true} {
		t.Run(fmt.Sprint(numeric), func(t *testing.T) {
			raw := map[string]any{"id": "1000000000000000001", "model": "viduq3-drama-std", "status": "succeeded", "seed": "0", "duration": "5", "created_at": "1790563514", "updated_at": "1790563750", "frames_per_second": "24", "resolution": "720p", "ratio": "16:9", "execution_expires_after": "172800", "generate_audio": true, "draft": false, "content": map[string]any{"video_url": "https://result.example/video.mp4", "last_frame_url": "https://result.example/frame.jpg"}, "usage": map[string]any{"completion_tokens": "108900", "total_tokens": "108900", "prompt_tokens": "0"}}
			if numeric {
				for _, k := range []string{"seed", "duration", "created_at", "updated_at", "frames_per_second", "execution_expires_after"} {
					var n int64
					_, err := fmt.Sscan(raw[k].(string), &n)
					require.NoError(t, err)
					raw[k] = n
				}
				raw["usage"] = map[string]any{"completion_tokens": 108900, "total_tokens": 108900, "prompt_tokens": 0}
			}
			body, err := common.Marshal(raw)
			require.NoError(t, err)
			normalized, err := normalizeViduTaskResponse(body, "1000000000000000001")
			require.NoError(t, err)
			var response responseTask
			require.NoError(t, common.Unmarshal(normalized, &response))
			assert.Equal(t, 5, response.Duration)
			assert.Equal(t, 24, response.FramesPerSecond)
			assert.EqualValues(t, 1790563514, response.CreatedAt)
			require.NotNil(t, response.Usage)
			assert.Equal(t, 108900, *response.Usage.CompletionTokens)
			assert.Contains(t, string(normalized), `"last_frame_url":"https://result.example/frame.jpg"`)
			plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
			require.NoError(t, err)
			result, err := plugin.Engine.CallPath(context.Background(), "seedance", []string{"vidu_modelark_v3", "parseTaskObservation"}, map[string]any{"taskId": response.ID, "body": string(normalized)})
			require.NoError(t, err)
			decoded, err := decodeOfficialPluginObservation(result, response.ID, 3, dto.VideoUpstreamProtocolViduModelArkV3)
			require.NoError(t, err)
			assert.JSONEq(t, string(normalized), string(decoded))
		})
	}
}

func TestViduUsageRequiresBoundedCompletionEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		reported    bool
		tokens      int
	}{
		{"string", `{"completion_tokens":"108900","total_tokens":"108900"}`, true, 108900},
		{"zero", `{"completion_tokens":"0"}`, true, 0},
		{"max", `{"completion_tokens":"2147483647"}`, true, 2147483647},
		{"total-only", `{"total_tokens":"108900"}`, false, 0},
		{"overflow", `{"completion_tokens":"18446744073709551615"}`, false, 0},
		{"negative", `{"completion_tokens":"-1"}`, false, 0},
		{"fraction", `{"completion_tokens":"1.0"}`, false, 0},
		{"exponent", `{"completion_tokens":1e2}`, false, 0},
		{"bool", `{"completion_tokens":true}`, false, 0},
		{"spaces", `{"completion_tokens":" 10 "}`, false, 0},
		{"invalid-total", `{"completion_tokens":"10","total_tokens":"bad"}`, false, 0},
		{"conflicting-total", `{"completion_tokens":"10","total_tokens":"9"}`, false, 0},
		{"null", `null`, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := []byte(`{"id":"task","status":"succeeded","content":{"video_url":"https://result.example/video.mp4"},"usage":` + tc.usage + `}`)
			normalized, err := normalizeViduTaskResponse(body, "task")
			require.NoError(t, err)
			var response responseTask
			require.NoError(t, common.Unmarshal(normalized, &response))
			assert.Equal(t, "succeeded", response.Status)
			if tc.reported {
				require.NotNil(t, response.Usage)
				assert.Equal(t, tc.tokens, *response.Usage.CompletionTokens)
			} else {
				assert.Nil(t, response.Usage)
			}
		})
	}
}

func TestViduUntrustedObservationsCannotFinishTask(t *testing.T) {
	for _, body := range []string{
		`{"id":"other","status":"succeeded"}`,
		`{"id":1000000000000000001,"status":"running"}`,
		`{"id":"task","status":"unknown"}`,
		`{"id":"task","status":"running","duration":"-1"}`,
		`{"id":"task","status":"running","created_at":"9223372036854775808"}`,
		`{"id":"task","status":"running","seed":true}`,
		`{"id":"task","status":"succeeded"}`,
		`{"id":"task","status":"succeeded","content":{"video_url":"not-a-url"},"usage":{"completion_tokens":"10"}}`,
		`{"id":"task","status":"succeeded","content":{"video_url":"http://cdn.example/video.mp4"},"usage":{"completion_tokens":"10"}}`,
		`{"id":"task","status":"succeeded","content":{"video_url":"https://user:password@cdn.example/video.mp4"},"usage":{"completion_tokens":"10"}}`,
		`{"id":"task","status":"succeeded","content":{"video_url":"https://cdn.example/video.mp4","last_frame_url":"not-a-url"},"usage":{"completion_tokens":"10"}}`,
		`{"id":"task","status":"failed","content":{"video_url":"https://result.example/video.mp4"}}`,
		`{"id":"task","status":"running","error":{"message":"query failed"}}`,
	} {
		_, err := normalizeViduTaskResponse([]byte(body), "task")
		require.Error(t, err, body)
	}
	for _, status := range []string{"queued", "running", "failed", "expired"} {
		body, err := normalizeViduTaskResponse([]byte(fmt.Sprintf(`{"id":"task","status":%q,"content":null,"usage":null,"error":null,"duration":"0"}`, status)), "task")
		require.NoError(t, err)
		assert.Contains(t, string(body), fmt.Sprintf(`"status":%q`, status))
	}
}

func TestViduCreatePreservesPublishedFieldsAndDefaults(t *testing.T) {
	for _, provider := range []string{"viduq3-drama-std", "viduq3-drama-ab-std", "viduq3-drama-fast", "viduq3-drama-ab-fast", "viduq3-drama-mini", "viduq3-drama-ab-mini", "viduq3.1-drama-std", "viduq3.1-drama-ab-std"} {
		t.Run(provider, func(t *testing.T) {
			c := seedancePluginTestContext(t)
			pinSeedanceExtensionForTest(t, c)
			var request taskdto.ModelArkVideoCreateRequest
			require.NoError(t, common.Unmarshal([]byte(`{"model":"customer","content":[{"type":"text","text":"fixture"},{"type":"image_url","role":"reference_image","image_url":{"url":"https://image.example/ref.png"}},{"type":"audio_url","role":"reference_audio","audio_url":{"url":"https://audio.example/ref.mp3"}}],"generate_audio":false,"watermark":false,"return_last_frame":false,"callback_url":"https://callback.example/status","execution_expires_after":3600,"tools":[{"type":"web_search"}],"safety_identifier":"fixture-user"}`), &request))
			relaycommon.SetVideoContractRequest(c, taskdto.VideoContractRequest{ContractID: taskdto.VideoContractModelArkV3, ModelArk: &request})
			info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{IsModelMapped: true, UpstreamModelName: provider, ChannelBaseUrl: "https://api.example/ent", ChannelOtherSettings: taskdto.ChannelOtherSettings{VideoUpstreamProtocol: dto.VideoUpstreamProtocolViduModelArkV3}}}
			adaptor := &TaskAdaptor{}
			adaptor.Init(info)
			probe, err := adaptor.BuildTaskBillingProbe(c, info)
			require.NoError(t, err)
			assert.Equal(t, false, probe["generate_audio"])
			assert.Equal(t, 5, probe["duration_seconds"])
			conversion, err := adaptor.ensureSeedanceCreateConversion(c, info)
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, common.Unmarshal(conversion.body, &body))
			assert.Equal(t, provider, body["model"])
			assert.Equal(t, false, body["generate_audio"])
			assert.Equal(t, false, body["watermark"])
			assert.Equal(t, false, body["return_last_frame"])
			assert.Equal(t, "adaptive", body["ratio"])
			assert.Equal(t, float64(5), body["duration"])
			assert.Equal(t, "https://callback.example/status", body["callback_url"])
			assert.Equal(t, "fixture-user", body["safety_identifier"])
			assert.Equal(t, float64(3600), body["execution_expires_after"])
			assert.Len(t, body["content"], 3)
			assert.Equal(t, []any{map[string]any{"type": "web_search"}}, body["tools"])
			url, err := adaptor.BuildRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://api.example/ent/api/v3/contents/generations/tasks", url)
		})
	}
}

func TestViduMediaModesAndCounts(t *testing.T) {
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	require.NoError(t, err)
	image := func(role string) map[string]any {
		return map[string]any{"type": "image_url", "role": role, "image_url": map[string]any{"url": "https://fixture.example/image.png"}}
	}
	for _, tc := range []struct {
		name, provider string
		content        []any
		valid          bool
	}{
		{"first-last", "viduq3-drama-std", []any{image("first_frame"), image("last_frame")}, true},
		{"last-only", "viduq3-drama-std", []any{image("last_frame")}, false},
		{"duplicate-first", "viduq3-drama-std", []any{image("first_frame"), image("first_frame")}, false},
		{"mixed-modes", "viduq3-drama-std", []any{image("first_frame"), image("reference_image")}, false},
		{"q3-limit", "viduq3-drama-std", []any{image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image")}, false},
		{"q31-expanded", "viduq3.1-drama-std", []any{image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image"), image("reference_image")}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := plugin.Engine.CallPath(context.Background(), "seedance", []string{"vidu_modelark_v3", "buildCreate"}, map[string]any{"providerModel": tc.provider, "request": map[string]any{"model": "customer", "content": tc.content}})
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

package seedance

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// pinSeedanceExtensionForTest compiles the embedded artifact and pins it the
// way ResolveSeedanceChannel does in production.
func pinSeedanceExtensionForTest(t *testing.T, c *gin.Context) *pluginruntime.LoadedPlugin {
	t.Helper()
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	require.NoError(t, err)
	c.Set(pluginruntime.ContextKeyPinnedPlugin, pluginruntime.PinnedPlugin{Plugin: plugin})
	return plugin
}

func seedancePluginTestContext(t *testing.T) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/api/v3/contents/generations/tasks", nil)
	return c
}

// ---------------------------------------------------------------------------
// Differential suite: the plugin conversion must reproduce the legacy Go
// implementation byte-for-byte, including error message strings.
// ---------------------------------------------------------------------------

func seedanceDifferentialCases() []struct {
	name          string
	providerModel string
	request       *taskdto.ModelArkVideoCreateRequest
} {
	duration := 4
	longDuration := 20
	resolution := "720p"
	wrongResolution := "1080p"
	ratio := "21:9"
	badRatio := "5:4"
	watermark := true
	return []struct {
		name          string
		providerModel string
		request       *taskdto.ModelArkVideoCreateRequest
	}{
		{
			name:          "text only mini 720p",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("a paper boat")}},
			},
		},
		{
			name:          "with reference image http url",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{
					{Type: "text", Text: common.GetPointer("a paper boat")},
					{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &taskdto.VideoMediaURL{URL: "https://example.com/a.png"}},
				},
			},
		},
		{
			name:          "unknown provider model",
			providerModel: "not-a-registered-provider-model",
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("x")}},
			},
		},
		{
			name:          "duration above model max",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &longDuration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("x")}},
			},
		},
		{
			name:          "resolution mismatch",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &wrongResolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("x")}},
			},
		},
		{
			name:          "ratio not supported",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &badRatio,
				Content: []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("x")}},
			},
		},
		{
			name:          "unsupported field watermark",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio, Watermark: &watermark,
				Content: []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("x")}},
			},
		},
		{
			name:          "image role missing",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "image_url", ImageURL: &taskdto.VideoMediaURL{URL: "https://example.com/a.png"}}},
			},
		},
		{
			name:          "image url invalid scheme",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &taskdto.VideoMediaURL{URL: "ftp://example.com/a.png"}}},
			},
		},
		{
			name:          "audio accepts HTTP",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "audio_url", Role: common.GetPointer("reference_audio"), AudioURL: &taskdto.VideoMediaURL{URL: "http://example.com/a.mp3"}}},
			},
		},
		{
			name:          "data url valid base64 image",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &taskdto.VideoMediaURL{URL: "data:image/png;base64,aGVsbG8="}}},
			},
		},
		{
			name:          "data url invalid base64",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &taskdto.VideoMediaURL{URL: "data:image/png;base64,aGVsbG8!"}}},
			},
		},
		{
			name:          "empty prompt",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("   ")}},
			},
		},
		{
			name:          "unsupported content type",
			providerModel: feicai.ProviderModelSeedance20Mini720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "weird_type"}},
			},
		},
		{
			name:          "sd2 requires at least one image",
			providerModel: feicai.ProviderModelSeedance20SD2720P,
			request: &taskdto.ModelArkVideoCreateRequest{
				Duration: &duration, Resolution: &resolution, Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("x")}},
			},
		},
	}
}

// TestSeedancePluginCreateConversionCoversEveryRegisteredModel exercises a
// valid request for each registered feicai provider model, so a spec-table
// drift between the plugin and the legacy oracle fails loudly.
func TestSeedancePluginCreateConversionCoversEveryRegisteredModel(t *testing.T) {
	for _, spec := range feicai.CurrentModelSpecs() {
		providerModel := spec.ProviderModel
		resolution := spec.Resolution
		ratio := spec.Ratios[0]
		duration := spec.MaxDuration
		if duration > spec.MinDuration {
			duration = spec.MinDuration
		}
		images := make([]taskdto.ModelArkVideoContent, 0, spec.MinImages)
		for i := 0; i < spec.MinImages; i++ {
			images = append(images, taskdto.ModelArkVideoContent{
				Type: "image_url", Role: common.GetPointer("reference_image"),
				ImageURL: &taskdto.VideoMediaURL{URL: fmt.Sprintf("https://example.com/ref-%d.png", i)},
			})
		}
		content := append([]taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("a boat")}}, images...)

		request := &taskdto.ModelArkVideoCreateRequest{
			Model:      "customer-model",
			Duration:   &duration,
			Resolution: &resolution,
			Ratio:      &ratio,
			Content:    content,
		}

		c := seedancePluginTestContext(t)
		pinSeedanceExtensionForTest(t, c)
		relaycommon.SetVideoContractRequest(c, taskdto.VideoContractRequest{
			ContractID: taskdto.VideoContractModelArkV3, ModelArk: request,
		})
		adaptor := &TaskAdaptor{
			protocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
			profile:  dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
		}
		conversion, conversionErr := adaptor.ensureSeedanceCreateConversion(c, &relaycommon.RelayInfo{
			ChannelMeta: &relaycommon.ChannelMeta{IsModelMapped: true, UpstreamModelName: providerModel},
		})
		legacyBody, legacyErr := feicai.CreateRequest(request, providerModel)

		if legacyErr != nil {
			require.Error(t, conversionErr, "provider model %s", providerModel)
			assert.Equal(t, legacyErr.Error(), conversionErr.Error(), "provider model %s", providerModel)
			continue
		}
		require.NoError(t, conversionErr, "provider model %s", providerModel)
		assert.Equal(t, string(legacyBody), string(conversion.body), "provider model %s", providerModel)
		assert.Equal(t, spec.Resolution, conversion.probe["resolution"], "provider model %s", providerModel)
		assert.Equal(t, ratio, conversion.probe["ratio"], "provider model %s", providerModel)
	}
}

func TestSeedancePluginCreateConversionMatchesLegacyGo(t *testing.T) {
	for _, testCase := range seedanceDifferentialCases() {
		t.Run(testCase.name, func(t *testing.T) {
			request := testCase.request
			request.Model = "customer-model"

			c := seedancePluginTestContext(t)
			pinSeedanceExtensionForTest(t, c)
			relaycommon.SetVideoContractRequest(c, taskdto.VideoContractRequest{
				ContractID: taskdto.VideoContractModelArkV3, ModelArk: request,
			})
			adaptor := &TaskAdaptor{
				protocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
				profile:  dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
			}
			info := &relaycommon.RelayInfo{
				ChannelMeta: &relaycommon.ChannelMeta{IsModelMapped: true, UpstreamModelName: testCase.providerModel},
			}
			conversion, conversionErr := adaptor.ensureSeedanceCreateConversion(c, info)
			pluginBody, pluginErr := func() ([]byte, error) {
				if conversionErr != nil {
					return nil, conversionErr
				}
				return conversion.body, nil
			}()

			legacyBody, legacyErr := feicai.CreateRequest(request, testCase.providerModel)
			if legacyErr != nil {
				require.Error(t, pluginErr, "plugin path must fail when legacy fails")
				assert.Equal(t, legacyErr.Error(), pluginErr.Error())
				return
			}
			require.NoError(t, pluginErr)
			assert.Equal(t, string(legacyBody), string(pluginBody))

			// Probe extension fields must match the legacy derivation.
			resolved, resolveErr := feicai.ResolveRequest(request, testCase.providerModel)
			require.NoError(t, resolveErr)
			assert.Equal(t, resolved.Spec.Resolution, conversion.probe["resolution"])
			assert.Equal(t, resolved.Ratio, conversion.probe["ratio"])
			assert.Equal(t, 1.0, conversion.probe["size_multiplier"])
			assert.Equal(t, feicai.BillingModePerSecond, conversion.probe["billing_mode"])
		})
	}
}

func TestSeedancePluginCreateResponseMatchesLegacyGo(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "valid id", body: `{"id":" task-123 "}`},
		{name: "no id", body: `{"other":1}`},
		{name: "empty id", body: `{"id":"  "}`},
		{name: "invalid json", body: `not-json`},
		{name: "non object", body: `"plain"`},
		{name: "numeric id", body: `{"id":123}`},
		{name: "id too long", body: `{"id":"` + strings.Repeat("a", 192) + `"}`},
		{name: "id with control char", body: "{\"id\":\"task\x01\"}"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			c := seedancePluginTestContext(t)
			pinSeedanceExtensionForTest(t, c)
			duration, resolution, ratio := 4, "720p", "21:9"
			relaycommon.SetVideoContractRequest(c, taskdto.VideoContractRequest{
				ContractID: taskdto.VideoContractModelArkV3,
				ModelArk: &taskdto.ModelArkVideoCreateRequest{
					Model:      feicai.ProviderModelSeedance20Mini720P,
					Duration:   &duration,
					Resolution: &resolution,
					Ratio:      &ratio,
					Content:    []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("warm up")}},
				},
			})
			adaptor := &TaskAdaptor{
				protocol: dto.VideoUpstreamProtocolFeicaiVideosV1,
				profile:  dto.VideoUpstreamProfileThirdPartyFeicaiVideos,
			}
			_, err := adaptor.ensureSeedanceCreateConversion(c, &relaycommon.RelayInfo{})
			require.NoError(t, err)

			pluginResult, pluginErr := parseSeedancePluginCreateResponse(c, adaptor, []byte(testCase.body))
			legacyResult, legacyErr := feicai.CreateResponse([]byte(testCase.body))
			if legacyErr != nil {
				require.Error(t, pluginErr)
				assert.Equal(t, legacyErr.Error(), pluginErr.Error())
				return
			}
			require.NoError(t, pluginErr)
			assert.Equal(t, string(legacyResult), string(pluginResult))
		})
	}
}

func TestSeedancePluginTaskObservationMatchesLegacyGo(t *testing.T) {
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	require.NoError(t, err)
	baseURL := "https://feicai.example.com"
	cases := []struct {
		name string
		body string
	}{
		{name: "queued", body: `{"id":"t1","status":"queued"}`},
		{name: "processing", body: `{"id":"t1","status":"processing"}`},
		{name: "in progress alias", body: `{"id":"t1","status":"in_progress"}`},
		{name: "succeeded", body: `{"id":"t1","status":"completed","video_url":"https://feicai.example.com/v/1.mp4"}`},
		{name: "succeeded cross origin url", body: `{"id":"t1","status":"completed","video_url":"https://evil.example.com/v/1.mp4"}`},
		{name: "succeeded missing url", body: `{"id":"t1","status":"completed"}`},
		{name: "failed with error", body: `{"id":"t1","status":"failed","error":{"code":"E01","message":"boom"}}`},
		{name: "failed with control chars", body: "{\"id\":\"t1\",\"status\":\"failed\",\"error\":{\"code\":\"E\x01\",\"message\":\"bad\x02message\"}}"},
		{name: "id mismatch", body: `{"id":"other","status":"queued"}`},
		{name: "unknown status", body: `{"id":"t1","status":"weird"}`},
		{name: "invalid json", body: `nope`},
		{name: "numeric status", body: `{"id":"t1","status":5}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, callErr := plugin.Engine.CallPathWithAdmissionTimeout(
				context.Background(), seedanceExtensionPollAdmissionTimeout,
				"seedance", []string{"feicai_videos_v1", "parseTaskObservation"},
				map[string]any{"taskId": "t1", "body": testCase.body},
			)
			require.NoError(t, callErr)
			pluginResult, pluginErr := decodeSeedanceTaskObservation(result, "t1", baseURL)
			legacyResult, legacyErr := feicai.TaskResponse([]byte(testCase.body), "t1", feicai.TaskResponseContext{BaseURL: baseURL})
			if legacyErr != nil {
				require.Error(t, pluginErr)
				if violation, ok := legacyErr.(*relaycommon.UpstreamContractViolation); ok {
					pluginViolation, ok := pluginErr.(*relaycommon.UpstreamContractViolation)
					require.True(t, ok)
					assert.Equal(t, violation.Reason, pluginViolation.Reason)
				}
				return
			}
			require.NoError(t, pluginErr)
			assert.Equal(t, string(legacyResult), string(pluginResult))
		})
	}
}

// ---------------------------------------------------------------------------
// Index isolation: loading and syncing the extension must not touch the
// native registry.
// ---------------------------------------------------------------------------

func TestSeedanceExtensionSyncLeavesNativeRegistryUntouched(t *testing.T) {
	nativeBefore := pluginruntime.DefaultRegistry.Snapshot()
	generationBefore := pluginruntime.DefaultRegistry.Generation()

	source := plugins.SeedanceSource()
	hash := func() string { return string(common.Sha256Raw([]byte(source))) }
	rows := []model.TaskPlugin{{
		Key: SeedanceExtensionPluginKey, Version: "1.0.2", Source: source,
		SourceHash: hash(), Enabled: true, Active: true,
	}}
	require.NoError(t, seedanceExtensions.SyncSnapshot(context.Background(), rows))

	assert.Equal(t, nativeBefore, pluginruntime.DefaultRegistry.Snapshot())
	assert.Same(t, generationBefore, pluginruntime.DefaultRegistry.Generation())
	_, byModel := pluginruntime.DefaultRegistry.Generation().GetByModel("seedance-link")
	assert.False(t, byModel)
}

func TestSeedanceExtensionPinBehavior(t *testing.T) {
	source := plugins.SeedanceSource()
	rows := []model.TaskPlugin{{
		Key: SeedanceExtensionPluginKey, Version: "1.0.2", Source: source,
		SourceHash: string(common.Sha256Raw([]byte(source))), Enabled: true, Active: true,
	}}
	require.NoError(t, seedanceExtensions.SyncSnapshot(context.Background(), rows))

	c := seedancePluginTestContext(t)
	require.NoError(t, PinSeedanceExtensionForChannel(c, dto.VideoUpstreamProtocolFeicaiVideosV1))
	value, exists := c.Get(pluginruntime.ContextKeyPinnedPlugin)
	require.True(t, exists)
	pinned, ok := value.(pluginruntime.PinnedPlugin)
	require.True(t, ok)
	assert.Equal(t, SeedanceExtensionPluginKey, pinned.Plugin.Meta.Key)
	assert.Nil(t, pinned.Generation)

	// Non-migrated protocols pin nothing and never fail.
	other := seedancePluginTestContext(t)
	require.NoError(t, PinSeedanceExtensionForChannel(other, dto.VideoUpstreamProtocolModelArkV3Volcengine))
	_, exists = other.Get(pluginruntime.ContextKeyPinnedPlugin)
	assert.False(t, exists)

	// With no active entry, migrated protocols fail closed.
	emptyStore := &seedanceExtensionStore{compiled: map[string]*seedanceExtensionEntry{}, seeded: map[string]bool{}}
	previous := seedanceExtensions
	seedanceExtensions = emptyStore
	defer func() { seedanceExtensions = previous }()
	unavailable := seedancePluginTestContext(t)
	err := PinSeedanceExtensionForChannel(unavailable, dto.VideoUpstreamProtocolFeicaiVideosV1)
	require.Error(t, err)
	_, exists = unavailable.Get(pluginruntime.ContextKeyPinnedPlugin)
	assert.False(t, exists)
}

// TestSeedancePollObservationErrorClassification pins the poll-time error
// contract: script defects park the task in reconciliation with the
// sanitized message; engine infrastructure failures (admission timeout,
// execution timeout/interrupt) wrap the shared sentinel so the poller skips
// the round without counting toward the failure cutoff.
func TestSeedancePollObservationErrorClassification(t *testing.T) {
	scriptError := seedancePollObservationError(&pluginruntime.HookError{
		Hook:    "seedance.feicai_videos_v1.parseTaskObservation",
		Message: "boom",
	})
	violation, ok := scriptError.(*relaycommon.UpstreamContractViolation)
	require.True(t, ok)
	assert.Equal(t, "boom", violation.Reason)

	admission := seedancePollObservationError(fmt.Errorf("%w: plugin seedance-link@1.0.1", pluginruntime.ErrCallAdmissionTimeout))
	assert.ErrorIs(t, admission, relaycommon.ErrUpstreamObservationUnavailable)

	interrupted := seedancePollObservationError(errors.New("plugin seedance-link@1.0.1 hook seedance.feicai_videos_v1.parseTaskObservation interrupted: plugin call timed out"))
	assert.ErrorIs(t, interrupted, relaycommon.ErrUpstreamObservationUnavailable)
}

// ---------------------------------------------------------------------------
// Capacity and deadline contracts (deterministic, no sleeps).
// ---------------------------------------------------------------------------

const seedanceBlockingTestSource = `
export const meta = {
  apiVersion: 1, key: "seedance-link", name: "t", version: "1.0.1",
  author: { name: "t" }, seedanceProtocols: ["feicai_videos_v1"],
};
export const seedance = {
  "feicai_videos_v1": {
    buildCreate() { while (true) {} },
    parseCreateResponse() { return { id: "ok" }; },
    parseTaskObservation() { return { violation: "x" }; },
  },
};
`

func TestSeedancePluginCallDeadlineReleasesSlot(t *testing.T) {
	plugin, _, err := pluginruntime.CompileSeedanceExtension(
		seedanceBlockingTestSource,
		pluginruntime.Options{Key: SeedanceExtensionPluginKey, Timeout: 100 * time.Millisecond, Concurrency: 1},
		SeedanceExtensionContract(),
	)
	require.NoError(t, err)

	// A pre-canceled context fails during admission without executing.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = plugin.Engine.CallPathWithAdmissionTimeout(
		canceled, seedanceExtensionCreateAdmissionTimeout,
		"seedance", []string{"feicai_videos_v1", "buildCreate"}, map[string]any{},
	)
	require.Error(t, err)

	// The busy-loop hook is interrupted by the engine execution timeout.
	_, err = plugin.Engine.CallPathWithAdmissionTimeout(
		context.Background(), seedanceExtensionCreateAdmissionTimeout,
		"seedance", []string{"feicai_videos_v1", "buildCreate"}, map[string]any{},
	)
	require.Error(t, err)

	// The slot is released after both failures.
	result, err := plugin.Engine.CallPathWithAdmissionTimeout(
		context.Background(), seedanceExtensionCreateAdmissionTimeout,
		"seedance", []string{"feicai_videos_v1", "parseCreateResponse"}, map[string]any{},
	)
	require.NoError(t, err)
	object, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ok", object["id"])
}

func TestSeedancePluginDecodeRejectsHostileOutputs(t *testing.T) {
	protocol := dto.VideoUpstreamProtocolFeicaiVideosV1
	requestDuration := 4

	_, err := decodeSeedanceCreateConversion(map[string]any{"body": "not-an-object"}, protocol, "m", &requestDuration)
	require.Error(t, err)

	oversizeDuration := map[string]any{
		"body": map[string]any{
			"model": "m", "prompt": "p", "ratio": "16:9",
			"duration": float64(relaycommon.MaxTaskDurationSeconds + 1),
		},
		"probe": map[string]any{"resolution": "720p"},
	}
	_, err = decodeSeedanceCreateConversion(oversizeDuration, protocol, "m", &requestDuration)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duration bound")

	// Host-owned billing inputs can never be rewritten by plugin output.
	hostProbe := map[string]any{
		"body":  map[string]any{"model": "m", "prompt": "p", "ratio": "16:9", "duration": float64(4)},
		"probe": map[string]any{"duration_seconds": float64(1)},
	}
	_, err = decodeSeedanceCreateConversion(hostProbe, protocol, "m", &requestDuration)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not permitted")

	hostVideoInput := map[string]any{
		"body":  map[string]any{"model": "m", "prompt": "p", "ratio": "16:9", "duration": float64(4)},
		"probe": map[string]any{"has_video_input": false},
	}
	_, err = decodeSeedanceCreateConversion(hostVideoInput, protocol, "m", &requestDuration)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not permitted")

	unknownProbe := map[string]any{
		"body":  map[string]any{"model": "m", "prompt": "p", "ratio": "16:9", "duration": float64(4)},
		"probe": map[string]any{"mystery_ratio": float64(0.1)},
	}
	_, err = decodeSeedanceCreateConversion(unknownProbe, protocol, "m", &requestDuration)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not permitted")

	nanProbe := map[string]any{
		"body":  map[string]any{"model": "m", "prompt": "p", "ratio": "16:9", "duration": float64(4)},
		"probe": map[string]any{"size_multiplier": math.NaN()},
	}
	_, err = decodeSeedanceCreateConversion(nanProbe, protocol, "m", &requestDuration)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")

	// The wire model must equal the resolved provider model.
	modelMismatch := map[string]any{
		"body":  map[string]any{"model": "other-provider-model", "prompt": "p", "ratio": "16:9", "duration": float64(4)},
		"probe": map[string]any{"resolution": "720p"},
	}
	_, err = decodeSeedanceCreateConversion(modelMismatch, protocol, "resolved-provider-model", &requestDuration)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolved provider model")

	// The wire duration must equal the request duration the probe was priced
	// against; a plugin cannot bill 4 seconds and send 15.
	durationMismatch := map[string]any{
		"body":  map[string]any{"model": "m", "prompt": "p", "ratio": "16:9", "duration": float64(15)},
		"probe": map[string]any{"resolution": "720p"},
	}
	_, err = decodeSeedanceCreateConversion(durationMismatch, protocol, "m", &requestDuration)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request duration")

	_, err = decodeSeedanceCreateConversion(durationMismatch, protocol, "m", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "request duration")

	// A faithful conversion still passes.
	faithful := map[string]any{
		"body": map[string]any{"model": "m", "prompt": "p", "ratio": "16:9", "duration": float64(4)},
		"probe": map[string]any{
			"resolution": "720p", "ratio": "16:9", "size_multiplier": float64(1), "billing_mode": "per-second",
		},
	}
	conversion, err := decodeSeedanceCreateConversion(faithful, protocol, "m", &requestDuration)
	require.NoError(t, err)
	assert.Equal(t, "per-second", conversion.probe["billing_mode"])
}

// TestSeedanceExtensionSeedingRetriesAfterTransientFailure pins the seeding
// contract: a transient database failure must not mark the version seeded,
// so the next sync retries and recovers without a process restart.
func TestSeedanceExtensionSeedingRetriesAfterTransientFailure(t *testing.T) {
	broken, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	originalDB := model.DB
	model.DB = broken // no AutoMigrate: version lookup fails with a database error
	t.Cleanup(func() { model.DB = originalDB })

	store := &seedanceExtensionStore{compiled: map[string]*seedanceExtensionEntry{}, seeded: map[string]bool{}}
	require.Error(t, store.EnsureSeeded(context.Background()))

	// The failure is not sticky: with a healthy database the same store seeds.
	healthy, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, healthy.AutoMigrate(&model.TaskPlugin{}, &model.Task{}, &model.TaskCreateAttempt{}))
	model.DB = healthy
	require.NoError(t, store.EnsureSeeded(context.Background()))
	version, err := model.GetTaskPluginVersion(SeedanceExtensionPluginKey, "")
	require.NoError(t, err)
	assert.Equal(t, "1.0.2", version.Version)

	// Success is sticky within the process: the version is not re-seeded
	// after an administrator deletes it (dependency-free).
	_, deleteErr := model.DeleteTaskPluginVersion(SeedanceExtensionPluginKey, "1.0.2")
	require.NoError(t, deleteErr)
	require.NoError(t, store.EnsureSeeded(context.Background()))
	versions, err := model.ListTaskPluginVersions(SeedanceExtensionPluginKey)
	require.NoError(t, err)
	assert.Empty(t, versions)
}

// ---------------------------------------------------------------------------
// Historical tasks keep the Go polling path; plugin tasks resolve their
// frozen version (store-level, no DB here: compile cache and coverage).
// ---------------------------------------------------------------------------

func TestSeedancePluginTaskSnapshotDetection(t *testing.T) {
	assert.Nil(t, seedancePluginTaskSnapshot(nil))
	assert.Nil(t, seedancePluginTaskSnapshot(&model.Task{}))
	task := &model.Task{}
	task.PrivateData.Execution = &model.TaskExecutionSnapshot{
		TaskPlugin: &model.TaskPluginSnapshot{Key: SeedanceExtensionPluginKey, Version: "1.0.1"},
	}
	snapshot := seedancePluginTaskSnapshot(task)
	require.NotNil(t, snapshot)
	assert.Equal(t, "1.0.1", snapshot.Version)
}

func TestSeedanceExtensionActiveForRequiresProtocolCoverage(t *testing.T) {
	source := plugins.SeedanceSource()
	entrySource := source
	rows := []model.TaskPlugin{{
		Key: SeedanceExtensionPluginKey, Version: "1.0.2", Source: entrySource,
		SourceHash: string(common.Sha256Raw([]byte(entrySource))), Enabled: true, Active: true,
	}}
	require.NoError(t, seedanceExtensions.SyncSnapshot(context.Background(), rows))

	plugin, err := seedanceExtensions.ActiveFor(dto.VideoUpstreamProtocolFeicaiVideosV1)
	require.NoError(t, err)
	assert.Equal(t, "1.0.2", plugin.Meta.Version)

	_, err = seedanceExtensions.ActiveFor(dto.VideoUpstreamProtocolSynlinkVideoV1)
	require.Error(t, err)
}

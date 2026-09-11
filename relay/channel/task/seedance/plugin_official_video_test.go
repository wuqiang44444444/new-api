package seedance

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/seedanceplugin"
	"github.com/QuantumNous/new-api/plugins"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestOfficialPluginPreservesModelArkWireAndProbe(t *testing.T) {
	for _, body := range []string{
		`{"model":"customer","content":[{"type":"text","text":"create a boat"}]}`,
		`{"model":"customer","content":[{"type":"text","text":"create a boat"}],"duration":5,"seed":0,"watermark":false,"camera_fixed":false,"generate_audio":false,"return_last_frame":false,"priority":0}`,
		`{"model":"customer","content":[{"type":"image_url","image_url":{"url":"asset://opaque"},"role":"first_frame"}],"duration":-1,"ratio":"adaptive","resolution":"720p"}`,
	} {
		for _, protocol := range []dto.VideoUpstreamProtocol{dto.VideoUpstreamProtocolModelArkV3Volcengine, dto.VideoUpstreamProtocolModelArkV3BytePlus} {
			t.Run(string(protocol)+body, func(t *testing.T) {
				c := seedancePluginTestContext(t)
				pinSeedanceExtensionForTest(t, c)
				var request taskdto.ModelArkVideoCreateRequest
				require.NoError(t, common.Unmarshal([]byte(body), &request))
				relaycommon.SetVideoContractRequest(c, taskdto.VideoContractRequest{ContractID: taskdto.VideoContractModelArkV3, ModelArk: &request})
				info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{IsModelMapped: true, UpstreamModelName: "ep-custom-deployment", ChannelOtherSettings: taskdto.ChannelOtherSettings{VideoUpstreamProtocol: protocol}, ChannelBaseUrl: "https://ark.example"}}
				adaptor := &TaskAdaptor{}
				adaptor.Init(info)
				payload, _, err := adaptor.modelArkContractPayload(c)
				require.NoError(t, err)
				payload.Model = info.UpstreamModelName
				expected, err := common.Marshal(payload)
				require.NoError(t, err)
				conversion, err := adaptor.ensureSeedanceCreateConversion(c, info)
				require.NoError(t, err)
				assert.Equal(t, string(expected), string(conversion.body))
				assert.Empty(t, conversion.probe)
				assert.Equal(t, "/api/v3/contents/generations/tasks/{task_id}", info.ChannelOtherSettings.VideoUpstreamQueryPathTemplate)
			})
		}
	}
}

func TestOfficialPluginUsageMatchesFrozenGoSemantics(t *testing.T) {
	plugin, _, err := CompileSeedanceExtensionSource(plugins.SeedanceSource())
	require.NoError(t, err)
	for _, usage := range []string{`null`, `{}`, `{"total_tokens":10}`, `{"completion_tokens":0}`, `{"completion_tokens":17,"total_tokens":20,"prompt_tokens":3}`, `{"completion_tokens":1.0}`, `{"completion_tokens":1e0}`, `{"completion_tokens":-1}`, `{"completion_tokens":2147483648}`, `{"completion_tokens":18446744073709551615}`, `{"completion_tokens":"15"}`} {
		t.Run(usage, func(t *testing.T) {
			raw := []byte(`{"id":"task-1","status":"succeeded","content":{"video_url":"https://result.example/video.mp4"},"usage":` + usage + `}`)
			// v3 official flow: the host derives usage from the raw upstream
			// bytes first; the artifact validates identity/status and passes
			// the derived body through unchanged.
			derived, err := normalizeOfficialTaskUsage(raw, "task-1")
			require.NoError(t, err)
			expected := derived
			result, err := plugin.Engine.CallPath(context.Background(), "seedance", []string{"modelark_v3_volcengine", "parseTaskObservation"}, map[string]any{"taskId": "task-1", "body": string(derived)})
			require.NoError(t, err)
			actual, err := decodeOfficialPluginObservation(result, "task-1", plugin.Meta.APIVersion, "modelark_v3_volcengine")
			require.NoError(t, err)
			assert.JSONEq(t, string(expected), string(actual))
		})
	}
}

func TestOfficialPluginTaskDoesNotFallBackWhenFrozenVersionMissing(t *testing.T) {
	previous := seedanceplugin.Default
	seedanceplugin.Default = seedanceplugin.NewStore()
	t.Cleanup(func() { seedanceplugin.Default = previous })
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.TaskPlugin{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	task := &model.Task{PrivateData: model.TaskPrivateData{VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine, Execution: &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{Key: "seedance-link", Version: "missing"}}}}
	_, err = normalizeSeedanceVideoTaskResponse(context.Background(), task, dto.VideoUpstreamProfileOfficial, relaycommon.VideoSouthboundAdapterVersion{}, []byte(`{"id":"task-1","status":"running"}`), "task-1", "https://ark.example", nil)
	require.Error(t, err)
}

func TestOfficialPluginCannotChangePricedRequest(t *testing.T) {
	request := map[string]any{"model": "customer", "duration": 5, "frames": 0, "generate_audio": false, "resolution": "720p", "content": []any{map[string]any{"type": "text", "text": "fixture"}}}
	for _, field := range []string{"duration", "frames", "generate_audio", "resolution", "content", "private_provider_option"} {
		t.Run(field, func(t *testing.T) {
			body := make(map[string]any, len(request))
			for key, value := range request {
				body[key] = value
			}
			body["model"] = "deployment"
			require.NoError(t, validateOfficialPluginBillingRequest(body, request, "deployment"))
			body[field] = "changed"
			require.ErrorContains(t, validateOfficialPluginBillingRequest(body, request, "deployment"), "priced by the host")
		})
	}
}

func TestUsageScanObservationDerivesAndAuthorizesUsage(t *testing.T) {
	t.Run("strips plugin usage and derives from the scan root", func(t *testing.T) {
		body := `{"id":"t1","status":"succeeded","content":{"video_url":"https://r.example/v.mp4"},` +
			`"usage":{"completion_tokens":999},"usage_source":"tampered","usage_evidence":{"tampered.path":1},` +
			`"usage_scan_root":{"usage":{"completion_tokens":17,"total_tokens":20}}}`
		normalized, err := decodeUsageScanPluginObservation(body, "t1")
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, common.Unmarshal(normalized, &payload))
		usage, ok := payload["usage"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, float64(17), usage["completion_tokens"], "host derivation replaces artifact-supplied charging facts")
		assert.Equal(t, float64(20), usage["total_tokens"])
		assert.Equal(t, "usage.completion_tokens", payload["usage_source"])
		_, tampered := payload["usage_evidence"].(map[string]any)["tampered.path"]
		assert.False(t, tampered)
		assert.NotContains(t, payload, "usage_scan_root")
	})

	t.Run("derives completion from total minus prompt", func(t *testing.T) {
		body := `{"id":"t1","status":"succeeded","content":{"video_url":"https://r.example/v.mp4"},` +
			`"usage_scan_root":{"usage":{"total_tokens":20,"prompt_tokens":3}}}`
		normalized, err := decodeUsageScanPluginObservation(body, "t1")
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, common.Unmarshal(normalized, &payload))
		usage, _ := payload["usage"].(map[string]any)
		assert.Equal(t, float64(17), usage["completion_tokens"])
		assert.Equal(t, "usage.total_tokens-usage.prompt_tokens", payload["usage_source"])
	})

	t.Run("invalid completion blocks the total fallback", func(t *testing.T) {
		body := `{"id":"t1","status":"succeeded","content":{"video_url":"https://r.example/v.mp4"},` +
			`"usage_scan_root":{"usage":{"completion_tokens":"corrupt","total_tokens":20}}}`
		normalized, err := decodeUsageScanPluginObservation(body, "t1")
		require.NoError(t, err)
		var payload map[string]any
		require.NoError(t, common.Unmarshal(normalized, &payload))
		assert.NotContains(t, payload, "usage", "a malformed completion field must not let total masquerade as completion")
	})

	t.Run("succeeded without a scan root is a violation", func(t *testing.T) {
		_, err := decodeUsageScanPluginObservation(`{"id":"t1","status":"succeeded"}`, "t1")
		require.ErrorContains(t, err, "scan root is missing")
	})

	t.Run("non-terminal tasks pass without a scan root", func(t *testing.T) {
		normalized, err := decodeUsageScanPluginObservation(`{"id":"t1","status":"running"}`, "t1")
		require.NoError(t, err)
		assert.JSONEq(t, `{"id":"t1","status":"running"}`, string(normalized))
	})
}

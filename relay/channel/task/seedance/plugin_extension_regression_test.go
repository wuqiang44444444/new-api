package seedance

import (
	"context"
	"testing"

	"github.com/QuantumNous/new-api/common"
	taskdto "github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/QuantumNous/new-api/relay/channel/task/seedance/thirdparty/feicai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeedancePluginBase64MatchesGo(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		valid         bool
	}{
		{"plain", "aGVsbG8=", true},
		{"LF", "aGVs\nbG8=", true},
		{"CRLF", "aGVs\r\nbG8=", true},
		{"space", "aGVs bG8=", false},
		{"missing padding", "aGVsbG8", false},
		{"extra padding", "aGVsbG8==", false},
		{"invalid alphabet", "aGVs-bG8=", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := &taskdto.ModelArkVideoCreateRequest{Model: "customer", Duration: common.GetPointer(4), Resolution: common.GetPointer("720p"), Ratio: common.GetPointer("21:9"),
				Content: []taskdto.ModelArkVideoContent{
					{Type: "text", Text: common.GetPointer("a boat")},
					{Type: "image_url", Role: common.GetPointer("reference_image"), ImageURL: &taskdto.VideoMediaURL{URL: "data:image/png;base64," + tc.payload}},
				}}
			c := seedancePluginTestContext(t)
			pinSeedanceExtensionForTest(t, c)
			relaycommon.SetVideoContractRequest(c, taskdto.VideoContractRequest{ContractID: taskdto.VideoContractModelArkV3, ModelArk: request})
			adaptor := &TaskAdaptor{protocol: dto.VideoUpstreamProtocolFeicaiVideosV1, profile: dto.VideoUpstreamProfileThirdPartyFeicaiVideos}
			actual, err := adaptor.ensureSeedanceCreateConversion(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{IsModelMapped: true, UpstreamModelName: feicai.ProviderModelSeedance20Mini720P}})
			expected, legacyErr := feicai.CreateRequest(request, feicai.ProviderModelSeedance20Mini720P)
			if tc.valid {
				require.NoError(t, legacyErr)
				require.NoError(t, err)
				assert.Equal(t, string(expected), string(actual.body))
			} else {
				require.Error(t, legacyErr)
				require.Error(t, err)
				assert.Equal(t, legacyErr.Error(), err.Error())
			}
		})
	}
}

func TestSeedancePluginCompileFailureStopsAdmissionAndIsVisible(t *testing.T) {
	store := &seedanceExtensionStore{compiled: map[string]*seedanceExtensionEntry{}}
	row := model.TaskPlugin{Key: SeedanceExtensionPluginKey, Version: "1.0.2", Source: plugins.SeedanceSource(), Enabled: true, Active: true}
	row.SourceHash = sourceHashOf(row.Source)
	require.NoError(t, store.SyncSnapshot(context.Background(), []model.TaskPlugin{row}))
	pinned, err := store.ActiveFor(dto.VideoUpstreamProtocolFeicaiVideosV1)
	require.NoError(t, err)
	broken := row
	broken.Source = "invalid javascript ("
	broken.SourceHash = sourceHashOf(broken.Source)
	require.NoError(t, store.SyncSnapshot(context.Background(), []model.TaskPlugin{broken}))
	version, diagnostics := store.Describe()
	assert.Empty(t, version)
	require.Len(t, diagnostics, 1)
	_, err = store.ActiveFor(dto.VideoUpstreamProtocolFeicaiVideosV1)
	require.Error(t, err)
	// Publishing failure did not mutate a running request's pinned engine.
	result, err := pinned.Engine.CallPath(context.Background(), "seedance", []string{"feicai_videos_v1", "parseCreateResponse"}, map[string]any{"body": `{"id":"existing"}`})
	require.NoError(t, err)
	assert.Equal(t, "existing", result.(map[string]any)["id"])
	require.NoError(t, store.SyncSnapshot(context.Background(), []model.TaskPlugin{row}))
	version, diagnostics = store.Describe()
	assert.Equal(t, row.Version, version)
	assert.Empty(t, diagnostics)
}

func TestSeedancePluginQuotedErrorsMatchGo(t *testing.T) {
	for _, ratio := range []string{"alpha", "bell\a", "quote\"slash\\", "line\nfeed"} {
		t.Run(ratio, func(t *testing.T) {
			request := &taskdto.ModelArkVideoCreateRequest{Model: "customer", Duration: common.GetPointer(4), Resolution: common.GetPointer("720p"), Ratio: &ratio,
				Content: []taskdto.ModelArkVideoContent{{Type: "text", Text: common.GetPointer("a boat")}}}
			c := seedancePluginTestContext(t)
			pinSeedanceExtensionForTest(t, c)
			relaycommon.SetVideoContractRequest(c, taskdto.VideoContractRequest{ContractID: taskdto.VideoContractModelArkV3, ModelArk: request})
			adaptor := &TaskAdaptor{protocol: dto.VideoUpstreamProtocolFeicaiVideosV1, profile: dto.VideoUpstreamProfileThirdPartyFeicaiVideos}
			_, err := adaptor.ensureSeedanceCreateConversion(c, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{IsModelMapped: true, UpstreamModelName: feicai.ProviderModelSeedance20Mini720P}})
			_, legacyErr := feicai.CreateRequest(request, feicai.ProviderModelSeedance20Mini720P)
			require.Error(t, legacyErr)
			require.Error(t, err)
			assert.Equal(t, legacyErr.Error(), err.Error())
		})
	}
}

package doubao

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/doubao/thirdparty/mediaarrays"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVideoAdapterVersionFromFetchBodyRequiresMediaArraysV2(t *testing.T) {
	_, err := videoAdapterVersionFromFetchBody(
		map[string]any{},
		constant.ChannelTypeDoubaoVideo,
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
	)
	require.Error(t, err)

	version, err := videoAdapterVersionFromFetchBody(
		map[string]any{videoUpstreamAdapterVersionKey: "54:third_party_json_video_media_arrays:v2"},
		constant.ChannelTypeDoubaoVideo,
		dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
	)
	require.NoError(t, err)
	assert.True(t, version.IsJSONVideoMediaArraysV2())
}

func TestVideoAdapterVersionFromFetchBodyRejectsMismatchedVersion(t *testing.T) {
	for _, frozen := range []string{
		"54:third_party_json_video_media_arrays:v1",
		"54:official:v2",
	} {
		_, err := videoAdapterVersionFromFetchBody(
			map[string]any{videoUpstreamAdapterVersionKey: frozen},
			constant.ChannelTypeDoubaoVideo,
			dto.VideoUpstreamProfileThirdPartyJSONVideoMediaArrays,
		)
		require.Error(t, err)
		var violation *relaycommon.UpstreamContractViolation
		assert.True(t, errors.As(err, &violation))
	}
}

func TestNormalizeFunCloudVideoTaskResponseAcceptsFrozenV2Snapshot(t *testing.T) {
	profile := dto.VideoUpstreamProfileThirdPartyFunCloudSeedanceV2
	version, err := videoAdapterVersionFromFetchBody(
		map[string]any{videoUpstreamAdapterVersionKey: "54:third_party_funcloud_seedance_v2:v2"},
		constant.ChannelTypeDoubaoVideo,
		profile,
	)
	require.NoError(t, err)

	body, err := normalizeVideoTaskResponse(
		profile,
		version,
		[]byte(`{"code":0,"data":{"taskId":"task_1","status":"completed","result":["https://cdn.example/video.mp4"]}}`),
		"task_1",
		"",
		mediaarrays.TaskResponseContext{},
	)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"task_1","status":"succeeded","content":{"video_url":"https://cdn.example/video.mp4"},"error":{}}`, string(body))
}

func TestRelayV2IgnoresUnverifiedUsageForBothMoxingModels(t *testing.T) {
	profile := dto.VideoUpstreamProfileThirdPartyRelay
	version, err := videoAdapterVersionFromFetchBody(
		map[string]any{videoUpstreamAdapterVersionKey: "54:third_party_relay:v2"},
		constant.ChannelTypeDoubaoVideo,
		profile,
	)
	require.NoError(t, err)
	providerBody := []byte(`{"task_id":"task_1","status":"succeeded","result":{"urls":["https://cdn.example/video.mp4"]},"usage":{"completion_tokens":100,"total_tokens":100}}`)

	moxing, err := normalizeVideoTaskResponse(
		profile, version, providerBody, "task_1",
		model.LinkImplementationMoxingSeedanceMedia, mediaarrays.TaskResponseContext{},
	)
	require.NoError(t, err)
	assert.NotContains(t, string(moxing), `"usage"`)

	tokenSave, err := normalizeVideoTaskResponse(
		profile, version, providerBody, "task_1",
		model.LinkImplementationTokenSaveSeedance, mediaarrays.TaskResponseContext{},
	)
	require.NoError(t, err)
	assert.NotContains(t, string(tokenSave), `"usage"`)
}

func TestRelayV1FrozenTaskRetainsLegacyUsageProjection(t *testing.T) {
	profile := dto.VideoUpstreamProfileThirdPartyRelay
	version, err := videoAdapterVersionFromFetchBody(
		map[string]any{videoUpstreamAdapterVersionKey: "54:third_party_relay:v1"},
		constant.ChannelTypeDoubaoVideo,
		profile,
	)
	require.NoError(t, err)
	body, err := normalizeVideoTaskResponse(
		profile,
		version,
		[]byte(`{"task_id":"task_1","status":"succeeded","result":{"urls":["https://cdn.example/video.mp4"]},"usage":{"total_tokens":100}}`),
		"ignored-by-v1",
		model.LinkImplementationTokenSaveSeedance,
		mediaarrays.TaskResponseContext{},
	)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"usage"`)
}

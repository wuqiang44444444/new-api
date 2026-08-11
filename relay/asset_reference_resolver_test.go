package relay

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func officialAssetReferenceRelayInfo(protocol dto.VideoUpstreamProtocol) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
		ChannelOtherSettings: dto.ChannelOtherSettings{VideoUpstreamProtocol: protocol},
	}}
}

func TestCollectModelArkAssetReferencesSeparatesPrivateAndPublicNamespaces(t *testing.T) {
	content := []dto.ModelArkVideoContent{
		{ImageURL: &dto.VideoMediaURL{URL: "asset://ast_private"}},
		{VideoURL: &dto.VideoMediaURL{URL: " asset://pubref_asset-20260811-public "}},
		{AudioURL: &dto.VideoMediaURL{URL: "https://example.com/audio.mp3"}},
	}

	privateIDs, replacements, err := collectModelArkAssetReferences(
		content,
		officialAssetReferenceRelayInfo(dto.VideoUpstreamProtocolModelArkV3Volcengine),
	)

	require.NoError(t, err)
	assert.Equal(t, []string{"ast_private"}, privateIDs)
	assert.Equal(t, map[string]string{
		"asset://pubref_asset-20260811-public": "asset://asset-20260811-public",
	}, replacements)
}

func TestResolveAssetReferencesForAttemptForwardsOfficialPublicReferenceWithoutPrivateAssetLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	relaycommon.SetVideoContractRequest(context, dto.VideoContractRequest{
		ContractID: dto.VideoContractModelArkV3,
		ModelArk: &dto.ModelArkVideoCreateRequest{Content: []dto.ModelArkVideoContent{{
			ImageURL: &dto.VideoMediaURL{URL: "asset://pubref_asset-public"},
		}}},
	})
	context.Set("task_request", relaycommon.TaskSubmitReq{Metadata: map[string]any{
		"content": []any{map[string]any{
			"image_url": map[string]any{"url": "asset://pubref_asset-public"},
		}},
	}})

	err := resolveAssetReferencesForAttempt(
		context,
		officialAssetReferenceRelayInfo(dto.VideoUpstreamProtocolModelArkV3BytePlus),
	)

	require.NoError(t, err)
	contract, ok := relaycommon.GetVideoContractRequest(context)
	require.True(t, ok)
	require.NotNil(t, contract.ModelArk)
	assert.Equal(t, "asset://asset-public", contract.ModelArk.Content[0].ImageURL.URL)
	request, err := relaycommon.GetTaskRequest(context)
	require.NoError(t, err)
	content, ok := request.Metadata["content"].([]any)
	require.True(t, ok)
	item, ok := content[0].(map[string]any)
	require.True(t, ok)
	media, ok := item["image_url"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "asset://asset-public", media["url"])
}

func TestCollectModelArkAssetReferencesRejectsUnnamespacedAndInvalidPublicReferences(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		info      *relaycommon.RelayInfo
		wantError error
	}{
		{
			name:      "raw provider id",
			url:       "asset://asset-private-or-public",
			info:      officialAssetReferenceRelayInfo(dto.VideoUpstreamProtocolModelArkV3Volcengine),
			wantError: service.ErrInvalidAssetRequest,
		},
		{
			name:      "invalid public id",
			url:       "asset://pubref_asset/escape",
			info:      officialAssetReferenceRelayInfo(dto.VideoUpstreamProtocolModelArkV3BytePlus),
			wantError: service.ErrInvalidAssetRequest,
		},
		{
			name:      "third party protocol",
			url:       "asset://pubref_asset-public",
			info:      officialAssetReferenceRelayInfo(dto.VideoUpstreamProtocolMediaTaskV1),
			wantError: service.ErrAssetReferenceUnresolvable,
		},
		{
			name:      "URL-only protocol",
			url:       "asset://pubref_asset-public",
			info:      officialAssetReferenceRelayInfo(dto.VideoUpstreamProtocolURLMediaArraysV1),
			wantError: service.ErrAssetReferenceUnresolvable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := collectModelArkAssetReferences(
				[]dto.ModelArkVideoContent{{ImageURL: &dto.VideoMediaURL{URL: test.url}}},
				test.info,
			)
			require.ErrorIs(t, err, test.wantError)
		})
	}
}

func TestResolvePrivateAssetReferencesRejectsChannelWithoutAssetProtocolBeforeLookup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := officialAssetReferenceRelayInfo(dto.VideoUpstreamProtocolURLMediaArraysV1)
	info.ChannelType = constant.ChannelTypeSeedanceLink
	info.ChannelOtherSettings.AssetUpstreamProtocol = dto.AssetUpstreamProtocolNone

	_, _, _, err := resolvePrivateAssetReferences(context, info, []string{"ast_private"})

	require.ErrorIs(t, err, service.ErrAssetReferenceUnresolvable)
}

func TestRewriteMetadataAssetReferencesOnlyRewritesRegisteredValues(t *testing.T) {
	metadata := map[string]any{
		"content": []any{
			map[string]any{"image_url": map[string]any{"url": "asset://ast_12345678901234567890123456789012"}},
			map[string]any{"video_url": map[string]any{"url": "https://example.com/video.mp4"}},
		},
	}
	rewriteMetadataAssetReferences(metadata, map[string]string{
		"asset://ast_12345678901234567890123456789012": "asset://asset-upstream",
	})

	content, ok := metadata["content"].([]any)
	require.True(t, ok)
	first, ok := content[0].(map[string]any)
	require.True(t, ok)
	image, ok := first["image_url"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "asset://asset-upstream", image["url"])
	second, ok := content[1].(map[string]any)
	require.True(t, ok)
	video, ok := second["video_url"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "https://example.com/video.mp4", video["url"])
}

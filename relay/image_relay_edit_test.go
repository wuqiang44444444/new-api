package relay

import (
	"context"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestImageRelayAsyncEditReusesStoredInputAtDispatch(t *testing.T) {
	config := system_setting.ObjectStorageConfig{Backend: "s3", Endpoint: "https://storage.example", Bucket: "images", AccountName: "account", Region: "us-east-1", Credential: "test-secret", Revision: "edit-dispatch"}
	raw, err := common.Marshal(config)
	require.NoError(t, err)
	model.NotifyObjectStorageSettingUpdate(string(raw))
	t.Cleanup(func() { model.NotifyObjectStorageSettingUpdate("") })
	ctx, err := service.WithImageObjectStore(context.Background())
	require.NoError(t, err)
	data := &model.TaskImageExecutionData{Parameters: &dto.ImageRequest{Prompt: "change cup color"}, UpstreamModel: constant.FunCloudImageProviderModelNanoBanana2, N: 1, ResponseFormat: "url", ChannelOther: dto.ChannelOtherSettings{ImageUpstreamProtocol: dto.ImageUpstreamProtocolFunCloudAIGCV2}, Inputs: []model.TaskImageInputRef{{ObjectKey: "images/tasks/task-test/input-0", MimeType: "image/png"}, {URL: "https://example.com/second.png"}}}
	request, err := rebuildImageRequest(ctx, data)
	require.NoError(t, err, "must sign existing input without downloading or uploading")
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits, ChannelMeta: &relaycommon.ChannelMeta{ApiType: constant.APITypeAsyncImage, UpstreamModelName: data.UpstreamModel, ChannelOtherSettings: data.ChannelOther}}
	require.Nil(t, validateImageAsyncFamilyContract(nil, info, request))
	adaptor := GetAdaptor(info.ApiType)
	adaptor.Init(info)
	converted, err := adaptor.ConvertImageRequest(nil, info, *request)
	require.NoError(t, err)
	require.NoError(t, service.PrepareImageUpstreamRequest(ctx, converted))
	encoded, err := common.Marshal(converted)
	require.NoError(t, err)
	var payload struct {
		URLs []string `json:"imageUrls"`
	}
	require.NoError(t, common.Unmarshal(encoded, &payload))
	require.Len(t, payload.URLs, 2)
	assert.Contains(t, payload.URLs[0], "images/tasks/task-test/input-0")
	assert.Contains(t, payload.URLs[0], "X-Amz-Expires=7200")
	assert.Equal(t, "https://example.com/second.png", payload.URLs[1])
}

package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func officialAssetControllerTestChannel() *model.Channel {
	channel := &model.Channel{
		Type: constant.ChannelTypeSeedanceLink,
		Key:  "video-api-key",
	}
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3BytePlus,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolBytePlusAction,
		AssetMinURLTTLSeconds: 3600,
		AssetProviderProject:  "project-a",
		AssetRegion:           "ap-southeast-1",
	})
	return channel
}

func TestValidateNewOfficialAssetCredential(t *testing.T) {
	channel := officialAssetControllerTestChannel()
	require.NoError(t, validateNewChannelAssetCredential(
		channel,
		&dto.ChannelAssetCredentialInput{AccessKeyID: "access", SecretAccessKey: "secret"},
		"single",
	))

	require.ErrorContains(t, validateNewChannelAssetCredential(channel, nil, "single"), "requires")
	require.ErrorContains(t, validateNewChannelAssetCredential(
		channel,
		&dto.ChannelAssetCredentialInput{AccessKeyID: "access"},
		"single",
	), "must both be provided")
	require.ErrorContains(t, validateNewChannelAssetCredential(
		channel,
		&dto.ChannelAssetCredentialInput{AccessKeyID: "access", SecretAccessKey: "secret"},
		"batch",
	), "single-key")
}

func TestVolcengineOfficialAssetsUseTheSameSeparatedCredentialStore(t *testing.T) {
	channel := officialAssetControllerTestChannel()
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3Volcengine,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolVolcengineAction,
		AssetMinURLTTLSeconds: 3600,
		AssetProviderProject:  "default",
		AssetRegion:           model.VolcengineAssetActionRegion,
	})

	require.True(t, channelUsesOfficialAssetCredential(channel))
	require.NoError(t, validateNewChannelAssetCredential(
		channel,
		&dto.ChannelAssetCredentialInput{AccessKeyID: "access", SecretAccessKey: "secret"},
		"single",
	))
}

func TestCMCCAssetsUseTheSeparatedCredentialStoreWithoutProjectOrRegion(t *testing.T) {
	channel := officialAssetControllerTestChannel()
	channel.SetOtherSettings(dto.ChannelOtherSettings{
		VideoUpstreamProtocol: dto.VideoUpstreamProtocolModelArkV3CMCC,
		AssetUpstreamProtocol: dto.AssetUpstreamProtocolCMCCAICCV2,
		AssetMinURLTTLSeconds: 3600,
	})

	require.True(t, channelUsesOfficialAssetCredential(channel))
	require.NoError(t, validateNewChannelAssetCredential(
		channel,
		&dto.ChannelAssetCredentialInput{AccessKeyID: "access", SecretAccessKey: "secret"},
		"single",
	))
}

func TestGetChannelReturnsOnlyAssetCredentialStatus(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAssetCredential{}))
	channel := officialAssetControllerTestChannel()
	channel.Name = "credential-status"
	channel.Models = "video-model"
	channel.Group = "default"
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.ChannelAssetCredential{
		ChannelID:       channel.Id,
		AccessKeyID:     "AK123456YAA",
		SecretAccessKey: "never-return-this-secret",
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/channel/1", nil)

	GetChannel(ctx)

	assert.Contains(t, recorder.Body.String(), `"configured":true`)
	assert.NotContains(t, recorder.Body.String(), "AK123456YAA")
	assert.NotContains(t, recorder.Body.String(), "never-return-this-secret")
}

func TestDeleteChannelAssetCredentialRequiresSavedProfileToBeDisabled(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(
		&model.ChannelAssetCredential{},
	))
	channel := officialAssetControllerTestChannel()
	channel.Name = "credential-clear"
	require.NoError(t, db.Create(channel).Error)
	require.NoError(t, db.Create(&model.ChannelAssetCredential{
		ChannelID:       channel.Id,
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", channel.Id)}}
	ctx.Request = httptest.NewRequest(http.MethodDelete, "/api/channel/1/asset_credential", nil)

	DeleteChannelAssetCredential(ctx)

	assert.Equal(t, http.StatusConflict, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"error_code":"asset_credential_profile_active"`)
	credential, err := model.GetChannelAssetCredential(channel.Id)
	require.NoError(t, err)
	assert.NotNil(t, credential)
}

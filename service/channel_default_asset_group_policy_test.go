package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/pkg/seedanceplugin"
	"github.com/QuantumNous/new-api/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelDefaultAssetGroupCreationFollowsDeclaredPolicy(t *testing.T) {
	for _, policy := range []string{"none", "hosted", "default_fallback"} {
		t.Run(policy, func(t *testing.T) {
			db := withAssetGroupPolicyDB(t)
			channel := createMoxingAssetPolicyChannel(t, db, "https://upstream.example")
			if policy == "hosted" {
				channel.SetOtherSettings(dto.ChannelOtherSettings{
					VideoUpstreamProtocol: dto.VideoUpstreamProtocolFunCloudModelArkV3,
					AssetUpstreamProtocol: dto.AssetUpstreamProtocolFunCloudHosted,
				})
				require.NoError(t, db.Save(channel).Error)
			}
			if policy == "none" {
				// Keep group operations implemented and published: interface
				// availability must not grant permission to create a default group.
				source := plugins.SeedanceSource() + `
meta.version = "9.0.1";
meta.channelConfiguration.assets.find(a => a.protocol === "moxing_volc_assets_v1").groupPolicy = "none";
`
				loaded, _, err := jsplugin.CompileSeedanceExtension(source, jsplugin.Options{}, jsplugin.SeedanceHostContract())
				require.NoError(t, err)
				require.NoError(t, seedanceplugin.Default.SyncSnapshot(context.Background(), []model.TaskPlugin{{
					Key: loaded.Meta.Key, Version: loaded.Meta.Version, APIVersion: loaded.Meta.APIVersion,
					Source: source, Enabled: true, Active: true,
				}}))
			}
			calls := 0
			withAssetPolicyHTTPClient(t, func(req *http.Request) (*http.Response, error) {
				calls++
				assert.Equal(t, http.MethodPost, req.Method)
				return assetPolicyHTTPResponse("provider-default-group"), nil
			})

			status, err := GetChannelDefaultAssetGroupStatus(channel)
			require.NoError(t, err)
			assert.Equal(t, policy == "default_fallback", status.Supported)
			assert.False(t, status.Configured)
			result, err := CreateOrReuseChannelDefaultAssetGroup(context.Background(), channel)
			if policy == "default_fallback" {
				require.NoError(t, err)
				assert.True(t, result.Supported)
				assert.True(t, result.Configured)
				assert.Equal(t, DefaultAssetGroupActionCreated, result.Action)
				assert.Equal(t, 1, calls)
			} else {
				require.ErrorIs(t, err, ErrUnsupportedAssetOperation)
				assert.False(t, result.Supported)
				assert.False(t, result.Configured)
				assert.Zero(t, calls)
			}
			record, err := model.GetChannelDefaultAssetGroup(channel.Id)
			require.NoError(t, err)
			if policy == "default_fallback" {
				require.NotNil(t, record)
				assert.Equal(t, "provider-default-group", record.ProviderGroupID)
			} else {
				assert.Nil(t, record)
			}
		})
	}
}

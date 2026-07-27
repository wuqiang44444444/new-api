package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSettingsAssetProfileContract(t *testing.T) {
	baseURL := common.GetPointer("https://example.com")
	cases := []struct {
		name        string
		channelType int
		key         string
		settings    string
		wantError   bool
	}{
		{"ark matches reverse proxy", constant.ChannelTypeDoubaoVideo, "sk-one", `{"video_upstream_profile":"third_party_reverse_proxy","video_upstream_create_path":"/v1/ark/media/generations","video_upstream_query_path_template":"/v1/ark/media/tasks/{task_id}","asset_upstream_profile":"ark_assets","asset_min_url_ttl_seconds":3600}`, false},
		{"ark cannot match relay", constant.ChannelTypeDoubaoVideo, "sk-one", `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks/{task_id}","asset_upstream_profile":"ark_assets","asset_min_url_ttl_seconds":3600}`, true},
		{"relay matches relay", constant.ChannelTypeDoubaoVideo, "sk-one", `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks/{task_id}","asset_upstream_profile":"relay_assets","asset_min_url_ttl_seconds":3600}`, false},
		{"asset profile rejects multiple keys", constant.ChannelTypeDoubaoVideo, "sk-one\nsk-two", `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks/{task_id}","asset_upstream_profile":"relay_assets","asset_min_url_ttl_seconds":3600}`, true},
		{"asset profile rejects other channel types", constant.ChannelTypeOpenAI, "sk-one", `{"asset_upstream_profile":"joycreator_assets","asset_min_url_ttl_seconds":3600}`, true},
		{"asset profile requires verified URL window", constant.ChannelTypeDoubaoVideo, "sk-one", `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks/{task_id}","asset_upstream_profile":"relay_assets"}`, true},
		{"official Action accepts a separate video API key", constant.ChannelTypeDoubaoVideo, "video-api-key", `{"video_upstream_profile":"official","asset_upstream_profile":"official_action_assets","asset_provider_project":"default","asset_region":"ap-southeast-1","asset_min_url_ttl_seconds":3600}`, false},
		{"official Action rejects a non-provider Region", constant.ChannelTypeDoubaoVideo, "video-api-key", `{"video_upstream_profile":"official","asset_upstream_profile":"official_action_assets","asset_provider_project":"default","asset_region":"a","asset_min_url_ttl_seconds":3600}`, true},
		{"official Action rejects unsafe Region", constant.ChannelTypeDoubaoVideo, "video-api-key", `{"video_upstream_profile":"official","asset_upstream_profile":"official_action_assets","asset_provider_project":"default","asset_region":"https://internal.example","asset_min_url_ttl_seconds":3600}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := Channel{Type: tc.channelType, Key: tc.key, BaseURL: baseURL, OtherSettings: tc.settings}
			err := channel.ValidateSettings()
			if tc.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateSettingsAssetProfileUsesPersistedKeyOnEdit(t *testing.T) {
	truncateTables(t)
	baseURL := common.GetPointer("https://example.com")
	existing := Channel{
		Id:      901,
		Type:    constant.ChannelTypeDoubaoVideo,
		Name:    "asset-edit-existing-key",
		Key:     "sk-existing",
		BaseURL: baseURL,
	}
	require.NoError(t, DB.Create(&existing).Error)

	update := Channel{
		Id:            existing.Id,
		Type:          existing.Type,
		BaseURL:       baseURL,
		OtherSettings: `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks/{task_id}","asset_upstream_profile":"relay_assets","asset_min_url_ttl_seconds":3600}`,
	}
	require.NoError(t, update.ValidateSettings())
}

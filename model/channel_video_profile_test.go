package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

// TestValidateSettingsDoubaoVideoProfile 验证 DoubaoVideo 渠道保存时对 video_upstream_profile
// 与第三方路径的联合校验（方案 §5、§10.1）。
func TestValidateSettingsDoubaoVideoProfile(t *testing.T) {
	cases := []struct {
		name      string
		baseURL   string
		settings  string
		wantError bool
	}{
		{"empty settings allowed", "", `{}`, false},
		{"official allowed", "", `{"video_upstream_profile":"official"}`, false},
		{"reverse proxy without paths rejected", "", `{"video_upstream_profile":"third_party_reverse_proxy"}`, true},
		{"reverse proxy with empty base url rejected", "", `{"video_upstream_profile":"third_party_reverse_proxy","video_upstream_create_path":"/v1/ark/media/generations","video_upstream_query_path_template":"/v1/ark/media/tasks/{task_id}"}`, true},
		{"reverse proxy with full config allowed", "https://example.com", `{"video_upstream_profile":"third_party_reverse_proxy","video_upstream_create_path":"/v1/ark/media/generations","video_upstream_query_path_template":"/v1/ark/media/tasks/{task_id}"}`, false},
		{"relay with full config allowed", "https://example.com", `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks/{task_id}"}`, false},
		{"relay bad base url rejected", "not-a-url", `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks/{task_id}"}`, true},
		{"relay query template missing placeholder rejected", "https://example.com", `{"video_upstream_profile":"third_party_relay","video_upstream_create_path":"/v1/media/generations","video_upstream_query_path_template":"/v1/media/tasks"}`, true},
		{"unknown rejected", "", `{"video_upstream_profile":"garbage"}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := &Channel{
				Type:          constant.ChannelTypeDoubaoVideo,
				BaseURL:       common.GetPointer[string](tc.baseURL),
				OtherSettings: tc.settings,
			}
			err := channel.ValidateSettings()
			if tc.wantError {
				assert.Errorf(t, err, "expected settings %s to be rejected", tc.settings)
			} else {
				assert.NoErrorf(t, err, "expected settings %s to pass", tc.settings)
			}
		})
	}
}

// TestValidateSettingsCleansOfficialPaths 验证 official 协议保存时清除残留第三方路径（方案 §5.1）。
func TestValidateSettingsCleansOfficialPaths(t *testing.T) {
	channel := &Channel{
		Type:          constant.ChannelTypeDoubaoVideo,
		OtherSettings: `{"video_upstream_profile":"official","video_upstream_create_path":"/v1/x","video_upstream_query_path_template":"/v1/x/{task_id}"}`,
	}
	assert.NoError(t, channel.ValidateSettings())
	assert.NotContains(t, channel.OtherSettings, "video_upstream_create_path")
	assert.NotContains(t, channel.OtherSettings, "video_upstream_query_path_template")
}

// TestValidateSettingsIgnoresProfileForNonDoubaoVideo 确认非 DoubaoVideo 渠道不校验该字段，
// 避免其它渠道类型被未知 profile 值误拒。
func TestValidateSettingsIgnoresProfileForNonDoubaoVideo(t *testing.T) {
	channel := &Channel{
		Type:          constant.ChannelTypeOpenAI,
		OtherSettings: `{"video_upstream_profile":"garbage"}`,
	}
	assert.NoError(t, channel.ValidateSettings())
}

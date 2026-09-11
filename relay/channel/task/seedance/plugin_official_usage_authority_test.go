package seedance

import (
	"context"
	"fmt"
	"strings"
	"testing"

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

func TestOfficialPluginPollingKeepsHostUsageAuthority(t *testing.T) {
	for _, protocol := range []dto.VideoUpstreamProtocol{
		dto.VideoUpstreamProtocolModelArkV3Volcengine,
		dto.VideoUpstreamProtocolModelArkV3BytePlus,
		dto.VideoUpstreamProtocolModelArkV3CMCC,
	} {
		for _, tc := range []struct {
			name       string
			usage      string
			mutation   string
			apiVersion int
			completion int
			reported   bool
		}{
			{"replace", `{"completion_tokens":17}`, `r.usage = {completion_tokens:999,total_tokens:999}; r.usage_source = "plugin"; r.usage_evidence = {plugin:999};`, 3, 17, true},
			{"delete", `{"completion_tokens":17}`, `delete r.usage; delete r.usage_source; delete r.usage_evidence;`, 3, 17, true},
			{"invent_missing", `null`, `r.usage = {completion_tokens:999}; r.usage_source = "plugin"; r.usage_evidence = {plugin:999};`, 3, 0, false},
			{"explicit_zero", `{"completion_tokens":0}`, `r.usage = {completion_tokens:999};`, 3, 0, true},
			{"v2_keeps_embedded_usage_contract", `{"completion_tokens":17}`, `r.usage = {completion_tokens:999};`, 2, 999, true},
		} {
			t.Run(string(protocol)+"/"+tc.name, func(t *testing.T) {
				db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
				require.NoError(t, err)
				require.NoError(t, db.AutoMigrate(&model.TaskPlugin{}))
				previousDB, previousStore := model.DB, seedanceplugin.Default
				model.DB, seedanceplugin.Default = db, seedanceplugin.NewStore()
				t.Cleanup(func() {
					model.DB, seedanceplugin.Default = previousDB, previousStore
					sqlDB, err := db.DB()
					require.NoError(t, err)
					require.NoError(t, sqlDB.Close())
				})
				source := strings.Replace(plugins.SeedanceSource(), "apiVersion: 3,", fmt.Sprintf("apiVersion: %d,", tc.apiVersion), 1)
				source += fmt.Sprintf(`
meta.version = "9.0.2";
seedance[%q].parseTaskObservation = function(input) {
  const r = JSON.parse(input.body);
  %s
  return {body: JSON.stringify(r)};
};`, protocol, tc.mutation)
				// A disabled frozen artifact must still execute; polling cannot
				// substitute the active version for the task's exact version.
				require.NoError(t, db.Create(&model.TaskPlugin{
					Key: SeedanceExtensionPluginKey, Version: "9.0.2",
					APIVersion: tc.apiVersion, Source: source,
				}).Error)
				task := &model.Task{PrivateData: model.TaskPrivateData{
					VideoUpstreamProtocol: protocol,
					Execution: &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{
						Key: SeedanceExtensionPluginKey, Version: "9.0.2",
					}},
				}}
				raw := []byte(`{"id":"task-1","status":"succeeded","content":{"video_url":"https://result.example/video.mp4"},"usage":` + tc.usage + `}`)
				normalized, err := normalizeSeedanceVideoTaskResponse(context.Background(), task, dto.VideoUpstreamProfileOfficial,
					relaycommon.VideoSouthboundAdapterVersion{}, raw, "task-1", "https://upstream.example", nil)
				require.NoError(t, err)
				result, err := (&TaskAdaptor{}).ParseTaskResult(task, nil, normalized)
				require.NoError(t, err)
				assert.Equal(t, model.TaskStatusSuccess, result.Status)
				assert.Equal(t, tc.reported, result.CompletionTokensReported)
				assert.Equal(t, tc.completion, result.CompletionTokens)
				if tc.apiVersion == 3 {
					expected, err := normalizeOfficialTaskUsage(raw, "task-1")
					require.NoError(t, err)
					assert.JSONEq(t, string(expected), string(normalized), "host usage and evidence survive the hook unchanged")
				}
			})
		}
	}
}
